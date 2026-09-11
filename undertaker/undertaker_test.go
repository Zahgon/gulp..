package undertaker

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/internal/lastrun"
)

func noop(context.Context) error { return nil }

func TestSetAndGet(t *testing.T) {
	u := New()
	u.Set("build", noop)

	got, ok := u.Get("build")
	if !ok {
		t.Fatal("build should be registered")
	}
	if got.Name != "build" {
		t.Errorf("Name = %q, want build", got.Name)
	}
	if _, ok := u.Get("missing"); ok {
		t.Error("missing should not resolve")
	}
}

// TestSeriesOrdering mirrors the counter assertion in gulp's own watch suite:
// task1 sets the counter to 1, task2 adds 10, so a correct series yields 11.
func TestSeriesOrdering(t *testing.T) {
	u := New()
	var a int64
	u.Set("task1", func(context.Context) error { atomic.StoreInt64(&a, 1); return nil })
	u.Set("task2", func(context.Context) error { atomic.AddInt64(&a, 10); return nil })

	composed := u.Series(Names("task1", "task2")...)
	if err := composed.Fn(context.Background()); err != nil {
		t.Fatalf("series: %v", err)
	}
	if got := atomic.LoadInt64(&a); got != 11 {
		t.Errorf("counter = %d, want 11", got)
	}
}

func TestSeriesStopsAtFirstError(t *testing.T) {
	u := New()
	boom := errors.New("boom")
	var ran []string
	var mu sync.Mutex
	record := func(name string, err error) TaskFunc {
		return func(context.Context) error {
			mu.Lock()
			ran = append(ran, name)
			mu.Unlock()
			return err
		}
	}
	u.Set("a", record("a", nil))
	u.Set("b", record("b", boom))
	u.Set("c", record("c", nil))

	err := u.Series(Names("a", "b", "c")...).Fn(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !reflect.DeepEqual(ran, []string{"a", "b"}) {
		t.Errorf("ran = %v, want [a b] (c must not run)", ran)
	}
}

func TestParallelRunsConcurrentlyAndCancelsOnError(t *testing.T) {
	u := New()
	boom := errors.New("boom")
	var cancelled atomic.Bool

	// The sibling signals that it is already blocked before the other task
	// fails, so the test observes real cancellation rather than the cheaper
	// "context was already done on entry" path.
	started := make(chan struct{})
	u.Set("slow", func(ctx context.Context) error {
		close(started)
		select {
		case <-ctx.Done():
			cancelled.Store(true)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})
	u.Set("fails", func(context.Context) error {
		<-started
		return boom
	})

	start := time.Now()
	err := u.Parallel(Names("slow", "fails")...).Fn(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v; the sibling should have been cancelled promptly", elapsed)
	}
	if !cancelled.Load() {
		t.Error("sibling task should observe context cancellation")
	}
}

// TestForwardReference locks undertaker's deferred name resolution: a
// composition may be built before the tasks it names exist.
func TestForwardReference(t *testing.T) {
	u := New()
	composed := u.Series(Name("later"))

	var ran atomic.Bool
	u.Set("later", func(context.Context) error { ran.Store(true); return nil })

	if err := composed.Fn(context.Background()); err != nil {
		t.Fatalf("series: %v", err)
	}
	if !ran.Load() {
		t.Error("forward-referenced task did not run")
	}
}

func TestUndefinedTask(t *testing.T) {
	u := New()
	err := u.Series(Name("nope")).Fn(context.Background())

	var undef *UndefinedTaskError
	if !errors.As(err, &undef) {
		t.Fatalf("err = %v, want *UndefinedTaskError", err)
	}
	if got, want := err.Error(), "Task never defined: nope"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestRunIsParallel documents the CLI contract from docs/CLI.md: naming
// several tasks runs them concurrently, not in series.
func TestRunIsParallel(t *testing.T) {
	u := New()
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(2)

	for _, name := range []string{"one", "two"} {
		u.Set(name, func(context.Context) error {
			started.Done()
			<-release
			return nil
		})
	}

	done := make(chan error, 1)
	go func() { done <- u.Run(context.Background(), Names("one", "two")...) }()

	waited := make(chan struct{})
	go func() { started.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("both tasks should start before either finishes")
	}
	close(release)

	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestLastRun(t *testing.T) {
	u := New()
	u.Set("build", noop)

	if _, ok, err := u.LastRun(Name("build"), 0); err != nil || ok {
		t.Fatalf("before any run: ok = %v, err = %v; want false, nil", ok, err)
	}

	before := time.Now()
	if err := u.Run(context.Background(), Name("build")); err != nil {
		t.Fatal(err)
	}
	at, ok, err := u.LastRun(Name("build"), 0)
	if err != nil || !ok {
		t.Fatalf("after a run: ok = %v, err = %v; want true, nil", ok, err)
	}
	if at.Before(before.Truncate(time.Millisecond)) {
		t.Errorf("timestamp %v predates the run start %v", at, before)
	}
}

// TestLastRunClearedOnError locks the documented rule that a failed task
// reports no lastRun, so the next incremental build cannot skip work.
func TestLastRunClearedOnError(t *testing.T) {
	u := New()
	fail := errors.New("nope")
	var shouldFail atomic.Bool
	u.Set("build", func(context.Context) error {
		if shouldFail.Load() {
			return fail
		}
		return nil
	})

	if err := u.Run(context.Background(), Name("build")); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := u.LastRun(Name("build"), 0); !ok {
		t.Fatal("expected a timestamp after the successful run")
	}

	shouldFail.Store(true)
	if err := u.Run(context.Background(), Name("build")); !errors.Is(err, fail) {
		t.Fatalf("err = %v, want fail", err)
	}
	if _, ok, _ := u.LastRun(Name("build"), 0); ok {
		t.Error("a failed run must clear lastRun")
	}
}

func TestLastRunRejectsNonTask(t *testing.T) {
	u := New()
	if _, _, err := u.LastRun(nil, 0); !errors.Is(err, ErrNotATask) {
		t.Errorf("err = %v, want ErrNotATask", err)
	}
}

// TestLastRunPrecision uses the exact figures from docs/api/last-run.md.
func TestLastRunPrecision(t *testing.T) {
	base := time.UnixMilli(1426000001111)
	tests := []struct {
		precision time.Duration
		want      int64
	}{
		{0, 1426000001111},
		{100 * time.Millisecond, 1426000001100},
		{time.Second, 1426000001000},
	}
	for _, tt := range tests {
		if got := lastrun.Truncate(base, tt.precision).UnixMilli(); got != tt.want {
			t.Errorf("Truncate(_, %v) = %d, want %d", tt.precision, got, tt.want)
		}
	}
}

func TestTreeShallow(t *testing.T) {
	u := New()
	u.Set("one", noop)
	u.Set("two", noop)

	tree := u.Tree(false)
	if tree.Label != "Tasks" {
		t.Errorf("root label = %q, want Tasks", tree.Label)
	}
	got := make([]string, len(tree.Nodes))
	for i, n := range tree.Nodes {
		got[i] = n.Label
		if len(n.Nodes) != 0 {
			t.Errorf("shallow tree node %q should have no children", n.Label)
		}
	}
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("nodes = %v, want [one two]", got)
	}
}

// TestTreeDeep checks the nesting shape docs/api/tree.md specifies: a
// registered task whose body is a composition shows the <series> node beneath
// its own name, with the children below that.
func TestTreeDeep(t *testing.T) {
	u := New()
	u.Set("one", noop)
	u.Set("two", noop)
	if _, err := u.SetTask("build", u.Series(Names("one", "two")...)); err != nil {
		t.Fatalf("SetTask: %v", err)
	}

	tree := u.Tree(true)
	var buildNode *Node
	for _, n := range tree.Nodes {
		if n.Label == "build" {
			buildNode = n
		}
	}
	if buildNode == nil {
		t.Fatal("build node missing")
	}
	if buildNode.Type != "task" {
		t.Errorf("build type = %q, want task", buildNode.Type)
	}
	if len(buildNode.Nodes) != 1 || buildNode.Nodes[0].Label != LabelSeries {
		t.Fatalf("build children = %+v, want a single <series> node", buildNode.Nodes)
	}
	series := buildNode.Nodes[0]
	if !series.Branch {
		t.Error("<series> node should be marked as a branch")
	}
	labels := make([]string, len(series.Nodes))
	for i, n := range series.Nodes {
		labels[i] = n.Label
	}
	if !reflect.DeepEqual(labels, []string{"one", "two"}) {
		t.Errorf("series children = %v, want [one two]", labels)
	}
}

func TestTreeAnonymous(t *testing.T) {
	u := New()
	if _, err := u.SetTask("build", u.Parallel(Anonymous(noop))); err != nil {
		t.Fatalf("SetTask: %v", err)
	}

	tree := u.Tree(true)
	parallel := tree.Nodes[0].Nodes[0]
	if parallel.Label != LabelParallel {
		t.Fatalf("label = %q, want %s", parallel.Label, LabelParallel)
	}
	if parallel.Nodes[0].Label != LabelAnonymous {
		t.Errorf("child label = %q, want %s", parallel.Nodes[0].Label, LabelAnonymous)
	}
	if parallel.Nodes[0].Type != "function" {
		t.Errorf("child type = %q, want function", parallel.Nodes[0].Type)
	}
}

// countingRegistry is a custom registry used to verify the transfer contract.
type countingRegistry struct {
	*DefaultRegistry
	inits int
}

func (r *countingRegistry) Init(*Undertaker) { r.inits++ }

func TestSetRegistryTransfersTasks(t *testing.T) {
	u := New()
	u.Set("one", noop)
	u.Set("two", noop)

	custom := &countingRegistry{DefaultRegistry: NewDefaultRegistry()}
	if err := u.SetRegistry(custom); err != nil {
		t.Fatalf("SetRegistry: %v", err)
	}
	if custom.inits != 1 {
		t.Errorf("Init called %d times, want 1", custom.inits)
	}
	for _, name := range []string{"one", "two"} {
		if _, ok := u.Get(name); !ok {
			t.Errorf("%q was not transferred to the new registry", name)
		}
	}
	if err := u.SetRegistry(nil); !errors.Is(err, ErrNilRegistry) {
		t.Errorf("SetRegistry(nil) = %v, want ErrNilRegistry", err)
	}
}

// TestEvents checks the lifecycle stream the CLI logger consumes, including
// the Branch flag it uses to hide synthetic composition nodes.
func TestEvents(t *testing.T) {
	u := New()
	u.Set("ok", noop)
	u.Set("bad", func(context.Context) error { return errors.New("x") })

	var mu sync.Mutex
	var events []Event
	u.On(func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	_ = u.Run(context.Background(), Name("ok"))
	_ = u.Run(context.Background(), Name("bad"))

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if events[0].Kind != EventStart || events[0].Name != "ok" {
		t.Errorf("events[0] = %+v, want start of ok", events[0])
	}
	if events[1].Kind != EventStop || events[1].Duration < 0 {
		t.Errorf("events[1] = %+v, want stop of ok", events[1])
	}
	if events[3].Kind != EventError || events[3].Err == nil {
		t.Errorf("events[3] = %+v, want error of bad", events[3])
	}
}

func TestEventsMarkBranches(t *testing.T) {
	u := New()
	u.Set("a", noop)

	var mu sync.Mutex
	branches := map[string]bool{}
	u.On(func(e Event) {
		mu.Lock()
		if e.Kind == EventStart {
			branches[e.Name] = e.Branch
		}
		mu.Unlock()
	})

	if _, err := u.SetTask("build", u.Series(Name("a"))); err != nil {
		t.Fatalf("SetTask: %v", err)
	}
	if err := u.Run(context.Background(), Name("build")); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if branches["build"] {
		t.Error("a registered task must not be flagged as a branch")
	}
	if !branches[LabelSeries] {
		t.Error("<series> must be flagged as a branch so the CLI can hide it")
	}
}

func TestConcurrentRegistrationIsRaceFree(t *testing.T) {
	u := New()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := string(rune('a' + n%26))
			u.Set(name, noop)
			u.Get(name)
			u.Tasks()
			u.Tree(true)
		}(i)
	}
	wg.Wait()
}
