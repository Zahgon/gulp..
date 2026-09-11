package asyncdone_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/internal/asyncdone"
)

func TestNilTasksAreRejected(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func() error{
		"Run":          func() error { return asyncdone.Run(ctx, nil) },
		"FromCallback": func() error { return asyncdone.FromCallback(nil)(ctx) },
		"FromChannel":  func() error { return asyncdone.FromChannel(nil)(ctx) },
		"FromCmd":      func() error { return asyncdone.FromCmd(nil)(ctx) },
		"FromPipeline": func() error { return asyncdone.FromPipeline(nil)(ctx) },
		"FromChannel returning nil": func() error {
			return asyncdone.FromChannel(func(context.Context) <-chan error { return nil })(ctx)
		},
		"FromCmd returning nil": func() error {
			return asyncdone.FromCmd(func(context.Context) *exec.Cmd { return nil })(ctx)
		},
	}
	for name, run := range cases {
		if err := run(); !errors.Is(err, asyncdone.ErrNoTask) {
			t.Errorf("%s = %v, want ErrNoTask", name, err)
		}
	}
}

func TestRunTurnsAPanicIntoAnError(t *testing.T) {
	err := asyncdone.Run(context.Background(), func(context.Context) error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("Run = nil, want the panic reported as an error")
	}
	if got := err.Error(); !strings.Contains(got, "boom") {
		t.Fatalf("Run = %q, want it to mention the panic value", got)
	}
}

func TestFromChannelReportsCancellationBeforeCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := asyncdone.FromChannel(func(context.Context) <-chan error {
		return make(chan error)
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := task(ctx)
	if !errors.Is(err, asyncdone.ErrIncomplete) {
		t.Fatalf("task = %v, want ErrIncomplete", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("task = %v, want it to wrap context.Canceled", err)
	}
}

func TestFromChannelPassesTheChannelsResultThrough(t *testing.T) {
	want := errors.New("task failed")
	task := asyncdone.FromChannel(func(context.Context) <-chan error {
		ch := make(chan error, 1)
		ch <- want
		close(ch)
		return ch
	})
	if err := task(context.Background()); !errors.Is(err, want) {
		t.Fatalf("task = %v, want %v", err, want)
	}

	closed := asyncdone.FromChannel(func(context.Context) <-chan error {
		ch := make(chan error)
		close(ch)
		return ch
	})
	if err := closed(context.Background()); err != nil {
		t.Fatalf("a closed channel reported %v, want nil", err)
	}
}

func TestFromCmdReportsAFailedCommand(t *testing.T) {
	task := asyncdone.FromCmd(func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	})
	if err := task(context.Background()); err == nil {
		t.Fatal("a failing command reported nil")
	}

	ok := asyncdone.FromCmd(func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	if err := ok(context.Background()); err != nil {
		t.Fatalf("a successful command reported %v", err)
	}
}
