package watch_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/internal/bach"
	"github.com/gulpjs/gulp-go/watch"
)

// The cases here mirror gulp's test/watch.js. The JavaScript suite leans on
// wall-clock sleeps to let chokidar settle; the same is unavoidable here, so
// the timings are generous multiples of the 200ms default delay to keep the
// tests from flaking on a loaded machine.
const (
	// settle is how long to wait after the watcher reports ready before
	// touching anything. Registering kqueue/inotify watches is not
	// instantaneous and a write that lands too early is genuinely missed.
	settle = 300 * time.Millisecond
	// react is how long to allow for an event to propagate through the
	// backend, the 200ms debounce, and the task run.
	react = 3 * time.Second
	// quiet is how long to watch for events that must never arrive.
	quiet = 1200 * time.Millisecond
)

const tempContents = "A test generated this file and it is safe to delete"

func writeTempFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(tempContents), 0o666); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func updateTempFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(tempContents+" changed"), 0o666); err != nil {
		t.Fatalf("update %s: %v", path, err)
	}
}

// signal is a one-shot notification that is safe to fire more than once, which
// matters because a filesystem backend may legitimately report a change twice.
type signal struct {
	once sync.Once
	ch   chan struct{}
}

func newSignal() *signal { return &signal{ch: make(chan struct{})} }

func (s *signal) fire()                 { s.once.Do(func() { close(s.ch) }) }
func (s *signal) wait() <-chan struct{} { return s.ch }

// await fails the test if the signal does not fire within react.
func await(t *testing.T, s *signal, what string) {
	t.Helper()
	select {
	case <-s.wait():
	case <-time.After(react):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// refuse fails the test if the signal fires within quiet.
func refuse(t *testing.T, s *signal, what string) {
	t.Helper()
	select {
	case <-s.wait():
		t.Fatalf("did not expect %s", what)
	case <-time.After(quiet):
	}
}

// start builds a watcher, waits for it to become ready, and registers cleanup.
func start(t *testing.T, globs []string, opts watch.Options, task watch.TaskFunc) *watch.Watcher {
	t.Helper()
	w, err := watch.New(globs, opts, task)
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("close watcher: %v", err)
		}
	})
	select {
	case <-w.Ready():
	case <-time.After(react):
		t.Fatal("watcher never became ready")
	}
	time.Sleep(settle)
	return w
}

func TestWatchCallsTheFunctionWhenFileChangesNoOptions(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-func.txt")
	writeTempFile(t, tempFile)

	fired := newSignal()
	start(t, []string{"watch-func.txt"}, watch.Options{Cwd: out}, func(context.Context) error {
		fired.fire()
		return nil
	})

	updateTempFile(t, tempFile)
	await(t, fired, "task to run")
}

func TestWatchExecutesTheGulpParallelTasks(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-parallel.txt")
	writeTempFile(t, tempFile)

	fired := newSignal()
	task := bach.Parallel(func(context.Context) error {
		fired.fire()
		return nil
	})

	start(t, []string{"watch-parallel.txt"}, watch.Options{Cwd: out}, task)

	updateTempFile(t, tempFile)
	await(t, fired, "parallel task to run")
}

func TestWatchDoesNotCallTheFunctionWhenNoFileChangesNoOptions(t *testing.T) {
	out := t.TempDir()
	writeTempFile(t, filepath.Join(out, "watch-func-nochange.txt"))

	fired := newSignal()
	start(t, []string{"watch-func-nochange.txt"}, watch.Options{Cwd: out}, func(context.Context) error {
		fired.fire()
		return nil
	})

	refuse(t, fired, "the task to run without a change")
}

func TestWatchCallsTheFunctionWhenFileChangesWithOptions(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-func-options.txt")
	writeTempFile(t, tempFile)

	fired := newSignal()
	opts := watch.Options{Cwd: out, Delay: watch.Ptr(10 * time.Millisecond)}
	start(t, []string{"watch-func-options.txt"}, opts, func(context.Context) error {
		fired.fire()
		return nil
	})

	updateTempFile(t, tempFile)
	await(t, fired, "task to run with options")
}

// TestWatchCallsTheFunctionWhenFileChangesAtAPathWithJapaneseCharacters is the reason watch normalizes to NFC on darwin: the
// directory name below is written composed but stored decomposed by APFS.
func TestWatchCallsTheFunctionWhenFileChangesAtAPathWithJapaneseCharacters(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "フォルダ", "watch-non-ascii.txt")
	writeTempFile(t, tempFile)

	fired := newSignal()
	start(t, []string{"フォルダ/*"}, watch.Options{Cwd: out}, func(context.Context) error {
		fired.fire()
		return nil
	})

	updateTempFile(t, tempFile)
	await(t, fired, "task to run for a non-ASCII path")
}

func TestWatchNegationDoesNotFire(t *testing.T) {
	out := t.TempDir()
	ignored := filepath.Join(out, "ignored.txt")
	writeTempFile(t, ignored)
	writeTempFile(t, filepath.Join(out, "watched.txt"))

	fired := newSignal()
	start(t, []string{"*", "!ignored.txt"}, watch.Options{Cwd: out}, func(context.Context) error {
		fired.fire()
		return nil
	})

	if err := os.Remove(ignored); err != nil {
		t.Fatalf("remove %s: %v", ignored, err)
	}
	refuse(t, fired, "the task to run for a negated path")
}

// TestWatchDoesNotDropOptionsWhenNoCallbackSpecified covers gulp's "should not drop options
// when no callback specified": the cwd must still be honoured when resolving a
// relative path that climbs out of it, and the emitted path is relative to cwd.
func TestWatchDoesNotDropOptionsWhenNoCallbackSpecified(t *testing.T) {
	out := t.TempDir()
	cwd := filepath.Join(out, "subdir")
	if err := os.MkdirAll(cwd, 0o777); err != nil {
		t.Fatalf("mkdir %s: %v", cwd, err)
	}
	relFile := "../watch-func-nodrop-options.txt"
	tempFile := filepath.Join(cwd, relFile)
	writeTempFile(t, tempFile)

	fired := newSignal()
	var got atomic.Value
	w := start(t, []string{relFile}, watch.Options{Cwd: cwd}, nil)
	w.On(watch.EventChange, func(path string) {
		got.Store(path)
		fired.fire()
	})

	updateTempFile(t, tempFile)
	await(t, fired, "a change event")

	reported, _ := got.Load().(string)
	if filepath.Clean(filepath.Join(cwd, reported)) != filepath.Clean(tempFile) {
		t.Fatalf("reported path %q does not resolve to %q from cwd %q", reported, tempFile, cwd)
	}
}

func TestWatchWorksWithoutOptionsOrCallback(t *testing.T) {
	out := t.TempDir()
	w, err := watch.New([]string{filepath.Join(out, "x")}, watch.Options{}, nil)
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Closing twice must stay safe, since gulpfiles routinely close watchers
	// from both a signal handler and a teardown path.
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestRunsManyTasksWithOptions(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-series.txt")
	writeTempFile(t, tempFile)

	var (
		mu    sync.Mutex
		value int
	)
	fired := newSignal()

	// The arithmetic is gulp's: task1 sets the value to 1 and task2 adds 10,
	// so a total of 11 proves the order rather than merely the count.
	task := bach.Series(
		func(context.Context) error {
			mu.Lock()
			value = 1
			mu.Unlock()
			return nil
		},
		func(context.Context) error {
			mu.Lock()
			value += 10
			mu.Unlock()
			fired.fire()
			return nil
		},
	)

	start(t, []string{"watch-series.txt"}, watch.Options{Cwd: out}, task)

	updateTempFile(t, tempFile)
	await(t, fired, "the series to run")

	mu.Lock()
	defer mu.Unlock()
	if value != 11 {
		t.Fatalf("value = %d, want 11 (series ran out of order)", value)
	}
}

func TestRunsManyTasksNoOptions(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-series-glob.txt")
	writeTempFile(t, tempFile)

	var count atomic.Int32
	fired := newSignal()
	task := bach.Series(
		func(context.Context) error { count.Add(1); return nil },
		func(context.Context) error { count.Add(10); fired.fire(); return nil },
	)

	start(t, []string{"./watch-series-glob.txt"}, watch.Options{Cwd: out}, task)

	updateTempFile(t, tempFile)
	await(t, fired, "the series to run for a path-style glob")

	if got := count.Load(); got != 11 {
		t.Fatalf("count = %d, want 11", got)
	}
}

// TestWatchRejectsEmptyPath is the Go analogue of gulp's two "throws on
// non-function task" cases. Passing a task name or a slice where a function is
// expected does not compile in Go, so the only reachable input error is an
// empty watch path.
func TestWatchRejectsEmptyPath(t *testing.T) {
	if _, err := watch.New([]string{""}, watch.Options{}, nil); !errors.Is(err, watch.ErrNonStringWatchPath) {
		t.Fatalf("err = %v, want ErrNonStringWatchPath", err)
	}
	if _, err := watch.New(nil, watch.Options{}, nil); !errors.Is(err, watch.ErrNoWatchPaths) {
		t.Fatalf("err = %v, want ErrNoWatchPaths", err)
	}
}

func TestWatchQueuesAtMostOneRun(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-queue.txt")
	writeTempFile(t, tempFile)

	var runs atomic.Int32
	release := make(chan struct{})
	first := newSignal()

	opts := watch.Options{Cwd: out, Delay: watch.Ptr(10 * time.Millisecond)}
	start(t, []string{"watch-queue.txt"}, opts, func(ctx context.Context) error {
		if runs.Add(1) == 1 {
			first.fire()
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	updateTempFile(t, tempFile)
	await(t, first, "the first run to start")

	// Several changes while the first run is blocked must collapse into a
	// single queued run, not one run each.
	for i := 0; i < 5; i++ {
		updateTempFile(t, tempFile)
		time.Sleep(30 * time.Millisecond)
	}
	close(release)

	time.Sleep(quiet)
	if got := runs.Load(); got != 2 {
		t.Fatalf("runs = %d, want 2 (one in flight plus one queued)", got)
	}
}

func TestWatchQueueDisabledDropsChanges(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-noqueue.txt")
	writeTempFile(t, tempFile)

	var runs atomic.Int32
	release := make(chan struct{})
	first := newSignal()

	opts := watch.Options{
		Cwd:   out,
		Delay: watch.Ptr(10 * time.Millisecond),
		Queue: watch.Ptr(false),
	}
	start(t, []string{"watch-noqueue.txt"}, opts, func(ctx context.Context) error {
		if runs.Add(1) == 1 {
			first.fire()
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	updateTempFile(t, tempFile)
	await(t, first, "the first run to start")

	for i := 0; i < 5; i++ {
		updateTempFile(t, tempFile)
		time.Sleep(30 * time.Millisecond)
	}
	close(release)

	time.Sleep(quiet)
	if got := runs.Load(); got != 1 {
		t.Fatalf("runs = %d, want 1 (queueing disabled)", got)
	}
}

func TestWatchReportsAddedFiles(t *testing.T) {
	out := t.TempDir()

	fired := newSignal()
	var got atomic.Value
	w := start(t, []string{"*.txt"}, watch.Options{Cwd: out}, nil)
	w.On(watch.EventAdd, func(path string) {
		got.Store(path)
		fired.fire()
	})

	writeTempFile(t, filepath.Join(out, "created.txt"))
	await(t, fired, "an add event")

	if reported, _ := got.Load().(string); reported != "created.txt" {
		t.Fatalf("reported path = %q, want %q", reported, "created.txt")
	}
}

func TestWatchIgnoreInitialFalseReportsExistingFiles(t *testing.T) {
	out := t.TempDir()
	writeTempFile(t, filepath.Join(out, "existing.txt"))

	seen := make(chan string, 8)
	w, err := watch.New([]string{"*.txt"}, watch.Options{
		Cwd:           out,
		IgnoreInitial: watch.Ptr(false),
	}, nil)
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	w.On(watch.EventAdd, func(path string) { seen <- path })

	select {
	case path := <-seen:
		if path != "existing.txt" {
			t.Fatalf("reported path = %q, want %q", path, "existing.txt")
		}
	case <-time.After(react):
		t.Fatal("timed out waiting for the initial add event")
	}
}

func TestWatchDoesNotCallTheFunctionWhenIgnoredFileChanges(t *testing.T) {
	out := t.TempDir()
	writeTempFile(t, filepath.Join(out, "node_modules", "dep.txt"))

	fired := newSignal()
	start(t, []string{"**/*.txt"}, watch.Options{
		Cwd:     out,
		Ignored: []string{"**/node_modules/**"},
	}, func(context.Context) error {
		fired.fire()
		return nil
	})

	updateTempFile(t, filepath.Join(out, "node_modules", "dep.txt"))
	refuse(t, fired, "the task to run for an ignored path")
}

func TestWatchPollingBackend(t *testing.T) {
	out := t.TempDir()
	tempFile := filepath.Join(out, "watch-polling.txt")
	writeTempFile(t, tempFile)

	fired := newSignal()
	opts := watch.Options{
		Cwd:        out,
		UsePolling: true,
		Interval:   watch.Ptr(30 * time.Millisecond),
		Delay:      watch.Ptr(10 * time.Millisecond),
	}
	start(t, []string{"watch-polling.txt"}, opts, func(context.Context) error {
		fired.fire()
		return nil
	})

	// A poller compares mtime and size; appending guarantees both differ even
	// on a filesystem with coarse timestamp resolution.
	updateTempFile(t, tempFile)
	await(t, fired, "the polling backend to notice a change")
}
