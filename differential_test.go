// Differential tests compare this port against gulp 5.0.1 itself.
//
// They come in two layers. The golden-vector tests carry expected values
// transcribed from the JavaScript sources and test suites. The probe tests run
// gulp itself and compare it against this port over the same fixtures; gulp's
// side of those is recorded in testdata/gulp_oracle.json so they run
// everywhere, and re-runs live against a real checkout when one is available.
//
//	npm install gulp@5.0.1 --prefix /tmp/gulp-js
//	GULP_JS_REPO=/tmp/gulp-js/node_modules/gulp go test -run Differential -v .
package gulp_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/internal/cli"
	"github.com/gulpjs/gulp-go/internal/glob"
	"github.com/gulpjs/gulp-go/undertaker"
)

// ---------------------------------------------------------------------------
// Golden vectors
// ---------------------------------------------------------------------------

// TestDifferentialGlobParent checks the glob base calculation against the
// table in glob-parent's own test suite. dest() strips this prefix from every
// path, so a divergence here silently reshapes the entire output tree.
func TestDifferentialGlobParent(t *testing.T) {
	cases := []struct{ pattern, want string }{
		{"path/to/*.js", "path/to"},
		{"/root/path/to/*.js", "/root/path/to"},
		{"/*.js", "/"},
		{"*.js", "."},
		{"**/*.js", "."},
		{"path/**/*.js", "path"},
		{"path/to/**/*.js", "path/to"},
		{"/src/js/**.js", "/src/js"},
		{"./fixtures/**/*.jade", "./fixtures"},
		{"./fixtures/*.coffee", "./fixtures"},
		{"./fixtures/stuff/run.dmc", "./fixtures/stuff"},
		{"fixtures/stuff/*.dmc", "fixtures/stuff"},
		{"path/{foo,bar}/", "path"},
		{"path/foo[bar]/", "path"},
		{"path/*foo/", "path"},
		{"foo/{bar,baz/qux}", "foo"},
		{"{a,b}/c", "."},
		{"a/(b|c)/d", "a"},
		{"a/!(b)/d", "a"},
		{"base/*/", "base"},
		{"base/**", "base"},
		{"a/b/c/d", "a/b/c"},
		{"path.js", "."},
		{"/", "/"},
		{"*", "."},
		{".", "."},
		{"./", "."},
		{"../", ".."},
		// is-glob treats a leading ! as negation only for the whole pattern,
		// so an interior !foo, @foo or +foo segment without a following ( is
		// a literal directory name and stays in the base.
		{"path/!foo/", "path/!foo"},
		{"path/@foo/", "path/@foo"},
		{"path/+foo/", "path/+foo"},
		{"path/?foo/", "path/?foo"},
		{`path/\*/`, "path/*"},
	}

	for _, tc := range cases {
		if got := glob.Parent(tc.pattern); got != tc.want {
			t.Errorf("Parent(%q) = %q, gulp gives %q", tc.pattern, got, tc.want)
		}
	}
}

// TestDifferentialDurationFormat checks the duration strings against
// gulp-cli's format-hrtime. Every "Finished 'x' after ..." line depends on it,
// and it is not Go's time.Duration formatting nor npm's pretty-hrtime.
func TestDifferentialDurationFormat(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{7 * time.Nanosecond, "7 ns"},
		{999 * time.Nanosecond, "999 ns"},
		{time.Microsecond, "1 μs"},
		{1320 * time.Nanosecond, "1.32 μs"},
		{1500 * time.Nanosecond, "1.5 μs"},
		{37 * time.Microsecond, "37 μs"},
		{time.Millisecond, "1 ms"},
		{5690 * time.Microsecond, "5.69 ms"},
		{21 * time.Millisecond, "21 ms"},
		{time.Second, "1 s"},
		{1500 * time.Millisecond, "1.5 s"},
		{9999 * time.Millisecond, "10 s"},
		{time.Minute, "1 min"},
		{90 * time.Second, "1.5 min"},
		{time.Hour, "1 h"},
		{90 * time.Minute, "1.5 h"},
	}

	for _, tc := range cases {
		if got := cli.FormatDuration(tc.in); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, gulp gives %q", tc.in, got, tc.want)
		}
	}
}

// TestDifferentialMessages checks the console wording against gulp-cli's
// message catalogue. Users grep these strings in CI logs, so they are part of
// the observable behaviour.
func TestDifferentialMessages(t *testing.T) {
	// Three of these messages route their path through Tildify, which
	// replaces a leading home directory with "~". The paths below are
	// literals, so the assertions would change meaning with the ambient
	// HOME: under HOME=/tmp they read "Using gulpfile ~/gulpfile.go". Pin
	// the home directory to a descendant of /tmp, which cannot prefix the
	// literals, so the test measures wording and nothing else.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	msg := cli.Messages{}

	cases := []struct{ got, want string }{
		{msg.TaskStart("build"), "Starting 'build'..."},
		{msg.TaskStop("build", 21*time.Millisecond), "Finished 'build' after 21 ms"},
		{msg.TaskFailure("build", time.Second), "'build' errored after 1 s"},
		{msg.Gulpfile("/tmp/gulpfile.go"), "Using gulpfile /tmp/gulpfile.go"},
		{msg.Description("/tmp/gulpfile.go"), "Tasks for /tmp/gulpfile.go"},
		{msg.CwdChanged("/tmp"), "Working directory changed to /tmp"},
		{msg.MissingGulpfile(), "No gulpfile found"},
		{
			msg.TaskMissing("buld", []string{"build"}),
			"Task never defined: buld - did you mean? build\n" +
				"To list available tasks, try running: gulp --tasks",
		},
		{
			msg.TaskMissing("nope", nil),
			"Task never defined: nope\n" +
				"To list available tasks, try running: gulp --tasks",
		},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("message = %q, gulp gives %q", tc.got, tc.want)
		}
	}
}

// TestDifferentialTreeShape checks the tree structure against the worked
// example in docs/api/tree.md. Only compositions carry branch, and a task
// reached through a composition is reported as a function rather than a task.
func TestDifferentialTreeShape(t *testing.T) {
	g := gulp.New()
	g.Task("one", func(ctx context.Context) error { return nil })
	g.Task("two", func(ctx context.Context) error { return nil })
	if _, err := g.TaskRef("three", g.Series(gulp.Names("one", "two")...)); err != nil {
		t.Fatalf("TaskRef: %v", err)
	}

	shallow := g.Tree(false)
	if shallow.Label != "Tasks" {
		t.Errorf("shallow root = %q, gulp gives %q", shallow.Label, "Tasks")
	}
	if got := labels(shallow); !equalStrings(got, []string{"one", "two", "three"}) {
		t.Errorf("shallow nodes = %v, want registration order [one two three]", got)
	}

	three := findNode(g.Tree(true), "three")
	if three == nil {
		t.Fatal("deep tree has no node for three")
	}
	if three.Type != "task" || three.Branch {
		t.Errorf("three = {type:%q branch:%v}, gulp gives {type:task, no branch}", three.Type, three.Branch)
	}
	if len(three.Nodes) != 1 {
		t.Fatalf("three has %d children, want 1 <series>", len(three.Nodes))
	}

	series := three.Nodes[0]
	if series.Label != "<series>" || !series.Branch {
		t.Errorf("child = {label:%q branch:%v}, gulp gives {<series>, branch:true}", series.Label, series.Branch)
	}
	// A registered task reached through a composition keeps type "task";
	// only the composition wrapper itself is a "function".
	for _, child := range series.Nodes {
		if child.Type != "task" {
			t.Errorf("%s.type = %q, gulp gives %q", child.Label, child.Type, "task")
		}
	}
}

func findNode(root *gulp.Node, label string) *gulp.Node {
	for _, node := range root.Nodes {
		if node.Label == label {
			return node
		}
	}
	return nil
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Comparison against the real gulp
// ---------------------------------------------------------------------------

// runNode executes a script with the gulp under test importable as "gulp".
func runNode(t *testing.T, repo, script string, env ...string) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "probe.js")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	cmd := exec.Command("node", path)
	cmd.Env = append(os.Environ(), "GULP_MODULE="+repo)
	cmd.Env = append(cmd.Env, env...)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node probe failed: %v\n%s", err, stderr.String())
	}
	return out
}

// orderedGlobsCase names the probe the src and src-order tests share, so both
// read the same recording instead of two copies that could drift apart.
const orderedGlobsCase = "multiple globs in order"

// jsFile is the projection of a vinyl file both implementations report.
type jsFile struct {
	Relative string `json:"relative"`
	Contents string `json:"contents"`
	Dir      bool   `json:"dir"`
	Null     bool   `json:"null"`
}

const srcProbe = `
const gulp = require(process.env.GULP_MODULE);
const globs = JSON.parse(process.env.GLOBS);
const opts = JSON.parse(process.env.OPTS);
const files = [];
gulp.src(globs, opts)
  .on('data', function (f) {
    files.push({
      relative: f.relative.split(require('path').sep).join('/'),
      contents: Buffer.isBuffer(f.contents) ? f.contents.toString('base64') : '',
      dir: f.isDirectory(),
      null: f.isNull(),
    });
  })
  .on('error', function (e) { console.error(e.message); process.exit(1); })
  .on('end', function () { console.log(JSON.stringify(files)); });
`

// collectJS runs gulp.src in Node and returns the files it emitted.
func collectJS(t *testing.T, repo string, globs []string, opts map[string]any, cwd string) []jsFile {
	t.Helper()

	if opts == nil {
		opts = map[string]any{}
	}
	opts["cwd"] = cwd

	globsJSON, err := json.Marshal(globs)
	if err != nil {
		t.Fatalf("marshal globs: %v", err)
	}
	optsJSON, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("marshal opts: %v", err)
	}

	out := runNode(t, repo, srcProbe,
		"GLOBS="+string(globsJSON),
		"OPTS="+string(optsJSON),
	)

	var files []jsFile
	if err := json.Unmarshal(out, &files); err != nil {
		t.Fatalf("decode probe output %q: %v", out, err)
	}
	return files
}

// collectGo runs the port's Src over the same globs and projects the result
// into the same shape.
func collectGo(t *testing.T, globs []string, opts gulp.SrcOptions, cwd string) []jsFile {
	t.Helper()

	opts.Cwd = cwd
	files := collect(t, gulp.Src(globs, opts))

	out := make([]jsFile, 0, len(files))
	for _, f := range files {
		rel, err := f.Relative()
		if err != nil {
			t.Fatalf("relative: %v", err)
		}

		entry := jsFile{
			Relative: filepath.ToSlash(rel),
			Dir:      f.IsDirectory(),
			Null:     f.IsNull(),
		}
		if f.IsBuffer() {
			body, err := f.Bytes()
			if err != nil {
				t.Fatalf("bytes: %v", err)
			}
			entry.Contents = base64.StdEncoding.EncodeToString(body)
		}
		out = append(out, entry)
	}
	return out
}

// TestDifferentialSrcMatchesGulp runs the same globs through both
// implementations over gulp's own fixtures and compares what comes out.
func TestDifferentialSrcMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)
	cwd := testCwd(t)

	cases := []struct {
		name  string
		globs []string
		js    map[string]any
		go_   gulp.SrcOptions
	}{
		{name: "flat glob", globs: []string{"./fixtures/*.coffee"}},
		{name: "deep glob", globs: []string{"./fixtures/**/*.jade"}},
		{name: "deeper glob", globs: []string{"./fixtures/**/*.dmc"}},
		{name: "text files", globs: []string{"./fixtures/**/*.txt"}},
		{
			name:  orderedGlobsCase,
			globs: []string{"./fixtures/stuff/run.dmc", "./fixtures/stuff/test.dmc"},
		},
		{
			name:  "negation",
			globs: []string{"./fixtures/stuff/*.dmc", "!fixtures/stuff/test.dmc"},
		},
		{
			name:  "read disabled",
			globs: []string{"./fixtures/*.coffee"},
			js:    map[string]any{"read": false},
			go_:   gulp.SrcOptions{Read: gulp.Value(false)},
		},
		{
			name:  "directory",
			globs: []string{"./fixtures/stuff"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := replay(t, oracle, oracle.Src, tc.name, func(repo string) []jsFile {
				return collectJS(t, repo, tc.globs, tc.js, cwd)
			})
			got := collectGo(t, tc.globs, tc.go_, cwd)
			compareFiles(t, got, want)
		})
	}
}

// compareFiles reports every difference rather than stopping at the first, so
// one run shows the whole picture.
func compareFiles(t *testing.T, got, want []jsFile) {
	t.Helper()

	byRelative := func(files []jsFile) map[string]jsFile {
		index := make(map[string]jsFile, len(files))
		for _, f := range files {
			index[f.Relative] = f
		}
		return index
	}

	if len(got) != len(want) {
		t.Errorf("emitted %d files, gulp emits %d\n  go:   %v\n  gulp: %v",
			len(got), len(want), relativesOf(got), relativesOf(want))
	}

	gotIndex, wantIndex := byRelative(got), byRelative(want)
	for rel, w := range wantIndex {
		g, ok := gotIndex[rel]
		if !ok {
			t.Errorf("gulp emitted %s, this port did not", rel)
			continue
		}
		if g.Dir != w.Dir {
			t.Errorf("%s: isDirectory = %v, gulp gives %v", rel, g.Dir, w.Dir)
		}
		if g.Null != w.Null {
			t.Errorf("%s: isNull = %v, gulp gives %v", rel, g.Null, w.Null)
		}
		if g.Contents != w.Contents {
			t.Errorf("%s: contents differ\n  go:   %s\n  gulp: %s", rel,
				decodeForLog(g.Contents), decodeForLog(w.Contents))
		}
	}
	for rel := range gotIndex {
		if _, ok := wantIndex[rel]; !ok {
			t.Errorf("this port emitted %s, gulp did not", rel)
		}
	}
}

func relativesOf(files []jsFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Relative)
	}
	sort.Strings(out)
	return out
}

func decodeForLog(encoded string) string {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded
	}
	return string(raw)
}

// TestDifferentialSrcOrderMatchesGulp checks emission order separately,
// because gulp guarantees glob-argument order and dest() relies on it.
func TestDifferentialSrcOrderMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)
	cwd := testCwd(t)
	globs := []string{"./fixtures/stuff/run.dmc", "./fixtures/stuff/test.dmc"}

	recorded := replay(t, oracle, oracle.Src, orderedGlobsCase, func(repo string) []jsFile {
		return collectJS(t, repo, globs, nil, cwd)
	})

	want := relativesInOrder(recorded)
	got := relativesInOrder(collectGo(t, globs, gulp.SrcOptions{}, cwd))

	if !equalStrings(got, want) {
		t.Errorf("order = %v, gulp gives %v", got, want)
	}
}

func relativesInOrder(files []jsFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Relative)
	}
	return out
}

const destProbe = `
const gulp = require(process.env.GULP_MODULE);
gulp.src(JSON.parse(process.env.GLOBS), { cwd: process.env.CWD })
  .pipe(gulp.dest(process.env.OUT))
  .on('error', function (e) { console.error(e.message); process.exit(1); })
  .on('end', function () { console.log('ok'); });
`

// TestDifferentialDestMatchesGulp writes the same source through both
// implementations and compares the resulting directory trees byte for byte.
func TestDifferentialDestMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)
	cwd := testCwd(t)
	globs := []string{"./fixtures/**/*.txt", "./fixtures/**/*.dmc"}

	globsJSON, err := json.Marshal(globs)
	if err != nil {
		t.Fatalf("marshal globs: %v", err)
	}

	jsTree := replay(t, oracle, oracle.Dest, "txt and dmc", func(repo string) map[string]string {
		out := t.TempDir()
		runNode(t, repo, destProbe,
			"GLOBS="+string(globsJSON),
			"CWD="+cwd,
			"OUT="+out,
		)
		return walkTree(t, out)
	})

	goOut := t.TempDir()
	if err := gulp.Src(globs, gulp.SrcOptions{Cwd: cwd}).
		Pipe(gulp.Dest(goOut)).
		Run(context.Background()); err != nil {
		t.Fatalf("go pipeline: %v", err)
	}

	compareTrees(t, walkTree(t, goOut), jsTree)
}

// compareTrees reports every structural or content difference between the two
// output trees.
func compareTrees(t *testing.T, goTree, jsTree map[string]string) {
	t.Helper()

	for rel, jsBody := range jsTree {
		goBody, ok := goTree[rel]
		if !ok {
			t.Errorf("gulp wrote %s, this port did not", rel)
			continue
		}
		if goBody != jsBody {
			t.Errorf("%s: contents differ\n  go:   %q\n  gulp: %q", rel, goBody, jsBody)
		}
	}
	for rel := range goTree {
		if _, ok := jsTree[rel]; !ok {
			t.Errorf("this port wrote %s, gulp did not", rel)
		}
	}
}

// walkTree maps every regular file to its contents, keyed by slash-separated
// relative path. Directories appear as keys with a trailing slash so a missing
// directory is reported too.
func walkTree(t *testing.T, root string) map[string]string {
	t.Helper()

	tree := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		key := filepath.ToSlash(rel)

		if entry.IsDir() {
			tree[key+"/"] = ""
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[key] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return tree
}

const treeProbe = `
const gulp = require(process.env.GULP_MODULE);
function one(cb) { cb(); }
function two(cb) { cb(); }
gulp.task('one', one);
gulp.task('two', two);
gulp.task('three', gulp.series('one', 'two'));
gulp.task('four', gulp.parallel('one', gulp.series('two')));
console.log(JSON.stringify(gulp.tree({ deep: true })));
`

// treeNode mirrors the JSON undertaker produces, so both trees decode into the
// same Go value and can be compared directly.
type treeNode struct {
	Label  string     `json:"label"`
	Type   string     `json:"type,omitempty"`
	Branch bool       `json:"branch,omitempty"`
	Nodes  []treeNode `json:"nodes,omitempty"`
}

// TestDifferentialTreeMatchesGulp builds the same task graph in both
// implementations and compares the deep tree, which is what `gulp --tasks`
// renders.
func TestDifferentialTreeMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)

	recorded := replay(t, oracle, oracle.Text, "tree", func(repo string) string {
		return string(runNode(t, repo, treeProbe))
	})

	var want treeNode
	if err := json.Unmarshal([]byte(recorded), &want); err != nil {
		t.Fatalf("decode gulp tree: %v", err)
	}

	g := gulp.New()
	g.Task("one", func(ctx context.Context) error { return nil })
	g.Task("two", func(ctx context.Context) error { return nil })
	mustTaskRef(t, g, "three", g.Series(gulp.Names("one", "two")...))
	mustTaskRef(t, g, "four", g.Parallel(gulp.Name("one"), g.Series(gulp.Name("two"))))

	encoded, err := undertaker.MarshalTree(g.Tree(true))
	if err != nil {
		t.Fatalf("marshal go tree: %v", err)
	}
	var got treeNode
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode go tree: %v", err)
	}

	compareNodes(t, "", got, want)
}

const treeJSONProbe = `
const gulp = require(process.env.GULP_MODULE);
function clean(cb) { cb(); }
function styles(cb) { cb(); }
gulp.task('clean', clean);
gulp.task('styles', styles);
gulp.task('build', gulp.series('clean', 'styles'));
console.log(JSON.stringify(gulp.tree()));
console.log(JSON.stringify(gulp.tree({ deep: true })));
`

// TestDifferentialTreeJSONMatchesGulp compares the serialised trees byte for
// byte rather than after decoding, which is the only way to catch the shape
// details that a decode hides: that a shallow node is a bare string, that a
// deep leaf still carries an empty nodes array, and that the composition
// labels survive unescaped. This is what `gulp --tasks-json` prints, so a
// consumer parsing that output sees exactly these bytes.
func TestDifferentialTreeJSONMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)

	recorded := replay(t, oracle, oracle.Text, "tree-json", func(repo string) string {
		return string(runNode(t, repo, treeJSONProbe))
	})

	lines := strings.Split(strings.TrimSpace(recorded), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two JSON lines from gulp, got %d", len(lines))
	}

	g := gulp.New()
	g.Task("clean", func(ctx context.Context) error { return nil })
	g.Task("styles", func(ctx context.Context) error { return nil })
	mustTaskRef(t, g, "build", g.Series(gulp.Names("clean", "styles")...))

	for i, deep := range []bool{false, true} {
		encoded, err := undertaker.MarshalTree(g.Tree(deep))
		if err != nil {
			t.Fatalf("marshal go tree(deep=%v): %v", deep, err)
		}
		if string(encoded) != lines[i] {
			t.Errorf("tree(deep=%v) mismatch\n go:   %s\n gulp: %s", deep, encoded, lines[i])
		}
	}
}

func mustTaskRef(t *testing.T, g *gulp.Gulp, name string, ref *undertaker.Task) {
	t.Helper()

	if _, err := g.TaskRef(name, ref); err != nil {
		t.Fatalf("TaskRef(%q): %v", name, err)
	}
}

// compareNodes walks both trees in parallel, reporting the path to any
// difference so a deep mismatch is readable.
func compareNodes(t *testing.T, path string, got, want treeNode) {
	t.Helper()

	where := path + "/" + want.Label
	if got.Label != want.Label {
		t.Errorf("%s: label = %q, gulp gives %q", where, got.Label, want.Label)
		return
	}
	if got.Type != want.Type {
		t.Errorf("%s: type = %q, gulp gives %q", where, got.Type, want.Type)
	}
	if got.Branch != want.Branch {
		t.Errorf("%s: branch = %v, gulp gives %v", where, got.Branch, want.Branch)
	}
	if len(got.Nodes) != len(want.Nodes) {
		t.Errorf("%s: %d children, gulp gives %d", where, len(got.Nodes), len(want.Nodes))
		return
	}
	for i := range want.Nodes {
		compareNodes(t, where, got.Nodes[i], want.Nodes[i])
	}
}

const simpleProbe = `
const gulp = require(process.env.GULP_MODULE);
gulp.task('clean', function clean(cb) { cb(); });
gulp.task('styles', function styles(cb) { cb(); });
gulp.task('build', gulp.series('clean', 'styles'));
const tree = gulp.tree();
console.log(tree.nodes.join('\n').trim());
`

// TestDifferentialTasksSimpleMatchesGulp checks that --tasks-simple lists the
// same names in the same order, which is registration order rather than
// alphabetical.
func TestDifferentialTasksSimpleMatchesGulp(t *testing.T) {
	oracle := loadOracle(t)

	recorded := replay(t, oracle, oracle.Text, "tasks-simple", func(repo string) string {
		return string(runNode(t, repo, simpleProbe))
	})
	want := strings.TrimSpace(recorded)

	g := gulp.New()
	g.Task("clean", func(ctx context.Context) error { return nil })
	g.Task("styles", func(ctx context.Context) error { return nil })
	mustTaskRef(t, g, "build", g.Series(gulp.Names("clean", "styles")...))

	got := cli.SimpleList(g.Tree(false))
	if got != want {
		t.Errorf("--tasks-simple =\n%s\ngulp gives\n%s", got, want)
	}
}
