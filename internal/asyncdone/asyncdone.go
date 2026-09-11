// Package asyncdone normalises the many shapes a gulp task can take.
//
// It is the Go port of npm `async-done`. In JavaScript a task may signal
// completion through an error-first callback, a returned stream, a promise, an
// event emitter, a child process, or an observable (docs/api/concepts.md).
// async-done detects the shape and reduces it to a single callback.
//
// Go collapses most of that: a task is simply a function that returns an
// error. What remains genuinely different -- callbacks, channels, child
// processes and pipelines -- is handled by the adapters below, each of which
// produces a TaskFunc.
package asyncdone

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	"github.com/gulpjs/gulp-go/pipeline"
)

// TaskFunc is the canonical gulp task signature in this port.
//
// The JS equivalent is a function accepting an optional `done` callback. The
// context replaces gulp's lack of cancellation: a parallel composition that
// fails can tear down its siblings, which bach cannot do.
type TaskFunc func(ctx context.Context) error

// ErrNoTask is returned when a nil task is executed. async-done throws
// "Invalid or missing task" in the same situation.
var ErrNoTask = errors.New("asyncdone: invalid or missing task")

// ErrIncomplete is returned when a callback-style task never calls done and
// the context is cancelled first.
var ErrIncomplete = errors.New("asyncdone: task did not complete before cancellation")

// Run executes fn, converting a panic into an error so one misbehaving task
// cannot take down the whole gulp process. A JS task that throws synchronously
// is reported the same way.
func Run(ctx context.Context, fn TaskFunc) (err error) {
	if fn == nil {
		return ErrNoTask
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("asyncdone: task panicked: %v", r)
		}
	}()
	return fn(ctx)
}

// FromCallback adapts the error-first callback style used throughout gulp's
// documentation:
//
//	function build(done) { ...; done(); }
//
// Calling done more than once is ignored, matching async-done's guard against
// double completion. If done is never called, the task ends when the context
// does.
func FromCallback(fn func(done func(error))) TaskFunc {
	return func(ctx context.Context) error {
		if fn == nil {
			return ErrNoTask
		}
		ch := make(chan error, 1)
		var once sync.Once
		done := func(err error) {
			once.Do(func() { ch <- err })
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					done(fmt.Errorf("asyncdone: task panicked: %v", r))
				}
			}()
			fn(done)
		}()

		select {
		case err := <-ch:
			return err
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrIncomplete, ctx.Err())
		}
	}
}

// FromChannel adapts a task that reports completion by closing or sending on a
// channel, which is the Go analogue of a task returning an EventEmitter.
//
// The first value received is the result; a closed channel means success.
func FromChannel(open func(ctx context.Context) <-chan error) TaskFunc {
	return func(ctx context.Context) error {
		if open == nil {
			return ErrNoTask
		}
		ch := open(ctx)
		if ch == nil {
			return ErrNoTask
		}
		select {
		case err, ok := <-ch:
			if !ok {
				return nil
			}
			return err
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrIncomplete, ctx.Err())
		}
	}
}

// FromCmd adapts a child-process task. gulp supports returning a ChildProcess
// from a task; a non-zero exit status is a failure.
//
// build is called per invocation so a retried or watched task gets a fresh
// command, and it receives the context so the process is killed on
// cancellation when the caller uses exec.CommandContext.
func FromCmd(build func(ctx context.Context) *exec.Cmd) TaskFunc {
	return func(ctx context.Context) error {
		if build == nil {
			return ErrNoTask
		}
		cmd := build(ctx)
		if cmd == nil {
			return ErrNoTask
		}
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return fmt.Errorf("asyncdone: %s exited with %d: %w",
					cmd.Path, exitErr.ExitCode(), err)
			}
			return fmt.Errorf("asyncdone: running %s: %w", cmd.Path, err)
		}
		return nil
	}
}

// FromPipeline adapts a streaming task, the most common kind in gulp:
//
//	src('*.js').pipe(dest('out'))
//
// The pipeline is run to completion and its terminal error returned.
func FromPipeline(p *pipeline.Pipeline) TaskFunc {
	return func(ctx context.Context) error {
		if p == nil {
			return ErrNoTask
		}
		return p.Run(ctx)
	}
}
