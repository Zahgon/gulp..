package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gulpjs/gulp-go/internal/asyncdone"
	"github.com/gulpjs/gulp-go/internal/fswatch"
	"github.com/gulpjs/gulp-go/internal/glob"
)

// TaskFunc is the signature of the function gulp.watch runs on change.
//
// It is the same signature used throughout the port, so a task registered with
// gulp.Task, or a composition built with gulp.Series or gulp.Parallel, can be
// handed to Watch unchanged.
type TaskFunc = asyncdone.TaskFunc

// ErrNonStringWatchPath is returned when a watch path is empty.
//
// JavaScript raises "Non-string provided as watch path" for anything that is
// not a string; Go's type system already rules that out, so the only remaining
// invalid input is the empty string.
var ErrNonStringWatchPath = errors.New("Non-string provided as watch path")

// ErrNoWatchPaths is returned when no watch paths are supplied at all.
var ErrNoWatchPaths = errors.New("watch: no paths provided")

// Watcher observes a set of globs and runs a task when they change.
//
// It corresponds to the chokidar instance gulp.watch returns: listeners are
// attached with On, extra paths with Add, and it must be closed with Close.
type Watcher struct {
	opts    resolved
	matcher *glob.MatchSet
	ignored *glob.MatchSet
	backend fswatch.Watcher
	task    TaskFunc

	mu        sync.Mutex
	listeners map[EventKind][]func(path string)
	errFns    []func(error)
	roots     map[string]struct{}

	trigger  chan struct{}
	deferred chan pendingEvent
	done     chan struct{}
	closeOne sync.Once
	closeErr error
	wg       sync.WaitGroup

	readyOnce sync.Once
	ready     chan struct{}

	// pending holds unlink events being held back by the atomic window,
	// keyed by absolute path.
	pendingMu sync.Mutex
	pending   map[string]*time.Timer
}

// pendingEvent is an event that was held back and is now ready to be emitted.
type pendingEvent struct {
	kind EventKind
	path string
}

// New creates a Watcher for the given globs.
//
// task may be nil, in which case the watcher only drives listeners registered
// with On. Watching begins immediately; call Close when finished.
func New(globs []string, opts Options, task TaskFunc) (*Watcher, error) {
	if len(globs) == 0 {
		return nil, ErrNoWatchPaths
	}
	for _, g := range globs {
		if g == "" {
			return nil, ErrNonStringWatchPath
		}
	}

	cwd := opts.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("watch: %w", err)
		}
		cwd = wd
	}
	abscwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("watch: %w", err)
	}
	abscwd = normalizeUnicode(abscwd)

	r := opts.resolve(abscwd)

	matcher, err := compilePatterns(globs, abscwd, r.disableGlobbing)
	if err != nil {
		return nil, err
	}
	ignored, err := compilePatterns(r.ignored, abscwd, false)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		opts:      r,
		matcher:   matcher,
		ignored:   ignored,
		task:      task,
		listeners: make(map[EventKind][]func(string)),
		roots:     make(map[string]struct{}),
		trigger:   make(chan struct{}, 1),
		deferred:  make(chan pendingEvent, 64),
		done:      make(chan struct{}),
		ready:     make(chan struct{}),
		pending:   make(map[string]*time.Timer),
	}

	cfg := fswatch.Config{
		Depth:                  r.depth,
		FollowSymlinks:         r.followSymlinks,
		IgnorePermissionErrors: r.ignorePermissionErrors,
		Interval:               r.interval,
		Filter:                 w.filter,
	}
	if r.usePolling {
		w.backend, err = fswatch.NewPolling(cfg)
	} else {
		w.backend, err = fswatch.NewNotify(cfg)
	}
	if err != nil {
		return nil, err
	}

	w.wg.Add(2)
	go w.readLoop()
	go w.runLoop()

	if err := w.Add(globs...); err != nil {
		_ = w.Close()
		return nil, err
	}

	go w.announceReady()

	return w, nil
}

// compilePatterns resolves each pattern against cwd and compiles the set.
// Negations survive resolution, so `!ignored.txt` stays a negation.
func compilePatterns(patterns []string, cwd string, literal bool) (*glob.MatchSet, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	resolvedPatterns := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if p == "" {
			continue
		}
		abs := glob.Resolve(normalizeUnicode(p), cwd)
		if literal {
			abs = escapeMagic(abs)
		}
		resolvedPatterns = append(resolvedPatterns, abs)
	}
	if len(resolvedPatterns) == 0 {
		return nil, nil
	}
	set, err := glob.CompileSet(resolvedPatterns, glob.Options{Dot: true})
	if err != nil {
		return nil, fmt.Errorf("watch: %w", err)
	}
	return set, nil
}

// escapeMagic neutralizes glob metacharacters so a path containing them is
// treated literally. This implements DisableGlobbing.
func escapeMagic(p string) string {
	negated := glob.IsNegative(p)
	if negated {
		p = glob.StripNegation(p)
	}
	out := make([]rune, 0, len(p)*2)
	for _, r := range p {
		switch r {
		case '*', '?', '[', ']', '{', '}', '(', ')', '!', '\\':
			out = append(out, '\\', r)
		default:
			out = append(out, r)
		}
	}
	result := string(out)
	if negated {
		return "!" + result
	}
	return result
}

// Add begins watching additional globs.
func (w *Watcher) Add(globs ...string) error {
	for _, g := range globs {
		if g == "" {
			return ErrNonStringWatchPath
		}
		root := w.rootFor(g)

		w.mu.Lock()
		_, seen := w.roots[root]
		if !seen {
			w.roots[root] = struct{}{}
		}
		w.mu.Unlock()

		if seen {
			continue
		}
		if err := w.backend.Add(root); err != nil {
			return err
		}
	}
	return nil
}

// Unwatch stops watching globs previously passed to New or Add.
func (w *Watcher) Unwatch(globs ...string) error {
	for _, g := range globs {
		if g == "" {
			continue
		}
		root := w.rootFor(g)

		w.mu.Lock()
		_, seen := w.roots[root]
		delete(w.roots, root)
		w.mu.Unlock()

		if !seen {
			continue
		}
		if err := w.backend.Remove(root); err != nil {
			return err
		}
	}
	return nil
}

// rootFor derives the directory that must be watched to observe a glob. For a
// pattern the answer is the segment before the first magic character, which is
// exactly what glob.Parent computes; for a literal path it is the path itself.
func (w *Watcher) rootFor(pattern string) string {
	p := normalizeUnicode(pattern)
	if glob.IsNegative(p) {
		p = glob.StripNegation(p)
	}
	abs := glob.Resolve(p, w.opts.cwd)
	if w.opts.disableGlobbing {
		return filepath.Clean(abs)
	}
	return filepath.Clean(glob.Parent(abs))
}

// On registers a listener for an event kind. Listeners receive the changed
// path, made relative to Options.Cwd when that option was supplied, which is
// how chokidar reports paths.
//
// Passing EventAll subscribes the listener to every kind.
func (w *Watcher) On(kind EventKind, fn func(path string)) {
	if fn == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listeners[kind] = append(w.listeners[kind], fn)
}

// OnError registers a listener for errors raised by the backend or returned by
// the task.
func (w *Watcher) OnError(fn func(error)) {
	if fn == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.errFns = append(w.errFns, fn)
}

// Ready returns a channel closed once the initial scan has finished. It is the
// equivalent of chokidar's 'ready' event.
func (w *Watcher) Ready() <-chan struct{} { return w.ready }

// Close stops watching and releases all resources. It is safe to call more
// than once and blocks until the watcher's goroutines have exited.
func (w *Watcher) Close() error {
	w.closeOne.Do(func() {
		close(w.done)
		w.closeErr = w.backend.Close()

		w.pendingMu.Lock()
		for path, timer := range w.pending {
			timer.Stop()
			delete(w.pending, path)
		}
		w.pendingMu.Unlock()

		w.wg.Wait()
	})
	return w.closeErr
}

// filter is handed to the backend so that ignored directories are pruned
// before they are ever walked or registered.
func (w *Watcher) filter(path string, isDir bool) bool {
	if w.ignored == nil {
		return true
	}
	return !w.ignored.Match(normalizeUnicode(path))
}

// announceReady performs the initial scan and then closes the ready channel.
//
// When IgnoreInitial is false the files that already match are reported as
// add events first, reproducing chokidar's startup burst.
func (w *Watcher) announceReady() {
	if !w.opts.ignoreInitial {
		w.emitInitial()
	}
	w.readyOnce.Do(func() { close(w.ready) })
}

// emitInitial walks the watch roots and reports everything that matches.
func (w *Watcher) emitInitial() {
	w.mu.Lock()
	roots := make([]string, 0, len(w.roots))
	for root := range w.roots {
		roots = append(roots, root)
	}
	w.mu.Unlock()

	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			select {
			case <-w.done:
				return filepath.SkipAll
			default:
			}
			if err != nil {
				return nil //nolint:nilerr // an unreadable subtree is skipped, not fatal
			}
			if w.ignored != nil && w.ignored.Match(normalizeUnicode(path)) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if path == root && d.IsDir() {
				return nil
			}
			kind := EventAdd
			if d.IsDir() {
				kind = EventAddDir
			}
			w.dispatch(kind, path)
			return nil
		})
	}
}

// readLoop consumes backend notifications, applies glob matching and the
// atomic window, and dispatches the result.
func (w *Watcher) readLoop() {
	defer w.wg.Done()

	events := w.backend.Events()
	errs := w.backend.Errors()

	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				if errs == nil {
					return
				}
				continue
			}
			w.handle(ev)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				if events == nil {
					return
				}
				continue
			}
			w.emitError(err)
		case ev := <-w.deferred:
			w.dispatch(ev.kind, ev.path)
		}
	}
}

// handle converts a backend event into a chokidar event kind.
func (w *Watcher) handle(ev fswatch.Event) {
	path := normalizeUnicode(ev.Path)

	var kind EventKind
	switch {
	case ev.Op.Has(fswatch.OpCreate):
		kind = EventAdd
		if ev.IsDir {
			kind = EventAddDir
		}
	case ev.Op.Has(fswatch.OpWrite), ev.Op.Has(fswatch.OpChmod):
		// chokidar reports a metadata-only change as a change too: `touch`
		// on an unmodified file is expected to trigger a rebuild.
		kind = EventChange
		if ev.IsDir {
			// A directory's mtime changes whenever an entry is added or
			// removed. Reporting that as a change would double every event,
			// so it is dropped; the entry's own event carries the news.
			return
		}
	case ev.Op.Has(fswatch.OpRemove), ev.Op.Has(fswatch.OpRename):
		kind = EventUnlink
		if ev.IsDir {
			kind = EventUnlinkDir
		}
	default:
		return
	}

	if w.opts.atomic > 0 {
		if w.absorbAtomic(kind, path) {
			return
		}
	}

	if w.opts.awaitWriteFinish != nil && (kind == EventAdd || kind == EventChange) {
		w.awaitWriteFinish(kind, path)
		return
	}

	w.dispatch(kind, path)
}

// absorbAtomic implements chokidar's `atomic` option. It reports true when the
// event has been consumed and must not be dispatched now.
//
// An unlink is held for the atomic window. If an add for the same path arrives
// while it is held, the pair was an editor rewriting the file in place and is
// reported as a single change; otherwise the unlink is released unchanged.
func (w *Watcher) absorbAtomic(kind EventKind, path string) bool {
	switch kind {
	case EventUnlink:
		timer := time.AfterFunc(w.opts.atomic, func() {
			w.pendingMu.Lock()
			delete(w.pending, path)
			w.pendingMu.Unlock()

			select {
			case w.deferred <- pendingEvent{kind: EventUnlink, path: path}:
			case <-w.done:
			}
		})
		w.pendingMu.Lock()
		if old, ok := w.pending[path]; ok {
			old.Stop()
		}
		w.pending[path] = timer
		w.pendingMu.Unlock()
		return true

	case EventAdd:
		w.pendingMu.Lock()
		timer, ok := w.pending[path]
		if ok {
			timer.Stop()
			delete(w.pending, path)
		}
		w.pendingMu.Unlock()
		if ok {
			w.dispatch(EventChange, path)
			return true
		}
		return false

	default:
		return false
	}
}

// awaitWriteFinish holds an event until the file's size stops changing.
func (w *Watcher) awaitWriteFinish(kind EventKind, path string) {
	awf := w.opts.awaitWriteFinish
	go func() {
		var lastSize int64 = -1
		stableSince := time.Time{}

		for {
			select {
			case <-w.done:
				return
			case <-time.After(awf.PollInterval):
			}

			info, err := os.Lstat(path)
			if err != nil {
				// The file vanished mid-write; the unlink event covers it.
				return
			}
			if info.Size() != lastSize {
				lastSize = info.Size()
				stableSince = time.Now()
				continue
			}
			if time.Since(stableSince) >= awf.StabilityThreshold {
				select {
				case w.deferred <- pendingEvent{kind: kind, path: path}:
				case <-w.done:
				}
				return
			}
		}
	}()
}

// dispatch applies glob matching, notifies listeners and, when the event kind
// is subscribed, schedules the task.
func (w *Watcher) dispatch(kind EventKind, path string) {
	if w.matcher == nil || !w.matcher.Match(path) {
		return
	}

	reported := w.reportPath(path)

	w.mu.Lock()
	fns := make([]func(string), 0, len(w.listeners[kind])+len(w.listeners[EventAll]))
	fns = append(fns, w.listeners[kind]...)
	fns = append(fns, w.listeners[EventAll]...)
	w.mu.Unlock()

	for _, fn := range fns {
		fn(reported)
	}

	if !w.opts.wants(kind) {
		return
	}

	select {
	case w.trigger <- struct{}{}:
	case <-w.done:
	default:
		// A trigger is already pending; the debounce below will collect this
		// change along with it.
	}
}

// reportPath renders an absolute path the way chokidar would: relative to cwd
// when a cwd was configured, absolute otherwise.
func (w *Watcher) reportPath(path string) string {
	rel, err := filepath.Rel(w.opts.cwd, path)
	if err != nil {
		return path
	}
	return rel
}

// runLoop debounces triggers and serializes task runs.
//
// Semantics follow glob-watcher: changes are collected for Delay, then the
// task runs. Changes arriving while the task is in flight set a single queued
// flag when Queue is enabled, so at most one further run is scheduled no
// matter how many changes land.
func (w *Watcher) runLoop() {
	defer w.wg.Done()

	if w.task == nil {
		// Still drain triggers so dispatch never blocks.
		for {
			select {
			case <-w.done:
				return
			case <-w.trigger:
			}
		}
	}

	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		running bool
		queued  bool
	)
	complete := make(chan error, 1)

	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			timerC = nil
		}
	}
	defer stopTimer()

	start := func() {
		running = true
		go func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				select {
				case <-w.done:
					cancel()
				case <-ctx.Done():
				}
			}()
			complete <- asyncdone.Run(ctx, w.task)
		}()
	}

	for {
		select {
		case <-w.done:
			return

		case <-w.trigger:
			if w.opts.delay <= 0 {
				if running {
					if w.opts.queue {
						queued = true
					}
					continue
				}
				start()
				continue
			}
			if timer == nil {
				timer = time.NewTimer(w.opts.delay)
				timerC = timer.C
			} else {
				timer.Stop()
				timer.Reset(w.opts.delay)
			}

		case <-timerC:
			timer = nil
			timerC = nil
			if running {
				if w.opts.queue {
					queued = true
				}
				continue
			}
			start()

		case err := <-complete:
			running = false
			if err != nil && !errors.Is(err, context.Canceled) {
				w.emitError(err)
			}
			if queued {
				queued = false
				start()
			}
		}
	}
}

// emitError forwards an error to registered error listeners.
func (w *Watcher) emitError(err error) {
	if err == nil {
		return
	}
	w.mu.Lock()
	fns := make([]func(error), len(w.errFns))
	copy(fns, w.errFns)
	w.mu.Unlock()

	for _, fn := range fns {
		fn(err)
	}
}
