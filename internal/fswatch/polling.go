package fswatch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// snapshot is the subset of fs.FileInfo the poller compares between ticks.
type snapshot struct {
	mode    os.FileMode
	size    int64
	modTime time.Time
	isDir   bool
}

// changedFrom reports whether the entry differs in a way that warrants an
// event, and which op describes the difference.
//
// Content changes take priority over metadata changes so that a write that
// also happens to touch the mode is reported as a write, matching what the
// native backends do.
func (s snapshot) changedFrom(prev snapshot) (Op, bool) {
	switch {
	case s.size != prev.size || !s.modTime.Equal(prev.modTime):
		return OpWrite, true
	case s.mode != prev.mode:
		return OpChmod, true
	default:
		return 0, false
	}
}

// pollWatcher re-stats the watched trees on an interval and reports the diff.
//
// This is chokidar's usePolling mode. It is slower and noisier than the native
// backend but it is the only thing that works on filesystems that do not
// deliver notifications, such as most network mounts and some container bind
// mounts.
type pollWatcher struct {
	cfg      Config
	interval time.Duration
	events   chan Event
	errs     chan error

	done     chan struct{}
	closeOne sync.Once
	wg       sync.WaitGroup

	mu    sync.Mutex
	roots map[string]struct{}
	state map[string]snapshot
	// primed is false until the first scan has established a baseline, so the
	// initial contents of a tree are not reported as a burst of creations.
	primed bool
}

// NewPolling creates a Watcher that discovers changes by stat'ing on an
// interval.
func NewPolling(cfg Config) (Watcher, error) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	w := &pollWatcher{
		cfg:      cfg,
		interval: interval,
		events:   make(chan Event, eventBuffer),
		errs:     make(chan error, eventBuffer),
		done:     make(chan struct{}),
		roots:    make(map[string]struct{}),
		state:    make(map[string]snapshot),
	}
	w.wg.Add(1)
	go w.loop()
	return w, nil
}

func (w *pollWatcher) Events() <-chan Event { return w.events }
func (w *pollWatcher) Errors() <-chan error { return w.errs }

// Add registers a root and immediately records a baseline for it, so that
// pre-existing entries are not reported as creations on the first tick.
func (w *pollWatcher) Add(root string) error {
	select {
	case <-w.done:
		return ErrClosed
	default:
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("fswatch: %w", err)
	}

	dir := abs
	if info, statErr := os.Lstat(abs); statErr != nil || !info.IsDir() {
		dir = existingAncestor(filepath.Dir(abs))
	}

	w.mu.Lock()
	w.roots[dir] = struct{}{}
	w.mu.Unlock()

	w.scan(false)
	return nil
}

// Remove stops watching a root and discards everything remembered beneath it.
func (w *pollWatcher) Remove(root string) error {
	select {
	case <-w.done:
		return ErrClosed
	default:
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("fswatch: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for dir := range w.roots {
		if dir == abs || isWithin(abs, dir) {
			delete(w.roots, dir)
		}
	}
	for path := range w.state {
		if path == abs || isWithin(abs, path) {
			delete(w.state, path)
		}
	}
	return nil
}

// Close stops polling and closes the channels once the loop has exited.
func (w *pollWatcher) Close() error {
	w.closeOne.Do(func() {
		close(w.done)
		w.wg.Wait()
	})
	return nil
}

func (w *pollWatcher) loop() {
	defer w.wg.Done()
	// Not closed here, for the reason given in notify.go's loop: sends can
	// originate on a caller's goroutine, and closing under them panics.

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.scan(true)
		}
	}
}

// scan walks every root and diffs the result against the previous snapshot.
// When emit is false the snapshot is recorded silently, which is how Add
// establishes a baseline.
func (w *pollWatcher) scan(emit bool) {
	w.mu.Lock()
	roots := make([]string, 0, len(w.roots))
	for dir := range w.roots {
		roots = append(roots, dir)
	}
	primed := w.primed
	w.mu.Unlock()

	// Deterministic root order keeps event ordering stable across ticks, which
	// matters for tests that assert on the first event they see.
	sort.Strings(roots)

	current := make(map[string]snapshot)
	for _, dir := range roots {
		w.walk(dir, 0, current)
	}

	// Invariant: w.state is only ever read by the goroutine that has swapped
	// it out. Aliasing it before the walk instead would let Remove delete
	// from the same map that diff is ranging over. Discarding paths whose
	// root is gone belongs in the same critical section, since a root can
	// stop being watched while the walk above is still running.
	w.mu.Lock()
	for path := range current {
		if !w.watchedLocked(path) {
			delete(current, path)
		}
	}
	previous := w.state
	w.state = current
	if !primed {
		w.primed = true
	}
	var events []Event
	if emit && primed {
		events = w.diff(previous, current)
	}
	w.mu.Unlock()

	// Emitting blocks on the event channel, so it happens after the unlock.
	for _, ev := range events {
		w.emit(ev)
	}
}

// watchedLocked reports whether path still falls under a registered root. The
// caller must hold w.mu.
func (w *pollWatcher) watchedLocked(path string) bool {
	for dir := range w.roots {
		if path == dir || isWithin(dir, path) {
			return true
		}
	}
	return false
}

// diff returns the events implied by moving from previous to current, in the
// order they should be delivered. The caller must hold w.mu: current is the
// live w.state, which Remove may prune at any time.
func (w *pollWatcher) diff(previous, current map[string]snapshot) []Event {
	added := make([]Event, 0)
	changed := make([]Event, 0)
	for path, now := range current {
		before, existed := previous[path]
		if !existed {
			added = append(added, Event{Path: path, Op: OpCreate, IsDir: now.isDir})
			continue
		}
		if op, ok := now.changedFrom(before); ok {
			changed = append(changed, Event{Path: path, Op: op, IsDir: now.isDir})
		}
	}

	removed := make([]Event, 0)
	for path, before := range previous {
		if _, ok := current[path]; !ok {
			removed = append(removed, Event{Path: path, Op: OpRemove, IsDir: before.isDir})
		}
	}

	// Shallow paths first, so that a directory's creation precedes the
	// creation of the files inside it.
	byPath := func(events []Event) func(i, j int) bool {
		return func(i, j int) bool { return events[i].Path < events[j].Path }
	}
	sort.Slice(added, byPath(added))
	sort.Slice(changed, byPath(changed))
	// Removals go the other way: children before the directory that held them.
	sort.Slice(removed, func(i, j int) bool { return removed[i].Path > removed[j].Path })

	return append(append(added, changed...), removed...)
}

// walk records dir and its descendants into out.
func (w *pollWatcher) walk(dir string, depth int, out map[string]snapshot) {
	if !depthAllows(w.cfg.Depth, depth) {
		return
	}
	if w.cfg.Filter != nil && !w.cfg.Filter(dir, true) {
		return
	}

	info, err := os.Lstat(dir)
	if err != nil {
		if !os.IsNotExist(err) && !(w.cfg.IgnorePermissionErrors && ignorablePermissionError(err)) {
			w.emitError(fmt.Errorf("fswatch: %w", err))
		}
		return
	}
	out[dir] = snapshotOf(info)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if !(w.cfg.IgnorePermissionErrors && ignorablePermissionError(err)) {
			w.emitError(fmt.Errorf("fswatch: %w", err))
		}
		return
	}

	for _, entry := range entries {
		child := filepath.Join(dir, entry.Name())
		childInfo, err := entry.Info()
		if err != nil {
			continue
		}

		isDir := entry.IsDir()
		if entry.Type()&fs.ModeSymlink != 0 {
			if !w.cfg.FollowSymlinks {
				isDir = false
			} else if target, terr := os.Stat(child); terr == nil {
				isDir = target.IsDir()
				childInfo = target
			} else {
				isDir = false
			}
		}

		if isDir {
			w.walk(child, depth+1, out)
			continue
		}
		if w.cfg.Filter != nil && !w.cfg.Filter(child, false) {
			continue
		}
		out[child] = snapshotOf(childInfo)
	}
}

func snapshotOf(info fs.FileInfo) snapshot {
	return snapshot{
		mode:    info.Mode(),
		size:    info.Size(),
		modTime: info.ModTime(),
		isDir:   info.IsDir(),
	}
}

func (w *pollWatcher) emit(ev Event) {
	if w.cfg.Filter != nil && !w.cfg.Filter(ev.Path, ev.IsDir) {
		return
	}
	select {
	case w.events <- ev:
	case <-w.done:
	}
}

func (w *pollWatcher) emitError(err error) {
	if err == nil {
		return
	}
	select {
	case w.errs <- err:
	case <-w.done:
	default:
	}
}
