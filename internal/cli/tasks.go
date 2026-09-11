package cli

import (
	"sort"
	"strings"

	"github.com/gulpjs/gulp-go/undertaker"
)

// TaskLookup resolves a task name to its registered definition, so the
// renderer can pick up descriptions and flags that live on the task rather
// than in the tree.
type TaskLookup func(name string) (*undertaker.Task, bool)

// TreeRenderer draws the dependency tree printed by --tasks.
type TreeRenderer struct {
	Messages     Messages
	Lookup       TaskLookup
	MaxDepth     int
	CompactTasks bool
	SortTasks    bool
}

type treeLine struct {
	label string
	width int
	desc  string
}

// Render returns the lines of the tree, root label first.
//
// The layout is gulp-cli's: a leaf hangs off "├── " and a branch off "├─┬ ",
// with descriptions aligned into a column. Only top-level tasks carry
// descriptions and flags, because those are properties of a registered task
// and the deeper nodes are the anonymous internals of a composition.
func (r TreeRenderer) Render(root *undertaker.Node) []string {
	if root == nil {
		return nil
	}
	lines := []treeLine{{label: root.Label, width: DisplayWidth(root.Label)}}

	top := root.Nodes
	if r.SortTasks {
		top = append([]*undertaker.Node(nil), top...)
		sort.SliceStable(top, func(i, j int) bool { return top[i].Label < top[j].Label })
	}

	topLabels := make(map[string]bool, len(top))
	for _, node := range top {
		topLabels[node.Label] = true
	}

	maxWidth := 0
	for i, node := range top {
		width := r.appendNode(&lines, node, "", i == len(top)-1, 1, topLabels)
		if width > maxWidth {
			maxWidth = width
		}
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		label := line.label
		if line.desc != "" {
			label += strings.Repeat(" ", maxWidth-line.width) + "  " + line.desc
		}
		if strings.TrimSpace(StripANSI(label)) == "" {
			continue
		}
		out = append(out, label)
	}
	return out
}

// appendNode writes one node and its subtree, returning the widest label it
// contributed at this level. Descendants are deliberately excluded from the
// returned width: gulp aligns descriptions against the top-level column only,
// so a deeply nested <series> label does not push the column out.
func (r TreeRenderer) appendNode(
	lines *[]treeLine,
	node *undertaker.Node,
	bars string,
	last bool,
	depth int,
	topLabels map[string]bool,
) int {
	leaf := r.isLeaf(node, depth, topLabels)
	label := bars + r.Messages.Branch(last, leaf) + r.Messages.TaskName(node.Label)

	line := treeLine{label: label, width: DisplayWidth(label)}
	if depth == 1 {
		if task, ok := r.task(node.Label); ok && task.Description != "" {
			line.desc = r.Messages.TaskDescription(task.Description)
		}
	}
	*lines = append(*lines, line)
	maxWidth := line.width

	if depth == 1 {
		flagBars := bars + r.Messages.Bars(last)
		if !leaf {
			flagBars += r.Messages.Bars(false)
		} else {
			flagBars += "  "
		}
		for _, flagLine := range r.flagLines(node.Label, flagBars) {
			*lines = append(*lines, flagLine)
			if flagLine.width > maxWidth {
				maxWidth = flagLine.width
			}
		}
	}

	if leaf {
		return maxWidth
	}
	childBars := bars + r.Messages.Bars(last)
	for i, child := range node.Nodes {
		r.appendNode(lines, child, childBars, i == len(node.Nodes)-1, depth+1, topLabels)
	}
	return maxWidth
}

// isLeaf decides whether a node's children are drawn.
//
// Beyond the obvious cases, --compact-tasks stops the tree from re-expanding a
// registered task that appears again as somebody else's dependency, which is
// what keeps the listing short in a gulpfile where many tasks share a step.
func (r TreeRenderer) isLeaf(node *undertaker.Node, depth int, topLabels map[string]bool) bool {
	switch {
	case depth >= r.maxDepth():
		return true
	case depth > 1 && r.CompactTasks && topLabels[node.Label]:
		return true
	default:
		return len(node.Nodes) == 0
	}
}

func (r TreeRenderer) flagLines(name, bars string) []treeLine {
	task, ok := r.task(name)
	if !ok || len(task.Flags) == 0 {
		return nil
	}
	flags := make([]string, 0, len(task.Flags))
	for flag := range task.Flags {
		flags = append(flags, flag)
	}
	sort.Strings(flags)

	out := make([]treeLine, 0, len(flags))
	for _, flag := range flags {
		label := bars + r.Messages.TaskFlag(flag)
		out = append(out, treeLine{
			label: label,
			width: DisplayWidth(label),
			desc:  r.Messages.TaskFlagDescription(task.Flags[flag]),
		})
	}
	return out
}

func (r TreeRenderer) task(name string) (*undertaker.Task, bool) {
	if r.Lookup == nil {
		return nil, false
	}
	return r.Lookup(name)
}

func (r TreeRenderer) maxDepth() int {
	if r.MaxDepth <= 0 {
		return DefaultTasksDepth
	}
	return r.MaxDepth
}

// SimpleList renders --tasks-simple: one top-level task name per line.
func SimpleList(root *undertaker.Node) string {
	if root == nil {
		return ""
	}
	names := make([]string, 0, len(root.Nodes))
	for _, node := range root.Nodes {
		names = append(names, node.Label)
	}
	return strings.TrimSpace(strings.Join(names, "\n"))
}

// TreeJSON renders --tasks-json, applying the same depth and compaction rules
// as the text tree so both views agree.
func TreeJSON(root *undertaker.Node, maxDepth int, compact bool) ([]byte, error) {
	if root == nil {
		return []byte("null"), nil
	}
	if maxDepth <= 0 {
		maxDepth = DefaultTasksDepth
	}
	topLabels := make(map[string]bool, len(root.Nodes))
	for _, node := range root.Nodes {
		topLabels[node.Label] = true
	}
	copied := &undertaker.Node{Label: root.Label, Nodes: []*undertaker.Node{}}
	for _, node := range root.Nodes {
		copied.Nodes = append(copied.Nodes, copyNode(node, 1, maxDepth, compact, topLabels))
	}
	return undertaker.MarshalTree(copied)
}

func copyNode(node *undertaker.Node, depth, maxDepth int, compact bool, topLabels map[string]bool) *undertaker.Node {
	// Nodes starts empty rather than nil so that a pruned or childless node
	// still serialises as "nodes": [], which is what undertaker emits for a
	// leaf of a deep tree. A nil slice would omit the key entirely.
	out := &undertaker.Node{
		Label:  node.Label,
		Type:   node.Type,
		Branch: node.Branch,
		Nodes:  []*undertaker.Node{},
	}
	if depth >= maxDepth {
		return out
	}
	if depth > 1 && compact && topLabels[node.Label] {
		return out
	}
	for _, child := range node.Nodes {
		out.Nodes = append(out.Nodes, copyNode(child, depth+1, maxDepth, compact, topLabels))
	}
	return out
}
