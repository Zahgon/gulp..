package watch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// watch_test.go lives in package watch_test, so its constants are not visible
// from this internal test file. These are the same values under local names.
const (
	apiSettle   = 300 * time.Millisecond
	apiReact    = 3 * time.Second
	apiQuiet    = 1200 * time.Millisecond
	apiContents = "A test generated this file and it is safe to delete"
)

func TestEscapeMagic(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain.txt", "plain.txt"},
		{"star*.txt", `star\*.txt`},
		{"a?b", `a\?b`},
		{"[abc].txt", `\[abc\].txt`},
		{"{a,b}.txt", `\{a,b\}.txt`},
		{"src/**/*.js", `src/\*\*/\*.js`},
	}
	for _, tc := range cases {
		if got := escapeMagic(tc.in); got != tc.want {
			t.Errorf("escapeMagic(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWatcherOnErrorReceivesEmittedErrors(t *testing.T) {
	dir := t.TempDir()
	w, err := New([]string{"*.txt"}, Options{Cwd: dir}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	received := make(chan error, 1)
	w.OnError(func(err error) {
		select {
		case received <- err:
		default:
		}
	})

	sentinel := errors.New("boom")
	w.emitError(sentinel)

	select {
	case got := <-received:
		if !errors.Is(got, sentinel) {
			t.Errorf("OnError got %v, want %v", got, sentinel)
		}
	case <-time.After(time.Second):
		t.Fatal("OnError listener was never called")
	}
}

func TestWatcherEmitErrorIgnoresNil(t *testing.T) {
	dir := t.TempDir()
	w, err := New([]string{"*.txt"}, Options{Cwd: dir}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	called := make(chan struct{}, 1)
	w.OnError(func(error) {
		select {
		case called <- struct{}{}:
		default:
		}
	})

	w.emitError(nil)

	select {
	case <-called:
		t.Fatal("emitError(nil) notified the listener")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatcherUnwatchStopsReporting(t *testing.T) {
	dir := t.TempDir()
	w, err := New([]string{"*.txt"}, Options{Cwd: dir}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	<-w.Ready()
	time.Sleep(apiSettle)

	if err := w.Unwatch("*.txt"); err != nil {
		t.Fatalf("Unwatch: %v", err)
	}

	seen := make(chan string, 1)
	w.On(EventAdd, func(path string) {
		select {
		case seen <- path:
		default:
		}
	})

	if err := os.WriteFile(filepath.Join(dir, "after.txt"), []byte(apiContents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case path := <-seen:
		t.Fatalf("unwatched glob still reported %q", path)
	case <-time.After(apiQuiet):
	}
}

// TestWatcherUnwatchIgnoresAnEmptyGlob pins the asymmetry with Add, which
// rejects an empty glob. Starting a watch on nothing is a mistake worth
// reporting; asking to stop watching something that was never watched is not,
// so Unwatch skips it. chokidar draws the line in the same place.
func TestWatcherUnwatchIgnoresAnEmptyGlob(t *testing.T) {
	dir := t.TempDir()
	w, err := New([]string{"*.txt"}, Options{Cwd: dir}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Unwatch(""); err != nil {
		t.Errorf("Unwatch(\"\") = %v, want nil", err)
	}
	if err := w.Unwatch("never-watched/*.md"); err != nil {
		t.Errorf("Unwatch of an unheld glob = %v, want nil", err)
	}
}

// TestAwaitWriteFinishWaitsForTheSizeToSettle covers the option chokidar added
// for files that arrive in pieces: a large copy or a download reports a change
// long before the last byte lands. The watcher must hold the event until the
// size stops growing, so a build never reads a half-written file.
func TestAwaitWriteFinishWaitsForTheSizeToSettle(t *testing.T) {
	dir := t.TempDir()
	w, err := New([]string{"*.bin"}, Options{
		Cwd: dir,
		AwaitWriteFinish: &AwaitWriteFinish{
			StabilityThreshold: 120 * time.Millisecond,
			PollInterval:       20 * time.Millisecond,
		},
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	<-w.Ready()
	time.Sleep(apiSettle)

	target := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(target, []byte("first"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	reported := make(chan string, 1)
	w.On(EventAdd, func(path string) {
		select {
		case reported <- path:
		default:
		}
	})

	// Keep growing the file past the first poll, so the event can only be
	// delivered after the writes stop.
	for i := 0; i < 3; i++ {
		time.Sleep(40 * time.Millisecond)
		handle, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := handle.WriteString("more"); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := handle.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}

	select {
	case path := <-reported:
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Size() != int64(len("first")+3*len("more")) {
			t.Errorf("reported at size %d, want the apiSettled size %d",
				info.Size(), len("first")+3*len("more"))
		}
	case <-time.After(apiReact):
		t.Fatal("awaitWriteFinish never released the event")
	}
}
