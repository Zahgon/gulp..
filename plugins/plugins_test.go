package plugins

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// file builds a vinyl file rooted in a real directory.
//
// The directory has to exist because Exec runs its command with the file's cwd
// as the working directory, and a child process cannot chdir into a path that
// is not there.
func file(t *testing.T, rel, body string) *vinyl.File {
	t.Helper()
	base := t.TempDir()
	f, err := vinyl.New(vinyl.Options{
		Cwd:      filepath.Dir(base),
		Base:     base,
		Path:     filepath.Join(base, rel),
		Contents: vinyl.Buffer([]byte(body)),
	})
	if err != nil {
		t.Fatalf("vinyl.New(%q): %v", rel, err)
	}
	return f
}

func run(t *testing.T, source []*vinyl.File, stages ...pipeline.Transform) []*vinyl.File {
	t.Helper()
	out, err := pipeline.New(pipeline.From(source...)).PipeAll(stages...).Collect(context.Background())
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	return out
}

func relatives(t *testing.T, files []*vinyl.File) []string {
	t.Helper()
	names := make([]string, 0, len(files))
	for _, f := range files {
		rel, err := f.Relative()
		if err != nil {
			t.Fatalf("Relative(): %v", err)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	return names
}

func body(t *testing.T, f *vinyl.File) string {
	t.Helper()
	b, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}
	return string(b)
}

func TestConcatJoinsFilesInOrder(t *testing.T) {
	in := []*vinyl.File{
		file(t, "a.js", "one"),
		file(t, "nested/b.js", "two"),
		file(t, "c.js", "three"),
	}

	out := run(t, in, Concat("bundle.js"))

	if len(out) != 1 {
		t.Fatalf("got %d files, want 1", len(out))
	}
	if got, want := body(t, out[0]), "one\ntwo\nthree"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
	if got, want := relatives(t, out)[0], "bundle.js"; got != want {
		t.Errorf("relative path = %q, want %q", got, want)
	}
	if got, want := out[0].Base(), in[0].Base(); got != want {
		t.Errorf("base = %q, want %q", got, want)
	}
}

func TestConcatSeparatedUsesSeparator(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "one"), file(t, "b.js", "two")}

	out := run(t, in, ConcatSeparated("bundle.js", ";"))

	if got, want := body(t, out[0]), "one;two"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestConcatEmitsNothingForEmptyStream(t *testing.T) {
	out := run(t, nil, Concat("bundle.js"))

	if len(out) != 0 {
		t.Fatalf("got %d files, want 0", len(out))
	}
}

func TestRenameRewritesEachPart(t *testing.T) {
	in := []*vinyl.File{file(t, "js/app.js", "x")}

	out := run(t, in, Rename(func(p *Path) {
		p.Dirname = filepath.Join(p.Dirname, "vendor")
		p.Basename += ".min"
		p.Extname = ".mjs"
	}))

	if got, want := relatives(t, out)[0], "js/vendor/app.min.mjs"; got != want {
		t.Errorf("relative path = %q, want %q", got, want)
	}
}

func TestRenameFlattens(t *testing.T) {
	in := []*vinyl.File{file(t, "deeply/nested/app.js", "x")}

	out := run(t, in, Rename(func(p *Path) { p.Dirname = "." }))

	if got, want := relatives(t, out)[0], "app.js"; got != want {
		t.Errorf("relative path = %q, want %q", got, want)
	}
}

func TestRenameToSetsWholeName(t *testing.T) {
	in := []*vinyl.File{file(t, "js/app.js", "x")}

	out := run(t, in, RenameTo("bundle.js"))

	if got, want := relatives(t, out)[0], "bundle.js"; got != want {
		t.Errorf("relative path = %q, want %q", got, want)
	}
}

func TestReplaceSubstitutesEveryOccurrence(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "a VERSION b VERSION")}

	out := run(t, in, Replace("VERSION", "1.2.3"))

	if got, want := body(t, out[0]), "a 1.2.3 b 1.2.3"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReplaceRegexpExpandsCaptureGroups(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "import x from './y.ts'")}

	out := run(t, in, ReplaceRegexp(regexp.MustCompile(`'\./(\w+)\.ts'`), `"./$1.js"`))

	if got, want := body(t, out[0]), `import x from "./y.js"`; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReplaceFuncTransformsEachMatch(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "one two")}

	out := run(t, in, ReplaceFunc(regexp.MustCompile(`\w+`), strings.ToUpper))

	if got, want := body(t, out[0]), "ONE TWO"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReplaceLeavesNullFilesAlone(t *testing.T) {
	f, err := vinyl.New(vinyl.Options{
		Cwd:  string(filepath.Separator),
		Base: filepath.Join(string(filepath.Separator), "src"),
		Path: filepath.Join(string(filepath.Separator), "src", "a.js"),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}

	out := run(t, []*vinyl.File{f}, Replace("a", "b"))

	if len(out) != 1 || !out[0].IsNull() {
		t.Fatalf("null file did not pass through unchanged")
	}
}

func TestFilterKeepsMatchesAndHonoursNegation(t *testing.T) {
	in := []*vinyl.File{
		file(t, "a.js", "1"),
		file(t, "a.min.js", "2"),
		file(t, "styles/b.css", "3"),
	}

	out := run(t, in, Filter([]string{"**/*.js", "!**/*.min.js"}))

	if got, want := relatives(t, out), []string{"a.js"}; !equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

func TestFilterReportsBadPatternWhenRun(t *testing.T) {
	_, err := pipeline.New(pipeline.From()).Pipe(Filter([]string{"["})).Collect(context.Background())

	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want a *plugins.Error", err)
	}
}

func TestIfRoutesToBothBranches(t *testing.T) {
	in := []*vinyl.File{
		file(t, "a.js", "keep"),
		file(t, "b.css", "keep"),
		file(t, "c.js", "keep"),
	}
	isJS := func(f *vinyl.File) bool { return filepath.Ext(f.Path()) == ".js" }

	out := run(t, in, If(isJS, Replace("keep", "js"), Replace("keep", "css")))

	got := map[string]string{}
	for _, f := range out {
		rel, _ := f.Relative()
		got[filepath.ToSlash(rel)] = body(t, f)
	}
	want := map[string]string{"a.js": "js", "b.css": "css", "c.js": "js"}
	for name, contents := range want {
		if got[name] != contents {
			t.Errorf("%s = %q, want %q", name, got[name], contents)
		}
	}
}

func TestIfPassesThroughNilBranch(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "x"), file(t, "b.css", "x")}
	isJS := func(f *vinyl.File) bool { return filepath.Ext(f.Path()) == ".js" }

	out := run(t, in, If(isJS, Replace("x", "y"), nil))

	if len(out) != 2 {
		t.Fatalf("got %d files, want 2", len(out))
	}
	for _, f := range out {
		want := "x"
		if filepath.Ext(f.Path()) == ".js" {
			want = "y"
		}
		if got := body(t, f); got != want {
			t.Errorf("%s = %q, want %q", f.Basename(), got, want)
		}
	}
}

func TestIfPropagatesBranchError(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "x")}
	boom := errors.New("boom")
	failing := pipeline.Map(func(context.Context, *vinyl.File) (*vinyl.File, error) { return nil, boom })

	_, err := pipeline.New(pipeline.From(in...)).
		Pipe(If(func(*vinyl.File) bool { return true }, failing, nil)).
		Collect(context.Background())

	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestExecPipesContentsThroughCommand(t *testing.T) {
	requireCommand(t, "tr")
	in := []*vinyl.File{file(t, "a.txt", "hello")}

	out := run(t, in, Exec(ExecOptions{
		Command: "tr",
		Args:    func(*vinyl.File) []string { return []string{"a-z", "A-Z"} },
	}))

	if got, want := body(t, out[0]), "HELLO"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestExecRenamesOutput(t *testing.T) {
	requireCommand(t, "cat")
	in := []*vinyl.File{file(t, "a.scss", "body{}")}

	out := run(t, in, Exec(ExecOptions{
		Command: "cat",
		Rename:  func(p *Path) { p.Extname = ".css" },
	}))

	if got, want := relatives(t, out)[0], "a.css"; got != want {
		t.Errorf("relative path = %q, want %q", got, want)
	}
}

func TestExecSubstitutesTempFilePath(t *testing.T) {
	requireCommand(t, "cat")
	in := []*vinyl.File{file(t, "a.txt", "from temp")}

	out := run(t, in, Exec(ExecOptions{
		Command:  "cat",
		TempFile: true,
		Args:     func(*vinyl.File) []string { return []string{Placeholder} },
	}))

	if got, want := body(t, out[0]), "from temp"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestExecReportsStderrOnFailure(t *testing.T) {
	requireCommand(t, "sh")
	in := []*vinyl.File{file(t, "a.txt", "x")}

	_, err := pipeline.New(pipeline.From(in...)).
		Pipe(Exec(ExecOptions{
			Name:    "boom",
			Command: "sh",
			Args:    func(*vinyl.File) []string { return []string{"-c", "echo detailed reason >&2; exit 3"} },
		})).
		Collect(context.Background())

	if err == nil {
		t.Fatal("expected an error")
	}
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want a *plugins.Error", err)
	}
	if perr.Plugin != "boom" {
		t.Errorf("plugin = %q, want %q", perr.Plugin, "boom")
	}
	if !strings.Contains(err.Error(), "detailed reason") {
		t.Errorf("error %q does not include stderr", err)
	}
}

func TestExecWithoutCommandFails(t *testing.T) {
	_, err := pipeline.New(pipeline.From()).Pipe(Exec(ExecOptions{})).Collect(context.Background())

	if err == nil || !strings.Contains(err.Error(), "no command specified") {
		t.Fatalf("error = %v, want a missing-command error", err)
	}
}

func TestSourcemapsInitAndWriteInline(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "var a = 1;\n")}

	out := run(t, in, SourcemapsInit(), SourcemapsWrite(""))

	if len(out) != 1 {
		t.Fatalf("got %d files, want 1 (inline maps emit no extra file)", len(out))
	}
	if !strings.Contains(body(t, out[0]), "sourceMappingURL=data:application/json") {
		t.Errorf("contents lack an inline source map: %q", body(t, out[0]))
	}
}

func TestSourcemapsWriteEmitsMapFile(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "var a = 1;\n")}

	out := run(t, in, SourcemapsInit(), SourcemapsWrite("maps"))

	names := relatives(t, out)
	sort.Strings(names)
	if want := []string{"a.js", "maps/a.js.map"}; !equal(names, want) {
		t.Fatalf("emitted %v, want %v", names, want)
	}
}

func TestSourcemapsWriteSkipsFilesWithoutMaps(t *testing.T) {
	in := []*vinyl.File{file(t, "a.js", "var a = 1;\n")}

	out := run(t, in, SourcemapsWrite("maps"))

	if len(out) != 1 {
		t.Fatalf("got %d files, want 1", len(out))
	}
}

func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not available", name)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
