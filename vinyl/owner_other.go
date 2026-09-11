//go:build !unix

package vinyl

import "io/fs"

// ownerOf reports no owner on platforms without Unix ids, which is how
// vinyl-fs treats Windows: `process.getuid` is undefined there, so ownership
// is never compared and never synchronised.
func ownerOf(fs.FileInfo) (int, int, bool) { return InvalidID, InvalidID, false }
