// Command gulpfile is the Go counterpart of the gulpfile.cjs and gulpfile.mjs
// fixtures, which assert that every documented export is reachable and then
// signal completion.
//
// It lives in the module so that `go build ./...` type-checks it, and it is
// copied into a throwaway module by TestGulpfileRuns to be executed for real.
package main

import (
	"context"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/undertaker"
)

// surface pins every symbol index.mjs exports as a named export. Assigning
// them to variables of a written-out type is the Go equivalent of the
// fixture's `assert(typeof src === 'function')` checks: a missing or reshaped
// export fails at compile time rather than at run time.
var surface = struct {
	Src      func([]string, ...gulp.SrcOptions) *gulp.Pipeline
	Dest     func(string, ...gulp.DestOptions) gulp.Transform
	Symlink  func(string, ...gulp.SymlinkOptions) gulp.Transform
	Watch    func([]string, gulp.WatchOptions, gulp.TaskFunc) (*gulp.Watcher, error)
	Task     func(string, gulp.TaskFunc) *undertaker.Task
	Series   func(...gulp.Ref) *undertaker.Task
	Parallel func(...gulp.Ref) *undertaker.Task
	Tree     func(bool) *gulp.Node
	LastRun  func(gulp.Ref, time.Duration) (time.Time, bool, error)
	Registry func() undertaker.Registry
}{
	Src:      gulp.Src,
	Dest:     gulp.Dest,
	Symlink:  gulp.Symlink,
	Watch:    gulp.Watch,
	Task:     gulp.Task,
	Series:   gulp.Series,
	Parallel: gulp.Parallel,
	Tree:     gulp.Tree,
	LastRun:  gulp.LastRun,
	Registry: gulp.Registry,
}

func main() {
	if surface.Src == nil || surface.Registry == nil {
		panic("gulp exports are missing")
	}
	gulp.Task("default", func(_ context.Context) error { return nil })
	gulp.Main()
}
