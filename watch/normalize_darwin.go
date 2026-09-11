//go:build darwin

package watch

import "golang.org/x/text/unicode/norm"

// normalizeUnicode folds a path into NFC.
//
// HFS+ and APFS store filenames decomposed (NFD), so a directory created from
// the composed literal "フォルダ" is read back from the kernel in a different
// byte sequence than the one the glob pattern was written with. Comparing the
// two directly fails even though they name the same directory. Normalizing
// both sides to NFC before matching makes non-ASCII watch patterns work, which
// is exactly what gulp's "should watch a non-ASCII path" test requires.
//
// Only darwin needs this: Linux and Windows preserve the bytes they are given.
func normalizeUnicode(path string) string {
	return norm.NFC.String(path)
}
