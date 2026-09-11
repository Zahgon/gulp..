// Package vfs is the Go port of the `vinyl-fs` npm package: it provides
// [Src], [Dest] and [Symlink], the three filesystem adapters that turn globs
// into a stream of [vinyl.File] values and write those values back to disk.
//
// # Option resolution
//
// Every vinyl-fs option may be either a plain value or a function of the file
// being processed (npm: `value-or-function` + `resolve-options`). That is
// modelled here by the generic [Option] type: an unset Option falls back to the
// documented default, a value Option always resolves to the same value, and a
// function Option is re-evaluated for each file.
//
//	vfs.Src([]string{"src/**/*.js"}, vfs.SrcOptions{
//		Buffer: vfs.Value(false),
//		Since:  vfs.Func(func(f *vinyl.File) time.Time { return lastBuild }),
//	})
package vfs

import "github.com/gulpjs/gulp-go/vinyl"

// Option holds a vinyl-fs option that may be a static value or derived from the
// file currently being processed. The zero Option is "unset" and resolves to
// whatever default the call site supplies.
//
// A function Option that returns the zero value of T is treated as "no opinion"
// by [Option.Resolve] only if the function reports false for its second result;
// use [Func] for total functions and [FuncOK] when the function needs to defer
// to the default for some files. This mirrors value-or-function, which ignores
// `undefined` results and falls back to the default.
type Option[T any] struct {
	set bool
	val T
	fn  func(*vinyl.File) (T, bool)
}

// Value returns an Option that always resolves to v.
func Value[T any](v T) Option[T] {
	return Option[T]{set: true, val: v}
}

// Func returns an Option computed from each file.
func Func[T any](fn func(*vinyl.File) T) Option[T] {
	if fn == nil {
		return Option[T]{}
	}
	return Option[T]{set: true, fn: func(f *vinyl.File) (T, bool) { return fn(f), true }}
}

// FuncOK returns an Option computed from each file that may decline to provide
// a value by returning false, in which case the default is used. This is the
// equivalent of a value-or-function callback returning `undefined`.
func FuncOK[T any](fn func(*vinyl.File) (T, bool)) Option[T] {
	if fn == nil {
		return Option[T]{}
	}
	return Option[T]{set: true, fn: fn}
}

// IsSet reports whether the option was explicitly provided.
func (o Option[T]) IsSet() bool { return o.set }

// Resolve returns the option's value for file f, falling back to def when the
// option is unset or its function declines to provide a value.
func (o Option[T]) Resolve(f *vinyl.File, def T) T {
	if !o.set {
		return def
	}
	if o.fn != nil {
		if v, ok := o.fn(f); ok {
			return v
		}
		return def
	}
	return o.val
}
