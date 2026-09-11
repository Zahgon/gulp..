package fswatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	settle  = 250 * time.Millisecond
	react   = 4 * time.Second
	quiet   = 700 * time.Millisecond
	pollInt = 30 * time.Millisecond
)

// backends runs a test body against both implementations, because usePolling
// has to behave the same as the native watcher.
func backends(t *testing.T, body func(t *testing.T, open func(Config) (Watcher, error))) {
	t.Helper()
	t.Run("notify", func(t *testing.T) { body(t, NewNotify) })
	t.Run("polling", func(t *testing.T) {
		body(t, func(cfg Config) (Watcher, error) {
			if cfg.Interval == 0 {
				cfg.Interval = pollInt
			}
			return NewPolling(cfg)
		})
	})
}

func start(t *testing.T, open func(Config) (Watcher, error), cfg Config, root string) Watcher {
	t.Helper()
	w, err := open(cfg)
	if err != nil {
		t.Fatalf("open watcher: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Add(root); err != nil {
		t.Fatalf("Add(%s): %v", root, err)
	}
	time.Sleep(settle)
	return w
}

// await drains events until one satisfies match, so that incidental events
// (directory mtime bumps, editor temp files) do not make the test flaky.
func await(t *testing.T, w Watcher, match func(Event) bool) Event {
	t.Helper()
	deadline := time.After(react)
	for {
		select {
		case ev := <-w.Events():
			if match(ev) {
				return ev
			}
		case err := <-w.Errors():
			t.Fatalf("watcher error: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for a matching event")
		}
	}
}

func refuse(t *testing.T, w Watcher, match func(Event) bool) {
	t.Helper()
	deadline := time.After(quiet)
	for {
		select {
		case ev := <-w.Events():
			if match(ev) {
				t.Fatalf("unexpected event: %+v", ev)
			}
		case <-deadline:
			return
		}
	}
}

func named(name string) func(Event) bool {
	return func(ev Event) bool { return filepath.Base(ev.Path) == name }
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestOpFlags(t *testing.T) {
	both := OpCreate | OpWrite
	if !both.Has(OpCreate) || !both.Has(OpWrite) {
		t.Fatal("Has failed on a combined op")
	}
	if both.Has(OpRemove) {
		t.Fatal("Has reported an op that is not set")
	}
	if got := OpCreate.String(); got == "" {
		t.Fatal("String returned empty for OpCreate")
	}
	if got := both.String(); !strings.Contains(got, "create") || !strings.Contains(got, "write") {
		t.Fatalf("String = %q, want it to name both ops", got)
	}
}

func TestReportsCreatedFile(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w := start(t, open, Config{Depth: -1}, root)

		write(t, filepath.Join(root, "created.txt"), "hello")

		ev := await(t, w, named("created.txt"))
		if !ev.Op.Has(OpCreate) && !ev.Op.Has(OpWrite) {
			t.Fatalf("op = %v, want create or write", ev.Op)
		}
		if ev.IsDir {
			t.Fatal("a regular file was reported as a directory")
		}
	})
}

func TestReportsModifiedFile(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		path := filepath.Join(root, "existing.txt")
		write(t, path, "before")

		w := start(t, open, Config{Depth: -1}, root)
		write(t, path, "before and after")

		await(t, w, func(ev Event) bool {
			return filepath.Base(ev.Path) == "existing.txt" && ev.Op.Has(OpWrite)
		})
	})
}

func TestReportsRemovedFile(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		path := filepath.Join(root, "doomed.txt")
		write(t, path, "hello")

		w := start(t, open, Config{Depth: -1}, root)
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove: %v", err)
		}

		await(t, w, func(ev Event) bool {
			return filepath.Base(ev.Path) == "doomed.txt" &&
				(ev.Op.Has(OpRemove) || ev.Op.Has(OpRename))
		})
	})
}

// A watcher that only saw the roots it was given at startup would miss
// everything written into a directory created later, which is the common case
// for a build that recreates its output folder.
func TestWatchesDirectoriesCreatedLater(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w := start(t, open, Config{Depth: -1}, root)

		write(t, filepath.Join(root, "nested", "deep", "late.txt"), "hello")

		await(t, w, named("late.txt"))
	})
}

func TestReportsNestedFilesUnderAnExistingTree(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		nested := filepath.Join(root, "a", "b")
		write(t, filepath.Join(nested, "seed.txt"), "seed")

		w := start(t, open, Config{Depth: -1}, root)
		write(t, filepath.Join(nested, "child.txt"), "hello")

		await(t, w, named("child.txt"))
	})
}

func TestFilterPrunesDirectories(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		write(t, filepath.Join(root, "vendor", "seed.txt"), "seed")

		cfg := Config{
			Depth: -1,
			Filter: func(path string, _ bool) bool {
				return filepath.Base(path) != "vendor"
			},
		}
		w := start(t, open, cfg, root)

		write(t, filepath.Join(root, "vendor", "ignored.txt"), "hello")
		refuse(t, w, named("ignored.txt"))

		write(t, filepath.Join(root, "watched.txt"), "hello")
		await(t, w, named("watched.txt"))
	})
}

func TestDepthLimit(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		write(t, filepath.Join(root, "a", "b", "seed.txt"), "seed")

		w := start(t, open, Config{Depth: 1}, root)

		write(t, filepath.Join(root, "a", "b", "too-deep.txt"), "hello")
		refuse(t, w, named("too-deep.txt"))

		write(t, filepath.Join(root, "shallow.txt"), "hello")
		await(t, w, named("shallow.txt"))
	})
}

func TestWatchingASingleFile(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		path := filepath.Join(root, "solo.txt")
		write(t, path, "before")

		w := start(t, open, Config{Depth: -1}, path)
		write(t, path, "before and after")

		await(t, w, named("solo.txt"))
	})
}

// gulp.watch is routinely pointed at a path that does not exist yet, such as a
// dist folder the build is about to create.
func TestWatchingAPathThatDoesNotExistYet(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		target := filepath.Join(root, "dist")

		w := start(t, open, Config{Depth: -1}, target)
		write(t, filepath.Join(target, "bundle.js"), "hello")

		await(t, w, func(ev Event) bool {
			base := filepath.Base(ev.Path)
			return base == "bundle.js" || base == "dist"
		})
	})
}

func TestRemoveStopsReporting(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w := start(t, open, Config{Depth: -1}, root)

		if err := w.Remove(root); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		time.Sleep(settle)

		write(t, filepath.Join(root, "after-remove.txt"), "hello")
		refuse(t, w, named("after-remove.txt"))
	})
}

func TestPathsAreAbsolute(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w := start(t, open, Config{Depth: -1}, root)

		write(t, filepath.Join(root, "abs.txt"), "hello")

		ev := await(t, w, named("abs.txt"))
		if !filepath.IsAbs(ev.Path) {
			t.Fatalf("path %q is not absolute", ev.Path)
		}
	})
}

func TestCloseIsIdempotent(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w, err := open(Config{Depth: -1})
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := w.Add(root); err != nil {
			t.Fatalf("Add: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("first Close: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	})
}

func TestAddAfterCloseFails(t *testing.T) {
	backends(t, func(t *testing.T, open func(Config) (Watcher, error)) {
		root := t.TempDir()
		w, err := open(Config{Depth: -1})
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := w.Add(root); err == nil {
			t.Fatal("Add succeeded after Close")
		}
	})
}

// The first poll establishes a baseline; reporting it would make every watcher
// fire for the whole tree on startup.
func TestPollingDoesNotReportTheInitialScan(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "seed.txt"), "seed")

	w, err := NewPolling(Config{Depth: -1, Interval: pollInt})
	if err != nil {
		t.Fatalf("NewPolling: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Add(root); err != nil {
		t.Fatalf("Add: %v", err)
	}
	refuse(t, w, named("seed.txt"))
}
