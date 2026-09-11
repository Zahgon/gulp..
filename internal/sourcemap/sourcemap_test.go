package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gulpjs/gulp-go/vinyl"
)

func jsFile(t *testing.T, rel, body string) *vinyl.File {
	t.Helper()
	base := t.TempDir()
	f, err := vinyl.New(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, rel),
		Contents: vinyl.Buffer(body),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}
	return f
}

func body(t *testing.T, f *vinyl.File) string {
	t.Helper()
	b, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return string(b)
}

func TestAddCreatesAnIdentityMap(t *testing.T) {
	f := jsFile(t, "app.js", "var a = 1;\n")

	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if f.SourceMap == nil {
		t.Fatal("Add did not attach a source map")
	}
	if f.SourceMap.Version != 3 {
		t.Fatalf("Version = %d, want 3", f.SourceMap.Version)
	}
	if f.SourceMap.File != "app.js" {
		t.Fatalf("File = %q, want app.js", f.SourceMap.File)
	}
	if len(f.SourceMap.Sources) != 1 || f.SourceMap.Sources[0] != "app.js" {
		t.Fatalf("Sources = %v, want [app.js]", f.SourceMap.Sources)
	}
	if len(f.SourceMap.SourcesContent) != 1 || f.SourceMap.SourcesContent[0] != "var a = 1;\n" {
		t.Fatalf("SourcesContent = %v", f.SourceMap.SourcesContent)
	}
	if f.SourceMap.Mappings != "" {
		t.Fatalf("Mappings = %q, want empty for an identity map", f.SourceMap.Mappings)
	}
	if f.SourceMap.Preexisting {
		t.Fatal("an identity map must not be marked pre-existing")
	}
}

func TestAddIsIdempotent(t *testing.T) {
	f := jsFile(t, "app.js", "var a = 1;")
	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	first := f.SourceMap

	if err := Add(f); err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if f.SourceMap != first {
		t.Fatal("Add replaced an existing source map")
	}
}

func TestAddIgnoresNullAndDirectoryFiles(t *testing.T) {
	base := t.TempDir()
	null := vinyl.MustNew(vinyl.Options{Cwd: base, Base: base, Path: filepath.Join(base, "a.js")})

	if err := Add(null); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if null.SourceMap != nil {
		t.Fatal("a null file must not gain a source map")
	}
}

func TestAddRejectsStreamingContents(t *testing.T) {
	base := t.TempDir()
	f := vinyl.MustNew(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "a.js"),
		Contents: vinyl.NewStreamFromBytes([]byte("var a = 1;")),
	})

	if err := Add(f); !errors.Is(err, ErrAddStreaming) {
		t.Fatalf("err = %v, want ErrAddStreaming", err)
	}
}

func TestAddLoadsAnInlineMap(t *testing.T) {
	raw := `{"version":3,"file":"app.js","names":["a"],"mappings":"AAAA","sources":["app.src.js"],"sourcesContent":["let a = 1;"]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	f := jsFile(t, "app.js", "var a=1;\n//# sourceMappingURL=data:application/json;base64,"+encoded+"\n")

	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !f.SourceMap.Preexisting {
		t.Fatal("a loaded map must be marked pre-existing")
	}
	if f.SourceMap.Mappings != "AAAA" {
		t.Fatalf("Mappings = %q, want AAAA", f.SourceMap.Mappings)
	}
	if got := body(t, f); strings.Contains(got, "sourceMappingURL") {
		t.Fatalf("the comment was not stripped: %q", got)
	}
}

func TestAddLoadsAnExternalMap(t *testing.T) {
	base := t.TempDir()
	raw := `{"version":3,"file":"app.js","names":[],"mappings":"AACA","sources":["app.src.js"],"sourcesContent":["let a = 1;"]}`
	if err := os.WriteFile(filepath.Join(base, "app.js.map"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}

	f := vinyl.MustNew(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "app.js"),
		Contents: vinyl.Buffer("var a=1;\n//# sourceMappingURL=app.js.map\n"),
	})

	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if f.SourceMap.Mappings != "AACA" {
		t.Fatalf("Mappings = %q, want AACA", f.SourceMap.Mappings)
	}
	if strings.Contains(body(t, f), "sourceMappingURL") {
		t.Fatal("the comment was not stripped")
	}
}

// A comment pointing at a map that is not on disk is common after a partial
// build; gulp tolerates it and falls back to an identity map.
func TestAddToleratesADanglingExternalMap(t *testing.T) {
	f := jsFile(t, "app.js", "var a=1;\n//# sourceMappingURL=missing.js.map\n")

	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if f.SourceMap == nil {
		t.Fatal("no fallback map was attached")
	}
	if strings.Contains(body(t, f), "sourceMappingURL") {
		t.Fatal("the comment was not stripped")
	}
}

func TestWriteInline(t *testing.T) {
	f := jsFile(t, "app.js", "var a = 1;\n")
	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}

	extra, err := Write(f, "")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if extra != nil {
		t.Fatal("an inline write must not produce a second file")
	}

	got := body(t, f)
	const marker = "//# sourceMappingURL=data:application/json;charset=utf-8;base64,"
	idx := strings.Index(got, marker)
	if idx < 0 {
		t.Fatalf("no inline comment in %q", got)
	}

	encoded := strings.TrimSpace(got[idx+len(marker):])
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(decoded, &parsed); err != nil {
		t.Fatalf("the embedded map is not JSON: %v", err)
	}
	if parsed["file"] != "app.js" {
		t.Fatalf("file = %v, want app.js", parsed["file"])
	}
}

func TestWriteExternal(t *testing.T) {
	f := jsFile(t, filepath.Join("js", "app.js"), "var a = 1;\n")
	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}

	extra, err := Write(f, "maps")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if extra == nil {
		t.Fatal("Write did not produce a map file")
	}

	rel, err := extra.Relative()
	if err != nil {
		t.Fatalf("Relative: %v", err)
	}
	if want := filepath.Join("maps", "js", "app.js.map"); rel != want {
		t.Fatalf("map path = %q, want %q", rel, want)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(body(t, extra)), &parsed); err != nil {
		t.Fatalf("map contents are not JSON: %v", err)
	}

	got := body(t, f)
	if !strings.Contains(got, "//# sourceMappingURL=") {
		t.Fatalf("no comment appended to %q", got)
	}
	// The URL is relative to the file's own directory, so js/app.js has to
	// climb out before descending into maps/.
	if !strings.Contains(got, "../maps/js/app.js.map") {
		t.Fatalf("comment URL is not relative to the file: %q", got)
	}
}

func TestWriteWithoutAMapIsANoOp(t *testing.T) {
	f := jsFile(t, "app.js", "var a = 1;\n")
	before := body(t, f)

	extra, err := Write(f, "maps")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if extra != nil {
		t.Fatal("a file without a map must not produce one")
	}
	if body(t, f) != before {
		t.Fatal("contents were modified")
	}
}

func TestWriteRejectsStreamingContents(t *testing.T) {
	base := t.TempDir()
	f := vinyl.MustNew(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "a.js"),
		Contents: vinyl.Buffer("var a = 1;"),
	})
	if err := Add(f); err != nil {
		t.Fatalf("Add: %v", err)
	}
	f.Contents = vinyl.NewStreamFromBytes([]byte("var a = 1;"))

	if _, err := Write(f, ""); !errors.Is(err, ErrWriteStreaming) {
		t.Fatalf("err = %v, want ErrWriteStreaming", err)
	}
}

func TestAddRejectsNil(t *testing.T) {
	if err := Add(nil); !errors.Is(err, ErrAddNotVinyl) {
		t.Fatalf("err = %v, want ErrAddNotVinyl", err)
	}
	if _, err := Write(nil, ""); !errors.Is(err, ErrWriteNotVinyl) {
		t.Fatalf("err = %v, want ErrWriteNotVinyl", err)
	}
}
