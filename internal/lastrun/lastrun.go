// Package lastrun records when each task last completed successfully.
//
// It is the Go port of npm `last-run`, which backs gulp's lastRun() API and
// therefore its incremental-build recipe:
//
//	src(globs, { since: gulp.lastRun(compile) })
//
// See docs/api/last-run.md.
package lastrun

import (
	"sync"
	"time"
)

// Registry stores capture timestamps keyed by task identity. It is safe for
// concurrent use, which matters because parallel compositions capture and
// release from several goroutines at once.
type Registry struct {
	mu       sync.RWMutex
	captures map[any]time.Time
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{captures: make(map[any]time.Time)}
}

// Capture records the instant a task started.
//
// The value is deliberately the START time, not the finish time: a source file
// modified while the task was running must still be picked up by the next run,
// so the watermark has to precede the work.
func (r *Registry) Capture(key any, at time.Time) {
	if key == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.captures[key] = at
}

// Release discards a task's timestamp.
//
// Undertaker calls this when a task errors, which is what makes lastRun report
// "never run" after a failure. Note that this also discards the timestamp of a
// previous successful run -- that is the JS behaviour, and it is the safe one:
// a failed build must not let the next run skip unchanged-looking files.
func (r *Registry) Release(key any) {
	if key == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.captures, key)
}

// LastRun returns the recorded timestamp for a task, truncated to precision.
//
// Precision rounds DOWN, never to nearest, so the watermark can only ever move
// earlier and files are never missed. With a raw value of 1426000001111ms,
// docs/api/last-run.md specifies:
//
//	precision 0     -> 1426000001111
//	precision 100ms -> 1426000001100
//	precision 1s    -> 1426000001000
//
// The boolean is false when the task has never completed successfully.
func (r *Registry) LastRun(key any, precision time.Duration) (time.Time, bool) {
	if key == nil {
		return time.Time{}, false
	}
	r.mu.RLock()
	at, ok := r.captures[key]
	r.mu.RUnlock()
	if !ok {
		return time.Time{}, false
	}
	return Truncate(at, precision), true
}

// Truncate floors t to a multiple of precision, working in milliseconds
// because that is the resolution the JS API is specified in.
func Truncate(t time.Time, precision time.Duration) time.Time {
	ms := t.UnixMilli()
	step := precision.Milliseconds()
	if step > 1 {
		ms -= ms % step
	}
	return time.UnixMilli(ms)
}

// Len reports how many tasks currently have a recorded timestamp. Used by
// tests and diagnostics.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.captures)
}
