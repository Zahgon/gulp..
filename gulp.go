// Package gulp is a Go port of gulp, the streaming build system.
//
// The JavaScript original is a thin façade: index.js defines a Gulp class that
// inherits from undertaker for task registration and borrows src, dest and
// symlink from vinyl-fs and watch from glob-watcher, then exports a single
// instance. This package keeps that shape. Gulp is the type, New builds one,
// and the package-level functions in default.go operate on a shared instance,
// which is the direct equivalent of `module.exports = new Gulp()` and of the
// named exports in index.mjs.
//
// A minimal build looks like this:
//
//	func build(ctx context.Context) error {
//		return gulp.Src([]string{"src/**/*.js"}).
//			Pipe(gulp.Dest("dist")).
//			Run(ctx)
//	}
//
//	func main() {
//		gulp.Task("build", build)
//		gulp.Task("default", gulp.Series(gulp.Name("build")).Fn)
//		gulp.Main()
//	}
//
// Differences from the JavaScript API are limited to those the type system
// forces. They are catalogued in MIGRATION.md; the two that surface most often
// are that a task is always func(context.Context) error rather than any of the
// six async conventions Node accepts, and that passing a task name where a
// function is expected is a compile error rather than a runtime one.
package gulp

import (
	"context"
	"time"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/undertaker"
	"github.com/gulpjs/gulp-go/vfs"
	"github.com/gulpjs/gulp-go/vinyl"
	"github.com/gulpjs/gulp-go/watch"
)

// Re-exported types. A gulpfile should be able to import this one package and
// nothing else for everyday work, which is what the JavaScript module offers.
type (
	// File is a vinyl file: metadata plus contents, the value that travels
	// through a pipeline.
	File = vinyl.File
	// Pipeline is a chain of transforms, the equivalent of a Node stream
	// built up with .pipe().
	Pipeline = pipeline.Pipeline
	// Transform is one stage of a pipeline.
	Transform = pipeline.Transform

	// TaskFunc is the signature of every task.
	TaskFunc = undertaker.TaskFunc
	// Ref identifies a task, either by name or directly.
	Ref = undertaker.Ref
	// Node is a node in the task tree reported by Tree.
	Node = undertaker.Node
	// Event is a task lifecycle notification.
	Event = undertaker.Event

	// SrcOptions configures Src.
	SrcOptions = vfs.SrcOptions
	// DestOptions configures Dest.
	DestOptions = vfs.DestOptions
	// SymlinkOptions configures Symlink.
	SymlinkOptions = vfs.SymlinkOptions
	// WatchOptions configures Watch.
	WatchOptions = watch.Options
	// Watcher is the handle returned by Watch.
	Watcher = watch.Watcher
)

// Re-exported constructors, so that building a Ref or an option value does not
// require importing a second package.
var (
	// Name refers to a task by name. The task need not exist yet; names are
	// resolved when the composition runs.
	Name = undertaker.Name
	// Names refers to several tasks by name.
	Names = undertaker.Names
	// Anonymous wraps a function as an unnamed task, reported as <anonymous>.
	Anonymous = undertaker.Anonymous
	// Fn wraps a function as a named task without registering it.
	Fn = undertaker.Func
)

// Value pins a src or dest option to a constant, as in
// SrcOptions{Buffer: gulp.Value(false)}.
//
// The option type itself is vfs.Option[T]. It is not re-exported here as
// gulp.Option[T] because a generic type alias requires Go 1.24, and this
// module builds on Go 1.23. Nothing is lost at the call site: these
// constructors are how an option is normally built, and the type only needs
// naming when a variable is declared for one.
func Value[T any](v T) vfs.Option[T] { return vfs.Value(v) }

// Func computes a src or dest option from each file, the equivalent of passing
// a function where vinyl-fs accepts a value.
func Func[T any](fn func(*File) T) vfs.Option[T] { return vfs.Func(fn) }

// Ptr returns a pointer to v, for the optional fields of WatchOptions whose
// default is not the zero value.
func Ptr[T any](v T) *T { return watch.Ptr(v) }

// Gulp is a task registry with the vinyl filesystem operations attached.
//
// It corresponds to the Gulp class in index.js, which calls Undertaker as a
// constructor and then hangs src, dest, symlink and watch off the prototype.
// Embedding gives Go the same inheritance: every undertaker method — Series,
// Parallel, Run, Tree, LastRun, Registry, SetRegistry, On — is promoted.
//
// A Gulp is safe for concurrent use.
type Gulp struct {
	*undertaker.Undertaker
}

// New creates an independent Gulp instance.
//
// The JavaScript module exposes its own class as `gulp.Gulp` so that a plugin
// can build a private instance rather than sharing the singleton; this is that
// escape hatch.
func New() *Gulp {
	return &Gulp{Undertaker: undertaker.New()}
}

// Src reads files matching globs and returns a pipeline that emits them.
//
// Globs are matched in the order given, and a leading "!" negates. At most one
// SrcOptions may be supplied; further values are ignored. Nothing touches the
// filesystem until the pipeline is started, which preserves the guarantee gulp
// 5.0.1 added in "Avoid globbing before read stream is opened": a src built
// but never run performs no I/O, so the file list reflects the moment the
// build actually begins.
func (g *Gulp) Src(globs []string, opts ...SrcOptions) *Pipeline {
	return vfs.Src(globs, first(opts))
}

// Dest writes incoming files into directory and re-emits them, so a pipeline
// can write to more than one destination.
//
// The path written is directory joined with the file's path relative to its
// base, which is what preserves the directory structure below the glob's
// non-magic prefix.
func (g *Gulp) Dest(directory string, opts ...DestOptions) Transform {
	return vfs.Dest(directory, first(opts))
}

// DestWith is Dest with the destination chosen per file, the equivalent of
// passing a function to dest() in JavaScript.
func (g *Gulp) DestWith(fn func(*File) string, opts ...DestOptions) Transform {
	return vfs.DestWith(fn, first(opts))
}

// Symlink creates links in directory pointing at the incoming files, instead
// of copying their contents.
func (g *Gulp) Symlink(directory string, opts ...SymlinkOptions) Transform {
	return vfs.Symlink(directory, first(opts))
}

// SymlinkWith is Symlink with the destination chosen per file.
func (g *Gulp) SymlinkWith(fn func(*File) string, opts ...SymlinkOptions) Transform {
	return vfs.SymlinkWith(fn, first(opts))
}

// Watch runs task whenever a file matching globs changes.
//
// task may be nil, in which case the returned Watcher only drives listeners
// registered with its On method. A non-nil task is wrapped in Parallel exactly
// as index.js does, so it participates in task eventing and appears in the
// CLI's output like any other run.
//
// The caller owns the returned Watcher and must Close it.
//
// JavaScript rejects a task name here at runtime, with "watch task has to be a
// function (optionally generated by using gulp.parallel or gulp.series)". In
// Go the parameter is typed, so the same mistake does not compile; use
// Series or Parallel to build a TaskFunc from names.
func (g *Gulp) Watch(globs []string, opts WatchOptions, task TaskFunc) (*Watcher, error) {
	var fn TaskFunc
	if task != nil {
		fn = g.Parallel(undertaker.Anonymous(task)).Fn
	}
	return watch.New(globs, opts, fn)
}

// Task registers fn under name and returns the stored task.
//
// Registering the same name twice replaces the earlier task, matching the
// JavaScript behaviour.
func (g *Gulp) Task(name string, fn TaskFunc) *undertaker.Task {
	return g.Set(name, fn)
}

// TaskRef registers an existing task or composition under name.
//
// This is the second form of JavaScript's gulp.task, where the value is the
// result of gulp.series or gulp.parallel rather than a bare function:
//
//	gulp.TaskRef("build", gulp.Series(gulp.Names("clean", "compile")...))
//
// Registering by reference rather than by value means the composition keeps
// its own identity, so Tree reports it as a branch and its children appear
// beneath the registered name.
func (g *Gulp) TaskRef(name string, ref Ref) (*undertaker.Task, error) {
	return g.SetTask(name, ref)
}

// GetTask returns a registered task by name.
func (g *Gulp) GetTask(name string) (*undertaker.Task, bool) {
	return g.Get(name)
}

// LastRun reports when a task last completed successfully, rounded down to
// precision, and false if it has never succeeded.
//
// Feeding this to SrcOptions.Since is the incremental-build pattern: only
// files modified since the previous successful run are read.
//
//	since, ok, _ := gulp.LastRun(gulp.Name("build"), 0)
//	opts := gulp.SrcOptions{}
//	if ok {
//		opts.Since = vfs.Value(since)
//	}
func (g *Gulp) LastRun(ref Ref, precision time.Duration) (time.Time, bool, error) {
	return g.Undertaker.LastRun(ref, precision)
}

// Run executes the named tasks and waits for them to finish.
//
// Multiple tasks run CONCURRENTLY, matching `gulp a b c` on the command line.
// Wrap them in Series when order matters.
func (g *Gulp) Run(ctx context.Context, refs ...Ref) error {
	return g.Undertaker.Run(ctx, refs...)
}

// first returns the single optional value from a variadic option parameter.
//
// The JavaScript signatures all end in an optional options object; a variadic
// parameter is the closest Go equivalent. Extra values are ignored rather than
// merged, because merging would silently half-apply a caller's mistake.
func first[T any](opts []T) T {
	if len(opts) > 0 {
		return opts[0]
	}
	var zero T
	return zero
}
