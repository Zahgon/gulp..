//go:build !darwin && !linux

package vinyl

import (
	"io/fs"
	"time"
)

// accessTime returns the zero time on platforms where the access time is not
// portably reachable. StatFromFileInfo falls back to the modification time,
// which matches how dest() behaves in the JS implementation on Windows (where
// futimes-based metadata syncing is disabled entirely).
func accessTime(fs.FileInfo) time.Time { return time.Time{} }
