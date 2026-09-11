package undertaker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEventKindString(t *testing.T) {
	cases := []struct {
		kind EventKind
		want string
	}{
		{EventStart, "start"},
		{EventStop, "stop"},
		{EventError, "error"},
		{EventKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestTaskIsBranch(t *testing.T) {
	u := New()
	leaf := u.Set("leaf", func(context.Context) error { return nil })
	if leaf.IsBranch() {
		t.Error("a registered leaf task reports IsBranch")
	}

	composed := u.Series(Name("leaf"))
	if !composed.IsBranch() {
		t.Error("a composition does not report IsBranch")
	}
}

func TestMarshalTreeShallowNodesAreStrings(t *testing.T) {
	u := New()
	u.Set("one", func(context.Context) error { return nil })
	u.Set("two", func(context.Context) error { return nil })

	body, err := MarshalTree(u.Tree(false))
	if err != nil {
		t.Fatalf("MarshalTree: %v", err)
	}

	const want = `{"label":"Tasks","nodes":["one","two"]}`
	if string(body) != want {
		t.Errorf("MarshalTree = %s, want %s", body, want)
	}
}

func TestMarshalTreeDeepLeavesCarryEmptyNodes(t *testing.T) {
	u := New()
	u.Set("one", func(context.Context) error { return nil })

	body, err := MarshalTree(u.Tree(true))
	if err != nil {
		t.Fatalf("MarshalTree: %v", err)
	}

	if !strings.Contains(string(body), `{"label":"one","type":"task","nodes":[]}`) {
		t.Errorf("deep leaf lost its empty nodes array: %s", body)
	}
}

func TestMarshalTreeDoesNotEscapeCompositionLabels(t *testing.T) {
	u := New()
	u.Set("one", func(context.Context) error { return nil })
	if _, err := u.SetTask("both", u.Series(Name("one"))); err != nil {
		t.Fatalf("SetTask: %v", err)
	}

	body, err := MarshalTree(u.Tree(true))
	if err != nil {
		t.Fatalf("MarshalTree: %v", err)
	}

	if !strings.Contains(string(body), LabelSeries) {
		t.Errorf("%q missing from %s", LabelSeries, body)
	}
	if strings.Contains(string(body), `\u003c`) {
		t.Errorf("composition label was HTML-escaped: %s", body)
	}
}

// TestMarshalTreeIsNotJSONMarshal pins the reason MarshalTree exists.
// encoding/json re-compacts the result of a nested MarshalJSON with HTML
// escaping switched back on, so a plain json.Marshal corrupts <series>.
func TestMarshalTreeIsNotJSONMarshal(t *testing.T) {
	u := New()
	u.Set("one", func(context.Context) error { return nil })
	if _, err := u.SetTask("both", u.Series(Name("one"))); err != nil {
		t.Fatalf("SetTask: %v", err)
	}
	tree := u.Tree(true)

	escaped, err := json.Marshal(tree)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(escaped), `\u003c`) {
		t.Skip("encoding/json no longer escapes nested MarshalJSON output")
	}

	body, err := MarshalTree(tree)
	if err != nil {
		t.Fatalf("MarshalTree: %v", err)
	}
	if strings.Contains(string(body), `\u003c`) {
		t.Errorf("MarshalTree escaped what json.Marshal escapes: %s", body)
	}
}

func TestMarshalTreeEncodesNilAsNull(t *testing.T) {
	body, err := MarshalTree(nil)
	if err != nil {
		t.Fatalf("MarshalTree(nil): %v", err)
	}
	if string(body) != "null" {
		t.Errorf("MarshalTree(nil) = %s, want null", body)
	}
}

func TestNodeMarshalJSONKeepsObjectShapeWhenTyped(t *testing.T) {
	node := &Node{Label: "one", Type: "task", Nodes: []*Node{}}
	body, err := node.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	const want = `{"label":"one","type":"task","nodes":[]}`
	if string(body) != want {
		t.Errorf("MarshalJSON = %s, want %s", body, want)
	}
}

func TestSettlesReportsTheEnvironment(t *testing.T) {
	if New().Settles() {
		t.Error("a default instance reports settle mode")
	}

	t.Setenv(SettleEnv, "true")
	if !New().Settles() {
		t.Errorf("%s=true was not honoured", SettleEnv)
	}
}

// TestSettleModeRunsEverySibling checks the behaviour UNDERTAKER_SETTLE buys:
// Series and Parallel stop cancelling on the first error, so a failing task no
// longer hides the ones after it.
func TestSettleModeRunsEverySibling(t *testing.T) {
	t.Setenv(SettleEnv, "true")
	u := New()

	var ran []string
	boom := errors.New("boom")
	u.Set("first", func(context.Context) error {
		ran = append(ran, "first")
		return boom
	})
	u.Set("second", func(context.Context) error {
		ran = append(ran, "second")
		return nil
	})

	err := u.Series(Names("first", "second")...).Fn(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	if len(ran) != 2 || ran[0] != "first" || ran[1] != "second" {
		t.Errorf("ran = %v, want both tasks", ran)
	}
}

func TestSettleSeriesRunsEverySibling(t *testing.T) {
	u := New()

	var ran []string
	first := errors.New("first failed")
	second := errors.New("second failed")
	u.Set("a", func(context.Context) error {
		ran = append(ran, "a")
		return first
	})
	u.Set("b", func(context.Context) error {
		ran = append(ran, "b")
		return second
	})

	err := u.SettleSeries(Names("a", "b")...).Fn(context.Background())
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("error = %v, want both failures joined", err)
	}
	if len(ran) != 2 {
		t.Errorf("ran = %v, want both tasks", ran)
	}
}

func TestSettleParallelDoesNotCancelSiblings(t *testing.T) {
	u := New()

	done := make(chan struct{})
	boom := errors.New("boom")
	u.Set("fails", func(context.Context) error { return boom })
	u.Set("survives", func(ctx context.Context) error {
		defer close(done)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	err := u.SettleParallel(Names("fails", "survives")...).Fn(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	<-done
	if errors.Is(err, context.Canceled) {
		t.Error("a settled sibling was cancelled")
	}
}

func TestRegistryAccessor(t *testing.T) {
	u := New()
	if u.Registry() == nil {
		t.Fatal("Registry() returned nil for a new instance")
	}

	replacement := NewDefaultRegistry()
	if err := u.SetRegistry(replacement); err != nil {
		t.Fatalf("SetRegistry: %v", err)
	}
	if u.Registry() != replacement {
		t.Error("Registry() did not report the replacement")
	}
}

// unorderedRegistry omits Names, so it exercises the alphabetical fallback that
// SetRegistry uses for registries which cannot report insertion order.
type unorderedRegistry struct {
	tasks map[string]*Task
	inits int
}

func newUnorderedRegistry() *unorderedRegistry {
	return &unorderedRegistry{tasks: map[string]*Task{}}
}

func (r *unorderedRegistry) Get(name string) (*Task, bool) {
	t, ok := r.tasks[name]
	return t, ok
}

func (r *unorderedRegistry) Set(name string, task *Task) *Task {
	r.tasks[name] = task
	return task
}

func (r *unorderedRegistry) Init(*Undertaker) { r.inits++ }

func (r *unorderedRegistry) Tasks() map[string]*Task {
	out := make(map[string]*Task, len(r.tasks))
	for name, task := range r.tasks {
		out[name] = task
	}
	return out
}

func TestSetRegistryTransfersToAnUnorderedRegistry(t *testing.T) {
	u := New()
	u.Set("beta", func(context.Context) error { return nil })
	u.Set("alpha", func(context.Context) error { return nil })

	replacement := newUnorderedRegistry()
	if err := u.SetRegistry(replacement); err != nil {
		t.Fatalf("SetRegistry: %v", err)
	}

	if replacement.inits != 1 {
		t.Errorf("Init called %d times, want 1", replacement.inits)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, ok := u.Get(name); !ok {
			t.Errorf("%q did not survive the transfer", name)
		}
	}

	// A registry without Names cannot report registration order, so listing
	// falls back to alphabetical. That is the whole reason OrderedRegistry is
	// optional, and this is the only place the fallback runs.
	if got := labelsOf(u.Tree(false)); got != "alpha,beta" {
		t.Errorf("Tree() = %s, want alpha,beta", got)
	}
}

func labelsOf(node *Node) string {
	labels := make([]string, 0, len(node.Nodes))
	for _, child := range node.Nodes {
		labels = append(labels, child.Label)
	}
	return strings.Join(labels, ",")
}

func TestDefaultRegistryInit(t *testing.T) {
	r := NewDefaultRegistry()
	r.Init(New())
	if len(r.Tasks()) != 0 {
		t.Error("Init populated the registry")
	}
}
