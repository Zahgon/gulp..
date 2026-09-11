package fswatch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"
)

func TestIgnorablePermissionError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"permission", os.ErrPermission, true},
		{"wrapped permission", fmt.Errorf("scan: %w", os.ErrPermission), true},
		{"path error", &fs.PathError{Op: "open", Path: "/x", Err: os.ErrPermission}, true},
		{"not exist", os.ErrNotExist, false},
		{"other", errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ignorablePermissionError(tc.err); got != tc.want {
				t.Errorf("ignorablePermissionError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestPollWatcherEmitError(t *testing.T) {
	watcher := newPollingForTest(t)

	sent := errors.New("boom")
	watcher.emitError(sent)

	select {
	case got := <-watcher.Errors():
		if !errors.Is(got, sent) {
			t.Errorf("error = %v, want %v", got, sent)
		}
	case <-time.After(time.Second):
		t.Fatal("no error delivered")
	}
}

func TestPollWatcherEmitErrorIgnoresNil(t *testing.T) {
	watcher := newPollingForTest(t)

	watcher.emitError(nil)

	select {
	case got := <-watcher.Errors():
		t.Fatalf("nil produced an error: %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDepthAllows(t *testing.T) {
	cases := []struct {
		limit, depth int
		want         bool
	}{
		{-1, 0, true},
		{-1, 99, true},
		{0, 0, true},
		{0, 1, false},
		{2, 2, true},
		{2, 3, false},
	}

	for _, tc := range cases {
		if got := depthAllows(tc.limit, tc.depth); got != tc.want {
			t.Errorf("depthAllows(%d, %d) = %v, want %v", tc.limit, tc.depth, got, tc.want)
		}
	}
}

// newPollingForTest returns the concrete polling watcher. The constructor is
// typed to the Watcher interface, so reaching the unexported methods needs the
// assertion; it is safe because NewPolling has exactly one implementation.
func newPollingForTest(t *testing.T) *pollWatcher {
	t.Helper()

	opened, err := NewPolling(Config{Interval: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewPolling: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })

	watcher, ok := opened.(*pollWatcher)
	if !ok {
		t.Fatalf("NewPolling returned %T, want *pollWatcher", opened)
	}
	return watcher
}

// newNotifyForTest opens the fsnotify-backed watcher and hands back the
// concrete type. NewNotify is typed to the Watcher interface, but there is
// exactly one implementation, so the assertion cannot fail.
func newNotifyForTest(t *testing.T) *notifyWatcher {
	t.Helper()
	w, err := NewNotify(Config{Depth: -1})
	if err != nil {
		t.Fatalf("NewNotify: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	notify, ok := w.(*notifyWatcher)
	if !ok {
		t.Fatalf("NewNotify returned %T, want *notifyWatcher", w)
	}
	return notify
}

func TestNotifyWatcherEmitError(t *testing.T) {
	w := newNotifyForTest(t)
	sentinel := errors.New("notify boom")

	w.emitError(sentinel)

	select {
	case got := <-w.Errors():
		if !errors.Is(got, sentinel) {
			t.Errorf("Errors() = %v, want %v", got, sentinel)
		}
	case <-time.After(time.Second):
		t.Fatal("emitError delivered nothing")
	}
}

func TestNotifyWatcherEmitErrorIgnoresNil(t *testing.T) {
	w := newNotifyForTest(t)

	w.emitError(nil)

	select {
	case got := <-w.Errors():
		t.Fatalf("emitError(nil) delivered %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
