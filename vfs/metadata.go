package vfs

import (
	"os"
	"time"

	"github.com/gulpjs/gulp-go/vinyl"
)

// modeDiff reports the permission bits that need changing, and whether any do.
//
// vinyl-fs compares `fsMode & 0o7777` with `vinylMode & 0o7777`, so the setuid,
// setgid and sticky bits participate alongside the ordinary rwx triples.
func modeDiff(current, wanted os.FileMode) (os.FileMode, bool) {
	const mask = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	a, b := current&mask, wanted&mask
	if a == b {
		return 0, false
	}
	return b, true
}

// timesDiff reports the timestamps to apply, and whether they need applying.
//
// The vinyl's mtime wins when it is set and differs from what is on disk; the
// atime falls back to the file's current atime, which is what vinyl-fs does
// when a vinyl carries no access time of its own.
func timesDiff(current, wanted *vinyl.Stat) (atime, mtime time.Time, ok bool) {
	if wanted == nil || wanted.MTime.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	if current != nil && !current.MTime.IsZero() && wanted.MTime.Equal(current.MTime) {
		return time.Time{}, time.Time{}, false
	}
	atime = wanted.ATime
	if atime.IsZero() {
		if current != nil && !current.ATime.IsZero() {
			atime = current.ATime
		} else {
			atime = time.Now()
		}
	}
	return atime, wanted.MTime, true
}

// ownerDiff reports the owner and group to apply, and whether they need
// applying. It is a direct port of vinyl-fs's getOwnerDiff: an id is only
// considered when both sides can express it, and a partial match still
// requires passing the id that is already correct to chown.
func ownerDiff(current, wanted *vinyl.Stat) (uid, gid int, ok bool) {
	if current == nil || wanted == nil {
		return vinyl.InvalidID, vinyl.InvalidID, false
	}
	if !vinyl.ValidID(wanted.UID) && !vinyl.ValidID(wanted.GID) {
		return vinyl.InvalidID, vinyl.InvalidID, false
	}
	if !vinyl.ValidID(current.UID) && !vinyl.ValidID(wanted.UID) {
		return vinyl.InvalidID, vinyl.InvalidID, false
	}
	if !vinyl.ValidID(current.GID) && !vinyl.ValidID(wanted.GID) {
		return vinyl.InvalidID, vinyl.InvalidID, false
	}

	uid, gid = current.UID, current.GID
	if vinyl.ValidID(wanted.UID) {
		uid = wanted.UID
	}
	if vinyl.ValidID(wanted.GID) {
		gid = wanted.GID
	}
	if uid == current.UID && gid == current.GID {
		return vinyl.InvalidID, vinyl.InvalidID, false
	}
	return uid, gid, true
}

// updateMetadata synchronises the written file's mode, timestamps and owner
// with the vinyl's stat, then replaces the vinyl's stat with what is actually
// on disk.
//
// handle is the still-open destination, so the changes go through fchmod,
// futimes and fchown rather than through the path, exactly as vinyl-fs does;
// it is nil for directories, which are adjusted by path instead. Every step is
// best effort: vinyl-fs swallows these errors too, because a build that copied
// a file correctly should not fail merely because it could not also copy the
// file's ownership.
func (w *writer) updateMetadata(handle *os.File, f *vinyl.File) {
	path := f.Path()

	current := statOf(handle, path)
	if current == nil {
		return
	}

	// isOwner gates all three adjustments, not just the ownership one: a
	// process that does not own the file cannot chmod or utime it either, and
	// vinyl-fs returns early on the same condition.
	if f.Stat != nil && isOwner(current) {
		if perm, ok := modeDiff(current.FileMode, f.Stat.FileMode); ok {
			_ = chmodTarget(handle, path, perm)
		}
		if atime, mtime, ok := timesDiff(current, f.Stat); ok {
			_ = chtimesTarget(handle, path, atime, mtime)
		}
		if uid, gid, ok := ownerDiff(current, f.Stat); ok {
			_ = chownTarget(handle, path, uid, gid)
		}
	}

	if refreshed := statOf(handle, path); refreshed != nil {
		f.Stat = refreshed
	}
}

// statOf reads the destination's current metadata, preferring the open handle
// so that the values describe the file that was actually written.
func statOf(handle *os.File, path string) *vinyl.Stat {
	if handle != nil {
		if info, err := handle.Stat(); err == nil {
			return vinyl.StatFromFileInfo(info)
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	return vinyl.StatFromFileInfo(info)
}
