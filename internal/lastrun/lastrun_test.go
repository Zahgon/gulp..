package lastrun

import (
	"sync"
	"testing"
	"time"
)

func TestCaptureAndRead(t *testing.T) {
	r := New()
	key := new(int)
	at := time.Now()

	r.Capture(key, at)

	got, ok := r.LastRun(key, 0)
	if !ok {
		t.Fatal("LastRun reported no capture")
	}
	// Even at zero precision the result is millisecond-resolution, because a
	// JavaScript timestamp is a millisecond count and gulpfiles compare these
	// values against fs.Stats.mtime.
	if got.UnixMilli() != at.UnixMilli() {
		t.Fatalf("got %v, want %v", got, at)
	}
}

func TestUnknownKey(t *testing.T) {
	if _, ok := New().LastRun(new(int), 0); ok {
		t.Fatal("an uncaptured key must report ok=false")
	}
}

// Release is what gulp calls when a task errors, so that lastRun reports the
// task as never having run rather than as having succeeded.
func TestReleaseForgetsTheCapture(t *testing.T) {
	r := New()
	key := new(int)

	r.Capture(key, time.Now())
	r.Release(key)

	if _, ok := r.LastRun(key, 0); ok {
		t.Fatal("Release did not clear the capture")
	}
}

func TestReleaseOfUnknownKeyIsSafe(t *testing.T) {
	New().Release(new(int))
}

func TestCaptureOverwrites(t *testing.T) {
	r := New()
	key := new(int)
	first := time.Now()
	second := first.Add(time.Hour)

	r.Capture(key, first)
	r.Capture(key, second)

	got, _ := r.LastRun(key, 0)
	if got.UnixMilli() != second.UnixMilli() {
		t.Fatalf("got %v, want the later capture %v", got, second)
	}
}

func TestLen(t *testing.T) {
	r := New()
	if r.Len() != 0 {
		t.Fatalf("Len = %d, want 0", r.Len())
	}

	a, b := new(int), new(int)
	r.Capture(a, time.Now())
	r.Capture(b, time.Now())
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}

	r.Release(a)
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
}

// The figures come straight from docs/api/last-run.md, which is the contract
// this rounding has to satisfy.
func TestTruncateMatchesDocumentedPrecision(t *testing.T) {
	base := time.UnixMilli(1426000001111)

	cases := []struct {
		precision time.Duration
		want      int64
	}{
		{0, 1426000001111},
		{time.Millisecond, 1426000001111},
		{100 * time.Millisecond, 1426000001100},
		{time.Second, 1426000001000},
		{time.Minute, 1425999960000},
	}

	for _, tc := range cases {
		got := Truncate(base, tc.precision).UnixMilli()
		if got != tc.want {
			t.Errorf("Truncate(%v) = %d, want %d", tc.precision, got, tc.want)
		}
	}
}

func TestTruncateIgnoresNegativePrecision(t *testing.T) {
	at := time.UnixMilli(1426000001111)
	if got := Truncate(at, -time.Second); !got.Equal(at) {
		t.Fatalf("got %v, want the value unchanged", got)
	}
}

func TestLastRunAppliesPrecision(t *testing.T) {
	r := New()
	key := new(int)
	r.Capture(key, time.UnixMilli(1426000001111))

	got, ok := r.LastRun(key, time.Second)
	if !ok {
		t.Fatal("LastRun reported no capture")
	}
	if got.UnixMilli() != 1426000001000 {
		t.Fatalf("got %d, want 1426000001000", got.UnixMilli())
	}
}

func TestConcurrentUse(t *testing.T) {
	r := New()
	keys := make([]*int, 16)
	for i := range keys {
		keys[i] = new(int)
	}

	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(3)
		go func() { defer wg.Done(); r.Capture(key, time.Now()) }()
		go func() { defer wg.Done(); r.LastRun(key, time.Second) }()
		go func() { defer wg.Done(); r.Release(key) }()
	}
	wg.Wait()
}
