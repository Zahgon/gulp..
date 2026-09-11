//go:build unix

package vinyl

import (
	"io/fs"
	"syscall"
)

// ownerOf extracts the numeric owner and group from the platform stat
// structure. The boolean is false when the platform does not supply them.
func ownerOf(fi fs.FileInfo) (int, int, bool) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || sys == nil {
		return InvalidID, InvalidID, false
	}
	return int(sys.Uid), int(sys.Gid), true
}
