// Package glob reproduces the glob semantics gulp depends on.
//
// It is the Go port of three npm packages that gulp pulls in transitively:
//
//	is-glob     -> IsGlob
//	glob-parent -> Parent
//	micromatch  -> Matcher / MatchSet
//
// Low-level pattern matching is delegated to github.com/bmatcuk/doublestar,
// which understands `**`, brace expansion and character classes. Everything
// this package adds on top -- dotfile handling, case folding, basename
// matching, negation ordering, and the glob-parent algorithm -- exists because
// gulp's public behaviour (specifically the `base` a vinyl file gets, which
// decides the output directory structure) depends on those details.
package glob

import (
	"regexp"
	"strings"
)

// magicChars are the characters that make a path segment a pattern rather than
// a literal name.
const magicChars = "*?[]{}()!+@"

// IsGlob reports whether s contains unescaped glob syntax.
//
// Port of npm `is-glob` in its default strict mode. gulp uses this in two
// load-bearing places: glob-stream resolves non-magic globs with a direct stat
// (and errors with "File not found with singular glob" when they miss), and
// glob-parent uses it to decide how many trailing segments to strip when
// computing a file's base. The base decides every dest() output path, so the
// port has to agree with is-glob character for character.
//
// Two of is-glob's rules are surprising enough to be worth stating, because
// both were originally ported wrong here and only a differential run against
// the real module caught them:
//
//   - `?` is not a glob on its own. It counts only as an extglob quantifier,
//     that is when the preceding character is one of `].+)`. So `path/?foo`
//     is a literal path, and glob-parent keeps the whole thing as the base.
//   - `(a|b)` is a glob even without an extglob prefix, because micromatch
//     accepts a bare alternation group.
func IsGlob(s string) bool {
	if s == "" {
		return false
	}
	if isExtglob(s) {
		return true
	}
	return strictCheck(s)
}

// extglobPattern is the regexp npm `is-extglob` scans with. The first
// alternative swallows an escape pair so that `\@(a)` is not mistaken for an
// extglob; the second is the extglob itself.
var extglobPattern = regexp.MustCompile(`(\\).|([@?!+*]\(.*\))`)

// isExtglob is the port of npm `is-extglob`.
func isExtglob(s string) bool {
	for s != "" {
		loc := extglobPattern.FindStringSubmatchIndex(s)
		if loc == nil {
			return false
		}
		if loc[4] >= 0 {
			return true
		}
		s = s[loc[1]:]
	}
	return false
}

// closingChar pairs the bracket characters is-glob skips over after a
// backslash escape.
var closingChar = map[byte]byte{'{': '}', '(': ')', '[': ']'}

// strictCheck is a statement-for-statement port of is-glob's strictCheck.
//
// The index bookkeeping looks redundant but is not: the sentinel -2 means
// "not searched yet" while -1 means "searched and absent", and the cached
// indices let a later iteration skip a scan. Deviating from the original
// structure risks changing which patterns are considered magic, so it is kept
// verbatim.
func strictCheck(s string) bool {
	if charAt(s, 0) == '!' {
		return true
	}

	index := 0
	pipeIndex, closeSquareIndex, closeCurlyIndex, closeParenIndex, backSlashIndex := -2, -2, -2, -2, -2

	for index < len(s) {
		if s[index] == '*' {
			return true
		}

		if charAt(s, index+1) == '?' && isOneOf(s[index], "].+)") {
			return true
		}

		if closeSquareIndex != -1 && s[index] == '[' && charAt(s, index+1) != ']' {
			if closeSquareIndex < index {
				closeSquareIndex = indexByteFrom(s, ']', index)
			}
			if closeSquareIndex > index {
				if backSlashIndex == -1 || backSlashIndex > closeSquareIndex {
					return true
				}
				backSlashIndex = indexByteFrom(s, '\\', index)
				if backSlashIndex == -1 || backSlashIndex > closeSquareIndex {
					return true
				}
			}
		}

		if closeCurlyIndex != -1 && s[index] == '{' && charAt(s, index+1) != '}' {
			closeCurlyIndex = indexByteFrom(s, '}', index)
			if closeCurlyIndex > index {
				backSlashIndex = indexByteFrom(s, '\\', index)
				if backSlashIndex == -1 || backSlashIndex > closeCurlyIndex {
					return true
				}
			}
		}

		if closeParenIndex != -1 && s[index] == '(' && charAt(s, index+1) == '?' &&
			isOneOf(charAt(s, index+2), ":!=") && charAt(s, index+3) != ')' {
			closeParenIndex = indexByteFrom(s, ')', index)
			if closeParenIndex > index {
				backSlashIndex = indexByteFrom(s, '\\', index)
				if backSlashIndex == -1 || backSlashIndex > closeParenIndex {
					return true
				}
			}
		}

		if pipeIndex != -1 && s[index] == '(' && charAt(s, index+1) != '|' {
			if pipeIndex < index {
				pipeIndex = indexByteFrom(s, '|', index)
			}
			if pipeIndex != -1 && charAt(s, pipeIndex+1) != ')' {
				closeParenIndex = indexByteFrom(s, ')', pipeIndex)
				if closeParenIndex > pipeIndex {
					backSlashIndex = indexByteFrom(s, '\\', pipeIndex)
					if backSlashIndex == -1 || backSlashIndex > closeParenIndex {
						return true
					}
				}
			}
		}

		if s[index] == '\\' {
			open := charAt(s, index+1)
			index += 2
			if closer, ok := closingChar[open]; ok {
				if n := indexByteFrom(s, closer, index); n != -1 {
					index = n + 1
				}
			}
			if charAt(s, index) == '!' {
				return true
			}
		} else {
			index++
		}
	}

	return false
}

// charAt returns the byte at i, or 0 when i is out of range. It stands in for
// JavaScript's out-of-range string index, which yields undefined and compares
// unequal to every character.
func charAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// isOneOf reports whether c appears in set, the port of a character class test.
func isOneOf(c byte, set string) bool {
	return c != 0 && strings.IndexByte(set, c) >= 0
}

// indexByteFrom is String.prototype.indexOf(char, from): an absolute index, or
// -1 when the character does not occur at or after from.
func indexByteFrom(s string, c byte, from int) int {
	if from < 0 {
		from = 0
	}
	if from >= len(s) {
		return -1
	}
	i := strings.IndexByte(s[from:], c)
	if i < 0 {
		return -1
	}
	return from + i
}

// IsNegative reports whether a pattern is a negation (`!foo`). A leading `\!`
// is an escaped literal `!` and is therefore not a negation.
func IsNegative(pattern string) bool {
	return strings.HasPrefix(pattern, "!")
}

// StripNegation removes a leading `!` from a negated pattern and unescapes a
// leading `\!` into a literal `!`.
func StripNegation(pattern string) string {
	switch {
	case strings.HasPrefix(pattern, "!"):
		return pattern[1:]
	case strings.HasPrefix(pattern, `\!`):
		return pattern[1:]
	default:
		return pattern
	}
}
