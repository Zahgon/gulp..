// Package globstream walks the file system and emits vinyl files.
//
// It is the Go port of npm `glob-stream`, the stage that sits underneath
// vinyl-fs's src(). It resolves globs to paths and produces *vinyl.File values
// carrying Cwd, Base, Path and Stat -- but never Contents. Reading contents is
// the next stage's job (see the vfs package), which is what lets src() support
// the read:false and buffer:false options.
package globstream

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/gulpjs/gulp-go/internal/glob"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Options configures the walk. The field set mirrors the glob-related options
// documented for gulp.src() in docs/api/src.md.
type Options struct {
	// Cwd is the directory relative globs resolve against. Defaults to the
	// process working directory.
	Cwd string
	// Base overrides the computed glob parent as every emitted file's base.
	Base string
	// CwdBase makes Cwd the base for every file, equivalent to Base: Cwd.
	CwdBase bool
	// Root anchors globs to a directory other than Cwd.
	Root string
	// AllowEmpty suppresses the "File not found with singular glob" error.
	AllowEmpty bool
	// UniqueBy selects the dedupe key: "path" (the default) or "" to disable.
	UniqueBy string
	// UniqueByFunc supplies a custom dedupe key, taking precedence over
	// UniqueBy.
	UniqueByFunc func(*vinyl.File) any
	// Ignore holds extra patterns to exclude, equivalent to prefixing each
	// with "!" in the glob list.
	Ignore []string
	// ResolveSymlinks makes the emitted Stat describe the link target rather
	// than the link itself. gulp defaults this to true.
	ResolveSymlinks bool
	// Since filters out files not modified strictly after this instant. The
	// zero value disables the filter.
	Since time.Time

	// The remaining fields are passed through to the matcher.
	Dot        bool
	NoBrace    bool
	NoGlobstar bool
	NoExt      bool
	NoCase     bool
	MatchBase  bool
}

// Sentinel errors, wrapped with the offending glob by the walker. The message
// text is fixed by docs/api/src.md, which promises these exact phrases.
var (
	// ErrInvalidGlob is reported for an empty glob, or a glob list with no
	// positive patterns.
	ErrInvalidGlob = errors.New("Invalid glob argument")
	// ErrSingularGlobNotFound is reported when a glob containing no magic
	// characters matches nothing and AllowEmpty is false.
	ErrSingularGlobNotFound = errors.New("File not found with singular glob")
)

// New returns a source stage that walks globs and emits one vinyl file per
// match.
//
// Nothing is read from disk until the returned Transform actually runs. gulp
// 5.0.1 shipped a fix titled "Avoid globbing before read stream is opened";
// deferring all work to Transform is how that guarantee is preserved here.
func New(globs []string, opts Options) pipeline.Transform {
	return pipeline.TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		w, err := newWalker(globs, opts)
		if err != nil {
			return err
		}
		return w.run(ctx, out)
	})
}

// walker holds the resolved state for one execution.
type walker struct {
	opts      Options
	cwd       string
	root      string
	positives []resolvedGlob
	negatives *glob.MatchSet
	seen      map[any]struct{}
}

// resolvedGlob pairs an absolute pattern with the base that files matched by it
// will receive, plus the original spelling for error messages.
type resolvedGlob struct {
	original string
	absolute string
	base     string
	magic    bool
	matcher  *glob.Matcher
}

func newWalker(globs []string, opts Options) (*walker, error) {
	if opts.Cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("globstream: resolving cwd: %w", err)
		}
		opts.Cwd = wd
	}
	cwd, err := filepath.Abs(opts.Cwd)
	if err != nil {
		return nil, fmt.Errorf("globstream: resolving cwd: %w", err)
	}

	// Root, when supplied, is the directory globs are anchored to instead of
	// Cwd. Cwd still governs how a relative Root itself is interpreted.
	root := cwd
	if opts.Root != "" {
		root = opts.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(cwd, root)
		}
	}

	if len(globs) == 0 {
		return nil, fmt.Errorf("%w: no globs supplied", ErrInvalidGlob)
	}

	matchOpts := glob.Options{
		Dot:        opts.Dot,
		NoBrace:    opts.NoBrace,
		NoGlobstar: opts.NoGlobstar,
		NoExt:      opts.NoExt,
		NoCase:     opts.NoCase,
		MatchBase:  opts.MatchBase,
	}

	w := &walker{opts: opts, cwd: cwd, root: root, seen: map[any]struct{}{}}

	var negativePatterns []string
	for _, ignored := range opts.Ignore {
		negativePatterns = append(negativePatterns, "!"+glob.Resolve(glob.StripNegation(ignored), root))
	}

	for _, g := range globs {
		if strings.TrimSpace(g) == "" {
			return nil, fmt.Errorf("%w: %q", ErrInvalidGlob, g)
		}
		resolved := glob.Resolve(g, root)
		if glob.IsNegative(resolved) {
			negativePatterns = append(negativePatterns, resolved)
			continue
		}
		m, err := glob.Compile(resolved, matchOpts)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidGlob, g)
		}
		w.positives = append(w.positives, resolvedGlob{
			original: g,
			absolute: resolved,
			base:     w.baseFor(resolved),
			magic:    glob.IsGlob(resolved),
			matcher:  m,
		})
	}

	if len(w.positives) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidGlob, strings.Join(globs, ", "))
	}

	w.negatives, err = glob.CompileSet(negativePatterns, matchOpts)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGlob, err)
	}
	return w, nil
}

// baseFor decides the base a file matched by this glob receives. dest() strips
// the base from the path to work out the output location, so this is what
// preserves directory structure.
func (w *walker) baseFor(absPattern string) string {
	switch {
	case w.opts.Base != "":
		abs := w.opts.Base
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(w.cwd, abs)
		}
		return filepath.Clean(abs)
	case w.opts.CwdBase:
		return w.cwd
	default:
		return filepath.Clean(glob.Parent(absPattern))
	}
}

func (w *walker) run(ctx context.Context, out chan<- *vinyl.File) error {
	// Globs are processed in the order supplied, and matches within one glob
	// are sorted, so the emitted order is deterministic. gulp's own src test
	// asserts that src(['.../run.dmc', '.../test.dmc']) emits run.dmc first.
	for _, rg := range w.positives {
		matches, err := w.expand(rg)
		if err != nil {
			return err
		}
		for _, absPath := range matches {
			if err := ctx.Err(); err != nil {
				return err
			}
			file, err := w.build(rg, absPath)
			if err != nil {
				return err
			}
			if file == nil {
				continue
			}
			if err := pipeline.Send(ctx, out, file); err != nil {
				return err
			}
		}
	}
	return nil
}

// expand turns one resolved glob into a sorted list of absolute paths.
func (w *walker) expand(rg resolvedGlob) ([]string, error) {
	if !rg.magic {
		// A glob with no magic characters names exactly one path. glob-stream
		// stats it directly and treats a miss as an error unless allowEmpty
		// was set, which is the documented "File not found with singular
		// glob" behaviour.
		p := filepath.Clean(rg.absolute)
		if _, err := os.Lstat(p); err != nil {
			if os.IsNotExist(err) {
				if w.opts.AllowEmpty {
					return nil, nil
				}
				return nil, fmt.Errorf("%w: %s (if this was purposeful, use `allowEmpty` option)",
					ErrSingularGlobNotFound, rg.original)
			}
			return nil, fmt.Errorf("globstream: stat %s: %w", p, err)
		}
		return []string{p}, nil
	}

	searchDir, rest := splitPattern(rg.absolute)
	if rest == "" {
		return []string{filepath.Clean(searchDir)}, nil
	}

	// The matcher's compiled form already reflects NoBrace/NoGlobstar/NoCase,
	// so hand that to doublestar rather than the raw pattern.
	restPattern := rest
	if compiled := rg.matcher.Compiled(); compiled != "" {
		if _, tail := splitPattern(compiled); tail != "" {
			restPattern = tail
		}
	}
	if w.opts.MatchBase && !strings.Contains(restPattern, "/") {
		restPattern = "**/" + restPattern
	}

	globOpts := []doublestar.GlobOption{doublestar.WithFailOnIOErrors()}
	if w.opts.NoCase {
		globOpts = append(globOpts, doublestar.WithCaseInsensitive())
	}
	if !w.opts.Dot && !rg.matcher.AllowsDot() {
		// Pruning hidden directories during the walk rather than filtering
		// afterwards keeps `**` patterns from descending into .git and
		// node_modules.
		globOpts = append(globOpts, doublestar.WithNoHidden())
	}

	fsys := os.DirFS(searchDir)
	found, err := doublestar.Glob(fsys, restPattern, globOpts...)
	if err != nil {
		// An I/O error during the walk is real; a pattern that simply matches
		// nothing is not an error for a magic glob.
		if errors.Is(err, doublestar.ErrBadPattern) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidGlob, rg.original)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("globstream: walking %s: %w", searchDir, err)
		}
		return nil, nil
	}

	out := make([]string, 0, len(found))
	for _, rel := range found {
		out = append(out, filepath.Join(searchDir, filepath.FromSlash(rel)))
	}
	sort.Strings(out)
	return out, nil
}

// build turns an absolute path into a vinyl file, applying the negation,
// uniqueness and since filters. A nil file means "skip".
func (w *walker) build(rg resolvedGlob, absPath string) (*vinyl.File, error) {
	slashed := filepath.ToSlash(absPath)
	if w.negatives.Excluded(slashed) {
		return nil, nil
	}

	st, err := w.stat(absPath)
	if err != nil {
		return nil, err
	}
	if !w.opts.Since.IsZero() && !st.MTime.After(w.opts.Since) {
		return nil, nil
	}

	file, err := vinyl.New(vinyl.Options{
		Cwd:  w.cwd,
		Base: rg.base,
		Path: absPath,
		Stat: st,
	})
	if err != nil {
		return nil, fmt.Errorf("globstream: building vinyl for %s: %w", absPath, err)
	}

	if key, ok := w.uniqueKey(file); ok {
		if _, dup := w.seen[key]; dup {
			return nil, nil
		}
		w.seen[key] = struct{}{}
	}
	return file, nil
}

// stat resolves the path's metadata, honouring ResolveSymlinks.
func (w *walker) stat(absPath string) (*vinyl.Stat, error) {
	li, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("globstream: lstat %s: %w", absPath, err)
	}
	if !w.opts.ResolveSymlinks || li.Mode()&os.ModeSymlink == 0 {
		return vinyl.StatFromFileInfo(li), nil
	}
	// Following the link is the default, but a dangling link must not abort
	// the whole stream -- fall back to the link's own metadata, which leaves
	// the file marked symbolic so dest() can recreate it.
	ti, err := os.Stat(absPath)
	if err != nil {
		return vinyl.StatFromFileInfo(li), nil
	}
	return vinyl.StatFromFileInfo(ti), nil
}

// uniqueKey computes the dedupe key for a file, or reports false when
// deduplication is disabled.
func (w *walker) uniqueKey(f *vinyl.File) (any, bool) {
	switch {
	case w.opts.UniqueByFunc != nil:
		return w.opts.UniqueByFunc(f), true
	case w.opts.UniqueBy == "":
		return nil, false
	case w.opts.UniqueBy == "path":
		return f.Path(), true
	default:
		if v, ok := f.Get(w.opts.UniqueBy); ok {
			return v, true
		}
		return f.Path(), true
	}
}

// splitPattern divides an absolute pattern into the deepest magic-free
// directory and the remaining pattern.
//
// glob.Parent computes the same boundary but also unescapes metacharacters, so
// its result cannot be used to slice the original string. Counting segments
// avoids that, since unescaping never changes the segment count.
func splitPattern(absPattern string) (searchDir, rest string) {
	slashed := filepath.ToSlash(absPattern)
	parent := glob.Parent(slashed)

	parentSegs := splitSegments(parent)
	allSegs := splitSegments(slashed)
	if len(parentSegs) > len(allSegs) {
		return filepath.FromSlash(parent), ""
	}

	dir := "/" + path.Join(allSegs[:len(parentSegs)]...)
	rest = path.Join(allSegs[len(parentSegs):]...)
	return filepath.FromSlash(dir), rest
}

// splitSegments splits an absolute slash path into its non-empty segments.
func splitSegments(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
