// Package fswatch provides the low-level filesystem notification backends used
// by the watch package.
//
// It replaces the parts of chokidar that gulp actually depends on: recursive
// directory watching with an optional polling fallback. chokidar is a large
// package with a lot of history; only the behaviour reachable through
// gulp.watch is reproduced here.
//
// Two backends are provided:
//
//   - NewNotify uses github.com/fsnotify/fsnotify (inotify on Linux, kqueue on
//     the BSDs and macOS, ReadDirectoryChangesW on Windows). fsnotify watches a
//     single directory at a time, so this backend walks the tree and registers
//     every directory, adding newly created directories as it sees them. That
//     mirrors chokidar's non-FSEvents mode.
//   - NewPolling stats the tree on an interval and diffs the result. It is what
//     chokidar's usePolling:true does, and it is the only thing that works over
//     network filesystems.
//
// Both backends emit the same Event values and are safe for concurrent use.
package fswatch

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Op describes what happened to a path.
type Op uint32

const (
	// OpCreate means the path appeared.
	OpCreate Op = 1 << iota
	// OpWrite means the path's contents changed.
	OpWrite
	// OpRemove means the path disappeared.
	OpRemove
	// OpRename means the path was renamed away. Backends report the old path.
	OpRename
	// OpChmod means the path's metadata changed.
	OpChmod
)

// Has reports whether op contains the given bit.
func (o Op) Has(other Op) bool { return o&other != 0 }

// String renders the op for diagnostics.
func (o Op) String() string {
	names := make([]string, 0, 5)
	for _, candidate := range []struct {
		op   Op
		name string
	}{
		{OpCreate, "create"},
		{OpWrite, "write"},
		{OpRemove, "remove"},
		{OpRename, "rename"},
		{OpChmod, "chmod"},
	} {
		if o.Has(candidate.op) {
			names = append(names, candidate.name)
		}
	}
	if len(names) == 0 {
		return "unknown"
	}
	return strings.Join(names, "|")
}

// Event is a single filesystem notification.
//
// Path is always absolute. IsDir is best-effort: for removals the entry is
// already gone, so backends report what they remember about it, and the notify
// backend reports false when it has no memory of the path.
type Event struct {
	Path  string
	Op    Op
	IsDir bool
}

// Config tunes a backend.
type Config struct {
	// Depth limits how far below each root the backend descends. A negative
	// value means unlimited. Zero watches only the root directory itself.
	Depth int

	// FollowSymlinks makes the backend descend into symlinked directories.
	FollowSymlinks bool

	// IgnorePermissionErrors suppresses EACCES/EPERM while walking instead of
	// surfacing them on the error channel.
	IgnorePermissionErrors bool

	// Filter, when non-nil, is consulted before a path is watched or reported.
	// Returning false prunes directories from the walk entirely, which is how
	// the watch package keeps `ignored` from costing anything.
	Filter func(path string, isDir bool) bool

	// Interval is the poll period for the polling backend. Defaults to 100ms.
	Interval time.Duration
}

// Watcher is the interface both backends satisfy.
type Watcher interface {
	// Events yields notifications until the watcher is closed, at which point
	// the channel is closed.
	Events() <-chan Event
	// Errors yields non-fatal errors encountered while watching.
	Errors() <-chan error
	// Add begins watching an absolute path. Directories are watched
	// recursively subject to Config.Depth.
	Add(root string) error
	// Remove stops watching a path previously passed to Add.
	Remove(root string) error
	// Close releases all resources. It is safe to call more than once.
	Close() error
}

// ErrClosed is returned by Add and Remove after Close.
var ErrClosed = errors.New("fswatch: watcher is closed")

const (
	// defaultInterval matches chokidar's `interval` default.
	defaultInterval = 100 * time.Millisecond
	// eventBuffer keeps a burst of filesystem activity from blocking the
	// backend's own goroutine while a consumer is busy running a task.
	eventBuffer = 256
)

// ignorablePermissionError reports whether err is the kind of thing
// IgnorePermissionErrors is meant to swallow.
func ignorablePermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission)
}

// depthAllows reports whether an entry `depth` levels below a root may be
// watched given the configured limit.
func depthAllows(limit, depth int) bool {
	return limit < 0 || depth <= limit
}
