<!--
name: creating-custom-registries
title: Creating Custom Registries
hide_title: true
sidebar_label: Creating Custom Registries
-->

# Creating Custom Registries

A registry is where tasks live. Replacing it lets you share a set of tasks
between projects, pre-register tasks from a directory of files, or wrap every
task with instrumentation of your own.

## The interface

A registry implements four methods:

```go
type Registry interface {
	Get(name string) (*undertaker.Task, bool)
	Set(name string, task *undertaker.Task) *undertaker.Task
	Init(u *undertaker.Undertaker)
	Tasks() map[string]*undertaker.Task
}
```

`Init` runs once, when the registry is installed. It receives the `Undertaker`
so a registry can register tasks of its own during setup. `Set` returns the
task that was stored, which lets a registry substitute a wrapped version.

> In JavaScript, gulp checks at runtime that the object you pass has all four
> methods and throws ``Custom registry must have `get` function.`` and four
> siblings when it does not. In Go the compiler checks this, so those five
> error messages have no counterpart. Passing `nil` is still rejected, with
> `ErrNilRegistry`.

## Installing one

```go
if err := gulp.SetRegistry(NewSharedRegistry()); err != nil {
	log.Fatal(err)
}
```

Tasks already registered are transferred into the new registry first, in
registration order, and only then is `Init` called. That ordering matters: it
means a registry can see the tasks a gulpfile registered before it was
installed, and it means installing a registry never loses work.

Only tasks registered with `Task` or `TaskRef` reach a registry. Functions
passed directly to `Series` or `Parallel` are anonymous and are never stored.

## A registry that shares tasks

The common case is a package that ships a standard set of tasks:

```go
package buildkit

import (
	"context"

	"github.com/gulpjs/gulp-go/undertaker"
)

type Registry struct {
	tasks map[string]*undertaker.Task
	order []string
}

func New() *Registry {
	return &Registry{tasks: make(map[string]*undertaker.Task)}
}

func (r *Registry) Init(u *undertaker.Undertaker) {
	u.Set("clean", func(ctx context.Context) error { return clean(ctx) })
	u.Set("lint", func(ctx context.Context) error { return lint(ctx) })
}

func (r *Registry) Get(name string) (*undertaker.Task, bool) {
	t, ok := r.tasks[name]
	return t, ok
}

func (r *Registry) Set(name string, task *undertaker.Task) *undertaker.Task {
	if _, exists := r.tasks[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tasks[name] = task
	return task
}

func (r *Registry) Tasks() map[string]*undertaker.Task {
	out := make(map[string]*undertaker.Task, len(r.tasks))
	for name, task := range r.tasks {
		out[name] = task
	}
	return out
}
```

A gulpfile then gets `clean` and `lint` for free and can still add its own:

```go
func main() {
	if err := gulp.SetRegistry(buildkit.New()); err != nil {
		log.Fatal(err)
	}
	gulp.Task("build", build)
	gulp.TaskRef("default", gulp.Series(gulp.Names("clean", "lint", "build")...))
	gulp.Main()
}
```

## Preserving registration order

Go maps have no order, but `--tasks` and `--tasks-simple` list tasks in the
order they were registered, and `--sort-tasks` exists to override that. A
registry can preserve the order by implementing one extra method:

```go
type OrderedRegistry interface {
	Names() []string
}
```

```go
func (r *Registry) Names() []string { return slices.Clone(r.order) }
```

That is why the example above keeps an `order` slice alongside the map. A
registry that does not implement `Names` still works; its tasks are simply
listed alphabetically.

> This interface has no JavaScript counterpart. There, a registry stores tasks
> on a plain object and `Object.keys` returns them in insertion order for
> free.

## A registry that wraps every task

Because `Set` returns the task that gets stored, a registry can decorate what
it is given. This one records how long each task takes:

```go
type TimingRegistry struct {
	*undertaker.DefaultRegistry
	mu      sync.Mutex
	elapsed map[string]time.Duration
}

func NewTimingRegistry() *TimingRegistry {
	return &TimingRegistry{
		DefaultRegistry: undertaker.NewDefaultRegistry(),
		elapsed:         make(map[string]time.Duration),
	}
}

func (r *TimingRegistry) Set(name string, task *undertaker.Task) *undertaker.Task {
	inner := task.Fn
	task.Fn = func(ctx context.Context) error {
		start := time.Now()
		err := inner(ctx)
		r.mu.Lock()
		r.elapsed[name] = time.Since(start)
		r.mu.Unlock()
		return err
	}
	return r.DefaultRegistry.Set(name, task)
}
```

Embedding `*undertaker.DefaultRegistry` supplies `Get`, `Init`, `Tasks` and
`Names`, so only the interesting method has to be written.

For timing specifically you do not need a registry at all — `gulp.On` already
reports a duration for every task. Reach for a wrapping registry when you need
to change what a task *does*, not merely observe it.

## Observing tasks instead

```go
gulp.On(func(evt gulp.Event) {
	if evt.Kind == undertaker.EventStop && !evt.Branch {
		log.Printf("%s took %s", evt.Name, evt.Duration)
	}
})
```

`Event` carries `UID`, `Name`, `Kind`, `Time`, `Duration`, `Branch` and `Err`.
`UID` identifies one *execution*, so a task that runs three times produces
three uids and a listener can pair a stop with its start. `Branch` is true for
the `<series>` and `<parallel>` wrappers, which is how the CLI hides them
unless you ask for `-LLLL`.

## Related

- [registry][registry-api] — the API reference
- [tree][tree-api] — what `--tasks` reads
- [task][task-api] — registering tasks

[registry-api]: ../api/registry.md
[tree-api]: ../api/tree.md
[task-api]: ../api/task.md
