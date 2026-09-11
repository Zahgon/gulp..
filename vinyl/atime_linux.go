//go:build linux

package vinyl

import (
	"io/fs"
	"syscall"
	"time"
)

// accessTime extracts the atime from the platform stat structure.
func accessTime(fi fs.FileInfo) time.Time {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || sys == nil {
		return time.Time{}
	}
	return time.Unix(sys.Atim.Sec, sys.Atim.Nsec)
}
