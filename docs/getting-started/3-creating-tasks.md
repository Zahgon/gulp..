<!-- front-matter
id: creating-tasks
title: Creating Tasks
hide_title: true
sidebar_label: Creating Tasks
-->

# Creating Tasks

Every task is a `func(context.Context) error`. That is the whole contract.

```go
func clean(ctx context.Context) error {
	return os.RemoveAll("dist")
}
```

## Exporting a task

A task becomes runnable from the command line when you register it by name:

```go
gulp.Task("clean", clean)
```

```sh
gulp clean
```

Registering the same name twice replaces the earlier task, exactly as it does
in JavaScript.

## Public and private tasks

A **public** task is registered and appears in `gulp --tasks`. A **private**
task is an ordinary Go function that is only referenced by a composition —
never registered, never listed, but still perfectly usable.

```go
func compile(ctx context.Context) error { /* ... */ }   // private
func minify(ctx context.Context) error  { /* ... */ }   // private

func main() {
	build := gulp.Series(
		gulp.Anonymous(compile),
		gulp.Anonymous(minify),
	)
	gulp.TaskRef("build", build)   // public
	gulp.Main()
}
```

`gulp.Anonymous` wraps a bare function so a composition can hold it. Such a
task is reported as `<anonymous>` in the tree. To give it a nicer label without
registering it, use `gulp.Fn("compile", compile)`.

## Composing tasks

`Series` runs tasks one after another, stopping at the first error.
`Parallel` runs them at the same time.

```go
build := gulp.Series(
	gulp.Name("clean"),
	gulp.Parallel(gulp.Names("styles", "scripts")...),
)
gulp.TaskRef("build", build)
```

Both take `Ref` values, which is either a name (`gulp.Name`, `gulp.Names`) or a
task itself. Names are resolved when the composition *runs*, not when it is
built, so you can refer to a task registered later in the file.

Both return a `*undertaker.Task`. Register it with `TaskRef`, or take its `Fn`
field to get a plain `TaskFunc`:

```go
gulp.Task("build", gulp.Series(gulp.Names("clean", "compile")...).Fn)
```

Prefer `TaskRef` when you want `gulp --tasks` to show the structure underneath
the name; use `.Fn` when you just want the behaviour.

Compositions nest to any depth, and a composition is itself a task, so it can
appear inside another one.

> `Parallel` cancels its remaining siblings when one of them fails. The
> JavaScript implementation cannot do this and lets every branch run to
> completion. See [MIGRATION.md](../../MIGRATION.md) §4.3, and use
> `--continue` on the command line when you want the JavaScript behaviour.

## The default task

The task named `default` is what runs when you name none:

```go
gulp.TaskRef("default", gulp.Series(gulp.Name("build")))
```

Running `gulp` with no `default` registered is an error.

## Descriptions and flags

Registration returns the stored task, so you can annotate it for `--tasks`:

```go
clean := gulp.Task("clean", cleanFn)
clean.Description = "Remove the build directory"
clean.Flags = map[string]string{
	"--dry": "Report what would be removed",
}
```

```
├── clean    Remove the build directory
│   --dry    …Report what would be removed
```

## Listing tasks

```sh
gulp --tasks          # the tree, with descriptions
gulp --tasks-simple   # plain list, one name per line
gulp --tasks-json     # machine-readable
```

## Next

Continue to [Async Completion][async-completion].

[async-completion]: 4-async-completion.md
