package vfs

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLookupEncodingResolvesAliases(t *testing.T) {
	cases := []struct {
		name     string
		want     string
		identity bool
		addBOM   bool
	}{
		{"utf8", "UTF-8", true, false},
		{"UTF-8", "UTF-8", true, false},
		{"  Utf8Bom  ", "UTF-8", true, false},
		{"latin1", "ISO-8859-1", false, false},
		{"binary", "ISO-8859-1", false, false},
		{"iso-8859-1", "ISO-8859-1", false, false},
		{"win1252", "windows-1252", false, false},
		{"utf16le", "UTF-16LE", false, true},
		{"ucs2", "UTF-16LE", false, true},
		{"utf16be", "UTF-16BE", false, true},
		{"utf-16", "UTF-16BE", false, true},
		{"shiftjis", "Shift_JIS", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := lookupEncoding(tc.name)
			if err != nil {
				t.Fatalf("lookupEncoding(%q): %v", tc.name, err)
			}
			if c.name != tc.want {
				t.Errorf("name = %q, want %q", c.name, tc.want)
			}
			if c.identity != tc.identity {
				t.Errorf("identity = %v, want %v", c.identity, tc.identity)
			}
			if c.addBOM != tc.addBOM {
				t.Errorf("addBOM = %v, want %v", c.addBOM, tc.addBOM)
			}
		})
	}
}

func TestLookupEncodingDisabledYieldsNoCodec(t *testing.T) {
	c, err := lookupEncoding(EncodingDisabled)
	if err != nil {
		t.Fatalf("lookupEncoding(disabled): %v", err)
	}
	if c != nil {
		t.Errorf("codec = %v, want nil", c)
	}
}

func TestLookupEncodingRejectsUnknownNames(t *testing.T) {
	for _, name := range []string{"not-a-real-encoding", "utf-9"} {
		if _, err := lookupEncoding(name); err == nil {
			t.Errorf("lookupEncoding(%q) succeeded, want an error", name)
		}
	}
}

// codecFor fails the test rather than returning an error, so the round-trip
// cases below stay readable.
func codecFor(t *testing.T, name string) *codec {
	t.Helper()
	c, err := lookupEncoding(name)
	if err != nil {
		t.Fatalf("lookupEncoding(%q): %v", name, err)
	}
	return c
}

func TestCodecRoundTripsThroughDisk(t *testing.T) {
	cases := []struct {
		encoding string
		text     string
		onDisk   []byte
	}{
		{"latin1", "caf\u00e9", []byte{'c', 'a', 'f', 0xe9}},
		{"win1252", "\u20acuro", []byte{0x80, 'u', 'r', 'o'}},
		{"utf16le", "hi", []byte{0xFF, 0xFE, 'h', 0, 'i', 0}},
		{"utf16be", "hi", []byte{0xFE, 0xFF, 0, 'h', 0, 'i'}},
	}

	for _, tc := range cases {
		t.Run(tc.encoding, func(t *testing.T) {
			c := codecFor(t, tc.encoding)

			encoded, err := c.encode([]byte(tc.text))
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !bytes.Equal(encoded, tc.onDisk) {
				t.Errorf("encode = % x, want % x", encoded, tc.onDisk)
			}

			decoded, err := c.decode(tc.onDisk)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(decoded) != tc.text {
				t.Errorf("decode = %q, want %q", decoded, tc.text)
			}
		})
	}
}

func TestCodecDecodesWithoutABOM(t *testing.T) {
	// iconv-lite strips a BOM when one is present but does not require it.
	c := codecFor(t, "utf16le")
	got, err := c.decode([]byte{'h', 0, 'i', 0})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("decode = %q, want %q", got, "hi")
	}
}

func TestCodecPassesThroughWhenNilOrIdentity(t *testing.T) {
	raw := []byte{0xff, 0xfe, 0x00}
	utf8Codec := codecFor(t, DefaultEncoding)

	for name, c := range map[string]*codec{"disabled": nil, "utf8": utf8Codec} {
		t.Run(name, func(t *testing.T) {
			encoded, err := c.encode(raw)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !bytes.Equal(encoded, raw) {
				t.Errorf("encode = % x, want % x", encoded, raw)
			}

			decoded, err := c.decode(raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !bytes.Equal(decoded, raw) {
				t.Errorf("decode = % x, want % x", decoded, raw)
			}

			src := strings.NewReader("body")
			if got := c.encodeReader(src); got != io.Reader(src) {
				t.Error("encodeReader wrapped the reader, want it returned unchanged")
			}
			if got := c.decodeReader(src); got != io.Reader(src) {
				t.Error("decodeReader wrapped the reader, want it returned unchanged")
			}
		})
	}
}

func TestCodecReadersMatchTheBufferedForm(t *testing.T) {
	for _, name := range []string{"latin1", "utf16le", "utf16be"} {
		t.Run(name, func(t *testing.T) {
			c := codecFor(t, name)
			const text = "caf\u00e9"

			want, err := c.encode([]byte(text))
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := io.ReadAll(c.encodeReader(strings.NewReader(text)))
			if err != nil {
				t.Fatalf("encodeReader: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("encodeReader = % x, want % x", got, want)
			}

			back, err := io.ReadAll(c.decodeReader(bytes.NewReader(stripEncodingBOM(c, want))))
			if err != nil {
				t.Fatalf("decodeReader: %v", err)
			}
			if string(back) != text {
				t.Errorf("decodeReader = %q, want %q", back, text)
			}
		})
	}
}

func TestCodecEncodeReportsUnrepresentableRunes(t *testing.T) {
	c := codecFor(t, "ascii")
	if _, err := c.encode([]byte("caf\u00e9")); err == nil {
		t.Error("encode accepted a non-ASCII rune, want an error")
	}
}

func TestRemoveUTF8BOM(t *testing.T) {
	withBOM := func(rest ...byte) []byte {
		return append(append([]byte{}, utf8BOM...), rest...)
	}

	cases := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{"no bom", []byte("hello"), []byte("hello")},
		{"empty", nil, nil},
		{"bom only", withBOM(), []byte{}},
		{"bom then text", withBOM('h', 'i'), []byte("hi")},
		// remove-bom-buffer leaves binary files alone even when their first
		// three bytes happen to spell a BOM.
		{"bom then binary", withBOM(0xff, 0xfe), withBOM(0xff, 0xfe)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := removeUTF8BOM(tc.input); !bytes.Equal(got, tc.want) {
				t.Errorf("removeUTF8BOM(% x) = % x, want % x", tc.input, got, tc.want)
			}
		})
	}
}

func TestRuneLen(t *testing.T) {
	cases := []struct {
		in   byte
		want int
	}{
		{0x00, 1}, {'a', 1}, {0x7f, 1},
		{0x80, 0}, {0xbf, 0}, {0xc0, 0}, {0xc1, 0},
		{0xc2, 2}, {0xdf, 2},
		{0xe0, 3}, {0xef, 3},
		{0xf0, 4}, {0xf4, 4},
		{0xf5, 0}, {0xff, 0},
	}

	for _, tc := range cases {
		if got := runeLen(tc.in); got != tc.want {
			t.Errorf("runeLen(%#02x) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBOMStripReaderStreamsWithoutTheMark(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{"bom then text", append(append([]byte{}, utf8BOM...), []byte("hello")...), []byte("hello")},
		{"no bom", []byte("hello"), []byte("hello")},
		{"empty", nil, nil},
		{"shorter than a bom", []byte{0xef}, []byte{0xef}},
		{"bom then binary", append(append([]byte{}, utf8BOM...), 0xff, 0xfe), append(append([]byte{}, utf8BOM...), 0xff, 0xfe)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := io.ReadAll(newBOMStripReader(bytes.NewReader(tc.input)))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("read = % x, want % x", got, tc.want)
			}
		})
	}
}

// oneByteReader forces the stripper's buffered probe to fill across several
// reads, which is the path a large file on a slow pipe takes.
type oneByteReader struct{ rest []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.rest) == 0 {
		return 0, io.EOF
	}
	p[0] = r.rest[0]
	r.rest = r.rest[1:]
	return 1, nil
}

func TestBOMStripReaderHandlesDribbledInput(t *testing.T) {
	input := append(append([]byte{}, utf8BOM...), []byte("streamed")...)
	got, err := io.ReadAll(newBOMStripReader(&oneByteReader{rest: input}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "streamed" {
		t.Errorf("read = %q, want %q", got, "streamed")
	}
}
