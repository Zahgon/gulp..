package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gulpjs/gulp-go/internal/globstream"
	"github.com/gulpjs/gulp-go/internal/sourcemap"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// SrcOptions configures [Src]. Every field is optional; the zero value selects
// the documented vinyl-fs default.
//
// Fields typed [Option] accept either a static value via [Value] or a per-file
// function via [Func], mirroring the JavaScript API where most options may be
// "a value or a function".
type SrcOptions struct {
	// Cwd is the directory that relative globs and the default base are
	// resolved against. Defaults to the process working directory.
	Cwd string

	// Base overrides the automatically computed glob base, which is otherwise
	// the path segment before the first glob magic character. The base is
	// removed from each path by [Dest] to preserve directory structure.
	Base string

	// CwdBase makes every file's base equal to Cwd instead of the glob base.
	CwdBase bool

	// Root is the directory that absolute globs are resolved against.
	Root string

	// AllowEmpty suppresses the "File not found with singular glob" error that
	// a non-magic glob raises when it matches nothing.
	AllowEmpty bool

	// UniqueBy is the vinyl property used to deduplicate matches. Defaults to
	// "path"; set it to the empty string together with a nil UniqueByFunc to
	// disable deduplication.
	UniqueBy string

	// UniqueByFunc deduplicates using an arbitrary key derived from each file
	// and takes precedence over UniqueBy.
	UniqueByFunc func(*vinyl.File) any

	// Ignore lists globs whose matches are excluded, equivalent to prefixing
	// those patterns with "!".
	Ignore []string

	// Dot allows glob wildcards to match paths whose segments begin with a dot.
	Dot bool
	// NoBrace disables brace expansion, so "{" and "}" match literally.
	NoBrace bool
	// NoGlobstar makes "**" behave like "*" and never cross directories.
	NoGlobstar bool
	// NoExt disables extglob patterns such as "+(a|b)".
	NoExt bool
	// NoCase makes glob matching case-insensitive.
	NoCase bool
	// MatchBase matches a pattern containing no slashes against the basename.
	MatchBase bool

	// Encoding names the character encoding of the files on disk; contents are
	// decoded to UTF-8. Defaults to [DefaultEncoding]. Use
	// Value([vfs.EncodingDisabled]) to read raw bytes without transcoding or
	// BOM handling, the equivalent of `encoding: false`.
	Encoding Option[string]

	// Buffer, when false, delivers contents as a lazily opened stream rather
	// than reading whole files into memory. Defaults to true.
	Buffer Option[bool]

	// Read, when false, leaves contents empty and delivers metadata only.
	// Defaults to true.
	Read Option[bool]

	// Since filters out files last modified at or before the given time, which
	// is how incremental builds are expressed. The zero time disables the
	// filter.
	Since Option[time.Time]

	// RemoveBOM strips a UTF-8 byte order mark from decoded contents. Defaults
	// to true.
	RemoveBOM Option[bool]

	// SourceMaps enables source map support: an inline or external map is
	// parsed, attached to the file and removed from its contents. Defaults to
	// false.
	SourceMaps Option[bool]

	// ResolveSymlinks makes the stream follow symbolic links and report the
	// stat of the target. Defaults to true. When false, the link itself is
	// emitted with its target recorded on the file's symlink property.
	ResolveSymlinks Option[bool]
}

// Src returns a lazy [pipeline.Pipeline] that emits one [vinyl.File] for every
// path matching globs. It is the port of `vinyl-fs`'s `src()`.
//
// A glob prefixed with "!" excludes matches; negation is order-independent.
// Files are emitted in glob-argument order and, within a single glob, in
// lexicographic order.
//
// Nothing touches the filesystem until the pipeline is started. This preserves
// the fix released in gulp 5.0.1 ("Avoid globbing before read stream is
// opened"), which matters when an earlier task creates the files a later task
// globs for.
//
//	files, err := vfs.Src([]string{"src/**/*.js"}, vfs.SrcOptions{}).
//		Pipe(plugins.Replace("foo", "bar")).
//		Collect(ctx)
func Src(globs []string, opts SrcOptions) *pipeline.Pipeline {
	source := globstream.New(globs, globstream.Options{
		Cwd:             opts.Cwd,
		Base:            opts.Base,
		CwdBase:         opts.CwdBase,
		Root:            opts.Root,
		AllowEmpty:      opts.AllowEmpty,
		UniqueBy:        opts.UniqueBy,
		UniqueByFunc:    opts.UniqueByFunc,
		Ignore:          opts.Ignore,
		ResolveSymlinks: opts.ResolveSymlinks.Resolve(nil, true),
		Dot:             opts.Dot,
		NoBrace:         opts.NoBrace,
		NoGlobstar:      opts.NoGlobstar,
		NoExt:           opts.NoExt,
		NoCase:          opts.NoCase,
		MatchBase:       opts.MatchBase,
	})

	p := pipeline.New(source)

	if opts.Since.IsSet() {
		p = p.Pipe(sinceFilter(opts.Since))
	}

	p = p.Pipe(&contentsReader{opts: opts})

	if opts.SourceMaps.IsSet() {
		p = p.Pipe(sourceMapReader{opts: opts})
	}

	return p
}

// sinceFilter drops files that were last modified at or before the resolved
// time, which is how `since` expresses "only rebuild what changed".
func sinceFilter(since Option[time.Time]) pipeline.Transform {
	return pipeline.Filter(func(f *vinyl.File) bool {
		cutoff := since.Resolve(f, time.Time{})
		if cutoff.IsZero() || f.Stat == nil {
			return true
		}
		return f.Stat.MTime.After(cutoff)
	})
}

// contentsReader populates each file's contents according to the read, buffer,
// encoding and removeBOM options. It is the port of vinyl-fs's
// `lib/src/read-contents`.
type contentsReader struct {
	opts   SrcOptions
	codecs sync.Map // string -> *codec
}

// codecFor resolves and caches the encoding for a file.
func (r *contentsReader) codecFor(f *vinyl.File) (*codec, error) {
	name := r.opts.Encoding.Resolve(f, DefaultEncoding)
	if cached, ok := r.codecs.Load(name); ok {
		c, _ := cached.(*codec)
		return c, nil
	}
	c, err := lookupEncoding(name)
	if err != nil {
		return nil, err
	}
	r.codecs.Store(name, c)
	return c, nil
}

func (r *contentsReader) Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	for {
		f, ok, err := pipeline.Recv(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := r.read(f); err != nil {
			return err
		}
		if err := pipeline.Send(ctx, out, f); err != nil {
			return err
		}
	}
}

// read attaches contents to a single file, dispatching on the same conditions
// as vinyl-fs: metadata-only, directory, symbolic link, buffer, then stream.
func (r *contentsReader) read(f *vinyl.File) error {
	if !r.opts.Read.Resolve(f, true) {
		return nil
	}
	// Directories have no contents to read, but must not be treated as an
	// error; dest() recreates them as directories.
	if f.IsDirectory() {
		return nil
	}
	// A symbolic link that was not resolved is described by its target rather
	// than by the bytes it points at.
	if f.Stat != nil && f.Stat.IsSymbolic() {
		target, err := os.Readlink(f.Path())
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", f.Path(), err)
		}
		f.SetSymlink(target)
		return nil
	}
	if r.opts.Buffer.Resolve(f, true) {
		return r.readBuffer(f)
	}
	return r.readStream(f)
}

// readBuffer loads the whole file, transcodes it to UTF-8 and strips a BOM.
func (r *contentsReader) readBuffer(f *vinyl.File) error {
	c, err := r.codecFor(f)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(f.Path())
	if err != nil {
		return fmt.Errorf("reading %s: %w", f.Path(), err)
	}
	decoded, err := c.decode(raw)
	if err != nil {
		return fmt.Errorf("reading %s: %w", f.Path(), err)
	}
	if c != nil && r.opts.RemoveBOM.Resolve(f, true) {
		decoded = removeUTF8BOM(decoded)
	}
	f.Contents = vinyl.Buffer(decoded)
	return nil
}

// readStream attaches a lazily opened stream so the file is not touched until
// something actually reads it. This is the port of vinyl-fs's use of
// `lazystream`.
func (r *contentsReader) readStream(f *vinyl.File) error {
	c, err := r.codecFor(f)
	if err != nil {
		return err
	}
	stripBOM := c != nil && r.opts.RemoveBOM.Resolve(f, true)
	path := f.Path()

	f.Contents = vinyl.NewStream(func() (io.ReadCloser, error) {
		handle, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var reader io.Reader = handle
		reader = c.decodeReader(reader)
		if stripBOM {
			reader = newBOMStripReader(reader)
		}
		return readCloser{Reader: reader, Closer: handle}, nil
	})
	return nil
}

// sourceMapReader parses and detaches inline or external source maps.
type sourceMapReader struct {
	opts SrcOptions
}

func (s sourceMapReader) Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	for {
		f, ok, err := pipeline.Recv(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if s.opts.SourceMaps.Resolve(f, false) {
			if err := sourcemap.Add(f); err != nil {
				return err
			}
		}
		if err := pipeline.Send(ctx, out, f); err != nil {
			return err
		}
	}
}

// readCloser pairs a decorated reader with the handle that must be closed.
type readCloser struct {
	io.Reader
	io.Closer
}
