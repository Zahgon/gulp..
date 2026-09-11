package cli

import (
	"regexp"
	"unicode"

	"golang.org/x/text/width"
)

// ansiPattern matches the escape sequences the palette emits, so that a
// coloured label can still be measured for column alignment. It covers CSI
// sequences (colour, cursor movement) and the OSC hyperlink form, which is the
// same ground the ansi-regex package covers for gulp-cli.
var ansiPattern = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

// StripANSI removes escape sequences from s.
func StripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// DisplayWidth returns the number of terminal columns s occupies once escape
// sequences are removed.
//
// gulp-cli pads task descriptions into a column using the string-width
// package, so a label containing CJK characters, which occupy two columns
// each, must not be measured by rune count or the tree would come out ragged.
func DisplayWidth(s string) int {
	total := 0
	for _, r := range StripANSI(s) {
		total += runeWidth(r)
	}
	return total
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case isWideEmoji(r):
		return 2
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

// isWideEmoji covers the pictographic blocks that render double-width but are
// classified as neutral by the East Asian Width property, which is the same
// special case string-width carries.
func isWideEmoji(r rune) bool {
	switch {
	case r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f680 && r <= 0x1f6ff,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x1fa70 && r <= 0x1faff,
		r >= 0x2600 && r <= 0x27bf:
		return true
	default:
		return false
	}
}
