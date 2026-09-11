package vinyl

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func abs(parts ...string) string {
	return string(filepath.Separator) + filepath.Join(parts...)
}

func sample(t *testing.T) *File {
	t.Helper()
	f, err := New(Options{
		Cwd:      abs("project"),
		Base:     abs("project", "src"),
		Path:     abs("project", "src", "js", "app.js"),
		Contents: Buffer("hello"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func TestNewDefaultsCwdToWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	f, err := New(Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if f.Cwd() != wd {
		t.Fatalf("cwd = %q, want %q", f.Cwd(), wd)
	}
}

func TestNewNormalizesPaths(t *testing.T) {
	f, err := New(Options{
		Cwd:  abs("project") + "/",
		Base: abs("project", "src") + "/",
		Path: abs("project", "src") + "/js/../js/app.js",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := f.Cwd(), abs("project"); got != want {
		t.Errorf("cwd = %q, want %q", got, want)
	}
	if got, want := f.Base(), abs("project", "src"); got != want {
		t.Errorf("base = %q, want %q", got, want)
	}
	if got, want := f.Path(), abs("project", "src", "js", "app.js"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestNewRejectsEmptyHistoryEntries(t *testing.T) {
	if _, err := New(Options{History: []string{""}}); !errors.Is(err, ErrPathNotString) {
		t.Fatalf("err = %v, want %v", err, ErrPathNotString)
	}
}

func TestNewAppendsPathToHistoryOnlyWhenItDiffers(t *testing.T) {
	p := abs("a", "b.js")
	f, err := New(Options{History: []string{p}, Path: p})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := f.History(); len(got) != 1 {
		t.Fatalf("history = %v, want one entry", got)
	}
}

func TestBaseFallsBackToCwd(t *testing.T) {
	f, err := New(Options{Cwd: abs("project")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := f.Base(), abs("project"); got != want {
		t.Fatalf("base = %q, want %q", got, want)
	}
}

func TestSetBaseEmptyResetsToCwd(t *testing.T) {
	f := sample(t)
	if err := f.SetBase(""); err != nil {
		t.Fatalf("SetBase: %v", err)
	}
	if got, want := f.Base(), f.Cwd(); got != want {
		t.Fatalf("base = %q, want %q", got, want)
	}
}

func TestSetCwdRejectsEmpty(t *testing.T) {
	f := sample(t)
	if err := f.SetCwd(""); !errors.Is(err, ErrCwdNotString) {
		t.Fatalf("err = %v, want %v", err, ErrCwdNotString)
	}
}

func TestSetPathRecordsHistory(t *testing.T) {
	f := sample(t)
	renamed := abs("project", "src", "js", "app.min.js")
	if err := f.SetPath(renamed); err != nil {
		t.Fatalf("SetPath: %v", err)
	}
	history := f.History()
	if len(history) != 2 || history[1] != renamed {
		t.Fatalf("history = %v", history)
	}
	if err := f.SetPath(renamed); err != nil {
		t.Fatalf("SetPath: %v", err)
	}
	if got := f.History(); len(got) != 2 {
		t.Fatalf("repeated assignment grew history to %v", got)
	}
	if err := f.SetPath(""); !errors.Is(err, ErrPathNotString) {
		t.Fatalf("err = %v, want %v", err, ErrPathNotString)
	}
}

func TestHistoryIsACopy(t *testing.T) {
	f := sample(t)
	got := f.History()
	got[0] = "mutated"
	if f.History()[0] == "mutated" {
		t.Fatal("History exposed the internal slice")
	}
}

func TestRelative(t *testing.T) {
	f := sample(t)
	rel, err := f.Relative()
	if err != nil {
		t.Fatalf("Relative: %v", err)
	}
	if want := filepath.Join("js", "app.js"); rel != want {
		t.Fatalf("relative = %q, want %q", rel, want)
	}
}

func TestRelativeErrors(t *testing.T) {
	noPath, err := New(Options{Cwd: abs("project")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := noPath.Relative(); !errors.Is(err, ErrNoPath) {
		t.Fatalf("err = %v, want %v", err, ErrNoPath)
	}
}

func TestPathParts(t *testing.T) {
	f := sample(t)
	if got, want := f.Dirname(), abs("project", "src", "js"); got != want {
		t.Errorf("dirname = %q, want %q", got, want)
	}
	if got, want := f.Basename(), "app.js"; got != want {
		t.Errorf("basename = %q, want %q", got, want)
	}
	if got, want := f.Extname(), ".js"; got != want {
		t.Errorf("extname = %q, want %q", got, want)
	}
	if got, want := f.Stem(), "app"; got != want {
		t.Errorf("stem = %q, want %q", got, want)
	}
}

func TestSetPathParts(t *testing.T) {
	t.Run("dirname", func(t *testing.T) {
		f := sample(t)
		if err := f.SetDirname(abs("project", "src", "css")); err != nil {
			t.Fatalf("SetDirname: %v", err)
		}
		if got, want := f.Path(), abs("project", "src", "css", "app.js"); got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	})
	t.Run("basename", func(t *testing.T) {
		f := sample(t)
		if err := f.SetBasename("main.ts"); err != nil {
			t.Fatalf("SetBasename: %v", err)
		}
		if got, want := f.Path(), abs("project", "src", "js", "main.ts"); got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	})
	t.Run("extname", func(t *testing.T) {
		f := sample(t)
		if err := f.SetExtname(".min.js"); err != nil {
			t.Fatalf("SetExtname: %v", err)
		}
		if got, want := f.Path(), abs("project", "src", "js", "app.min.js"); got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	})
	t.Run("stem", func(t *testing.T) {
		f := sample(t)
		if err := f.SetStem("vendor"); err != nil {
			t.Fatalf("SetStem: %v", err)
		}
		if got, want := f.Path(), abs("project", "src", "js", "vendor.js"); got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	})
}

func TestPredicates(t *testing.T) {
	buffered := sample(t)
	if !buffered.IsBuffer() || buffered.IsStream() || buffered.IsNull() {
		t.Error("buffered file misclassified")
	}

	streamed := sample(t)
	streamed.Contents = NewStreamFromBytes([]byte("hello"))
	if !streamed.IsStream() || streamed.IsBuffer() || streamed.IsNull() {
		t.Error("streamed file misclassified")
	}

	null := sample(t)
	null.Contents = nil
	if !null.IsNull() || null.IsBuffer() || null.IsStream() {
		t.Error("null file misclassified")
	}

	dir := sample(t)
	dir.Contents = nil
	dir.Stat = &Stat{Dir: true, FileMode: os.ModeDir | 0o755}
	if !dir.IsDirectory() {
		t.Error("directory misclassified")
	}

	link := sample(t)
	link.Contents = nil
	link.Stat = &Stat{Symlink: true, FileMode: os.ModeSymlink | 0o777}
	if !link.IsSymbolic() {
		t.Error("symlink misclassified")
	}
}

func TestDirectoryRequiresNullContents(t *testing.T) {
	f := sample(t)
	f.Stat = &Stat{Dir: true, FileMode: os.ModeDir | 0o755}
	if f.IsDirectory() {
		t.Fatal("a file with contents must not report as a directory")
	}
}

func TestSymlinkAccessor(t *testing.T) {
	f := sample(t)
	if err := f.SetSymlink(abs("project", "src", "js") + "/"); err != nil {
		t.Fatalf("SetSymlink: %v", err)
	}
	if got, want := f.Symlink(), abs("project", "src", "js"); got != want {
		t.Fatalf("symlink = %q, want %q", got, want)
	}
	if err := f.SetSymlink(""); err != nil {
		t.Fatalf("SetSymlink: %v", err)
	}
	if f.Symlink() != "" {
		t.Fatal("empty assignment did not clear the symlink")
	}
}

func TestBytes(t *testing.T) {
	t.Run("buffer", func(t *testing.T) {
		got, err := sample(t).Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("bytes = %q", got)
		}
	})
	t.Run("stream", func(t *testing.T) {
		f := sample(t)
		f.Contents = NewStreamFromBytes([]byte("streamed"))
		got, err := f.Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if string(got) != "streamed" {
			t.Fatalf("bytes = %q", got)
		}
	})
	t.Run("null", func(t *testing.T) {
		f := sample(t)
		f.Contents = nil
		got, err := f.Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if got != nil {
			t.Fatalf("bytes = %q, want nil", got)
		}
	})
}

func TestCustomProperties(t *testing.T) {
	f := sample(t)
	if _, ok := f.Get("sourceMap"); ok {
		t.Fatal("unset property reported as present")
	}
	f.Set("coverage", 42)
	value, ok := f.Get("coverage")
	if !ok || value != 42 {
		t.Fatalf("Get = %v, %v", value, ok)
	}
	f.Custom()["coverage"] = 0
	if value, _ := f.Get("coverage"); value != 42 {
		t.Fatal("Custom exposed the internal map")
	}
}

func TestIsCustomProp(t *testing.T) {
	reserved := []string{"_contents", "_symlink", "contents", "cwd", "base", "stat", "history", "path", "symlink", "_isVinyl"}
	for _, name := range reserved {
		if IsCustomProp(name) {
			t.Errorf("%q reported as a custom property", name)
		}
	}
	for _, name := range []string{"sourceMap", "coverage", ""} {
		if !IsCustomProp(name) {
			t.Errorf("%q reported as reserved", name)
		}
	}
}

func TestIsVinyl(t *testing.T) {
	if !IsVinyl(sample(t)) {
		t.Error("a *File is a vinyl file")
	}
	if IsVinyl(nil) || IsVinyl("path") || IsVinyl(struct{}{}) {
		t.Error("non-vinyl values misclassified")
	}
}

func TestCloneCopiesMetadata(t *testing.T) {
	f := sample(t)
	f.Set("coverage", 42)
	f.Stat = &Stat{FileMode: 0o644, ByteSize: 5, MTime: time.Unix(1426000001, 0)}
	if err := f.SetPath(abs("project", "src", "js", "app.min.js")); err != nil {
		t.Fatalf("SetPath: %v", err)
	}

	clone := f.Clone()
	if clone.Cwd() != f.Cwd() || clone.Base() != f.Base() || clone.Path() != f.Path() {
		t.Fatal("clone lost its location")
	}
	if len(clone.History()) != len(f.History()) {
		t.Fatalf("clone history = %v, want %v", clone.History(), f.History())
	}
	if value, _ := clone.Get("coverage"); value != 42 {
		t.Fatal("clone lost its custom properties")
	}
	if clone.Stat == f.Stat {
		t.Fatal("clone shares the original stat")
	}
	if clone.Stat.FileMode != f.Stat.FileMode || !clone.Stat.MTime.Equal(f.Stat.MTime) {
		t.Fatal("clone stat differs")
	}

	clone.Set("coverage", 0)
	if value, _ := f.Get("coverage"); value != 42 {
		t.Fatal("clone shares the custom property map")
	}
	if err := clone.SetPath(abs("elsewhere.js")); err != nil {
		t.Fatalf("SetPath: %v", err)
	}
	if f.Path() == clone.Path() {
		t.Fatal("clone shares the history slice")
	}
}

func TestCloneDuplicatesBufferByDefault(t *testing.T) {
	f := sample(t)
	clone := f.Clone()
	original, ok := f.Contents.(Buffer)
	if !ok {
		t.Fatal("expected a buffer")
	}
	copied, ok := clone.Contents.(Buffer)
	if !ok {
		t.Fatal("clone lost its buffer")
	}
	if &original[0] == &copied[0] {
		t.Fatal("clone shares the buffer backing array")
	}
	copied[0] = 'H'
	if string(original) != "hello" {
		t.Fatalf("original mutated to %q", original)
	}
}

func TestCloneSharesBufferWhenContentsIsFalse(t *testing.T) {
	f := sample(t)
	no := false
	clone := f.Clone(CloneOptions{Contents: &no})

	original, ok := f.Contents.(Buffer)
	if !ok {
		t.Fatal("expected a buffer")
	}
	shared, ok := clone.Contents.(Buffer)
	if !ok {
		t.Fatal("clone lost its buffer")
	}
	if &original[0] != &shared[0] {
		t.Fatal("contents:false must reuse the buffer rather than copy it")
	}
}

func TestCloneStreamsIndependently(t *testing.T) {
	f := sample(t)
	f.Contents = NewStream(func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("streamed")), nil
	})
	clone := f.Clone()

	first, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	second, err := clone.Bytes()
	if err != nil {
		t.Fatalf("clone Bytes: %v", err)
	}
	if string(first) != "streamed" || string(second) != "streamed" {
		t.Fatalf("first = %q, second = %q", first, second)
	}
}

func TestString(t *testing.T) {
	f := sample(t)
	if got, want := f.String(), "<File "+filepath.Join("js", "app.js")+" <Buffer>>"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	f.Contents = NewStreamFromBytes(nil)
	if !strings.Contains(f.String(), "<Stream>") {
		t.Fatalf("String() = %q", f.String())
	}
	f.Contents = nil
	if strings.Contains(f.String(), "<Buffer>") || strings.Contains(f.String(), "<Stream>") {
		t.Fatalf("String() = %q", f.String())
	}
}

func TestStreamReadsOnce(t *testing.T) {
	opened := 0
	s := NewStream(func() (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader("payload")), nil
	})

	first, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(first) != "payload" {
		t.Fatalf("read %q", first)
	}
	second, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("a drained stream yielded %q", second)
	}
	if opened != 1 {
		t.Fatalf("opened %d times, want 1", opened)
	}
}

func TestStreamResetRearms(t *testing.T) {
	s := NewStreamFromBytes([]byte("payload"))
	if _, err := io.ReadAll(s); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := s.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	again, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(again) != "payload" {
		t.Fatalf("after Reset read %q", again)
	}
}

func TestStreamCloseIsFinal(t *testing.T) {
	s := NewStreamFromBytes([]byte("payload"))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.Read(make([]byte, 4)); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("err = %v, want %v", err, ErrStreamClosed)
	}
}

func TestStreamPropagatesOpenErrors(t *testing.T) {
	sentinel := errors.New("cannot open")
	s := NewStream(func() (io.ReadCloser, error) { return nil, sentinel })
	if _, err := s.Bytes(); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestStatFromFileInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	stat := StatFromFileInfo(info)
	if stat.Name() != "example.txt" {
		t.Errorf("name = %q", stat.Name())
	}
	if stat.Size() != int64(len("payload")) {
		t.Errorf("size = %d", stat.Size())
	}
	if stat.Perm() != 0o640 {
		t.Errorf("perm = %v", stat.Perm())
	}
	if stat.IsDir() || stat.IsSymbolic() {
		t.Error("a regular file reported as a directory or symlink")
	}
	if !stat.ModTime().Equal(info.ModTime()) {
		t.Errorf("modtime = %v, want %v", stat.ModTime(), info.ModTime())
	}

	clone := stat.Clone()
	clone.ByteSize = 0
	if stat.ByteSize == 0 {
		t.Error("Clone aliased the original")
	}
}

func TestStatFromDirectoryAndSymlink(t *testing.T) {
	dir := t.TempDir()
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if !StatFromFileInfo(info).IsDir() {
		t.Error("directory not detected")
	}

	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if !StatFromFileInfo(linkInfo).IsSymbolic() {
		t.Error("symlink not detected")
	}
}

func TestSourceMapClone(t *testing.T) {
	original := &SourceMap{
		Version:        3,
		File:           "app.js",
		Names:          []string{"a"},
		Mappings:       "AAAA",
		Sources:        []string{"app.js"},
		SourcesContent: []string{"var a = 1;"},
	}
	clone := original.Clone()
	clone.Names[0] = "b"
	if original.Names[0] != "a" {
		t.Fatal("Clone shared the names slice")
	}
	clone.Sources[0] = "other.js"
	if original.Sources[0] != "app.js" {
		t.Fatal("Clone shared the sources slice")
	}
}

func TestBufferContentsRoundTrip(t *testing.T) {
	body := []byte("some bytes")
	f, err := New(Options{Path: abs("a.txt"), Contents: Buffer(body)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("bytes = %q, want %q", got, body)
	}
}
