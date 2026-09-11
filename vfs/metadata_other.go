//go:build !unix

package vfs

import (
	"os"
	"time"

	"github.com/gulpjs/gulp-go/vinyl"
)

// isOwner always reports false where there are no Unix ids.
//
// vinyl-fs reaches the same conclusion on Windows: its isOwner returns false
// when process.getuid is undefined, which disables metadata synchronisation
// entirely rather than attempting a call the platform cannot honour.
func isOwner(*vinyl.Stat) bool { return false }

func chmodTarget(handle *os.File, path string, perm os.FileMode) error {
	if handle != nil {
		return handle.Chmod(perm)
	}
	return os.Chmod(path, perm)
}

func chownTarget(*os.File, string, int, int) error { return nil }

func chtimesTarget(_ *os.File, path string, atime, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}
