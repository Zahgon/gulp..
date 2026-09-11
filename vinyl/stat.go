// Package vinyl implements the virtual file object used throughout gulp-go.
//
// It is the Go port of the JavaScript `vinyl` package
// (https://github.com/gulpjs/vinyl). A vinyl File is a metadata object that
// describes a file: primarily its `path` and `contents`, plus the `cwd` and
// `base` used to compute a relative output path.
package vinyl

import (
	"io/fs"
	"os"
	"time"
)

// Stat is the gulp-go analogue of Node's `fs.Stats`.
//
// The JS implementation stores an `fs.Stats` instance on `file.stat` and uses
// it for three things: deciding whether a file is a directory or a symbolic
// link, and synchronising mode/mtime/atime onto files written by dest().
// Go's os.FileInfo exposes neither the access time nor the "was this path a
// symlink" bit in a portable way once a link has been resolved, so we keep an
// explicit struct instead.
//
// Stat implements fs.FileInfo so it can be used interchangeably with values
// returned by os.Stat.
type Stat struct {
	// FileMode carries the permission bits plus Go's type bits.
	FileMode os.FileMode
	// ByteSize is the file size in bytes.
	ByteSize int64
	// MTime is the modification time (fs.Stats#mtime).
	MTime time.Time
	// ATime is the access time (fs.Stats#atime). May be zero if unavailable.
	ATime time.Time
	// Dir reports whether the described path is a directory.
	Dir bool
	// Symlink reports whether the described path was itself a symbolic link.
	// This is retained separately because src() with resolveSymlinks:true
	// replaces the link's stat with the target's stat.
	Symlink bool
	// UID is the owning user id, or InvalidID when the platform does not
	// report one. dest() copies it onto the written file when it differs and
	// the process is allowed to make the change.
	UID int
	// GID is the owning group id, or InvalidID when unavailable.
	GID int
	// name is the base name, cached for the fs.FileInfo implementation.
	name string
}

// InvalidID marks an owner or group id as unknown.
//
// vinyl-fs guards every ownership comparison with `isValidUnixId`, which
// rejects anything that is not a non-negative number, because Windows reports
// no ids at all. A negative sentinel gives Go the same three-state answer
// without a pointer.
const InvalidID = -1

// ValidID reports whether id is a usable Unix owner or group id, the port of
// vinyl-fs's isValidUnixId.
func ValidID(id int) bool { return id >= 0 }

// Name implements fs.FileInfo.
func (s *Stat) Name() string { return s.name }

// Size implements fs.FileInfo.
func (s *Stat) Size() int64 { return s.ByteSize }

// Mode implements fs.FileInfo.
func (s *Stat) Mode() os.FileMode { return s.FileMode }

// ModTime implements fs.FileInfo.
func (s *Stat) ModTime() time.Time { return s.MTime }

// IsDir implements fs.FileInfo. Mirrors fs.Stats#isDirectory().
func (s *Stat) IsDir() bool { return s.Dir }

// Sys implements fs.FileInfo.
func (s *Stat) Sys() any { return nil }

// IsSymbolic mirrors fs.Stats#isSymbolicLink().
func (s *Stat) IsSymbolic() bool { return s.Symlink }

// Perm returns just the permission bits, which is what JS `stat.mode & 0o777`
// yields after masking off the file-type bits.
func (s *Stat) Perm() os.FileMode { return s.FileMode.Perm() }

// Clone returns a deep copy of the Stat, or nil if s is nil.
func (s *Stat) Clone() *Stat {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

// StatFromFileInfo converts an fs.FileInfo (typically from os.Stat or
// os.Lstat) into a Stat, filling in the access time where the platform
// exposes it.
func StatFromFileInfo(fi fs.FileInfo) *Stat {
	if fi == nil {
		return nil
	}
	if s, ok := fi.(*Stat); ok {
		return s.Clone()
	}
	st := &Stat{
		FileMode: fi.Mode(),
		ByteSize: fi.Size(),
		MTime:    fi.ModTime(),
		Dir:      fi.IsDir(),
		Symlink:  fi.Mode()&os.ModeSymlink != 0,
		UID:      InvalidID,
		GID:      InvalidID,
		name:     fi.Name(),
	}
	if uid, gid, ok := ownerOf(fi); ok {
		st.UID, st.GID = uid, gid
	}
	st.ATime = accessTime(fi)
	if st.ATime.IsZero() {
		st.ATime = st.MTime
	}
	return st
}
