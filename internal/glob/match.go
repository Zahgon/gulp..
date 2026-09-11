package glob

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Options mirrors the subset of node-glob / micromatch flags that gulp exposes
// through src() and watch(). See docs/api/src.md.
type Options struct {
	// Dot allows patterns to match paths whose segments begin with a period.
	// node-glob defaults this to false, which is why `*` does not pick up
	// dotfiles in gulp.
	Dot bool
	// NoBrace disables `{a,b}` expansion.
	NoBrace bool
	// NoGlobstar makes `**` behave like a single `*`.
	NoGlobstar bool
	// NoExt disables extglob syntax such as `+(a|b)`.
	NoExt bool
	// NoCase makes matching case-insensitive.
	NoCase bool
	// MatchBase lets a pattern containing no separator match against a path's
	// base name anywhere in the tree.
	MatchBase bool
}

// Matcher is a compiled single pattern.
type Matcher struct {
	pattern  string
	compiled string
	opts     Options
	// allowsDot records whether the pattern itself names a dot segment, in
	// which case dotfile filtering must not reject the match.
	allowsDot bool
	// baseOnly records that MatchBase applies: the pattern has no separator.
	baseOnly bool
}

// Compile prepares a pattern for repeated matching.
func Compile(pattern string, opts Options) (*Matcher, error) {
	p := filepath.ToSlash(pattern)

	if opts.NoBrace {
		// doublestar always expands braces, so neutralise them by escaping.
		p = strings.NewReplacer("{", `\{`, "}", `\}`).Replace(p)
	}
	if opts.NoGlobstar {
		p = strings.ReplaceAll(p, "**", "*")
	}
	if opts.NoCase {
		p = strings.ToLower(p)
	}
	if !doublestar.ValidatePattern(p) {
		return nil, fmt.Errorf("glob: invalid pattern %q", pattern)
	}

	return &Matcher{
		pattern:   pattern,
		compiled:  p,
		opts:      opts,
		allowsDot: patternNamesDotSegment(p),
		baseOnly:  opts.MatchBase && !strings.Contains(p, "/"),
	}, nil
}

// Pattern returns the original, uncompiled pattern.
func (m *Matcher) Pattern() string { return m.pattern }

// Compiled returns the pattern after the Options-driven rewrites (brace
// neutralisation, globstar degradation, case folding) have been applied. The
// file-system walker in internal/globstream feeds this to doublestar directly.
func (m *Matcher) Compiled() string { return m.compiled }

// AllowsDot reports whether the pattern explicitly names a dot-prefixed
// segment. The walker uses it to decide whether it may prune hidden
// directories outright, which matters for performance on `**` patterns.
func (m *Matcher) AllowsDot() bool { return m.allowsDot }

// Opts returns the options the matcher was compiled with.
func (m *Matcher) Opts() Options { return m.opts }

// Match reports whether p satisfies the pattern. p may use either separator;
// it is normalised to forward slashes first.
func (m *Matcher) Match(p string) bool {
	subject := filepath.ToSlash(p)
	if m.opts.NoCase {
		subject = strings.ToLower(subject)
	}
	if m.baseOnly {
		subject = path.Base(subject)
	}
	if !m.opts.Dot && !m.allowsDot && hasDotSegment(subject) {
		return false
	}
	ok, err := doublestar.Match(m.compiled, subject)
	// A malformed pattern is rejected at Compile time, so an error here can
	// only mean the pattern cannot match; treat it as a non-match rather than
	// propagating, which is what micromatch does.
	return err == nil && ok
}

// hasDotSegment reports whether any segment of p begins with a period,
// ignoring the `.` and `..` navigation segments and a leading `./`.
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(strings.TrimPrefix(p, "./"), "/") {
		if len(seg) > 1 && seg[0] == '.' && seg != ".." {
			return true
		}
	}
	return false
}

// patternNamesDotSegment reports whether the pattern explicitly mentions a
// dot-prefixed segment, e.g. `.git/**` or `**/.*rc`.
//
// The test is identical to hasDotSegment because a pattern segment that names a
// dotfile is spelled exactly like a path segment that is one; the two names are
// kept apart because the callers ask different questions of the answer.
func patternNamesDotSegment(p string) bool {
	return hasDotSegment(p)
}

// MatchSet is an ordered collection of positive and negative patterns, which is
// how gulp accepts globs: `src(['a/*.js', '!a/skip.js'])`.
type MatchSet struct {
	positives []*Matcher
	negatives []*Matcher
}

// CompileSet compiles an ordered pattern list. Patterns beginning with `!` are
// negations; `\!` escapes a literal leading exclamation mark.
//
// Semantics match micromatch: a path is included when it matches at least one
// positive pattern and no negative pattern. Unlike a plain sequential filter,
// a negation applies to the whole set regardless of its position in the list,
// which is what makes `['!skip', 'a/*']` behave the same as `['a/*', '!skip']`.
func CompileSet(patterns []string, opts Options) (*MatchSet, error) {
	set := &MatchSet{}
	for _, raw := range patterns {
		neg := IsNegative(raw)
		m, err := Compile(StripNegation(raw), opts)
		if err != nil {
			return nil, err
		}
		if neg {
			set.negatives = append(set.negatives, m)
		} else {
			set.positives = append(set.positives, m)
		}
	}
	return set, nil
}

// Match reports whether p is included by the set.
func (s *MatchSet) Match(p string) bool {
	if s.Excluded(p) {
		return false
	}
	for _, m := range s.positives {
		if m.Match(p) {
			return true
		}
	}
	return false
}

// Excluded reports whether any negative pattern rejects p. globstream uses this
// separately from Match because it filters walk results that were produced by a
// single positive pattern at a time.
func (s *MatchSet) Excluded(p string) bool {
	for _, m := range s.negatives {
		if m.Match(p) {
			return true
		}
	}
	return false
}

// Positives returns the compiled non-negated patterns, in order.
func (s *MatchSet) Positives() []*Matcher { return s.positives }

// Negatives returns the compiled negated patterns, in order.
func (s *MatchSet) Negatives() []*Matcher { return s.negatives }

// Resolve makes a pattern absolute against cwd while preserving its negation
// marker and its glob syntax.
//
// This exists because gulp allows the positive and negative patterns in one
// call to be written with different conventions -- gulp's own test suite uses
// `['./fixtures/stuff/*.dmc', '!fixtures/stuff/test.dmc']` -- and they can only
// be compared once both have been anchored to the same directory.
func Resolve(pattern, cwd string) string {
	neg := IsNegative(pattern)
	p := filepath.ToSlash(StripNegation(pattern))
	if !path.IsAbs(p) {
		p = path.Join(filepath.ToSlash(cwd), p)
	}
	if neg {
		return "!" + p
	}
	return p
}
