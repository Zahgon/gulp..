// Package plugins provides the transforms that a gulp build reaches for most
// often, plus the machinery to write more.
//
// gulp's JavaScript ecosystem lives on npm: gulp-concat, gulp-rename,
// gulp-replace, gulp-if, gulp-sourcemaps and several thousand others, all of
// them Node streams that a build wires together with .pipe(). Those packages
// cannot be ported, so this package supplies three things in their place.
//
// First, the small set of plugins that are effectively part of gulp's working
// vocabulary: Concat, Rename, Replace, Filter, If, and sourcemap init/write.
//
// Second, Exec, which turns any command-line tool into a transform. Most of
// what the npm plugins wrap — esbuild, sass, terser, imagemin, svgo — ships a
// CLI, and piping a file through it is usually all a plugin ever did:
//
//	gulp.Src([]string{"src/**/*.scss"}).
//		Pipe(plugins.Exec(plugins.ExecOptions{
//			Name:    "sass",
//			Command: "sass",
//			Args:    func(*gulp.File) []string { return []string{"--stdin"} },
//			Rename:  func(p *plugins.Path) { p.Extname = ".css" },
//		})).
//		Pipe(gulp.Dest("dist"))
//
// Third, the pipeline.Transform interface itself, which is all a plugin has to
// satisfy. pipeline.Map is usually enough to write one in a few lines.
//
// Every transform here follows gulp's plugin conventions: null files and
// directories pass through untouched, and a streaming file is either handled
// or reported as an error rather than silently dropped.
package plugins

import (
	"fmt"

	"github.com/gulpjs/gulp-go/vinyl"
)

// Error identifies which plugin failed and on which file, the way
// plugin-error does for the JavaScript plugins.
type Error struct {
	Plugin  string
	Path    string
	Message string
	Err     error
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := e.Message
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if e.Path != "" {
		return fmt.Sprintf("%s: %s: %s", e.Plugin, e.Path, msg)
	}
	return fmt.Sprintf("%s: %s", e.Plugin, msg)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Err }

func pluginErr(plugin string, f *vinyl.File, err error) error {
	if err == nil {
		return nil
	}
	e := &Error{Plugin: plugin, Err: err}
	if f != nil {
		e.Path = f.Path()
	}
	return e
}

func pluginErrf(plugin string, f *vinyl.File, format string, args ...any) error {
	e := &Error{Plugin: plugin, Message: fmt.Sprintf(format, args...)}
	if f != nil {
		e.Path = f.Path()
	}
	return e
}

// passthrough reports whether a transform should leave the file alone.
//
// gulp plugins are expected to forward null files (read: false) and directory
// entries rather than fail on them, because a pipeline is routinely built once
// and reused with different src options.
func passthrough(f *vinyl.File) bool {
	return f.IsNull() || f.IsDirectory()
}
