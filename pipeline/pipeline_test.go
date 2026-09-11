package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/vinyl"
)

func file(t *testing.T, name string) *vinyl.File {
	t.Helper()
	f, err := vinyl.New(vinyl.Options{
		Cwd:      "/project",
		Base:     "/project/src",
		Path:     "/project/src/" + name,
		Contents: vinyl.Buffer(name),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}
	return f
}

func files(t *testing.T, names ...string) []*vinyl.File {
	t.Helper()
	out := make([]*vinyl.File, len(names))
	for i, name := range names {
		out[i] = file(t, name)
	}
	return out
}

func names(files []*vinyl.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Basename()
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCollectPreservesOrder(t *testing.T) {
	out, err := New(From(files(t, "a.js", "b.js", "c.js")...)).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	equal(t, names(out), []string{"a.js", "b.js", "c.js"})
}

func TestPipeChainsStages(t *testing.T) {
	out, err := New(From(files(t, "a.js", "b.js")...)).
		Pipe(Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
			return f, f.SetBasename("x-" + f.Basename())
		})).
		Pipe(Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
			return f, f.SetBasename("y-" + f.Basename())
		})).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	equal(t, names(out), []string{"y-x-a.js", "y-x-b.js"})
}

func TestPipeAll(t *testing.T) {
	stages := []Transform{
		Filter(func(f *vinyl.File) bool { return f.Extname() == ".js" }),
		Tap(func(f *vinyl.File) error { return f.SetBasename("ok-" + f.Basename()) }),
	}
	out, err := New(From(files(t, "a.js", "b.css", "c.js")...)).PipeAll(stages...).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	equal(t, names(out), []string{"ok-a.js", "ok-c.js"})
}

func TestMapDroppingNilRemovesTheFile(t *testing.T) {
	out, err := New(From(files(t, "a.js", "b.js")...)).
		Pipe(Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
			if f.Basename() == "a.js" {
				return nil, nil
			}
			return f, nil
		})).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	equal(t, names(out), []string{"b.js"})
}

func TestFlushSeesTheWholeStream(t *testing.T) {
	var seen []string
	out, err := New(From(files(t, "a.js", "b.js", "c.js")...)).
		Pipe(Flush(func(_ context.Context, batch []*vinyl.File) ([]*vinyl.File, error) {
			seen = names(batch)
			return batch[:1], nil
		})).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	equal(t, seen, []string{"a.js", "b.js", "c.js"})
	equal(t, names(out), []string{"a.js"})
}

func TestDiscardConsumesEverything(t *testing.T) {
	out, err := New(From(files(t, "a.js", "b.js")...)).Pipe(Discard()).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("got %v, want nothing", names(out))
	}
}

func TestRunDrainsWithoutCollecting(t *testing.T) {
	var count atomic.Int64
	err := New(From(files(t, "a.js", "b.js", "c.js")...)).
		Pipe(Tap(func(*vinyl.File) error { count.Add(1); return nil })).
		Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count.Load() != 3 {
		t.Fatalf("saw %d files, want 3", count.Load())
	}
}

func TestEach(t *testing.T) {
	var seen []string
	err := New(From(files(t, "a.js", "b.js")...)).Each(context.Background(), func(f *vinyl.File) error {
		seen = append(seen, f.Basename())
		return nil
	})
	if err != nil {
		t.Fatalf("Each: %v", err)
	}
	equal(t, seen, []string{"a.js", "b.js"})
}

func TestEachErrorStopsTheStream(t *testing.T) {
	sentinel := errors.New("stop")
	err := New(From(files(t, "a.js", "b.js", "c.js")...)).Each(context.Background(), func(f *vinyl.File) error {
		if f.Basename() == "b.js" {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestStageErrorPropagates(t *testing.T) {
	sentinel := errors.New("stage failed")
	_, err := New(From(files(t, "a.js")...)).
		Pipe(Map(func(context.Context, *vinyl.File) (*vinyl.File, error) { return nil, sentinel })).
		Collect(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestStageErrorIsAttributedToItsPosition(t *testing.T) {
	sentinel := errors.New("stage failed")
	_, err := New(From(files(t, "a.js")...)).
		Pipe(Tap(func(*vinyl.File) error { return nil })).
		Pipe(Map(func(context.Context, *vinyl.File) (*vinyl.File, error) { return nil, sentinel })).
		Collect(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if want := "stage 2"; !contains(err.Error(), want) {
		t.Fatalf("err = %q, want it to mention %q", err, want)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 ||
		indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestPanicInAStageBecomesAnError(t *testing.T) {
	_, err := New(From(files(t, "a.js")...)).
		Pipe(Tap(func(*vinyl.File) error { panic("boom") })).
		Collect(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !contains(err.Error(), "boom") {
		t.Fatalf("err = %q, want it to mention the panic", err)
	}
}

func TestFailedShortCircuits(t *testing.T) {
	sentinel := errors.New("bad glob")
	p := Failed(sentinel)
	if !errors.Is(p.Err(), sentinel) {
		t.Fatalf("Err = %v, want %v", p.Err(), sentinel)
	}
	if _, err := p.Pipe(Discard()).Collect(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestNothingRunsUntilATerminalCall(t *testing.T) {
	var started atomic.Bool
	source := TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		started.Store(true)
		return Send(ctx, out, file(t, "a.js"))
	})

	p := New(source).Pipe(Discard())
	time.Sleep(20 * time.Millisecond)
	if started.Load() {
		t.Fatal("the source ran before the pipeline was started")
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !started.Load() {
		t.Fatal("the source never ran")
	}
}

func TestCancellationStopsTheStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for i := 0; ; i++ {
			if err := Send(ctx, out, file(t, fmt.Sprintf("f%d.js", i))); err != nil {
				return err
			}
		}
	})

	p := New(source)
	out, errCh := p.Start(ctx)
	<-out
	cancel()
	for range out { //nolint:revive // draining
	}
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled or nil", err)
	}
}

func TestSourceErrorCancelsDownstream(t *testing.T) {
	sentinel := errors.New("source failed")
	source := TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		if err := Send(ctx, out, file(t, "a.js")); err != nil {
			return err
		}
		return sentinel
	})

	var seen atomic.Int64
	err := New(source).
		Pipe(Tap(func(*vinyl.File) error { seen.Add(1); return nil })).
		Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestStagesRunConcurrently(t *testing.T) {
	release := make(chan struct{})
	var reached sync.WaitGroup
	reached.Add(1)

	source := TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		if err := Send(ctx, out, file(t, "a.js")); err != nil {
			return err
		}
		reached.Wait()
		return Send(ctx, out, file(t, "b.js"))
	})

	var once sync.Once
	p := New(source).Pipe(Tap(func(*vinyl.File) error {
		once.Do(func() { reached.Done(); close(release) })
		return nil
	}))

	done := make(chan error, 1)
	go func() { done <- p.Run(context.Background()) }()

	select {
	case <-release:
	case <-time.After(2 * time.Second):
		t.Fatal("the second stage never saw the first file, so stages are not concurrent")
	}
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAsTask(t *testing.T) {
	var count atomic.Int64
	task := New(From(files(t, "a.js", "b.js")...)).
		Pipe(Tap(func(*vinyl.File) error { count.Add(1); return nil })).
		AsTask()
	if err := task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
	if count.Load() != 2 {
		t.Fatalf("saw %d files, want 2", count.Load())
	}
}

func TestSendRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan *vinyl.File)
	if err := Send(ctx, out, file(t, "a.js")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRecv(t *testing.T) {
	in := make(chan *vinyl.File, 1)
	in <- file(t, "a.js")
	close(in)

	f, ok, err := Recv(context.Background(), in)
	if err != nil || !ok || f.Basename() != "a.js" {
		t.Fatalf("Recv = %v, %v, %v", f, ok, err)
	}
	if _, ok, err := Recv(context.Background(), in); err != nil || ok {
		t.Fatalf("Recv on a closed channel = %v, %v", ok, err)
	}
}

func TestRecvRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Recv(ctx, make(chan *vinyl.File)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestBackpressureIsBounded(t *testing.T) {
	produced := make(chan int, 1024)
	source := TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for i := 0; i < bufferSize*4; i++ {
			if err := Send(ctx, out, file(t, fmt.Sprintf("f%d.js", i))); err != nil {
				return err
			}
			produced <- i
		}
		return nil
	})

	gate := make(chan struct{})
	p := New(source).Pipe(Tap(func(*vinyl.File) error {
		<-gate
		return nil
	}))

	done := make(chan error, 1)
	go func() { done <- p.Run(context.Background()) }()

	time.Sleep(100 * time.Millisecond)
	if got := len(produced); got > bufferSize*3 {
		t.Fatalf("the source produced %d files with a blocked consumer, so backpressure is not applied", got)
	}

	close(gate)
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
