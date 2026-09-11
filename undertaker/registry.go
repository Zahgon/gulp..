package undertaker

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
)

// Registry stores tasks by name. Custom implementations let a project share a
// task set, which is what docs/api/registry.md describes.
//
// The JS version validates at runtime that a supplied registry object has get,
// set, init and tasks methods, raising errors such as "Custom registry must
// have `get` function." Go's type system makes those checks unreachable: a
// value either satisfies this interface at compile time or the program does
// not build. The only failure mode left is a nil registry, which SetRegistry
// rejects.
type Registry interface {
	// Get returns a task by name.
	Get(name string) (*Task, bool)
	// Set stores a task and returns it.
	Set(name string, t *Task) *Task
	// Init is called when the registry is installed, giving it a chance to
	// register its own tasks against the owning Undertaker.
	Init(u *Undertaker)
	// Tasks returns a snapshot of every registered task.
	Tasks() map[string]*Task
}

// OrderedRegistry is an optional interface a Registry may implement to report
// its task names in registration order.
//
// JavaScript needs no such thing: a registry's tasks() returns an object, and
// object keys iterate in insertion order, so `gulp --tasks` lists tasks as the
// gulpfile declared them and --sort-tasks exists to override that. A Go map
// has no order at all, so registries that care must say so explicitly.
// Registries that do not implement this are listed alphabetically.
type OrderedRegistry interface {
	// Names returns the registered task names in registration order.
	Names() []string
}

// UndefinedTaskError is returned when a name cannot be resolved. The message
// matches undertaker's own so existing tooling and docs stay accurate.
type UndefinedTaskError struct{ Name string }

func (e *UndefinedTaskError) Error() string {
	return fmt.Sprintf("Task never defined: %s", e.Name)
}

// ErrNilRegistry is returned by SetRegistry when given a nil registry.
var ErrNilRegistry = errors.New("undertaker: registry must not be nil")

// ErrInvalidTaskName is returned when registering a task under an empty name.
var ErrInvalidTaskName = errors.New("undertaker: task name must not be empty")

// ErrNotATask is returned by LastRun when its argument does not denote a task.
// It stands in for the JS message "Only functions can check lastRun".
var ErrNotATask = errors.New("undertaker: only tasks can check lastRun")

// DefaultRegistry is the in-memory registry gulp uses unless one is supplied.
type DefaultRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	order []string
}

// NewDefaultRegistry returns an empty DefaultRegistry.
func NewDefaultRegistry() *DefaultRegistry {
	return &DefaultRegistry{tasks: make(map[string]*Task)}
}

// Get returns a task by name.
func (r *DefaultRegistry) Get(name string) (*Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[name]
	return t, ok
}

// Set stores a task under name and returns it.
func (r *DefaultRegistry) Set(name string, t *Task) *Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tasks == nil {
		r.tasks = make(map[string]*Task)
	}
	if _, exists := r.tasks[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tasks[name] = t
	return t
}

// Names returns the registered task names in registration order, satisfying
// OrderedRegistry. Re-registering a name keeps its original position, matching
// the way assigning to an existing JavaScript object key does not move it.
func (r *DefaultRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.order)
}

// Init does nothing for the default registry; it exists to satisfy Registry.
// A custom registry uses the hook to capture the undertaker or seed tasks.
func (r *DefaultRegistry) Init(u *Undertaker) { _ = u }

// Tasks returns a snapshot of the registered tasks.
func (r *DefaultRegistry) Tasks() map[string]*Task {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.tasks)
}

// registeredNames returns task names in the order they should be listed:
// registration order when the registry tracks it, alphabetical otherwise. Both
// are stable, so `gulp --tasks` never shuffles between runs the way iterating
// a Go map would.
func registeredNames(r Registry) []string {
	if ordered, ok := r.(OrderedRegistry); ok {
		return ordered.Names()
	}
	return sortedNames(r.Tasks())
}

func sortedNames(tasks map[string]*Task) []string {
	names := make([]string, 0, len(tasks))
	for n := range tasks {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
