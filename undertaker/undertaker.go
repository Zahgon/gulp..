package undertaker

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gulpjs/gulp-go/internal/asyncdone"
	"github.com/gulpjs/gulp-go/internal/bach"
	"github.com/gulpjs/gulp-go/internal/lastrun"
)

// SettleEnv is the environment variable that switches Series and Parallel to
// their settling behaviour, so that every task runs even after one fails.
const SettleEnv = "UNDERTAKER_SETTLE"

// runUID numbers task executions. JavaScript undertaker assigns a uid when the
// runtime creates the storage for one run, not when the task is registered, so
// a task that runs three times produces three uids and a listener can pair a
// stop event with the start it belongs to.
var runUID atomic.Uint64

// Undertaker owns a task registry and composes tasks into runnable units.
// It is safe for concurrent use.
type Undertaker struct {
	mu       sync.RWMutex
	registry Registry
	lastRun  *lastrun.Registry
	events   emitter
	settle   atomic.Bool
}

// New returns an Undertaker backed by a DefaultRegistry.
//
// Setting UNDERTAKER_SETTLE=true makes Series and Parallel settle: every task
// runs even after one fails, and the failures are reported together. The
// JavaScript constructor reads the same variable.
func New() *Undertaker {
	u := &Undertaker{
		registry: NewDefaultRegistry(),
		lastRun:  lastrun.New(),
	}
	u.settle.Store(os.Getenv(SettleEnv) == "true")
	return u
}

// Settles reports whether Series and Parallel currently settle.
func (u *Undertaker) Settles() bool { return u.settle.Load() }

// SetSettle switches settling on or off for compositions that have already been
// built.
//
// JavaScript gulp-cli sets UNDERTAKER_SETTLE before the gulpfile is loaded, so
// --continue reaches every gulp.series/gulp.parallel in it. A Go gulpfile has
// already composed its tasks by the time the CLI parses argv, so the flag has
// to be applied afterwards instead.
func (u *Undertaker) SetSettle(on bool) { u.settle.Store(on) }

// Get returns a registered task.
func (u *Undertaker) Get(name string) (*Task, bool) {
	return u.activeRegistry().Get(name)
}

// Set registers a plain function under a name and returns the stored Task.
func (u *Undertaker) Set(name string, fn TaskFunc) *Task {
	t, err := u.SetTask(name, Func(name, fn))
	if err != nil {
		return nil
	}
	return t
}

// SetTask registers anything Ref can denote under a name, which is what
// `gulp.task('build', gulp.series('clean', 'compile'))` does.
//
// A composition is stored by reference rather than being flattened, for two
// reasons. tree(deep) can then show the `<series>` node nested beneath the
// registered name, matching `gulp --tasks`. And the composition is resolved
// and instrumented at run time, so it emits its own branch-flagged lifecycle
// events and any names inside it may still be registered later.
func (u *Undertaker) SetTask(name string, ref Ref) (*Task, error) {
	if name == "" {
		return nil, ErrInvalidTaskName
	}
	if ref == nil {
		return nil, asyncdone.ErrNoTask
	}

	stored := &Task{Name: name, kind: kindTask, refs: []Ref{ref}}
	if t, ok := ref.(*Task); ok {
		stored.Description = t.Description
		stored.Flags = t.Flags
		if !t.branch {
			// A plain function has nothing to recurse into, so adopt it
			// directly and let tree() render it as a leaf.
			stored.Fn = t.Fn
			stored.refs = nil
			return u.activeRegistry().Set(name, stored), nil
		}
	}

	stored.Fn = func(ctx context.Context) error {
		inner, err := ref.resolve(u)
		if err != nil {
			return err
		}
		return u.instrument(inner)(ctx)
	}
	return u.activeRegistry().Set(name, stored), nil
}

// activeRegistry returns the installed registry under a read lock.
func (u *Undertaker) activeRegistry() Registry {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.registry
}

// Tasks returns a snapshot of every registered task.
func (u *Undertaker) Tasks() map[string]*Task {
	return u.activeRegistry().Tasks()
}

// Registry returns the active registry.
func (u *Undertaker) Registry() Registry {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.registry
}

// SetRegistry installs a custom registry.
//
// Every task already registered is transferred into the new registry before it
// is initialised, which is the behaviour docs/api/registry.md specifies:
// "the tasks from the previous registry are transferred to the new one".
func (u *Undertaker) SetRegistry(r Registry) error {
	if r == nil {
		return ErrNilRegistry
	}
	u.mu.Lock()
	existing := u.registry.Tasks()
	for _, name := range registeredNames(u.registry) {
		if t, ok := existing[name]; ok {
			r.Set(name, t)
		}
	}
	u.registry = r
	u.mu.Unlock()

	r.Init(u)
	return nil
}

// Series composes refs into a task that runs them in order.
//
// Refs are resolved when the composed task runs, not when it is built, so a
// name may refer to a task registered later.
// Whether it settles is decided when the composed task runs, so SetSettle
// applies to compositions a gulpfile built before the CLI parsed its flags.
func (u *Undertaker) Series(refs ...Ref) *Task {
	return u.composeSettleable(LabelSeries, bach.Series, bach.SettleSeries, refs)
}

// Parallel composes refs into a task that runs them concurrently.
//
// Whether it settles is decided when the composed task runs, so SetSettle
// applies to compositions a gulpfile built before the CLI parsed its flags.
func (u *Undertaker) Parallel(refs ...Ref) *Task {
	return u.composeSettleable(LabelParallel, bach.Parallel, bach.SettleParallel, refs)
}

// SettleSeries is Series but every task runs even after one fails, and the
// failures are reported together. It backs the CLI's --continue flag.
func (u *Undertaker) SettleSeries(refs ...Ref) *Task {
	return u.compose(LabelSeries, bach.SettleSeries, refs)
}

// SettleParallel is Parallel but a failing task does not cancel its siblings,
// and the failures are reported together.
func (u *Undertaker) SettleParallel(refs ...Ref) *Task {
	return u.compose(LabelParallel, bach.SettleParallel, refs)
}

// compose builds a branch task around one of bach's combinators.
func (u *Undertaker) composeSettleable(label string, normal, settled func(...TaskFunc) TaskFunc, refs []Ref) *Task {
	return u.compose(label, func(fns ...TaskFunc) TaskFunc {
		return func(ctx context.Context) error {
			if u.settle.Load() {
				return settled(fns...)(ctx)
			}
			return normal(fns...)(ctx)
		}
	}, refs)
}

func (u *Undertaker) compose(label string, combine func(...TaskFunc) TaskFunc, refs []Ref) *Task {
	t := &Task{Name: label, kind: kindFunction, branch: true, refs: refs}
	t.Fn = func(ctx context.Context) error {
		fns, err := u.instrumentAll(refs)
		if err != nil {
			return err
		}
		return combine(fns...)(ctx)
	}
	return t
}

// instrumentAll resolves every ref and wraps it for eventing and lastRun
// bookkeeping. Resolution failures surface as "Task never defined: <name>".
func (u *Undertaker) instrumentAll(refs []Ref) ([]TaskFunc, error) {
	fns := make([]TaskFunc, 0, len(refs))
	for _, ref := range refs {
		t, err := ref.resolve(u)
		if err != nil {
			return nil, err
		}
		fns = append(fns, u.instrument(t))
	}
	return fns, nil
}

// instrument wraps a task so that running it emits lifecycle events and
// maintains its lastRun timestamp.
//
// The timestamp is captured before the work starts and released if the work
// fails, which together produce the documented semantics: lastRun reports the
// start of the most recent SUCCESSFUL run, and reports nothing after an error.
func (u *Undertaker) instrument(t *Task) TaskFunc {
	return func(ctx context.Context) error {
		uid := runUID.Add(1)
		start := time.Now()
		u.lastRun.Capture(t, start)
		u.events.emit(Event{
			UID:    uid,
			Name:   t.Label(),
			Kind:   EventStart,
			Time:   start,
			Branch: t.branch,
		})

		err := asyncdone.Run(ctx, t.Fn)
		elapsed := time.Since(start)

		if err != nil {
			u.lastRun.Release(t)
			u.events.emit(Event{
				UID:      uid,
				Name:     t.Label(),
				Kind:     EventError,
				Time:     time.Now(),
				Duration: elapsed,
				Branch:   t.branch,
				Err:      err,
			})
			return err
		}

		u.events.emit(Event{
			UID:      uid,
			Name:     t.Label(),
			Kind:     EventStop,
			Time:     time.Now(),
			Duration: elapsed,
			Branch:   t.branch,
		})
		return nil
	}
}

// Run executes the named tasks.
//
// Multiple tasks run CONCURRENTLY, not in sequence. This mirrors the CLI
// contract in docs/CLI.md: "gulp build test" starts both at once. Use Series
// explicitly when ordering matters.
func (u *Undertaker) Run(ctx context.Context, refs ...Ref) error {
	if len(refs) == 0 {
		return nil
	}
	fns, err := u.instrumentAll(refs)
	if err != nil {
		return err
	}
	if u.settle.Load() {
		return bach.SettleParallel(fns...)(ctx)
	}
	return bach.Parallel(fns...)(ctx)
}

// LastRun reports when a task last completed successfully, truncated to
// precision. The boolean is false if it has never succeeded.
func (u *Undertaker) LastRun(ref Ref, precision time.Duration) (time.Time, bool, error) {
	if ref == nil {
		return time.Time{}, false, ErrNotATask
	}
	t, err := ref.resolve(u)
	if err != nil {
		return time.Time{}, false, err
	}
	at, ok := u.lastRun.LastRun(t, precision)
	return at, ok, nil
}

// On registers a listener for task lifecycle events. The CLI uses this to
// print "Starting 'build'..." and "Finished 'build' after 2.3 ms".
func (u *Undertaker) On(fn func(Event)) { u.events.on(fn) }
