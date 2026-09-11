package gulp

import (
	"context"
	"time"

	"github.com/gulpjs/gulp-go/undertaker"
)

// Default is the shared Gulp instance, the equivalent of the object
// `module.exports = new Gulp()` produces in index.js.
//
// The package-level functions below all delegate to it, so a gulpfile can call
// gulp.Task and gulp.Src directly the way index.mjs's named exports allow.
// Reach for New instead when a library needs a registry of its own.
var Default = New()

// Src reads files matching globs from the default instance.
func Src(globs []string, opts ...SrcOptions) *Pipeline {
	return Default.Src(globs, opts...)
}

// Dest writes incoming files into directory and re-emits them.
func Dest(directory string, opts ...DestOptions) Transform {
	return Default.Dest(directory, opts...)
}

// DestWith is Dest with the destination chosen per file.
func DestWith(fn func(*File) string, opts ...DestOptions) Transform {
	return Default.DestWith(fn, opts...)
}

// Symlink links incoming files into directory instead of copying them.
func Symlink(directory string, opts ...SymlinkOptions) Transform {
	return Default.Symlink(directory, opts...)
}

// SymlinkWith is Symlink with the destination chosen per file.
func SymlinkWith(fn func(*File) string, opts ...SymlinkOptions) Transform {
	return Default.SymlinkWith(fn, opts...)
}

// Watch runs task whenever a file matching globs changes.
//
// The caller owns the returned Watcher and must Close it.
func Watch(globs []string, opts WatchOptions, task TaskFunc) (*Watcher, error) {
	return Default.Watch(globs, opts, task)
}

// Task registers fn under name on the default instance.
func Task(name string, fn TaskFunc) *undertaker.Task {
	return Default.Task(name, fn)
}

// TaskRef registers an existing task or composition under name.
func TaskRef(name string, ref Ref) (*undertaker.Task, error) {
	return Default.TaskRef(name, ref)
}

// GetTask returns a registered task by name.
func GetTask(name string) (*undertaker.Task, bool) {
	return Default.GetTask(name)
}

// Tasks returns every registered task, keyed by name.
func Tasks() map[string]*undertaker.Task {
	return Default.Tasks()
}

// Series composes tasks to run one after another, stopping at the first error.
//
// The result is a task, not a running one; take its Fn field to get a TaskFunc
// or pass it straight to TaskRef.
func Series(refs ...Ref) *undertaker.Task {
	return Default.Series(refs...)
}

// Parallel composes tasks to run concurrently.
//
// Unlike JavaScript's bach, a failing sibling cancels the context the others
// received, so a well-behaved task stops early instead of running to
// completion after the build has already failed.
func Parallel(refs ...Ref) *undertaker.Task {
	return Default.Parallel(refs...)
}

// Tree describes the registered tasks. Passing true recurses into
// compositions, reporting them as <series> and <parallel> branches.
func Tree(deep bool) *Node {
	return Default.Tree(deep)
}

// LastRun reports when a task last completed successfully, rounded down to
// precision, and false if it never has.
func LastRun(ref Ref, precision time.Duration) (time.Time, bool, error) {
	return Default.LastRun(ref, precision)
}

// Registry returns the registry backing the default instance.
func Registry() undertaker.Registry {
	return Default.Registry()
}

// SetRegistry replaces the registry, transferring the tasks already
// registered before handing control over.
func SetRegistry(r undertaker.Registry) error {
	return Default.SetRegistry(r)
}

// Run executes the named tasks concurrently and waits for them to finish.
func Run(ctx context.Context, refs ...Ref) error {
	return Default.Run(ctx, refs...)
}

// On registers a listener for task start, stop and error events. It is how the
// CLI reports progress, and how a gulpfile can add its own logging.
func On(fn func(Event)) {
	Default.On(fn)
}
