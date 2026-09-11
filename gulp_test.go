package gulp_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/vinyl"
)

// testCwd is the directory the JavaScript suites run from: test/, so that
// their globs can be written as ./fixtures/*.
func testCwd(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("test")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return dir
}

func collect(t *testing.T, p *gulp.Pipeline) []*vinyl.File {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files, err := p.Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return files
}

func contents(t *testing.T, f *vinyl.File) string {
	t.Helper()
	body, err := f.Bytes()
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}
	return string(body)
}

func onDisk(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// TestSrcThroughFacade ports the cases of test/src.js that prove the wiring,
// leaving exhaustive option coverage to vfs's own suite.
func TestSrcThroughFacade(t *testing.T) {
	cwd := testCwd(t)

	t.Run("returns a pipeline", func(t *testing.T) {
		if gulp.Src([]string{"./fixtures/*.coffee"}, gulp.SrcOptions{Cwd: cwd}) == nil {
			t.Fatal("Src returned nil")
		}
	})

	t.Run("reads a flat glob", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{"./fixtures/*.coffee"}, gulp.SrcOptions{Cwd: cwd}))
		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		want := filepath.Join(cwd, "fixtures", "test.coffee")
		if files[0].Path() != want {
			t.Errorf("path = %q, want %q", files[0].Path(), want)
		}
		if got := contents(t, files[0]); got != onDisk(t, want) {
			t.Errorf("contents = %q", got)
		}
	})

	t.Run("keeps glob argument order", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{
			"./fixtures/stuff/run.dmc",
			"./fixtures/stuff/test.dmc",
		}, gulp.SrcOptions{Cwd: cwd}))
		if len(files) != 2 {
			t.Fatalf("got %d files, want 2", len(files))
		}
		if filepath.Base(files[0].Path()) != "run.dmc" || filepath.Base(files[1].Path()) != "test.dmc" {
			t.Errorf("order = %s, %s", files[0].Path(), files[1].Path())
		}
	})

	t.Run("honours negation", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{
			"./fixtures/stuff/*.dmc",
			"!fixtures/stuff/test.dmc",
		}, gulp.SrcOptions{Cwd: cwd}))
		if len(files) != 1 || filepath.Base(files[0].Path()) != "run.dmc" {
			t.Fatalf("got %v, want only run.dmc", paths(files))
		}
	})

	t.Run("read false yields null contents", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{"./fixtures/*.coffee"}, gulp.SrcOptions{
			Cwd:  cwd,
			Read: gulp.Value(false),
		}))
		if len(files) != 1 || !files[0].IsNull() {
			t.Fatalf("expected one null file, got %v", paths(files))
		}
	})

	t.Run("buffer false yields a stream", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{"./fixtures/*.coffee"}, gulp.SrcOptions{
			Cwd:    cwd,
			Buffer: gulp.Value(false),
		}))
		if len(files) != 1 || !files[0].IsStream() {
			t.Fatalf("expected one streaming file, got %v", paths(files))
		}
		want := onDisk(t, filepath.Join(cwd, "fixtures", "test.coffee"))
		if got := contents(t, files[0]); got != want {
			t.Errorf("contents = %q, want %q", got, want)
		}
	})

	t.Run("walks a deep glob", func(t *testing.T) {
		files := collect(t, gulp.Src([]string{"./fixtures/**/*.jade"}, gulp.SrcOptions{Cwd: cwd}))
		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		want := filepath.Join(cwd, "fixtures", "test", "run.jade")
		if files[0].Path() != want {
			t.Errorf("path = %q, want %q", files[0].Path(), want)
		}
	})
}

// TestDestThroughFacade ports test/dest.js.
func TestDestThroughFacade(t *testing.T) {
	cwd := testCwd(t)

	t.Run("writes and re-emits", func(t *testing.T) {
		out := t.TempDir()
		files := collect(t, gulp.Src([]string{"./fixtures/**/*.txt"}, gulp.SrcOptions{Cwd: cwd}).
			Pipe(gulp.Dest(out)))

		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		want := filepath.Join(out, "copy", "example.txt")
		if files[0].Path() != want {
			t.Errorf("path = %q, want %q", files[0].Path(), want)
		}
		source := onDisk(t, filepath.Join(cwd, "fixtures", "copy", "example.txt"))
		if got := onDisk(t, want); got != source {
			t.Errorf("written %q, want %q", got, source)
		}
	})

	t.Run("read false writes nothing", func(t *testing.T) {
		out := t.TempDir()
		collect(t, gulp.Src([]string{"./fixtures/**/*.txt"}, gulp.SrcOptions{
			Cwd:  cwd,
			Read: gulp.Value(false),
		}).Pipe(gulp.Dest(out)))

		if _, err := os.Stat(filepath.Join(out, "copy", "example.txt")); !os.IsNotExist(err) {
			t.Fatalf("file should not have been written, stat err = %v", err)
		}
	})

	t.Run("writes directories", func(t *testing.T) {
		out := t.TempDir()
		files := collect(t, gulp.Src([]string{"./fixtures/stuff"}, gulp.SrcOptions{Cwd: cwd}).
			Pipe(gulp.Dest(out)))

		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		want := filepath.Join(out, "stuff")
		if files[0].Path() != want {
			t.Errorf("path = %q, want %q", files[0].Path(), want)
		}
		info, err := os.Stat(want)
		if err != nil || !info.IsDir() {
			t.Fatalf("stat %s: %v", want, err)
		}
	})
}

// TestWatchThroughFacade ports the cases of test/watch.js that exercise the
// façade rather than the watcher internals, which watch's own suite covers.
func TestWatchThroughFacade(t *testing.T) {
	if testing.Short() {
		t.Skip("filesystem timing; skipped under -short")
	}

	t.Run("runs a task on change", func(t *testing.T) {
		out := t.TempDir()
		target := filepath.Join(out, "watch-func.txt")
		writeFile(t, target, "A test generated this file and it is safe to delete")

		fired := make(chan struct{}, 1)
		w, err := gulp.Watch([]string{"watch-func.txt"}, gulp.WatchOptions{Cwd: out},
			func(context.Context) error {
				select {
				case fired <- struct{}{}:
				default:
				}
				return nil
			})
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		defer w.Close()
		<-w.Ready()
		time.Sleep(300 * time.Millisecond)

		writeFile(t, target, "A test generated this file and it is safe to delete changed")
		select {
		case <-fired:
		case <-time.After(5 * time.Second):
			t.Fatal("watcher never fired")
		}
	})

	t.Run("runs a series in order", func(t *testing.T) {
		out := t.TempDir()
		target := filepath.Join(out, "watch-series.txt")
		writeFile(t, target, "A test generated this file and it is safe to delete")

		g := gulp.New()
		var counter atomic.Int64
		done := make(chan int64, 1)
		g.Task("task1", func(context.Context) error {
			counter.Store(1)
			return nil
		})
		g.Task("task2", func(context.Context) error {
			value := counter.Add(10)
			select {
			case done <- value:
			default:
			}
			return nil
		})

		w, err := g.Watch([]string{"watch-series.txt"}, gulp.WatchOptions{Cwd: out},
			g.Series(gulp.Names("task1", "task2")...).Fn)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		defer w.Close()
		<-w.Ready()
		time.Sleep(300 * time.Millisecond)

		writeFile(t, target, "A test generated this file and it is safe to delete changed")
		select {
		case got := <-done:
			if got != 11 {
				t.Fatalf("counter = %d, want 11", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("series never ran")
		}
	})

	t.Run("without a task it is still closable", func(t *testing.T) {
		w, err := gulp.Watch([]string{"nothing-here.txt"}, gulp.WatchOptions{Cwd: t.TempDir()}, nil)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("second close: %v", err)
		}
	})
}

// TestLastRunThroughFacade covers docs/api/last-run.md, including the
// precision rounding and the rule that a failed run does not count.
func TestLastRunThroughFacade(t *testing.T) {
	g := gulp.New()
	g.Task("ok", func(context.Context) error { return nil })
	g.Task("bad", func(context.Context) error { return errFailed })

	if _, ok, err := g.LastRun(gulp.Name("ok"), 0); err != nil || ok {
		t.Fatalf("before running: ok = %v, err = %v", ok, err)
	}

	before := time.Now()
	if err := g.Run(context.Background(), gulp.Name("ok")); err != nil {
		t.Fatalf("run: %v", err)
	}
	at, ok, err := g.LastRun(gulp.Name("ok"), 0)
	if err != nil || !ok {
		t.Fatalf("after running: ok = %v, err = %v", ok, err)
	}
	if at.Before(before.Truncate(time.Millisecond)) {
		t.Errorf("lastRun %v predates the run at %v", at, before)
	}

	coarse, _, err := g.LastRun(gulp.Name("ok"), time.Second)
	if err != nil {
		t.Fatalf("precision: %v", err)
	}
	if coarse.UnixMilli()%1000 != 0 {
		t.Errorf("precision 1s should floor to a whole second, got %v", coarse)
	}

	if err := g.Run(context.Background(), gulp.Name("bad")); err == nil {
		t.Fatal("expected the failing task to error")
	}
	if _, ok, _ := g.LastRun(gulp.Name("bad"), 0); ok {
		t.Error("a failed run should not be recorded")
	}
}

// TestTreeThroughFacade covers docs/api/tree.md.
func TestTreeThroughFacade(t *testing.T) {
	g := gulp.New()
	g.Task("one", func(context.Context) error { return nil })
	g.Task("two", func(context.Context) error { return nil })
	if _, err := g.TaskRef("both", g.Series(gulp.Names("one", "two")...)); err != nil {
		t.Fatalf("register: %v", err)
	}

	shallow := g.Tree(false)
	if shallow.Label != "Tasks" {
		t.Errorf("root label = %q, want \"Tasks\"", shallow.Label)
	}
	if got := labels(shallow); len(got) != 3 {
		t.Errorf("shallow nodes = %v, want three", got)
	}

	deep := g.Tree(true)
	var both *gulp.Node
	for _, node := range deep.Nodes {
		if node.Label == "both" {
			both = node
		}
	}
	if both == nil {
		t.Fatal("deep tree is missing \"both\"")
	}
	if both.Type != "task" || both.Branch {
		t.Errorf("a registered task stays type=task with no branch flag, got %+v", both)
	}
	if len(both.Nodes) != 1 || both.Nodes[0].Label != "<series>" {
		t.Fatalf("deep tree for \"both\" = %+v", both)
	}
	series := both.Nodes[0]
	if !series.Branch || series.Type != "function" {
		t.Errorf("<series> = %+v, want branch=true type=function", series)
	}
	if got := labels(series); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("<series> children = %v, want [one two]", got)
	}
}

func labels(node *gulp.Node) []string {
	out := make([]string, 0, len(node.Nodes))
	for _, child := range node.Nodes {
		out = append(out, child.Label)
	}
	return out
}

func paths(files []*vinyl.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path())
	}
	return out
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

var errFailed = errTask("boom")

type errTask string

func (e errTask) Error() string { return string(e) }
