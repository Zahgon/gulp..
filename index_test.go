package gulp_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/internal/cli"
	"github.com/gulpjs/gulp-go/undertaker"
)

// The ten tests below port test/index.test.js, which asserts each documented
// property is present on the exported instance. Go resolves membership at
// compile time, so the typed binding opening each test is the presence check.

func TestSrc(t *testing.T) {
	var src func([]string, ...gulp.SrcOptions) *gulp.Pipeline = gulp.Src

	dir := t.TempDir()
	sourceFile(t, dir, "a.txt", "one")

	if files := collect(t, src([]string{filepath.Join(dir, "*.txt")})); len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
}

func TestDest(t *testing.T) {
	var dest func(string, ...gulp.DestOptions) gulp.Transform = gulp.Dest

	src, out := t.TempDir(), t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}).Pipe(dest(out)))
	if got := onDisk(t, filepath.Join(out, "a.txt")); got != "one" {
		t.Errorf("written contents = %q, want %q", got, "one")
	}
}

func TestSymlink(t *testing.T) {
	var symlink func(string, ...gulp.SymlinkOptions) gulp.Transform = gulp.Symlink

	src, out := t.TempDir(), t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}).Pipe(symlink(out)))
	info, err := os.Lstat(filepath.Join(out, "a.txt"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("mode = %v, want a symlink", info.Mode())
	}
}

func TestWatch(t *testing.T) {
	var watch func([]string, gulp.WatchOptions, gulp.TaskFunc) (*gulp.Watcher, error) = gulp.Watch

	w, err := watch([]string{filepath.Join(t.TempDir(), "*.txt")}, gulp.WatchOptions{}, nil)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestTask(t *testing.T) {
	var task func(string, gulp.TaskFunc) *undertaker.Task = gulp.Task

	registered := task("pkg-export-task", func(context.Context) error { return nil })
	if registered == nil {
		t.Fatal("task should hand back the registration")
	}
	if _, ok := gulp.GetTask("pkg-export-task"); !ok {
		t.Error("task should be reachable by name afterwards")
	}
}

func TestSeries(t *testing.T) {
	var series func(...gulp.Ref) *undertaker.Task = gulp.Series

	var order []string
	step := func(name string) gulp.Ref {
		return gulp.Fn(name, func(context.Context) error {
			order = append(order, name)
			return nil
		})
	}

	if err := gulp.Run(context.Background(), series(step("a"), step("b"))); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(order, ","); got != "a,b" {
		t.Errorf("order = %q, want %q", got, "a,b")
	}
}

func TestParallel(t *testing.T) {
	var parallel func(...gulp.Ref) *undertaker.Task = gulp.Parallel

	var ran sync.Map
	step := func(name string) gulp.Ref {
		return gulp.Fn(name, func(context.Context) error {
			ran.Store(name, true)
			return nil
		})
	}

	if err := gulp.Run(context.Background(), parallel(step("a"), step("b"))); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if _, ok := ran.Load(name); !ok {
			t.Errorf("%q did not run", name)
		}
	}
}

func TestTree(t *testing.T) {
	var tree func(bool) *gulp.Node = gulp.Tree

	gulp.Task("pkg-export-tree", func(context.Context) error { return nil })

	root := tree(false)
	if !contains(labels(root), "pkg-export-tree") {
		t.Errorf("tree %v does not list pkg-export-tree", labels(root))
	}
}

func TestLastRun(t *testing.T) {
	var lastRun func(gulp.Ref, time.Duration) (time.Time, bool, error) = gulp.LastRun

	gulp.Task("pkg-export-last-run", func(context.Context) error { return nil })
	if err := gulp.Run(context.Background(), gulp.Name("pkg-export-last-run")); err != nil {
		t.Fatalf("run: %v", err)
	}

	at, ok, err := lastRun(gulp.Name("pkg-export-last-run"), 0)
	if err != nil {
		t.Fatalf("last run: %v", err)
	}
	if !ok || at.IsZero() {
		t.Errorf("last run = (%v, %v), want a recorded time", at, ok)
	}
}

func TestRegistry(t *testing.T) {
	var registry func() undertaker.Registry = gulp.Registry

	if registry() == nil {
		t.Fatal("registry should back the default instance")
	}
}

// TestInstanceIsAnEmitter covers the gulpfile.mjs fixture's
// `gulp instanceof EventEmitter` assertion. Go has no EventEmitter; the
// equivalent capability is the task event stream the CLI subscribes to.
func TestInstanceIsAnEmitter(t *testing.T) {
	g := gulp.New()
	seen := make(chan gulp.Event, 4)
	g.On(func(evt gulp.Event) { seen <- evt })
	g.Task("emit", func(context.Context) error { return nil })

	if err := g.Run(context.Background(), gulp.Name("emit")); err != nil {
		t.Fatalf("run: %v", err)
	}
	close(seen)

	var kinds []string
	for evt := range seen {
		kinds = append(kinds, evt.Kind.String())
	}
	if got := strings.Join(kinds, ","); got != "start,stop" {
		t.Fatalf("events = %q, want \"start,stop\"", got)
	}
}

// TestNewInstancesAreIndependent covers `Gulp.prototype.Gulp = Gulp`, the line
// in index.js that lets callers build a registry of their own rather than
// mutating the shared one.
func TestNewInstancesAreIndependent(t *testing.T) {
	a, b := gulp.New(), gulp.New()
	a.Task("only-on-a", func(context.Context) error { return nil })

	if _, ok := b.GetTask("only-on-a"); ok {
		t.Fatal("instances should not share a registry")
	}
	if _, ok := gulp.GetTask("only-on-a"); ok {
		t.Fatal("a new instance should not write to the default one")
	}
}

// TestRunsAgainstAGulpfile and TestRunsAgainstAnInstanceGulpfile port the two
// spawn cases in test/index.test.js, which run `node bin/gulp.js` inside the
// cjs and mjs fixture directories. Go has one module system, so the two shapes
// a gulpfile can take here are the package-level API and an explicit instance.
func TestRunsAgainstAGulpfile(t *testing.T) {
	runGulpfile(t, "go")
}

func TestRunsAgainstAnInstanceGulpfile(t *testing.T) {
	runGulpfile(t, "instance")
}

// TestCanRunAgainstAnAlternateGulpfileName and TestCanRunAgainstAGulpfileDirectory
// port the two spawn cases in test/index.test.js. The JavaScript suite runs the
// CLI once per gulpfile shape it accepts beyond the default, and
// cli.GulpfileNames is the Go equivalent of that list: gulpfile.cjs and
// gulpfile.mjs there, Gulpfile.go and gulpfile/main.go here. Running in process
// keeps both cases free of a compiler and a network while asserting what the
// JavaScript pair assert.
func TestCanRunAgainstAnAlternateGulpfileName(t *testing.T) {
	runAgainstGulpfileNamed(t, cli.GulpfileNames[1])
}

func TestCanRunAgainstAGulpfileDirectory(t *testing.T) {
	runAgainstGulpfileNamed(t, cli.GulpfileNames[2])
}

func runAgainstGulpfileNamed(t *testing.T, name string) {
	t.Helper()
	keepCwd(t)
	gulpfile := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Dir(gulpfile), 0o755); err != nil {
		t.Fatalf("create gulpfile directory: %v", err)
	}
	if err := os.WriteFile(gulpfile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write gulpfile: %v", err)
	}

	var stdout, stderr strings.Builder
	runtime := cli.Runtime{
		Undertaker: undertaker.New(),
		Stdout:     &stdout,
		Stderr:     &stderr,
	}
	runtime.Undertaker.Set("build", func(context.Context) error { return nil })

	if code := cli.Run(runtime, []string{"--gulpfile", gulpfile, "--tasks"}); code != cli.ExitOK {
		t.Fatalf("gulp exited with %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), name) {
		t.Errorf("stdout should name the gulpfile, got:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty, got:\n%s", stderr.String())
	}
}

// keepCwd guards against locating a gulpfile chdir-ing the whole process into a
// t.TempDir that cleanup then deletes: on Linux every later os.Getwd fails.
func keepCwd(t *testing.T) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func runGulpfile(t *testing.T, fixture string) {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles a gulpfile; skipped under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	dir := stageGulpfile(t, fixture)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("gulp exited with %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "gulpfile.go") {
		t.Errorf("stdout should name the gulpfile, got:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty, got:\n%s", stderr.String())
	}
}

// stageGulpfile copies the gulpfile fixture into a throwaway module wired back
// to this checkout.
//
// The replace directive points at a symlink rather than at the repository
// path, because go.mod has no quoting syntax and this checkout may live under
// a directory whose name contains spaces.
func stageGulpfile(t *testing.T, fixture string) string {
	t.Helper()

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "gulp-go")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, "test", "fixtures", "gulpfiles", fixture, "gulpfile.go"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("gulpfile.go", string(body))
	write("go.mod", strings.Join([]string{
		"module gulpfilefixture",
		"",
		"go " + moduleGoVersion(t, root),
		"",
		"require github.com/gulpjs/gulp-go v0.0.0",
		"",
		"replace github.com/gulpjs/gulp-go => ./gulp-go",
		"",
	}, "\n"))

	copyFile(t, filepath.Join(root, "go.sum"), filepath.Join(dir, "go.sum"))
	return dir
}

// moduleGoVersion reads the go directive out of the module's own go.mod.
//
// The staged gulpfile needs a go.mod of its own, and writing a version into it
// by hand creates a second place where the language floor is declared. Those
// two copies drifted apart once already: the module dropped to 1.23 while this
// fixture still asked for 1.24, so the test failed on the very toolchain the
// change was meant to support. Reading the real value is what stops that
// happening again.
func moduleGoVersion(t *testing.T, root string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if version, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
			return strings.TrimSpace(version)
		}
	}
	t.Fatalf("no go directive in %s/go.mod", root)
	return ""
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	body, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	if err := os.WriteFile(to, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
}
