package fswatch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// notifyWatcher is the fsnotify-backed implementation of Watcher.
//
// fsnotify registers interest in one directory at a time and does not recurse,
// so this type maintains the recursion itself:
//
//   - Add walks the tree and registers every directory within the depth limit.
//   - When a directory is created the watch is extended to it, and the
//     directory is walked so that entries created between the mkdir and our
//     registration are still reported. Without that walk, `mkdir -p a/b/c`
//     reliably loses events.
//   - A memory of every path seen is kept so that removals, which cannot be
//     stat'ed, can still report whether they were directories.
type notifyWatcher struct {
	inner  *fsnotify.Watcher
	cfg    Config
	events chan Event
	errs   chan error

	done     chan struct{}
	closeOne sync.Once
	closeErr error

	mu sync.Mutex
	// roots maps an added root to the directory actually registered for it.
	roots map[string]string
	// watched maps a registered directory to its depth below its root.
	watched map[string]int
	// known remembers whether a path was a directory, for removal events.
	known map[string]bool
}

// NewNotify creates a Watcher backed by the operating system's native
// notification API.
func NewNotify(cfg Config) (Watcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fswatch: %w", err)
	}
	w := &notifyWatcher{
		inner:   inner,
		cfg:     cfg,
		events:  make(chan Event, eventBuffer),
		errs:    make(chan error, eventBuffer),
		done:    make(chan struct{}),
		roots:   make(map[string]string),
		watched: make(map[string]int),
		known:   make(map[string]bool),
	}
	go w.loop()
	return w, nil
}

func (w *notifyWatcher) Events() <-chan Event { return w.events }
func (w *notifyWatcher) Errors() <-chan error { return w.errs }

// Add registers a root. A file root is handled by watching its parent
// directory, because a file that does not exist yet cannot be watched and
// gulp.watch is routinely pointed at paths that are about to be created.
func (w *notifyWatcher) Add(root string) error {
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
		// Either the path does not exist yet or it is not a directory; watch
		// the closest existing ancestor so creation is observed.
		dir = existingAncestor(filepath.Dir(abs))
	}

	w.mu.Lock()
	w.roots[abs] = dir
	w.mu.Unlock()

	return w.watchTree(dir, 0)
}

// Remove stops watching a root and everything registered beneath it.
func (w *notifyWatcher) Remove(root string) error {
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
	dir, ok := w.roots[abs]
	delete(w.roots, abs)
	var stale []string
	if ok {
		for path := range w.watched {
			if path == dir || isWithin(dir, path) {
				stale = append(stale, path)
			}
		}
		for _, path := range stale {
			delete(w.watched, path)
		}
	}
	w.mu.Unlock()

	for _, path := range stale {
		// A directory that has already vanished was unregistered by the kernel.
		_ = w.inner.Remove(path)
	}
	return nil
}

// Close stops the watcher. The event and error channels are closed by the
// event loop once fsnotify has shut down.
func (w *notifyWatcher) Close() error {
	w.closeOne.Do(func() {
		close(w.done)
		w.closeErr = w.inner.Close()
	})
	return w.closeErr
}

// watchTree registers dir and, subject to the depth limit and filter, every
// directory beneath it.
func (w *notifyWatcher) watchTree(dir string, depth int) error {
	if !depthAllows(w.cfg.Depth, depth) {
		return nil
	}
	if w.cfg.Filter != nil && !w.cfg.Filter(dir, true) {
		return nil
	}

	if err := w.register(dir, depth); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if w.cfg.IgnorePermissionErrors && ignorablePermissionError(err) {
			return nil
		}
		return fmt.Errorf("fswatch: %w", err)
	}

	for _, entry := range entries {
		child := filepath.Join(dir, entry.Name())
		isDir, err := w.classify(child, entry)
		if err != nil {
			continue
		}
		w.remember(child, isDir)
		if !isDir {
			continue
		}
		if err := w.watchTree(child, depth+1); err != nil {
			w.emitError(err)
		}
	}
	return nil
}

// classify resolves whether an entry should be treated as a directory,
// following symlinks only when configured to.
func (w *notifyWatcher) classify(path string, entry fs.DirEntry) (bool, error) {
	if entry.Type()&fs.ModeSymlink == 0 {
		return entry.IsDir(), nil
	}
	if !w.cfg.FollowSymlinks {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		// A dangling symlink is not an error; it simply is not a directory.
		return false, nil //nolint:nilerr // dangling links are expected
	}
	return info.IsDir(), nil
}

// register adds a single directory to the underlying watcher, skipping
// directories already registered.
func (w *notifyWatcher) register(dir string, depth int) error {
	w.mu.Lock()
	if _, ok := w.watched[dir]; ok {
		w.mu.Unlock()
		return nil
	}
	w.watched[dir] = depth
	w.known[dir] = true
	w.mu.Unlock()

	if err := w.inner.Add(dir); err != nil {
		w.mu.Lock()
		delete(w.watched, dir)
		w.mu.Unlock()
		if w.cfg.IgnorePermissionErrors && ignorablePermissionError(err) {
			return nil
		}
		return fmt.Errorf("fswatch: watch %s: %w", dir, err)
	}
	return nil
}

// depthOf reports the recorded depth of a registered directory.
func (w *notifyWatcher) depthOf(dir string) (int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	depth, ok := w.watched[dir]
	return depth, ok
}

func (w *notifyWatcher) remember(path string, isDir bool) {
	w.mu.Lock()
	w.known[path] = isDir
	w.mu.Unlock()
}

func (w *notifyWatcher) forget(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	isDir := w.known[path]
	delete(w.known, path)
	delete(w.watched, path)
	return isDir
}

// loop translates fsnotify events into Events until the watcher is closed.
func (w *notifyWatcher) loop() {
	// The channels are deliberately never closed. emitError is reached from
	// Add, register and watchTree, which run on the caller's goroutine, so
	// closing here would race a send in progress and eventually panic with
	// "send on closed channel". Shutdown is signalled through w.done, which
	// Close closes and which every send already selects on.

	for {
		select {
		case <-w.done:
			// Drain whatever fsnotify has already queued so a change made
			// immediately before Close is not silently dropped.
			return
		case err, ok := <-w.inner.Errors:
			if !ok {
				return
			}
			if err != nil {
				w.emitError(fmt.Errorf("fswatch: %w", err))
			}
		case ev, ok := <-w.inner.Events:
			if !ok {
				return
			}
			w.handle(ev)
		}
	}
}

// handle converts one fsnotify event, extending or trimming the watch set as
// directories come and go.
func (w *notifyWatcher) handle(ev fsnotify.Event) {
	path := ev.Name
	if path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	switch {
	case ev.Op.Has(fsnotify.Create):
		info, err := os.Lstat(path)
		if err != nil {
			// Created and removed again before we looked. Report the create so
			// that a subsequent remove is not orphaned.
			w.emit(Event{Path: path, Op: OpCreate})
			return
		}
		isDir := info.IsDir()
		if !isDir && info.Mode()&fs.ModeSymlink != 0 && w.cfg.FollowSymlinks {
			if target, terr := os.Stat(path); terr == nil {
				isDir = target.IsDir()
			}
		}
		w.remember(path, isDir)
		w.emit(Event{Path: path, Op: OpCreate, IsDir: isDir})
		if isDir {
			w.extend(path)
		}

	case ev.Op.Has(fsnotify.Write):
		w.mu.Lock()
		isDir := w.known[path]
		w.mu.Unlock()
		w.emit(Event{Path: path, Op: OpWrite, IsDir: isDir})

	case ev.Op.Has(fsnotify.Remove):
		isDir := w.forget(path)
		w.emit(Event{Path: path, Op: OpRemove, IsDir: isDir})

	case ev.Op.Has(fsnotify.Rename):
		isDir := w.forget(path)
		w.emit(Event{Path: path, Op: OpRename, IsDir: isDir})

	case ev.Op.Has(fsnotify.Chmod):
		w.mu.Lock()
		isDir := w.known[path]
		w.mu.Unlock()
		w.emit(Event{Path: path, Op: OpChmod, IsDir: isDir})
	}
}

// extend registers a newly created directory and replays anything that landed
// inside it before the registration completed.
func (w *notifyWatcher) extend(dir string) {
	parentDepth, ok := w.depthOf(filepath.Dir(dir))
	if !ok {
		parentDepth = 0
	}
	depth := parentDepth + 1
	if !depthAllows(w.cfg.Depth, depth) {
		return
	}
	if w.cfg.Filter != nil && !w.cfg.Filter(dir, true) {
		return
	}
	if err := w.register(dir, depth); err != nil {
		w.emitError(err)
		return
	}
	w.replay(dir, depth)
}

// replay emits synthetic create events for entries that already exist inside a
// freshly watched directory.
func (w *notifyWatcher) replay(dir string, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !(w.cfg.IgnorePermissionErrors && ignorablePermissionError(err)) {
			w.emitError(fmt.Errorf("fswatch: %w", err))
		}
		return
	}
	for _, entry := range entries {
		child := filepath.Join(dir, entry.Name())
		isDir, err := w.classify(child, entry)
		if err != nil {
			continue
		}
		w.mu.Lock()
		_, seen := w.known[child]
		w.mu.Unlock()
		if seen {
			continue
		}
		w.remember(child, isDir)
		w.emit(Event{Path: child, Op: OpCreate, IsDir: isDir})
		if isDir {
			if err := w.register(child, depth+1); err == nil {
				w.replay(child, depth+1)
			}
		}
	}
}

func (w *notifyWatcher) emit(ev Event) {
	if w.cfg.Filter != nil && !w.cfg.Filter(ev.Path, ev.IsDir) {
		return
	}
	select {
	case w.events <- ev:
	case <-w.done:
	}
}

func (w *notifyWatcher) emitError(err error) {
	if err == nil {
		return
	}
	select {
	case w.errs <- err:
	case <-w.done:
	default:
		// The consumer is not draining errors; dropping is preferable to
		// stalling the event loop, which would lose change notifications.
	}
}

// existingAncestor walks up from dir until it finds a directory that exists,
// so that a watch can be placed for a path that has not been created yet.
func existingAncestor(dir string) string {
	for {
		if info, err := os.Lstat(dir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// isWithin reports whether path lies below dir.
func isWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !filepath.IsAbs(rel) &&
		(len(rel) < 3 || rel[:3] != ".."+string(filepath.Separator))
}
