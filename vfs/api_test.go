package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

func TestOptionFunc(t *testing.T) {
	opt := Func(func(f *vinyl.File) bool {
		return f.Extname() == ".js"
	})

	if !opt.IsSet() {
		t.Fatal("Func option reports unset")
	}

	js := vinyl.MustNew(vinyl.Options{Cwd: os.TempDir(), Base: os.TempDir(), Path: filepath.Join(os.TempDir(), "a.js")})
	css := vinyl.MustNew(vinyl.Options{Cwd: os.TempDir(), Base: os.TempDir(), Path: filepath.Join(os.TempDir(), "a.css")})

	if got := opt.Resolve(js, false); !got {
		t.Error("Resolve(a.js) = false, want true")
	}
	if got := opt.Resolve(css, true); got {
		t.Error("Resolve(a.css) = true, want false")
	}
}

func TestOptionFuncOK(t *testing.T) {
	// FuncOK lets the callback decline, which is how vinyl-fs distinguishes
	// "this file wants false" from "this file has no opinion". A declining
	// callback must fall through to the caller's default.
	opt := FuncOK(func(f *vinyl.File) (bool, bool) {
		if f.Extname() == ".js" {
			return false, true
		}
		return false, false
	})

	js := vinyl.MustNew(vinyl.Options{Cwd: os.TempDir(), Base: os.TempDir(), Path: filepath.Join(os.TempDir(), "a.js")})
	css := vinyl.MustNew(vinyl.Options{Cwd: os.TempDir(), Base: os.TempDir(), Path: filepath.Join(os.TempDir(), "a.css")})

	if got := opt.Resolve(js, true); got {
		t.Error("Resolve(a.js) = true, want the callback's false")
	}
	if got := opt.Resolve(css, true); !got {
		t.Error("Resolve(a.css) = false, want the default true")
	}
}

func TestOptionValueIgnoresTheFile(t *testing.T) {
	opt := Value(true)
	if !opt.IsSet() {
		t.Fatal("Value option reports unset")
	}
	if got := opt.Resolve(nil, false); !got {
		t.Error("Resolve = false, want true")
	}
}

func TestOptionUnsetUsesTheDefault(t *testing.T) {
	var opt Option[bool]
	if opt.IsSet() {
		t.Fatal("zero Option reports set")
	}
	if got := opt.Resolve(nil, true); !got {
		t.Error("Resolve = false, want the default true")
	}
}

func TestAbsoluteFrom(t *testing.T) {
	cwd := filepath.FromSlash("/project")

	cases := []struct {
		path string
		want string
	}{
		{filepath.FromSlash("/already/absolute"), filepath.FromSlash("/already/absolute")},
		{"relative", filepath.Join(cwd, "relative")},
		{filepath.Join("nested", "path"), filepath.Join(cwd, "nested", "path")},
	}

	for _, tc := range cases {
		if got := absoluteFrom(cwd, tc.path); got != tc.want {
			t.Errorf("absoluteFrom(%q, %q) = %q, want %q", cwd, tc.path, got, tc.want)
		}
	}
}

func TestValidUTF8Prefix(t *testing.T) {
	// The streaming BOM stripper probes a chunk that may end mid-rune, so a
	// truncated trailing sequence has to count as valid or the probe would
	// reject every multi-byte file that happens to straddle the boundary.
	euro := []byte("\u20ac")

	cases := []struct {
		name  string
		input []byte
		want  bool
	}{
		{"empty", nil, true},
		{"ascii", []byte("hello"), true},
		{"complete rune", euro, true},
		{"truncated trailing rune", euro[:2], true},
		{"invalid byte", []byte{0xff, 0xfe, 'a'}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validUTF8Prefix(tc.input); got != tc.want {
				t.Errorf("validUTF8Prefix(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEncodedBOMAndStrip(t *testing.T) {
	utf16, err := lookupEncoding("utf16le")
	if err != nil {
		t.Fatalf("lookupEncoding(utf16le): %v", err)
	}

	bom := encodedBOM(utf16)
	if len(bom) == 0 {
		t.Fatal("encodedBOM(utf16le) is empty, want a byte order mark")
	}

	body := append(append([]byte{}, bom...), 'h', 0)
	stripped := stripEncodingBOM(utf16, body)
	if len(stripped) != len(body)-len(bom) {
		t.Errorf("stripEncodingBOM kept %d bytes, want %d", len(stripped), len(body)-len(bom))
	}

	if again := stripEncodingBOM(utf16, stripped); len(again) != len(stripped) {
		t.Error("stripEncodingBOM removed bytes from content that has no BOM")
	}
}

func TestEncodedBOMIsEmptyForUTF8(t *testing.T) {
	utf8Codec, err := lookupEncoding(DefaultEncoding)
	if err != nil {
		t.Fatalf("lookupEncoding(%s): %v", DefaultEncoding, err)
	}
	if bom := encodedBOM(utf8Codec); len(bom) != 0 {
		t.Errorf("encodedBOM(utf8) = %v, want empty", bom)
	}
}

func TestSinceFilterDropsOlderFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	fresh := filepath.Join(dir, "fresh.txt")

	for _, path := range []string{old, fresh} {
		if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// The cutoff has to sit between the two modification times. Using
	// time.Now() would drop both files, because a file written a moment ago
	// is not modified *after* this instant.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	cutoff := time.Now().Add(-30 * time.Minute)

	collected, err := Src([]string{filepath.Join(dir, "*.txt")}, SrcOptions{
		Cwd:   dir,
		Since: Value(cutoff),
	}).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	if len(collected) != 1 {
		t.Fatalf("collected %d files, want only the fresh one", len(collected))
	}
	if got := filepath.Base(collected[0].Path()); got != "fresh.txt" {
		t.Errorf("kept %s, want fresh.txt", got)
	}
}

func TestSourceMapsReaderAttachesAMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte("var a = 1;\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, err := Src([]string{path}, SrcOptions{
		Cwd:        dir,
		SourceMaps: Value(true),
	}).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("collected %d files, want 1", len(files))
	}
	if files[0].SourceMap == nil {
		t.Fatal("SourceMaps enabled but no map attached")
	}
	if files[0].SourceMap.Version != 3 {
		t.Errorf("map version = %d, want 3", files[0].SourceMap.Version)
	}
}

func TestDestWritesASymbolicFile(t *testing.T) {
	// A vinyl file carrying a symlink property makes dest() create a link
	// rather than write contents. This is the path vinyl-fs takes for files
	// that came from src() with resolveSymlinks disabled.
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("body"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	base := filepath.Join(dir, "src")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	file := vinyl.MustNew(vinyl.Options{
		Cwd:  dir,
		Base: base,
		Path: filepath.Join(base, "link.txt"),
		Stat: &vinyl.Stat{FileMode: os.ModeSymlink | 0o777, Symlink: true},
	})
	if err := file.SetSymlink(target); err != nil {
		t.Fatalf("SetSymlink: %v", err)
	}

	out := filepath.Join(dir, "out")
	if err := pipeline.New(pipeline.From(file)).Pipe(Dest(out, DestOptions{Cwd: dir})).Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	link := filepath.Join(out, "link.txt")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", link)
	}

	resolved, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if resolved != target {
		t.Errorf("link points at %s, want %s", resolved, target)
	}
}
