package gulp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/undertaker"
	"github.com/gulpjs/gulp-go/vinyl"
)

// The tests below drive the package-level functions rather than the methods on
// a Gulp value. They are the Go spelling of index.mjs's named exports, so a
// gulpfile that imports this package reaches these and nothing else. Exercising
// the methods alone would leave the whole delegation layer unproven.
//
// Everything registers under a "pkg-" prefix because these share the Default
// instance with the rest of the suite, and assertions check membership rather
// than counts for the same reason.

func sourceFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPackageSrcAndDest(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	sourceFile(t, src, "js/app.js", "app")

	files := collect(t, gulp.Src([]string{filepath.Join(src, "js", "*.js")}).
		Pipe(gulp.Dest(out)))

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if got := onDisk(t, filepath.Join(out, "app.js")); got != "app" {
		t.Errorf("written contents = %q, want %q", got, "app")
	}
}

func TestPackageDestWith(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	// A per-file destination is how a JavaScript gulpfile passes a function
	// to dest(); the Go spelling is a separate constructor because Go has no
	// union types.
	files := collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}).
		Pipe(gulp.DestWith(func(f *gulp.File) string {
			return filepath.Join(out, filepath.Ext(f.Path())[1:])
		})))

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if got := onDisk(t, filepath.Join(out, "txt", "a.txt")); got != "one" {
		t.Errorf("written contents = %q, want %q", got, "one")
	}
}

func TestPackageSymlink(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	files := collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Read: gulp.Value(false),
	}).Pipe(gulp.Symlink(out)))

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	link := filepath.Join(out, "a.txt")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", link)
	}
	if got := onDisk(t, link); got != "one" {
		t.Errorf("link resolves to %q, want %q", got, "one")
	}
}

func TestPackageSymlinkWith(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	files := collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Read: gulp.Value(false),
	}).Pipe(gulp.SymlinkWith(func(*gulp.File) string {
		return filepath.Join(out, "nested")
	})))

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if _, err := os.Lstat(filepath.Join(out, "nested", "a.txt")); err != nil {
		t.Fatalf("expected a link under nested/: %v", err)
	}
}

func TestPackageTaskRegistrationAndRun(t *testing.T) {
	var ran bool
	task := gulp.Task("pkg-run", func(context.Context) error {
		ran = true
		return nil
	})
	if task == nil {
		t.Fatal("Task returned nil")
	}

	if got, ok := gulp.GetTask("pkg-run"); !ok || got == nil {
		t.Fatal("GetTask did not find the task just registered")
	}
	if _, ok := gulp.Tasks()["pkg-run"]; !ok {
		t.Error("Tasks() does not contain pkg-run")
	}

	if err := gulp.Run(context.Background(), gulp.Name("pkg-run")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran {
		t.Error("the task did not run")
	}
}

func TestPackageSeriesParallelAndTaskRef(t *testing.T) {
	var order []string
	gulp.Task("pkg-first", func(context.Context) error {
		order = append(order, "first")
		return nil
	})
	gulp.Task("pkg-second", func(context.Context) error {
		order = append(order, "second")
		return nil
	})

	series := gulp.Series(gulp.Names("pkg-first", "pkg-second")...)
	if _, err := gulp.TaskRef("pkg-series", series); err != nil {
		t.Fatalf("TaskRef: %v", err)
	}
	if err := gulp.Run(context.Background(), gulp.Name("pkg-series")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("order = %v, want [first second]", order)
	}

	// The counter is atomic because Parallel really does run these on
	// separate goroutines. A plain int here is a data race, and the race
	// detector reports it as one.
	var count atomic.Int64
	parallel := gulp.Parallel(
		gulp.Anonymous(func(context.Context) error { count.Add(1); return nil }),
		gulp.Anonymous(func(context.Context) error { count.Add(1); return nil }),
	)
	if err := parallel.Fn(context.Background()); err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if count.Load() != 2 {
		t.Errorf("count = %d, want 2", count.Load())
	}
}

func TestPackageTree(t *testing.T) {
	gulp.Task("pkg-tree-leaf", func(context.Context) error { return nil })

	shallow := gulp.Tree(false)
	if shallow.Label != "Tasks" {
		t.Errorf("shallow root label = %q, want %q", shallow.Label, "Tasks")
	}
	if !contains(labels(shallow), "pkg-tree-leaf") {
		t.Errorf("shallow tree %v does not list pkg-tree-leaf", labels(shallow))
	}

	deep := gulp.Tree(true)
	if !contains(labels(deep), "pkg-tree-leaf") {
		t.Errorf("deep tree %v does not list pkg-tree-leaf", labels(deep))
	}
}

func TestPackageLastRun(t *testing.T) {
	gulp.Task("pkg-lastrun", func(context.Context) error { return nil })

	if _, ok, err := gulp.LastRun(gulp.Name("pkg-lastrun"), 0); err != nil {
		t.Fatalf("LastRun: %v", err)
	} else if ok {
		t.Error("a task that has never run reports a last run")
	}

	before := time.Now()
	if err := gulp.Run(context.Background(), gulp.Name("pkg-lastrun")); err != nil {
		t.Fatalf("Run: %v", err)
	}

	at, ok, err := gulp.LastRun(gulp.Name("pkg-lastrun"), 0)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if !ok {
		t.Fatal("no last run recorded after a successful run")
	}
	// LastRun floors to milliseconds, matching JavaScript timestamps, so the
	// recorded value can sit just before the reference reading.
	if at.Add(time.Millisecond).Before(before) {
		t.Errorf("last run %v is before the run started at %v", at, before)
	}
}

func TestPackageOnReportsEvents(t *testing.T) {
	var kinds []string
	gulp.On(func(evt gulp.Event) {
		if evt.Name == "pkg-events" {
			kinds = append(kinds, evt.Kind.String())
		}
	})

	gulp.Task("pkg-events", func(context.Context) error { return nil })
	if err := gulp.Run(context.Background(), gulp.Name("pkg-events")); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(kinds) != 2 || kinds[0] != "start" || kinds[1] != "stop" {
		t.Errorf("kinds = %v, want [start stop]", kinds)
	}
}

func TestPackageRegistry(t *testing.T) {
	original := gulp.Registry()
	if original == nil {
		t.Fatal("Registry returned nil")
	}

	gulp.Task("pkg-registry", func(context.Context) error { return nil })

	replacement := undertaker.NewDefaultRegistry()
	if err := gulp.SetRegistry(replacement); err != nil {
		t.Fatalf("SetRegistry: %v", err)
	}
	// Restore before asserting, so a failure below cannot leave the shared
	// instance pointing at this test's registry.
	t.Cleanup(func() {
		if err := gulp.SetRegistry(original); err != nil {
			t.Errorf("restoring the registry: %v", err)
		}
	})

	if gulp.Registry() != replacement {
		t.Error("Registry did not return the registry just set")
	}
	if _, ok := replacement.Get("pkg-registry"); !ok {
		t.Error("SetRegistry did not transfer the existing tasks")
	}
	if err := gulp.SetRegistry(nil); err == nil {
		t.Error("SetRegistry(nil) was accepted")
	}
}

func TestPackageWatchIsClosable(t *testing.T) {
	dir := t.TempDir()

	w, err := gulp.Watch([]string{"*.txt"}, gulp.WatchOptions{Cwd: dir}, nil)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestPackageOptionHelpers(t *testing.T) {
	src := t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	// Value pins an option; Func computes it per file. Both must reach
	// vfs.Option[T] identically, so a stream is produced either way.
	byValue := collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Buffer: gulp.Value(false),
	}))
	if len(byValue) != 1 || !byValue[0].IsStream() {
		t.Fatalf("Value(false) did not produce a streaming file")
	}

	byFunc := collect(t, gulp.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Buffer: gulp.Func(func(*gulp.File) bool { return false }),
	}))
	if len(byFunc) != 1 || !byFunc[0].IsStream() {
		t.Fatalf("Func returning false did not produce a streaming file")
	}

	delay := gulp.Ptr(250 * time.Millisecond)
	if delay == nil || *delay != 250*time.Millisecond {
		t.Errorf("Ptr = %v, want a pointer to 250ms", delay)
	}
}

func TestInstanceDestWithAndSymlinkWith(t *testing.T) {
	g := gulp.New()
	src := t.TempDir()
	out := t.TempDir()
	sourceFile(t, src, "a.txt", "one")

	files := collect(t, g.Src([]string{filepath.Join(src, "*.txt")}).
		Pipe(g.DestWith(func(*gulp.File) string { return out })))
	if len(files) != 1 {
		t.Fatalf("DestWith emitted %d files, want 1", len(files))
	}
	if got := onDisk(t, filepath.Join(out, "a.txt")); got != "one" {
		t.Errorf("contents = %q, want %q", got, "one")
	}

	links := t.TempDir()
	linked := collect(t, g.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Read: gulp.Value(false),
	}).Pipe(g.Symlink(links)))
	if len(linked) != 1 {
		t.Fatalf("Symlink emitted %d files, want 1", len(linked))
	}

	more := t.TempDir()
	linkedWith := collect(t, g.Src([]string{filepath.Join(src, "*.txt")}, gulp.SrcOptions{
		Read: gulp.Value(false),
	}).Pipe(g.SymlinkWith(func(*gulp.File) string { return more })))
	if len(linkedWith) != 1 {
		t.Fatalf("SymlinkWith emitted %d files, want 1", len(linkedWith))
	}
}

func TestPackageRunReportsAnUndefinedTask(t *testing.T) {
	err := gulp.Run(context.Background(), gulp.Name("pkg-does-not-exist"))
	if err == nil {
		t.Fatal("running an undefined task succeeded")
	}
	var undefined *undertaker.UndefinedTaskError
	if !errors.As(err, &undefined) {
		t.Fatalf("error = %v, want an UndefinedTaskError", err)
	}
	if undefined.Name != "pkg-does-not-exist" {
		t.Errorf("name = %q, want %q", undefined.Name, "pkg-does-not-exist")
	}
}

func TestPackageFnBuildsANamedTask(t *testing.T) {
	var ran bool
	task := gulp.Fn("pkg-fn", func(context.Context) error {
		ran = true
		return nil
	})
	if task.Name != "pkg-fn" {
		t.Errorf("name = %q, want %q", task.Name, "pkg-fn")
	}
	if _, registered := gulp.GetTask("pkg-fn"); registered {
		t.Error("Fn registered the task; it should only build one")
	}
	if err := task.Fn(context.Background()); err != nil {
		t.Fatalf("running the task: %v", err)
	}
	if !ran {
		t.Error("the task did not run")
	}
}

func TestPackageSrcAcceptsAVinylFileThrough(t *testing.T) {
	// pipeline.From is the seam a gulpfile uses to inject a generated file,
	// so it must accept a hand-built vinyl and carry it to Dest unchanged.
	out := t.TempDir()
	base := t.TempDir()

	file, err := vinyl.New(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "generated.txt"),
		Contents: vinyl.Buffer([]byte("made up")),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}

	files := collect(t, pipeline.New(pipeline.From(file)).Pipe(gulp.Dest(out)))
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if got := onDisk(t, filepath.Join(out, "generated.txt")); got != "made up" {
		t.Errorf("contents = %q, want %q", got, "made up")
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestPackageExecuteReportsTheVersion(t *testing.T) {
	if code := gulp.Execute([]string{"--version"}); code != 0 {
		t.Errorf("Execute(--version) = %d, want 0", code)
	}
}
