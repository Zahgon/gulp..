package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// ErrInvalidSymlinkFolder is returned when [Symlink] is given an empty
// destination, worded exactly as in vinyl-fs.
var ErrInvalidSymlinkFolder = errors.New("Invalid symlink() folder argument. Please specify a non-empty string or a function.")

// SymlinkOptions configures [Symlink]. Every field is optional; the zero value
// selects the documented vinyl-fs default.
type SymlinkOptions struct {
	// Cwd is the directory a relative destination folder is resolved against.
	// Defaults to the process working directory.
	Cwd string

	// DirMode is the permission mode for directories created along the way.
	// Defaults to 0777 before umask.
	DirMode Option[os.FileMode]

	// Overwrite allows replacing an existing entry at the link path. Defaults
	// to true.
	Overwrite Option[bool]

	// RelativeSymlinks makes created links point at a path relative to the
	// destination instead of an absolute one. Defaults to false.
	RelativeSymlinks Option[bool]

	// UseJunctions requests NTFS junctions rather than directory symlinks on
	// Windows. Defaults to true. It has no effect on other platforms.
	//
	// Go's os.Symlink does not expose the link type, so on Windows this port
	// always creates the link kind that os.Symlink selects. See MIGRATION.md.
	UseJunctions Option[bool]
}

// Symlink returns a [pipeline.Transform] that creates, inside directory, a
// symbolic link back to each file's original location. It is the port of
// `vinyl-fs`'s `symlink()`.
//
// Each re-emitted file is modified to describe the link rather than the target:
// cwd, base and path point at the link, stat is re-read from it, contents are
// cleared, and the symlink property holds the original path.
//
//	vfs.Src([]string{"input/*.js"}, vfs.SrcOptions{}).
//		Pipe(vfs.Symlink("output", vfs.SymlinkOptions{})).
//		Run(ctx)
func Symlink(directory string, opts SymlinkOptions) pipeline.Transform {
	if directory == "" {
		return failedTransform{err: ErrInvalidSymlinkFolder}
	}
	return SymlinkWith(func(*vinyl.File) string { return directory }, opts)
}

// SymlinkWith is [Symlink] with the destination directory computed per file,
// the equivalent of passing a function as vinyl-fs's `symlink()` folder
// argument.
func SymlinkWith(directory func(*vinyl.File) string, opts SymlinkOptions) pipeline.Transform {
	if directory == nil {
		return failedTransform{err: ErrInvalidSymlinkFolder}
	}
	return &linker{directory: directory, opts: opts}
}

// linker implements vinyl-fs's symlink() stream.
type linker struct {
	directory func(*vinyl.File) string
	opts      SymlinkOptions
}

func (l *linker) Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	for {
		f, ok, err := pipeline.Recv(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := l.link(f); err != nil {
			return err
		}
		if err := pipeline.Send(ctx, out, f); err != nil {
			return err
		}
	}
}

// link creates the symbolic link for a single file.
func (l *linker) link(f *vinyl.File) error {
	// The target is the file's location before it is redirected at the output
	// directory, so it must be captured first.
	target := f.Path()

	cwd, err := resolveCwd(l.opts.Cwd)
	if err != nil {
		return err
	}

	folder := l.directory(f)
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
	linkPath := filepath.Join(base, relative)

	if err := f.SetCwd(cwd); err != nil {
		return err
	}
	if err := f.SetBase(base); err != nil {
		return err
	}
	if err := f.SetPath(linkPath); err != nil {
		return err
	}
	f.Contents = nil
	if err := f.SetSymlink(target); err != nil {
		return err
	}

	dirMode := l.opts.DirMode.Resolve(f, defaultDirMode)
	if err := os.MkdirAll(filepath.Dir(linkPath), dirMode); err != nil {
		return fmt.Errorf("creating directory %s: %w", filepath.Dir(linkPath), err)
	}

	// Relative links are computed against the output directory so that the pair
	// stays valid if the tree is moved. Both sides are made absolute first
	// because filepath.Rel refuses to mix the two forms, whereas Node's
	// path.relative resolves each against the cwd and so always answers.
	linkTarget := target
	if l.opts.RelativeSymlinks.Resolve(f, false) {
		if rel, err := filepath.Rel(absoluteFrom(f.Cwd(), base), absoluteFrom(f.Cwd(), target)); err == nil {
			linkTarget = rel
			if err := f.SetSymlink(rel); err != nil {
				return err
			}
		}
	}

	if err := l.create(f, linkTarget, linkPath); err != nil {
		return err
	}

	// The re-emitted stat must describe the link itself, never its target.
	if info, err := os.Lstat(linkPath); err == nil {
		f.Stat = vinyl.StatFromFileInfo(info)
	}
	return nil
}

// create makes the link, replacing an existing entry only when overwriting is
// enabled. An existing entry with overwriting disabled is not an error.
func (l *linker) create(f *vinyl.File, target, linkPath string) error {
	err := os.Symlink(target, linkPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("creating symlink %s: %w", linkPath, err)
	}
	if !l.opts.Overwrite.Resolve(f, true) {
		return nil
	}
	if err := os.Remove(linkPath); err != nil {
		return fmt.Errorf("replacing symlink %s: %w", linkPath, err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("creating symlink %s: %w", linkPath, err)
	}
	return nil
}

// absoluteFrom resolves path against cwd when it is not already absolute,
// reproducing how Node's path.relative anchors its arguments.
func absoluteFrom(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}
