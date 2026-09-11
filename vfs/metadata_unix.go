//go:build unix

package vfs

import (
	"os"
	"time"

	"github.com/gulpjs/gulp-go/vinyl"
	"golang.org/x/sys/unix"
)

// isOwner reports whether this process may change the file's metadata.
//
// vinyl-fs refuses to touch mode, times or ownership unless the effective uid
// owns the file, or the process is root. Without that guard a build running as
// one user over another user's tree would fail on every write.
func isOwner(current *vinyl.Stat) bool {
	euid := os.Geteuid()
	if euid < 0 {
		return false
	}
	if euid == 0 {
		return true
	}
	return current != nil && current.UID == euid
}

func chmodTarget(handle *os.File, path string, perm os.FileMode) error {
	if handle != nil {
		return handle.Chmod(perm)
	}
	return os.Chmod(path, perm)
}

func chownTarget(handle *os.File, path string, uid, gid int) error {
	if handle != nil {
		return handle.Chown(uid, gid)
	}
	return os.Lchown(path, uid, gid)
}

func chtimesTarget(handle *os.File, path string, atime, mtime time.Time) error {
	if handle == nil {
		return os.Chtimes(path, atime, mtime)
	}
	return unix.Futimes(int(handle.Fd()), []unix.Timeval{
		unix.NsecToTimeval(atime.UnixNano()),
		unix.NsecToTimeval(mtime.UnixNano()),
	})
}
