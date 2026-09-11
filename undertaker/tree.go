package undertaker

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Node is one entry in the task tree returned by Tree.
//
// The field set matches the plain object undertaker's tree() produces, which
// `gulp --tasks` renders and which the archy-based CLI output is derived from.
// See docs/api/tree.md.
type Node struct {
	// Label is the display text: a task name, or one of the synthetic labels
	// <series>, <parallel> and <anonymous>.
	Label string `json:"label"`
	// Type is "task" for registered tasks and "function" for inline ones.
	// It is omitted from the shallow tree, matching JS.
	Type string `json:"type,omitempty"`
	// Branch marks composition nodes.
	Branch bool `json:"branch,omitempty"`
	// Nodes are the children, present only in a deep tree.
	Nodes []*Node `json:"nodes,omitempty"`
}

// isLabelOnly reports whether the node carries nothing but a label.
//
// undertaker's tree() maps each task to meta.tree for a deep tree but to
// meta.tree.label -- a bare string -- for a shallow one, so the two trees have
// genuinely different JSON element types. A shallow child is the only node
// that reaches this state: a deep node always has a type, and the root always
// has a non-nil Nodes slice.
func (n Node) isLabelOnly() bool {
	return n.Type == "" && !n.Branch && n.Nodes == nil
}

// MarshalJSON reproduces undertaker's exact JSON shape, which `gulp
// --tasks-json` prints verbatim.
//
// Two distinctions a plain struct tag cannot express are handled here. A
// shallow node degrades to a bare JSON string, per isLabelOnly. And a deep
// tree emits "nodes": [] for a leaf while a shallow tree omits the key
// entirely, which omitempty cannot express because encoding/json treats a nil
// slice and an empty slice identically; routing through a pointer separates
// the two cases.
func (n Node) MarshalJSON() ([]byte, error) {
	if n.isLabelOnly() {
		return encodeJSON(n.Label)
	}

	shape := struct {
		Label  string   `json:"label"`
		Type   string   `json:"type,omitempty"`
		Branch bool     `json:"branch,omitempty"`
		Nodes  *[]*Node `json:"nodes,omitempty"`
	}{Label: n.Label, Type: n.Type, Branch: n.Branch}

	if n.Nodes != nil {
		shape.Nodes = &n.Nodes
	}
	return encodeJSON(shape)
}

// MarshalTree encodes a tree the way `gulp --tasks-json` prints it.
//
// Prefer it to json.Marshal. Marshal re-compacts the output of a nested
// MarshalJSON with HTML escaping switched back on, so the <series> and
// <parallel> labels come back as \u003cseries\u003e however carefully this
// package encodes them. JSON.stringify leaves them alone, and so does this.
func MarshalTree(n *Node) ([]byte, error) {
	return encodeJSON(n)
}

// encodeJSON marshals v with HTML escaping disabled.
//
// json.Marshal always escapes <, > and &, which would corrupt the synthetic
// <series>, <parallel> and <anonymous> labels.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Tree returns the registered task hierarchy.
//
// A shallow tree lists the registered task names only. A deep tree recurses
// through series and parallel compositions so the caller can see exactly what
// a task will run.
func (u *Undertaker) Tree(deep bool) *Node {
	tasks := u.Tasks()
	root := &Node{Label: "Tasks", Nodes: []*Node{}}

	for _, name := range registeredNames(u.activeRegistry()) {
		t, ok := tasks[name]
		if !ok {
			continue
		}
		if !deep {
			root.Nodes = append(root.Nodes, &Node{Label: name})
			continue
		}
		// A composition can in principle refer back to a task that refers to
		// it. Tracking the tasks on the current path turns that from an
		// infinite recursion into a single truncated node.
		root.Nodes = append(root.Nodes, u.nodeFor(t, map[*Task]bool{}))
	}
	return root
}

// nodeFor renders one task and, for compositions, its children.
func (u *Undertaker) nodeFor(t *Task, visiting map[*Task]bool) *Node {
	node := &Node{
		Label:  t.Label(),
		Type:   string(t.kind),
		Branch: t.branch,
		Nodes:  []*Node{},
	}
	if node.Type == "" {
		node.Type = string(kindFunction)
	}
	if len(t.refs) == 0 || visiting[t] {
		return node
	}

	visiting[t] = true
	defer delete(visiting, t)

	for _, ref := range t.refs {
		child, err := ref.resolve(u)
		if err != nil {
			// An unresolved name is still worth showing: `gulp --tasks`
			// should reveal a typo rather than hide the branch entirely.
			var undef *UndefinedTaskError
			if errors.As(err, &undef) {
				node.Nodes = append(node.Nodes, &Node{
					Label: undef.Name,
					Type:  string(kindTask),
					Nodes: []*Node{},
				})
			}
			continue
		}
		node.Nodes = append(node.Nodes, u.nodeFor(child, visiting))
	}
	return node
}
