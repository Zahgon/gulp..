// Package bach composes tasks into sequential and concurrent groups.
//
// It is the Go port of npm `bach`, the engine behind gulp's series() and
// parallel(). The four entry points mirror bach's own: series, parallel, and
// the "settle" variants that run every task even after one fails.
package bach

import (
	"context"
	"errors"
	"sync"

	"github.com/gulpjs/gulp-go/internal/asyncdone"
)

// TaskFunc is re-exported from asyncdone so callers need only one import.
type TaskFunc = asyncdone.TaskFunc

// Series returns a task that runs each task in order and stops at the first
// failure, propagating that error.
func Series(tasks ...TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		for _, t := range tasks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := asyncdone.Run(ctx, t); err != nil {
				return err
			}
		}
		return nil
	}
}

// Parallel returns a task that runs every task concurrently and reports the
// first error.
//
// Deliberate divergence from JS: on the first failure the remaining tasks have
// their context cancelled. bach cannot do this because JS has no cancellation
// primitive, so it lets the losers run to completion and discards their
// results. Cancelling is both the idiomatic Go behaviour and strictly better --
// it stops a long compile from continuing after the build has already failed.
// Tasks that ignore their context still run to completion, so nothing breaks.
func Parallel(tasks ...TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		if len(tasks) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			firstErr error
		)
		for _, t := range tasks {
			wg.Add(1)
			go func(task TaskFunc) {
				defer wg.Done()
				if err := asyncdone.Run(runCtx, task); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
				}
			}(t)
		}
		wg.Wait()

		mu.Lock()
		defer mu.Unlock()
		return firstErr
	}
}

// SettleSeries runs every task in order regardless of failures and returns all
// errors joined together.
func SettleSeries(tasks ...TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		errs := make([]error, 0, len(tasks))
		for _, t := range tasks {
			errs = append(errs, asyncdone.Run(ctx, t))
		}
		return errors.Join(errs...)
	}
}

// SettleParallel runs every task concurrently, waits for all of them, and
// returns all errors joined together. Unlike Parallel it does not cancel
// siblings, since the point of settling is to learn every outcome.
func SettleParallel(tasks ...TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		if len(tasks) == 0 {
			return nil
		}
		var wg sync.WaitGroup
		errs := make([]error, len(tasks))
		for i, t := range tasks {
			wg.Add(1)
			go func(idx int, task TaskFunc) {
				defer wg.Done()
				errs[idx] = asyncdone.Run(ctx, task)
			}(i, t)
		}
		wg.Wait()
		return errors.Join(errs...)
	}
}
