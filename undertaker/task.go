// Package undertaker is the task registry and composition engine behind gulp.
//
// It is the Go port of npm `undertaker` plus `undertaker-registry`. Every
// public method on the gulp object except src/dest/symlink/watch comes from
// here: task, series, parallel, registry, tree and lastRun.
//
// The central design problem in this port is that JavaScript attaches metadata
// to function objects -- `fn.displayName`, `fn.description`, `fn.flags`, and a
// hidden marker used by tree() to recurse into compositions. Go functions
// cannot carry properties, so that metadata moves onto an explicit Task struct
// and callers pass Refs rather than bare functions.
package undertaker

import (
	"github.com/gulpjs/gulp-go/internal/asyncdone"
)

// TaskFunc is the canonical gulp task signature, re-exported from asyncdone.
type TaskFunc = asyncdone.TaskFunc

// kind distinguishes a registered task from an inline function, which tree()
// reports as the "type" field.
type kind string

const (
	kindTask     kind = "task"
	kindFunction kind = "function"
)

// Labels used for compositions and unnamed functions. They are part of gulp's
// observable output: `gulp --tasks` prints them verbatim.
const (
	LabelSeries    = "<series>"
	LabelParallel  = "<parallel>"
	LabelAnonymous = "<anonymous>"
)

// Task is a named unit of work plus the metadata gulp surfaces through
// `gulp --tasks`.
type Task struct {
	// Name is the display name. Empty means anonymous.
	Name string
	// Description is shown beside the task by `gulp --tasks`.
	Description string
	// Flags documents CLI flags the task understands, shown by `gulp --tasks`.
	Flags map[string]string
	// Fn performs the work.
	Fn TaskFunc

	// kind records whether this task was registered by name or supplied
	// inline, which tree() reports.
	kind kind
	// branch marks a composition produced by Series or Parallel. gulp-cli uses
	// the equivalent flag to suppress log lines for synthetic nodes.
	branch bool
	// refs holds the unresolved children of a composition. They stay
	// unresolved so that a name referring to a task registered later still
	// works, which is the behaviour JS gets for free by looking up at call
	// time.
	refs []Ref
}

// Label returns the name to display, substituting the anonymous placeholder.
func (t *Task) Label() string {
	if t.Name == "" {
		return LabelAnonymous
	}
	return t.Name
}

// IsBranch reports whether the task is a series or parallel composition.
func (t *Task) IsBranch() bool { return t.branch }

// resolve implements Ref: a Task refers to itself.
func (t *Task) resolve(*Undertaker) (*Task, error) { return t, nil }

// Ref is anything that can name a task: a registered task's name, or a Task
// value.
//
// Resolution is deferred to execution time, matching undertaker's behaviour of
// looking names up in the registry when the composed function is finally
// called rather than when it is composed.
type Ref interface {
	resolve(u *Undertaker) (*Task, error)
}

// nameRef refers to a task by its registered name.
type nameRef string

// Name returns a Ref that looks a task up in the registry when it runs.
// This is what makes `series('clean', 'build')` work.
func Name(name string) Ref { return nameRef(name) }

// Names is a convenience for referring to several registered tasks.
func Names(names ...string) []Ref {
	refs := make([]Ref, len(names))
	for i, n := range names {
		refs[i] = nameRef(n)
	}
	return refs
}

func (n nameRef) resolve(u *Undertaker) (*Task, error) {
	t, ok := u.Get(string(n))
	if !ok {
		return nil, &UndefinedTaskError{Name: string(n)}
	}
	return t, nil
}

// Func builds a named, unregistered task. Registering it is a separate step,
// mirroring how a JS function with a displayName can be passed straight into
// series() without ever being registered.
func Func(name string, fn TaskFunc) *Task {
	return &Task{Name: name, Fn: fn, kind: kindFunction}
}

// Anonymous builds an unnamed task. tree() labels it `<anonymous>`.
func Anonymous(fn TaskFunc) *Task {
	return &Task{Fn: fn, kind: kindFunction}
}
