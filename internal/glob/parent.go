package glob

import (
	"regexp"
	"runtime"
	"strings"
)

var (
	// escapedMagic matches a backslash-escaped glob metacharacter, which the
	// final step of the algorithm unescapes.
	escapedMagic = regexp.MustCompile(`\\([!*?|\[\](){}])`)
	// unclosedParen detects a dangling `(` left behind after a dirname step,
	// which means the segment was part of an extglob.
	unclosedParen = regexp.MustCompile(`\([^()]+$`)
	// unescapedBraceOrBracket detects `{` or `[` that is not backslash-escaped.
	unescapedBraceOrBracket = regexp.MustCompile(`[^\\][{\[]`)
)

// Parent returns the deepest directory prefix of a glob that contains no glob
// syntax. It is the Go port of npm `glob-parent`, reproduced step for step
// because gulp assigns the result to `file.base`, and `dest()` strips exactly
// that prefix when computing where to write. Getting this wrong silently
// flattens or duplicates output directory structure.
//
// Examples:
//
//	/src/js/**.js           -> /src/js
//	./fixtures/**/*.jade    -> ./fixtures
//	./fixtures/stuff/run.js -> ./fixtures/stuff   (no magic: strip the file)
//	path/{foo,bar}/         -> path
//	path/\*/                -> path/*             (escape removed)
func Parent(pattern string) string {
	str := pattern

	// On Windows a pattern written with only backslashes is a path, not a
	// pattern with escapes, so flip the separators first.
	if runtime.GOOS == "windows" && !strings.Contains(str, "/") {
		str = strings.ReplaceAll(str, `\`, "/")
	}

	// A pattern ending in an enclosure that itself spans a separator, such as
	// `foo/{bar,baz/qux}`, must not have its closing brace treated as the last
	// path segment. Appending a separator makes the dirname loop behave.
	if endsInEnclosureWithSeparator(str) {
		str += "/"
	}

	// Appending a character preserves a trailing separator: without it,
	// `path/foo/` would lose the `foo` segment on the first dirname step.
	str += "a"

	for {
		str = posixDirname(str)
		if !isGlobbySegment(str) {
			break
		}
	}

	return escapedMagic.ReplaceAllString(str, "$1")
}

// endsInEnclosureWithSeparator reports whether the pattern ends with `}` or `]`
// whose opening counterpart encloses a path separator.
func endsInEnclosureWithSeparator(str string) bool {
	if str == "" {
		return false
	}
	var open byte
	switch str[len(str)-1] {
	case '}':
		open = '{'
	case ']':
		open = '['
	default:
		return false
	}
	i := strings.IndexByte(str, open)
	if i < 0 {
		return false
	}
	return strings.Contains(str[i+1:len(str)-1], "/")
}

// isGlobbySegment reports whether a directory prefix still contains glob
// syntax and therefore needs another dirname step. It mirrors glob-parent's
// internal `isGlobby`, which is deliberately broader than IsGlob: a fragment
// left mid-extglob or mid-brace is globby even though it would not parse as a
// complete pattern on its own.
func isGlobbySegment(str string) bool {
	if unclosedParen.MatchString(str) {
		return true
	}
	if str != "" && (str[0] == '{' || str[0] == '[') {
		return true
	}
	if unescapedBraceOrBracket.MatchString(str) {
		return true
	}
	return IsGlob(str)
}

// posixDirname is Node's `path.posix.dirname`.
//
// Go's path.Dir cannot be substituted because it calls Clean, which would
// rewrite `./fixtures` to `fixtures` and collapse `..` segments. glob-parent
// relies on the raw, non-normalising behaviour, and so does the base that ends
// up on every vinyl file.
func posixDirname(p string) string {
	if p == "" {
		return "."
	}
	hasRoot := p[0] == '/'
	end := -1
	matchedSlash := true
	for i := len(p) - 1; i >= 1; i-- {
		if p[i] == '/' {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return p[:end]
}
