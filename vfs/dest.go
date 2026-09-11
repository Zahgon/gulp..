package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gulpjs/gulp-go/internal/sourcemap"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Errors reported by [Dest] and [Symlink], worded exactly as in vinyl-fs so
// that existing build output and documentation stay accurate.
var (
	// ErrInvalidDestFolder is returned when the destination is empty.
	ErrInvalidDestFolder = errors.New("Invalid dest() folder argument. Please specify a non-empty string or a function.")

	// ErrInvalidOutputFolder is returned when a destination function yields an
	// empty path for some file.
	ErrInvalidOutputFolder = errors.New("Invalid output folder")

	// ErrMissingSymlink is returned when a file is marked symbolic but carries
	// no target.
	ErrMissingSymlink = errors.New("Missing symlink property on symbolic vinyl")
)

// defaultFileMode and defaultDirMode mirror Node's defaults for fs.writeFile
// and fs.mkdir; the process umask is applied by the operating system.
const (
	defaultFileMode os.FileMode = 0o666
	defaultDirMode  os.FileMode = 0o777
)

// DestOptions configures [Dest]. Every field is optional; the zero value
// selects the documented vinyl-fs default.
type DestOptions struct {
	// Cwd is the directory a relative destination folder is resolved against.
	// Defaults to the process working directory.
	Cwd string

	// Mode is the permission mode for written files. Defaults to the mode
	// recorded on the incoming file's stat, so permissions survive a copy.
	Mode Option[os.FileMode]

	// DirMode is the permission mode for directories created along the way.
	// Defaults to 0777 before umask.
	DirMode Option[os.FileMode]

	// Overwrite allows replacing existing files. Defaults to true. When false,
	// an existing destination is left untouched and is not an error.
	Overwrite Option[bool]

	// Append adds to the end of an existing file instead of replacing it.
	// Defaults to false.
	Append Option[bool]

	// SourceMaps enables writing source maps. Defaults to false. Combine with
	// SourceMapsPath to emit external ".map" files.
	SourceMaps Option[bool]

	// SourceMapsPath is the directory, relative to each file, where external
	// source maps are written. When empty the map is embedded in the file as a
	// base64 data URI.
	SourceMapsPath Option[string]

	// RelativeSymlinks makes created links point at a path relative to the
	// destination instead of an absolute one. Defaults to false.
	RelativeSymlinks Option[bool]

	// UseJunctions requests NTFS junctions rather than directory symlinks on
	// Windows. Defaults to true. It has no effect on other platforms.
	//
	// Go's os.Symlink does not expose the link type, so on Windows this port
	// always creates the link kind that os.Symlink selects. See MIGRATION.md.
	UseJunctions Option[bool]

	// Encoding names the character encoding to write contents in; contents are
	// held as UTF-8 and encoded on the way out. Defaults to [DefaultEncoding].
	// Use Value([vfs.EncodingDisabled]) to write raw bytes.
	Encoding Option[string]
}

// Dest returns a [pipeline.Transform] that writes each file to directory and
// re-emits it with cwd, base, path and stat updated to describe the copy on
// disk. It is the port of `vinyl-fs`'s `dest()`.
//
// The file's base is removed from its path and the remainder is appended to
// directory, which is what preserves directory structure across a build:
//
//	vfs.Src([]string{"src/**/*.js"}, vfs.SrcOptions{}).
//		Pipe(vfs.Dest("build", vfs.DestOptions{})).
//		Run(ctx)
//
// Files carrying no contents are passed through without being written, so
// src() with Read disabled followed by dest() produces nothing on disk.
// Directories are recreated, and files marked symbolic become links.
func Dest(directory string, opts DestOptions) pipeline.Transform {
	if directory == "" {
		return failedTransform{err: ErrInvalidDestFolder}
	}
	return DestWith(func(*vinyl.File) string { return directory }, opts)
}

// DestWith is [Dest] with the destination directory computed per file, the
// equivalent of passing a function as vinyl-fs's `dest()` folder argument.
func DestWith(directory func(*vinyl.File) string, opts DestOptions) pipeline.Transform {
	if directory == nil {
		return failedTransform{err: ErrInvalidDestFolder}
	}
	return &writer{directory: directory, opts: opts}
}

// writer implements the prepare, sourcemap and write-contents stages of
// vinyl-fs's dest() as a single transform.
type writer struct {
	directory func(*vinyl.File) string
	opts      DestOptions
	codecs    sync.Map // string -> *codec
}

func (w *writer) Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	for {
		f, ok, err := pipeline.Recv(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		if err := w.prepare(f); err != nil {
			return err
		}

		mapFile, err := w.writeSourceMap(f)
		if err != nil {
			return err
		}

		if err := w.write(f); err != nil {
			return err
		}
		if err := pipeline.Send(ctx, out, f); err != nil {
			return err
		}

		// The map file is written after its owner so that a consumer never
		// observes a map without the file it belongs to.
		if mapFile != nil {
			if err := w.prepare(mapFile); err != nil {
				return err
			}
			if err := w.write(mapFile); err != nil {
				return err
			}
			if err := pipeline.Send(ctx, out, mapFile); err != nil {
				return err
			}
		}
	}
}

// prepare rewrites the file's location to point at its destination, which must
// happen before anything is written because the write path, the symlink base
// and the source map URL are all derived from it.
func (w *writer) prepare(f *vinyl.File) error {
	cwd, err := resolveCwd(w.opts.Cwd)
	if err != nil {
		return err
	}

	folder := w.directory(f)
	if folder == "" {
		return ErrInvalidOutputFolder
	}

	relative, err := f.Relative()
	if err != nil {
		return err
	}

	base := folder
	if !filepath.IsAbs(base) {
		base = filepath.Join(cwd, base)
	}
	writePath := filepath.Join(base, relative)

	if err := f.SetCwd(cwd); err != nil {
		return err
	}
	if err := f.SetBase(base); err != nil {
		return err
	}
	if err := f.SetPath(writePath); err != nil {
		return err
	}

	// A symbolic file's mode describes the link, which cannot be changed, so
	// only regular files and directories get a mode.
	if !f.IsSymbolic() {
		mode := w.opts.Mode.Resolve(f, defaultModeFor(f))
		if f.Stat == nil {
			f.Stat = &vinyl.Stat{}
		}
		f.Stat.FileMode = applyType(f.Stat.FileMode, mode)
	}
	return nil
}

// writeSourceMap serialises the attached source map, returning the sibling
// ".map" file when one should be written alongside.
func (w *writer) writeSourceMap(f *vinyl.File) (*vinyl.File, error) {
	if !w.opts.SourceMaps.Resolve(f, false) {
		return nil, nil
	}
	return sourcemap.Write(f, w.opts.SourceMapsPath.Resolve(f, ""))
}

// write dispatches on the kind of file, in the same order as vinyl-fs.
func (w *writer) write(f *vinyl.File) error {
	switch {
	case f.IsSymbolic():
		return w.writeSymlink(f)
	case f.IsDirectory():
		return w.writeDir(f)
	case f.IsStream():
		return w.writeStream(f)
	case f.IsBuffer():
		return w.writeBuffer(f)
	default:
		// A file with no contents carries metadata only and is not written.
		return nil
	}
}

// writeDir recreates a directory and syncs its metadata.
func (w *writer) writeDir(f *vinyl.File) error {
	mode := defaultDirMode
	if f.Stat != nil && f.Stat.FileMode.Perm() != 0 {
		mode = f.Stat.FileMode.Perm()
	}
	if err := os.MkdirAll(f.Path(), mode); err != nil {
		return fmt.Errorf("creating directory %s: %w", f.Path(), err)
	}
	w.updateMetadata(nil, f)
	return nil
}

// writeBuffer writes buffered contents in one shot.
func (w *writer) writeBuffer(f *vinyl.File) error {
	c, err := w.codecFor(f)
	if err != nil {
		return err
	}
	raw, err := f.Bytes()
	if err != nil {
		return fmt.Errorf("writing %s: %w", f.Path(), err)
	}
	encoded, err := c.encode(raw)
	if err != nil {
		return fmt.Errorf("writing %s: %w", f.Path(), err)
	}

	handle, err := w.open(f)
	if err != nil {
		return err
	}
	if handle == nil {
		// The destination exists and overwriting is disabled.
		return nil
	}

	if _, err := handle.Write(encoded); err != nil {
		handle.Close()
		return fmt.Errorf("writing %s: %w", f.Path(), err)
	}
	w.updateMetadata(handle, f)
	if err := handle.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", f.Path(), err)
	}
	return nil
}

// writeStream copies streaming contents to disk and then re-arms the file with
// a fresh stream over the written copy, so that a later stage can still read
// the contents even though the original stream has been consumed.
func (w *writer) writeStream(f *vinyl.File) error {
	c, err := w.codecFor(f)
	if err != nil {
		return err
	}

	stream, ok := f.Contents.(*vinyl.Stream)
	if !ok {
		return fmt.Errorf("writing %s: unsupported stream contents", f.Path())
	}

	handle, err := w.open(f)
	if err != nil {
		return err
	}
	if handle == nil {
		return nil
	}

	copyErr := func() error {
		defer stream.Close()
		if _, err := io.Copy(handle, c.encodeReader(stream)); err != nil {
			return fmt.Errorf("writing %s: %w", f.Path(), err)
		}
		return nil
	}()
	if copyErr == nil {
		w.updateMetadata(handle, f)
	}
	closeErr := handle.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return fmt.Errorf("writing %s: %w", f.Path(), closeErr)
	}

	path := f.Path()
	decoder := c
	f.Contents = vinyl.NewStream(func() (io.ReadCloser, error) {
		reopened, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		return readCloser{Reader: decoder.decodeReader(reopened), Closer: reopened}, nil
	})
	return nil
}

// writeSymlink creates a link at the destination pointing at the file's
// symlink target.
func (w *writer) writeSymlink(f *vinyl.File) error {
	target := f.Symlink()
	if target == "" {
		return ErrMissingSymlink
	}

	if err := w.mkdirp(filepath.Dir(f.Path())); err != nil {
		return err
	}

	// Relative links are computed against the destination base, which prepare()
	// has already pointed at the output directory.
	if w.opts.RelativeSymlinks.Resolve(f, false) && filepath.IsAbs(target) {
		if rel, err := filepath.Rel(f.Base(), target); err == nil {
			target = rel
		}
	}

	err := os.Symlink(target, f.Path())
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("creating symlink %s: %w", f.Path(), err)
	}

	// The destination exists. Replacing it is only correct when overwriting is
	// enabled; otherwise the existing entry wins, without an error.
	if !w.opts.Overwrite.Resolve(f, true) {
		return nil
	}
	if err := os.Remove(f.Path()); err != nil {
		return fmt.Errorf("replacing symlink %s: %w", f.Path(), err)
	}
	if err := os.Symlink(target, f.Path()); err != nil {
		return fmt.Errorf("creating symlink %s: %w", f.Path(), err)
	}
	return nil
}

// open creates the parent directories and opens the destination with flags
// derived from the overwrite and append options.
//
// A nil handle with a nil error means "the destination already exists and
// overwriting is disabled", which vinyl-fs treats as success rather than as an
// error.
func (w *writer) open(f *vinyl.File) (*os.File, error) {
	if err := w.mkdirp(filepath.Dir(f.Path())); err != nil {
		return nil, err
	}

	overwrite := w.opts.Overwrite.Resolve(f, true)
	append_ := w.opts.Append.Resolve(f, false)

	flags := os.O_WRONLY | os.O_CREATE
	if append_ {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	if !overwrite {
		// O_EXCL turns "already there" into an error, which is then swallowed
		// below so that existing files are simply left alone.
		flags = flags&^os.O_TRUNC | os.O_EXCL
	}

	mode := defaultFileMode
	if f.Stat != nil && f.Stat.FileMode.Perm() != 0 {
		mode = f.Stat.FileMode.Perm()
	}

	handle, err := os.OpenFile(f.Path(), flags, mode)
	if err != nil {
		if !overwrite && errors.Is(err, os.ErrExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("writing %s: %w", f.Path(), err)
	}
	return handle, nil
}

// mkdirp creates dir and any missing parents using the dirMode option.
func (w *writer) mkdirp(dir string) error {
	mode := w.opts.DirMode.Resolve(nil, defaultDirMode)
	if err := os.MkdirAll(dir, mode); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	return nil
}

// codecFor resolves and caches the output encoding for a file.
func (w *writer) codecFor(f *vinyl.File) (*codec, error) {
	name := w.opts.Encoding.Resolve(f, DefaultEncoding)
	if cached, ok := w.codecs.Load(name); ok {
		c, _ := cached.(*codec)
		return c, nil
	}
	c, err := lookupEncoding(name)
	if err != nil {
		return nil, err
	}
	w.codecs.Store(name, c)
	return c, nil
}

// defaultModeFor returns the mode a file should be written with when the mode
// option is unset: the incoming file's own mode, so that copies preserve
// permissions, falling back to Node's default.
func defaultModeFor(f *vinyl.File) os.FileMode {
	if f.Stat != nil {
		if perm := f.Stat.FileMode.Perm(); perm != 0 {
			return perm
		}
	}
	if f.IsDirectory() {
		return defaultDirMode
	}
	return defaultFileMode
}

// applyType replaces the permission bits of mode while keeping its type bits,
// which distinguish directories and links from regular files.
func applyType(mode, perm os.FileMode) os.FileMode {
	return mode&^os.ModePerm | perm.Perm()
}

// resolveCwd turns a possibly empty or relative cwd into an absolute path.
func resolveCwd(cwd string) (string, error) {
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolving cwd: %w", err)
		}
		return wd, nil
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolving cwd %s: %w", cwd, err)
	}
	return abs, nil
}

// failedTransform reports a construction-time error when the pipeline runs,
// which keeps the Src/Dest builders free of error returns.
type failedTransform struct{ err error }

func (t failedTransform) Transform(context.Context, <-chan *vinyl.File, chan<- *vinyl.File) error {
	return t.err
}
