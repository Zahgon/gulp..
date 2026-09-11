package vinyl

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// File is the Go port of the JS `Vinyl` class: a metadata object describing a
// file, decoupled from where that file lives.
//
// Fields that the JS class exposes as accessors with validation (path, base,
// cwd, symlink) are unexported here and reached through methods, because the
// validation and the `history` bookkeeping are load-bearing: dest() computes
// output paths from base, and plugins are expected to push new paths onto the
// history rather than overwrite it.
type File struct {
	cwd     string
	base    string   // empty means "fall back to cwd", matching JS `_base`
	history []string // every path this file has had; last entry is current

	// Stat carries file system metadata. May be nil.
	Stat *Stat

	// Contents is Buffer, *Stream, or nil. See the Contents interface.
	Contents Contents

	// SourceMap is populated when src() is used with the sourcemaps option
	// and consumed by dest(). Mirrors the `file.sourceMap` property that the
	// vinyl-sourcemap package attaches.
	SourceMap *SourceMap

	// symlink holds the link target when this file represents a symbolic
	// link that should be recreated rather than copied.
	symlink string

	// custom holds arbitrary plugin-defined properties, the Go stand-in for
	// assigning ad-hoc properties to a JS object.
	custom map[string]any
}

// Options configures New. Zero values fall back to the same defaults as the
// JS constructor.
type Options struct {
	Cwd      string
	Base     string
	Path     string
	History  []string
	Stat     *Stat
	Contents Contents
	Symlink  string
	Custom   map[string]any
}

// Errors reported by the constructor and accessors. The messages are kept
// identical to the JS implementation because gulp's own tests and plugins
// match on them.
var (
	ErrCwdNotString  = errors.New("cwd must be a non-empty string")
	ErrBaseNotString = errors.New("base must be a non-empty string, or null/undefined")
	ErrPathNotString = errors.New("path should be a string")
	ErrNoPath        = errors.New("no path specified! can not get relative")
	ErrNoBase        = errors.New("no base specified! can not get relative")
)

// New constructs a File, applying the same normalisation the JS constructor
// applies: cwd and base are normalised and stripped of trailing separators,
// and an explicit Path is appended to History.
func New(opts Options) (*File, error) {
	cwd := opts.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("vinyl: resolving cwd: %w", err)
		}
		cwd = wd
	}
	f := &File{
		cwd:      normalizePath(cwd),
		Stat:     opts.Stat,
		Contents: opts.Contents,
		symlink:  opts.Symlink,
	}
	if opts.Base != "" {
		f.base = normalizePath(opts.Base)
	}
	for _, h := range opts.History {
		if h == "" {
			return nil, ErrPathNotString
		}
		f.history = append(f.history, normalizePath(h))
	}
	if opts.Path != "" {
		p := normalizePath(opts.Path)
		if len(f.history) == 0 || f.history[len(f.history)-1] != p {
			f.history = append(f.history, p)
		}
	}
	if len(opts.Custom) > 0 {
		f.custom = maps.Clone(opts.Custom)
	}
	return f, nil
}

// MustNew is New but panics on error. Intended for tests and for call sites
// that have already validated their inputs.
func MustNew(opts Options) *File {
	f, err := New(opts)
	if err != nil {
		panic(err)
	}
	return f
}

// normalizePath mirrors vinyl's `normalize`: clean the path and drop any
// trailing separator (except for a bare root).
func normalizePath(p string) string {
	if p == "" {
		return p
	}
	cleaned := filepath.Clean(p)
	if len(cleaned) > 1 {
		cleaned = strings.TrimRight(cleaned, string(filepath.Separator))
		if cleaned == "" {
			cleaned = string(filepath.Separator)
		}
	}
	return cleaned
}

// Cwd returns the working directory this file is relative to.
func (f *File) Cwd() string { return f.cwd }

// SetCwd sets the working directory, rejecting empty values as JS does.
func (f *File) SetCwd(cwd string) error {
	if cwd == "" {
		return ErrCwdNotString
	}
	f.cwd = normalizePath(cwd)
	return nil
}

// Base returns the glob base: the path segment removed from Path when
// computing the output location in dest(). Falls back to Cwd when unset,
// matching the JS getter.
func (f *File) Base() string {
	if f.base == "" {
		return f.cwd
	}
	return f.base
}

// SetBase sets the glob base. Passing an empty string resets it to Cwd, which
// is how the JS setter treats null/undefined.
func (f *File) SetBase(base string) error {
	if base == "" {
		f.base = ""
		return nil
	}
	f.base = normalizePath(base)
	return nil
}

// Path returns the current path, i.e. the last entry in the history.
func (f *File) Path() string {
	if len(f.history) == 0 {
		return ""
	}
	return f.history[len(f.history)-1]
}

// SetPath appends a new path to the history. Repeated assignment of the same
// value is a no-op, exactly as in JS, so plugins can assign defensively.
func (f *File) SetPath(p string) error {
	if p == "" {
		return ErrPathNotString
	}
	n := normalizePath(p)
	if len(f.history) > 0 && f.history[len(f.history)-1] == n {
		return nil
	}
	f.history = append(f.history, n)
	return nil
}

// History returns a copy of every path this file has had, oldest first.
func (f *File) History() []string {
	out := make([]string, len(f.history))
	copy(out, f.history)
	return out
}

// Relative returns the path relative to Base. dest() joins this onto the
// output directory, which is what preserves directory structure.
func (f *File) Relative() (string, error) {
	if f.Path() == "" {
		return "", ErrNoPath
	}
	if f.Base() == "" {
		return "", ErrNoBase
	}
	rel, err := filepath.Rel(f.Base(), f.Path())
	if err != nil {
		return "", fmt.Errorf("vinyl: computing relative path: %w", err)
	}
	return rel, nil
}

// Dirname returns the directory portion of Path.
func (f *File) Dirname() string { return filepath.Dir(f.Path()) }

// SetDirname re-parents the file, keeping its base name.
func (f *File) SetDirname(dir string) error {
	return f.SetPath(filepath.Join(dir, f.Basename()))
}

// Basename returns the final path segment, including the extension.
func (f *File) Basename() string { return filepath.Base(f.Path()) }

// SetBasename renames the file within its current directory.
func (f *File) SetBasename(name string) error {
	return f.SetPath(filepath.Join(f.Dirname(), name))
}

// Extname returns the extension including the leading dot, or "" if none.
func (f *File) Extname() string { return filepath.Ext(f.Path()) }

// SetExtname replaces the extension. ext should include the leading dot.
func (f *File) SetExtname(ext string) error {
	return f.SetBasename(f.Stem() + ext)
}

// Stem returns the base name without its extension.
func (f *File) Stem() string {
	b := f.Basename()
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// SetStem replaces the base name while keeping the extension.
func (f *File) SetStem(stem string) error {
	return f.SetBasename(stem + f.Extname())
}

// Symlink returns the link target for files that represent symbolic links.
func (f *File) Symlink() string { return f.symlink }

// SetSymlink marks this file as a symbolic link pointing at target. dest()
// creates a link instead of writing contents when this is set.
func (f *File) SetSymlink(target string) error {
	if target == "" {
		f.symlink = ""
		return nil
	}
	f.symlink = normalizePath(target)
	return nil
}

// IsBuffer reports whether Contents is an in-memory buffer.
func (f *File) IsBuffer() bool { return f.Contents != nil && f.Contents.contents() == kindBuffer }

// IsStream reports whether Contents is a stream.
func (f *File) IsStream() bool { return f.Contents != nil && f.Contents.contents() == kindStream }

// IsNull reports whether Contents is absent (src read:false, or a directory).
func (f *File) IsNull() bool { return f.Contents == nil }

// IsDirectory reports whether this file describes a directory. As in JS, this
// requires null contents plus a stat that says directory.
func (f *File) IsDirectory() bool {
	return f.IsNull() && f.Stat != nil && f.Stat.IsDir()
}

// IsSymbolic reports whether this file describes a symbolic link that should
// be recreated rather than followed.
func (f *File) IsSymbolic() bool {
	return f.IsNull() && f.Stat != nil && f.Stat.IsSymbolic()
}

// Bytes returns the contents as a byte slice regardless of representation,
// draining and resetting a stream if necessary. Returns nil for null files.
func (f *File) Bytes() ([]byte, error) {
	switch c := f.Contents.(type) {
	case Buffer:
		return c, nil
	case *Stream:
		return c.Bytes()
	default:
		return nil, nil
	}
}

// Get returns a custom property previously set by a plugin.
func (f *File) Get(key string) (any, bool) {
	v, ok := f.custom[key]
	return v, ok
}

// Set stores a custom property. This is the Go equivalent of assigning an
// ad-hoc property onto a JS vinyl object; Clone carries these across.
func (f *File) Set(key string, value any) {
	if f.custom == nil {
		f.custom = make(map[string]any)
	}
	f.custom[key] = value
}

// Custom returns a copy of all custom properties.
func (f *File) Custom() map[string]any { return maps.Clone(f.custom) }

// CloneOptions configures Clone.
type CloneOptions struct {
	// Contents, when false, shares the contents with the original instead of
	// copying them. Mirrors `clone({ contents: false })`.
	Contents *bool
	// Deep controls whether custom properties are deep-copied. Defaults to
	// true, as in JS.
	Deep *bool
}

// Clone returns a copy of the file. By default contents are copied: buffers
// are duplicated so mutations do not leak between clones, and streams are
// re-derived from the same opener so both copies can be read independently.
func (f *File) Clone(opts ...CloneOptions) *File {
	var o CloneOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	copyContents := o.Contents == nil || *o.Contents

	c := &File{
		cwd:       f.cwd,
		base:      f.base,
		history:   append([]string(nil), f.history...),
		Stat:      f.Stat.Clone(),
		SourceMap: f.SourceMap.Clone(),
		symlink:   f.symlink,
		custom:    maps.Clone(f.custom),
	}

	switch contents := f.Contents.(type) {
	case Buffer:
		if copyContents {
			dup := make(Buffer, len(contents))
			copy(dup, contents)
			c.Contents = dup
		} else {
			c.Contents = contents
		}
	case *Stream:
		if copyContents {
			// Both clones must be independently readable, so give the clone
			// its own Stream over the same opener rather than sharing the
			// reader. This is the Go analogue of the JS `cloneStream` helper.
			c.Contents = NewStream(contents.open)
		} else {
			c.Contents = contents
		}
	default:
		c.Contents = nil
	}
	return c
}

// String renders the file the way vinyl's inspect() does, which keeps test
// failures and CLI logs readable.
func (f *File) String() string {
	var b strings.Builder
	b.WriteString("<File ")
	if p := f.Path(); p != "" {
		if rel, err := f.Relative(); err == nil {
			b.WriteString(rel)
		} else {
			b.WriteString(p)
		}
	}
	switch {
	case f.IsBuffer():
		b.WriteString(" <Buffer>")
	case f.IsStream():
		b.WriteString(" <Stream>")
	}
	b.WriteString(">")
	return b.String()
}

// IsVinyl reports whether v is a vinyl File. It is the port of
// `Vinyl.isVinyl()`, used by plugins to validate stream contents.
func IsVinyl(v any) bool {
	_, ok := v.(*File)
	return ok
}

// builtinProps are the property names the JS Vinyl class reserves.
var builtinProps = map[string]struct{}{
	"_contents": {}, "_symlink": {}, "contents": {}, "cwd": {}, "base": {},
	"stat": {}, "history": {}, "path": {}, "symlink": {}, "_isVinyl": {},
}

// IsCustomProp reports whether a property name is safe for a plugin to use,
// i.e. it is not one of vinyl's own. Port of `Vinyl.isCustomProp()`.
func IsCustomProp(name string) bool {
	_, reserved := builtinProps[name]
	return !reserved
}
