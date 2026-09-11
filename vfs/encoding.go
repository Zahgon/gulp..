package vfs

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DefaultEncoding is the encoding vinyl-fs assumes when none is supplied.
const DefaultEncoding = "utf8"

// EncodingDisabled is the [Option] value that turns transcoding and BOM
// handling off entirely, equivalent to passing `encoding: false` in JavaScript.
// Contents are then read and written as raw bytes.
const EncodingDisabled = ""

// utf8BOM is the UTF-8 byte order mark.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// codec is a resolved encoding: the transformers needed to move between the
// on-disk bytes and the UTF-8 bytes held in a vinyl file, plus whether the
// encoding conventionally carries a byte order mark.
//
// The JavaScript implementation delegates to `iconv-lite`; this port uses
// golang.org/x/text, which covers the same major families (UTF-16/32, the
// ISO-8859 and Windows code pages, and the CJK encodings). Encodings that
// golang.org/x/text does not implement are reported as an error rather than
// silently producing mojibake.
type codec struct {
	name     string
	identity bool // true for UTF-8: no transformation required
	enc      encoding.Encoding
	addBOM   bool
}

// encodingAliases maps the spellings iconv-lite accepts onto names that
// golang.org/x/text's IANA index understands.
var encodingAliases = map[string]string{
	"utf8":       "UTF-8",
	"utf-8":      "UTF-8",
	"utf8bom":    "UTF-8",
	"utf-8-bom":  "UTF-8",
	"ucs2":       "UTF-16LE",
	"ucs-2":      "UTF-16LE",
	"utf16le":    "UTF-16LE",
	"utf-16le":   "UTF-16LE",
	"utf16be":    "UTF-16BE",
	"utf-16be":   "UTF-16BE",
	"utf16":      "UTF-16BE",
	"utf-16":     "UTF-16BE",
	"latin1":     "ISO-8859-1",
	"binary":     "ISO-8859-1",
	"ascii":      "US-ASCII",
	"win1252":    "windows-1252",
	"cp1252":     "windows-1252",
	"win1251":    "windows-1251",
	"cp1251":     "windows-1251",
	"cp936":      "GBK",
	"gb2312":     "GBK",
	"cp949":      "EUC-KR",
	"cp950":      "Big5",
	"shiftjis":   "Shift_JIS",
	"shift-jis":  "Shift_JIS",
	"sjis":       "Shift_JIS",
	"cp932":      "Shift_JIS",
	"eucjp":      "EUC-JP",
	"euc-jp":     "EUC-JP",
	"iso88591":   "ISO-8859-1",
	"iso-8859-1": "ISO-8859-1",
}

// bomEncodings are the encodings for which iconv-lite writes a byte order mark
// by default when encoding.
var bomEncodings = map[string]bool{
	"UTF-16LE": true,
	"UTF-16BE": true,
}

// lookupEncoding resolves an encoding name to a codec. An empty name means
// [EncodingDisabled] and yields a nil codec.
func lookupEncoding(name string) (*codec, error) {
	if name == EncodingDisabled {
		return nil, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(name))
	canonical, ok := encodingAliases[normalized]
	if !ok {
		canonical = name
	}

	if canonical == "UTF-8" {
		return &codec{name: "UTF-8", identity: true}, nil
	}

	enc, err := ianaindex.IANA.Encoding(canonical)
	if err != nil || enc == nil {
		return nil, fmt.Errorf("unsupported encoding: %s", name)
	}

	// UTF-16 variants must not consume or emit a BOM implicitly; the BOM is
	// managed explicitly so that behaviour matches iconv-lite's `addBOM`.
	switch canonical {
	case "UTF-16LE":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "UTF-16BE":
		enc = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	}

	return &codec{name: canonical, enc: enc, addBOM: bomEncodings[canonical]}, nil
}

// decode converts on-disk bytes into the UTF-8 bytes stored on a vinyl file.
func (c *codec) decode(b []byte) ([]byte, error) {
	if c == nil || c.identity {
		return b, nil
	}
	out, _, err := transform.Bytes(c.enc.NewDecoder(), stripEncodingBOM(c, b))
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", c.name, err)
	}
	return out, nil
}

// encode converts the UTF-8 bytes stored on a vinyl file into on-disk bytes.
func (c *codec) encode(b []byte) ([]byte, error) {
	if c == nil || c.identity {
		return b, nil
	}
	out, _, err := transform.Bytes(c.enc.NewEncoder(), b)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", c.name, err)
	}
	if c.addBOM {
		out = append(encodedBOM(c), out...)
	}
	return out, nil
}

// decodeReader wraps r so that it yields UTF-8 bytes.
func (c *codec) decodeReader(r io.Reader) io.Reader {
	if c == nil || c.identity {
		return r
	}
	return transform.NewReader(r, c.enc.NewDecoder())
}

// encodeReader wraps r, which yields UTF-8 bytes, so that it produces on-disk
// bytes in this encoding.
func (c *codec) encodeReader(r io.Reader) io.Reader {
	if c == nil || c.identity {
		return r
	}
	encoded := transform.NewReader(r, c.enc.NewEncoder())
	if !c.addBOM {
		return encoded
	}
	return io.MultiReader(bytes.NewReader(encodedBOM(c)), encoded)
}

// encodedBOM returns the byte order mark for a BOM-carrying encoding.
func encodedBOM(c *codec) []byte {
	switch c.name {
	case "UTF-16LE":
		return []byte{0xFF, 0xFE}
	case "UTF-16BE":
		return []byte{0xFE, 0xFF}
	default:
		return nil
	}
}

// stripEncodingBOM removes a leading byte order mark before decoding, matching
// iconv-lite's `stripBOM` default.
func stripEncodingBOM(c *codec, b []byte) []byte {
	bom := encodedBOM(c)
	if len(bom) > 0 && bytes.HasPrefix(b, bom) {
		return b[len(bom):]
	}
	return b
}

// removeUTF8BOM strips a leading UTF-8 byte order mark, but only when the
// remaining bytes really are UTF-8. This is a port of the `remove-bom-buffer`
// package, which guards the strip with an `is-utf8` check so that binary files
// that happen to begin with EF BB BF are left untouched.
func removeUTF8BOM(b []byte) []byte {
	if !bytes.HasPrefix(b, utf8BOM) {
		return b
	}
	rest := b[len(utf8BOM):]
	if !utf8.Valid(rest) {
		return b
	}
	return rest
}

// bomStripReader lazily removes a UTF-8 byte order mark from a stream. It is
// the port of `remove-bom-stream`, which buffers only the first few bytes so
// that large files are still streamed.
type bomStripReader struct {
	src     io.Reader
	checked bool
	pending []byte
	err     error
}

// newBOMStripReader returns a reader that yields src with any leading UTF-8 BOM
// removed.
func newBOMStripReader(src io.Reader) io.Reader {
	return &bomStripReader{src: src}
}

func (r *bomStripReader) Read(p []byte) (int, error) {
	if !r.checked {
		r.check()
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	return r.src.Read(p)
}

// check reads just enough of the source to decide whether a BOM is present.
//
// `remove-bom-stream` inspects the first chunk and only strips the BOM when the
// leading bytes form valid UTF-8. Reading a small prefix is sufficient: a BOM
// followed by an incomplete rune would be resolved by later bytes, so the
// prefix check tolerates a truncated final rune.
func (r *bomStripReader) check() {
	r.checked = true

	const probe = 4096
	buf := make([]byte, probe)
	n, err := io.ReadFull(r.src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		r.err = err
	} else if err == io.EOF || err == io.ErrUnexpectedEOF {
		// The whole source fit in the probe, so no trailing bytes remain.
		r.err = io.EOF
	}
	buf = buf[:n]

	if bytes.HasPrefix(buf, utf8BOM) && validUTF8Prefix(buf[len(utf8BOM):]) {
		buf = buf[len(utf8BOM):]
	}
	r.pending = buf
}

// validUTF8Prefix reports whether b is valid UTF-8, ignoring a trailing rune
// that was cut short by the end of the probe buffer.
func validUTF8Prefix(b []byte) bool {
	for len(b) > 0 {
		rn, size := utf8.DecodeRune(b)
		if rn == utf8.RuneError && size <= 1 {
			// The probe stops at a fixed byte count, so it can end in the
			// middle of a rune. That tail is acceptable; genuine garbage is
			// not. Requiring a real start byte is what separates them:
			// length alone would accept 0xFF, which no UTF-8 sequence can
			// begin with, and the file would be misread as UTF-8.
			need := runeLen(b[0])
			return need > 1 && len(b) < need
		}
		b = b[size:]
	}
	return true
}

// runeLen reports how many bytes the rune starting with c occupies, or 0 when
// c cannot start one. The excluded ranges are the ones UTF-8 forbids outright:
// 0x80-0xC1 are continuation bytes or overlong two-byte forms, and 0xF5-0xFF
// would encode a code point above U+10FFFF.
func runeLen(c byte) int {
	switch {
	case c < 0x80:
		return 1
	case c >= 0xC2 && c < 0xE0:
		return 2
	case c >= 0xE0 && c < 0xF0:
		return 3
	case c >= 0xF0 && c <= 0xF4:
		return 4
	default:
		return 0
	}
}
