// Package watch is the Go port of glob-watcher, the module behind gulp.watch.
//
// It layers glob matching, event filtering, debouncing and run-queueing on top
// of the raw filesystem notifications produced by internal/fswatch, which
// stands in for chokidar.
//
// The defaults deliberately match glob-watcher rather than chokidar:
// IgnoreInitial is true and Delay is 200ms, both of which chokidar leaves off.
package watch

import (
	"time"
)

// EventKind identifies a normalized filesystem event.
//
// These are chokidar's event names, kept verbatim so that gulpfiles ported
// from JavaScript read the same.
type EventKind string

const (
	// EventAdd fires when a file appears.
	EventAdd EventKind = "add"
	// EventChange fires when a file's contents or metadata change.
	EventChange EventKind = "change"
	// EventUnlink fires when a file disappears.
	EventUnlink EventKind = "unlink"
	// EventAddDir fires when a directory appears.
	EventAddDir EventKind = "addDir"
	// EventUnlinkDir fires when a directory disappears.
	EventUnlinkDir EventKind = "unlinkDir"
	// EventAll is not emitted; listing it in Options.Events subscribes the
	// task to every event kind, as chokidar's 'all' does.
	EventAll EventKind = "all"
)

// DefaultEvents is what gulp.watch listens for when Options.Events is empty.
var DefaultEvents = []EventKind{EventAdd, EventChange, EventUnlink}

const (
	// DefaultDelay is glob-watcher's `delay`. Changes are collected for this
	// long before the task runs, so a save-all in an editor produces one run.
	DefaultDelay = 200 * time.Millisecond
	// DefaultAtomic is chokidar's `atomic`. Editors that save by writing a
	// temporary file and renaming it over the original produce an unlink
	// immediately followed by an add; within this window the pair collapses
	// into a single change.
	DefaultAtomic = 100 * time.Millisecond
	// DefaultInterval is chokidar's `interval`, used when UsePolling is set.
	DefaultInterval = 100 * time.Millisecond
	// DefaultBinaryInterval is chokidar's `binaryInterval`.
	DefaultBinaryInterval = 300 * time.Millisecond
)

// Ptr returns a pointer to v.
//
// Several options default to true or to a non-zero duration, so their zero
// value cannot mean "unset". Those fields are pointers, and this helper keeps
// setting them to a literal readable:
//
//	watch.Options{IgnoreInitial: watch.Ptr(false)}
func Ptr[T any](v T) *T { return &v }

// Options mirrors the option object accepted by gulp.watch.
//
// Every field is optional. Fields whose documented default is true or non-zero
// are pointers; leave them nil to accept the default.
type Options struct {
	// Cwd is the directory relative globs are resolved against, and the
	// directory emitted paths are made relative to. Defaults to the process
	// working directory.
	Cwd string

	// IgnoreInitial suppresses the burst of add events describing the files
	// that already exist when watching starts. Defaults to true, which is the
	// opposite of chokidar's own default.
	IgnoreInitial *bool

	// Delay is how long to collect changes before running the task.
	// Defaults to DefaultDelay. A non-positive value runs immediately.
	Delay *time.Duration

	// Queue allows at most one run to be pending while another is in flight.
	// Defaults to true. When false, changes arriving during a run are dropped.
	Queue *bool

	// Events selects which event kinds trigger the task.
	// Defaults to DefaultEvents. Include EventAll to subscribe to everything.
	Events []EventKind

	// Persistent is accepted for parity with chokidar. A Go watcher runs on
	// its own goroutines and never keeps the process alive by itself, so this
	// option has no effect; keeping a program running is the caller's job.
	Persistent *bool

	// Ignored lists glob patterns that are never watched. Matching
	// directories are pruned from the walk, so ignoring node_modules is cheap.
	Ignored []string

	// FollowSymlinks descends into symlinked directories. Defaults to true.
	FollowSymlinks *bool

	// DisableGlobbing treats the watch paths as literal paths rather than
	// glob patterns, so a path containing glob characters can be watched.
	DisableGlobbing bool

	// UsePolling selects the stat-polling backend instead of native
	// notifications. Required on most network filesystems.
	UsePolling bool

	// Interval is the poll period when UsePolling is set.
	// Defaults to DefaultInterval.
	Interval *time.Duration

	// BinaryInterval is accepted for parity with chokidar, which polls binary
	// files less often than text files. The Go poller does not distinguish
	// between the two, so this option has no effect.
	BinaryInterval *time.Duration

	// AlwaysStat is accepted for parity with chokidar. The Go backends always
	// have stat information available where it is meaningful, so this option
	// has no effect.
	AlwaysStat bool

	// Depth limits how many directory levels below each watch root are
	// traversed. Zero watches only the root's immediate contents. Leave nil
	// for unlimited depth.
	Depth *int

	// AwaitWriteFinish delays the event for a file until its size has stopped
	// changing, so that a large or slow copy produces one event at the end
	// rather than a stream of partial writes. Leave nil to disable.
	AwaitWriteFinish *AwaitWriteFinish

	// IgnorePermissionErrors continues past directories that cannot be read
	// instead of surfacing the error.
	IgnorePermissionErrors bool

	// Atomic collapses an unlink immediately followed by an add of the same
	// path into a single change. Defaults to DefaultAtomic. A non-positive
	// value disables the behaviour.
	Atomic *time.Duration
}

// AwaitWriteFinish configures chokidar's awaitWriteFinish behaviour.
type AwaitWriteFinish struct {
	// StabilityThreshold is how long a file's size must hold steady before
	// the event is released. Defaults to 2s.
	StabilityThreshold time.Duration
	// PollInterval is how often the file size is sampled. Defaults to 100ms.
	PollInterval time.Duration
}

const (
	defaultStabilityThreshold = 2 * time.Second
	defaultPollInterval       = 100 * time.Millisecond
)

// resolved is Options with every default applied, so the rest of the package
// never has to reason about nil.
type resolved struct {
	cwd                    string
	ignoreInitial          bool
	delay                  time.Duration
	queue                  bool
	events                 map[EventKind]bool
	ignored                []string
	followSymlinks         bool
	disableGlobbing        bool
	usePolling             bool
	interval               time.Duration
	depth                  int
	awaitWriteFinish       *AwaitWriteFinish
	ignorePermissionErrors bool
	atomic                 time.Duration
}

// resolve applies defaults. cwd must already be absolute.
func (o Options) resolve(cwd string) resolved {
	r := resolved{
		cwd:                    cwd,
		ignoreInitial:          true,
		delay:                  DefaultDelay,
		queue:                  true,
		ignored:                o.Ignored,
		followSymlinks:         true,
		disableGlobbing:        o.DisableGlobbing,
		usePolling:             o.UsePolling,
		interval:               DefaultInterval,
		depth:                  -1,
		ignorePermissionErrors: o.IgnorePermissionErrors,
		atomic:                 DefaultAtomic,
	}

	if o.IgnoreInitial != nil {
		r.ignoreInitial = *o.IgnoreInitial
	}
	if o.Delay != nil {
		r.delay = *o.Delay
	}
	if o.Queue != nil {
		r.queue = *o.Queue
	}
	if o.FollowSymlinks != nil {
		r.followSymlinks = *o.FollowSymlinks
	}
	if o.Interval != nil && *o.Interval > 0 {
		r.interval = *o.Interval
	}
	if o.Depth != nil {
		r.depth = *o.Depth
	}
	if o.Atomic != nil {
		r.atomic = *o.Atomic
	}

	if o.AwaitWriteFinish != nil {
		awf := *o.AwaitWriteFinish
		if awf.StabilityThreshold <= 0 {
			awf.StabilityThreshold = defaultStabilityThreshold
		}
		if awf.PollInterval <= 0 {
			awf.PollInterval = defaultPollInterval
		}
		r.awaitWriteFinish = &awf
	}

	kinds := o.Events
	if len(kinds) == 0 {
		kinds = DefaultEvents
	}
	r.events = make(map[EventKind]bool, len(kinds))
	for _, kind := range kinds {
		if kind == EventAll {
			for _, all := range []EventKind{EventAdd, EventChange, EventUnlink, EventAddDir, EventUnlinkDir} {
				r.events[all] = true
			}
			continue
		}
		r.events[kind] = true
	}

	return r
}

// wants reports whether an event kind should trigger the task.
func (r resolved) wants(kind EventKind) bool { return r.events[kind] }
