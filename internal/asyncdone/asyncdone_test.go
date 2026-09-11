package asyncdone

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

var errBoom = errors.New("boom")

func TestRunCallsTheTask(t *testing.T) {
	var called bool
	err := Run(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Fatal("task was not called")
	}
}

func TestRunPropagatesError(t *testing.T) {
	err := Run(context.Background(), func(context.Context) error { return errBoom })
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestRunRejectsNilTask(t *testing.T) {
	if err := Run(context.Background(), nil); !errors.Is(err, ErrNoTask) {
		t.Fatalf("err = %v, want ErrNoTask", err)
	}
}

func TestRunChecksContextBeforeStarting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var called bool
	err := Run(ctx, func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("task ran despite a cancelled context")
	}
}

func TestRunRecoversPanics(t *testing.T) {
	err := Run(context.Background(), func(context.Context) error {
		panic("exploded")
	})
	if err == nil {
		t.Fatal("expected an error from a panicking task")
	}
	if got := err.Error(); !contains(got, "exploded") {
		t.Fatalf("err = %q, want it to mention the panic value", got)
	}
}

func TestFromCallback(t *testing.T) {
	task := FromCallback(func(done func(error)) { done(nil) })
	if err := task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
}

func TestFromCallbackPropagatesError(t *testing.T) {
	task := FromCallback(func(done func(error)) { done(errBoom) })
	if err := task(context.Background()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

// A JavaScript task that calls done() twice crashes the process. Guarding with
// sync.Once is what lets the Go port survive the same mistake.
func TestFromCallbackIgnoresASecondCompletion(t *testing.T) {
	task := FromCallback(func(done func(error)) {
		done(nil)
		done(errBoom)
	})
	if err := task(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil from the first completion", err)
	}
}

func TestFromCallbackHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	defer close(release)

	task := FromCallback(func(done func(error)) {
		<-release
		done(nil)
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := task(ctx)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want cancellation", err)
	}
}

func TestFromChannel(t *testing.T) {
	task := FromChannel(func(context.Context) <-chan error {
		ch := make(chan error, 1)
		ch <- nil
		close(ch)
		return ch
	})
	if err := task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
}

func TestFromChannelPropagatesError(t *testing.T) {
	task := FromChannel(func(context.Context) <-chan error {
		ch := make(chan error, 1)
		ch <- errBoom
		close(ch)
		return ch
	})
	if err := task(context.Background()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestFromCmd(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true is not available")
	}
	task := FromCmd(func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	if err := task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
}

func TestFromCmdReportsFailure(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false is not available")
	}
	task := FromCmd(func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	})
	if err := task(context.Background()); err == nil {
		t.Fatal("expected a non-zero exit to surface as an error")
	}
}

func TestFromPipeline(t *testing.T) {
	f := vinyl.MustNew(vinyl.Options{Cwd: "/p", Base: "/p", Path: "/p/a.js"})

	var seen atomic.Int32
	p := pipeline.New(pipeline.From(f)).Pipe(pipeline.Tap(func(*vinyl.File) error {
		seen.Add(1)
		return nil
	}))

	if err := FromPipeline(p)(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
	if got := seen.Load(); got != 1 {
		t.Fatalf("saw %d files, want 1", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
