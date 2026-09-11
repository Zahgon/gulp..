package vinyl

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestContentsIsASealedInterface pins the contract that only this package can
// supply a contents shape. The marker method is unexported, so a third package
// cannot satisfy Contents no matter what it declares, and every consumer can
// therefore switch on Buffer and *Stream exhaustively.
func TestContentsIsASealedInterface(t *testing.T) {
	var buffer Contents = Buffer("hello")
	var stream Contents = NewStreamFromBytes([]byte("hello"))

	buffer.contents()
	stream.contents()

	if _, ok := buffer.(Buffer); !ok {
		t.Error("Buffer does not satisfy Contents as itself")
	}
	if _, ok := stream.(*Stream); !ok {
		t.Error("*Stream does not satisfy Contents as itself")
	}
}

func TestMustNewReturnsAFile(t *testing.T) {
	f := MustNew(Options{
		Cwd:      abs("project"),
		Base:     abs("project", "src"),
		Path:     abs("project", "src", "app.js"),
		Contents: Buffer("body"),
	})

	if f.Basename() != "app.js" {
		t.Errorf("Basename() = %q, want app.js", f.Basename())
	}
}

func TestMustNewPanicsOnInvalidOptions(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustNew with an empty history entry did not panic")
		}
	}()

	MustNew(Options{History: []string{""}})
}

func TestValidID(t *testing.T) {
	cases := []struct {
		id   int
		want bool
	}{
		{0, true},
		{501, true},
		{InvalidID, false},
		{-7, false},
	}

	for _, tc := range cases {
		if got := ValidID(tc.id); got != tc.want {
			t.Errorf("ValidID(%d) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestStatModeAndSys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte("body"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}

	stat := StatFromFileInfo(info)

	if stat.Mode().Perm() != 0o640 {
		t.Errorf("Mode().Perm() = %v, want 0640", stat.Mode().Perm())
	}
	if stat.Mode().IsDir() {
		t.Error("Mode() reports a directory for a regular file")
	}

	// Sys always returns nil, even for a stat read from disk. Stat is a value
	// that outlives the os.FileInfo it came from, so the platform block is not
	// retained; what a caller would want from it is lifted into fields
	// instead, which is why UID, GID and ATime exist.
	if stat.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", stat.Sys())
	}
}

func TestStatModeReportsDirectories(t *testing.T) {
	stat := &Stat{Dir: true, FileMode: fs.ModeDir | 0o755}

	if !stat.Mode().IsDir() {
		t.Error("Mode().IsDir() = false for a directory stat")
	}
}

func TestStatSysIsNilWhenSynthesised(t *testing.T) {
	stat := &Stat{FileMode: 0o644, MTime: time.Now()}

	if stat.Sys() != nil {
		t.Error("Sys() returned a value for a hand-built stat")
	}
}

func TestStreamContentsRoundTrip(t *testing.T) {
	stream := NewStreamFromBytes([]byte("hello"))

	body, err := stream.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("Bytes() = %q, want hello", body)
	}

	if err := stream.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	again, err := stream.Bytes()
	if err != nil {
		t.Fatalf("Bytes after Reset: %v", err)
	}
	if string(again) != "hello" {
		t.Errorf("Bytes() after Reset = %q, want hello", again)
	}
}

func TestBufferContentsIsTheBytes(t *testing.T) {
	if got := string(Buffer("hello")); got != "hello" {
		t.Errorf("Buffer = %q, want hello", got)
	}
	if !strings.HasPrefix(string(Buffer("hello world")), "hello") {
		t.Error("Buffer does not behave as a byte slice")
	}
}
