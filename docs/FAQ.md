<!--
id: FAQ
title: Frequently Asked Questions
hide_title: true
sidebar_label: FAQ
-->

# FAQ

Questions that come up often, especially from people arriving from JavaScript gulp.

## Is this the real gulp?

No. This is an independent Go port of gulp 5.0.1. It reproduces gulp's behaviour — the task
registry, the vinyl filesystem, the watcher, and the command line — but it is not maintained by
the gulp team and it cannot run npm plugins.

Where behaviour differs, [MIGRATION.md](../MIGRATION.md) says so and explains why.

## Why is my gulpfile a Go program?

Because Go is compiled. JavaScript gulp loads `gulpfile.js` at runtime through Liftoff, which is
why it needs `--require`, `interpret`, and `rechoir` to cope with TypeScript, Babel, and CoffeeScript
gulpfiles. A Go gulpfile is just a program that ends in `gulp.Main()`, so there is nothing to
transpile and nothing to resolve.

The practical consequences are all good ones: `gulp.Name("bulid")` is caught by your editor,
`--tasks` cannot disagree with what runs, and `go run .` works without the `gulp` binary installed
at all.

## Do I need the `gulp` binary?

No. `go run . build` and `gulp build` do the same thing. The launcher exists so that muscle memory
keeps working; it finds your gulpfile and runs `go run .` in that directory.

## Can I use gulp plugins from npm?

No. They are JavaScript modules that operate on Node streams and Node Buffers; there is no way to
load them into a Go process.

There are three replacements, described in [Using plugins](getting-started/7-using-plugins.md):

- `pipeline.Map` and friends, for anything you would have written by hand.
- `plugins.Exec`, which turns any command-line tool into a pipeline stage. This covers `esbuild`,
  `sass`, `terser`, `svgo`, `imagemin` and most of what people actually used plugins for.
- The bundled `plugins` package, which ships Go versions of the handful that are effectively part
  of gulp's vocabulary: `Concat`, `Rename`, `Replace`, `Filter`, `If`, and the sourcemap pair.

In practice `plugins.Exec` is better than the plugin it replaces, because it runs the tool the tool's
authors ship rather than a wrapper that lags behind it.

## Why does `gulp a b c` run the tasks at the same time?

Because that is what gulp does. Command-line tasks run concurrently unless you pass `--series`.
If you want a fixed order, compose them:

```go
gulp.TaskRef("release", gulp.Series(gulp.Names("clean", "build", "publish")...))
```

## Why did my task get cancelled?

`Parallel` cancels its remaining siblings when one of them fails. JavaScript's `bach` cannot do this
— it has no way to interrupt a running function — so in gulp the other branches keep running to
completion after the build has already failed.

If you want every branch to finish regardless, use `SettleParallel`, or pass `--continue` on the
command line. This is covered in [MIGRATION.md §4.3](../MIGRATION.md).

## What happened to `done()`, promises, and streams as task return values?

A task is `func(context.Context) error`. That single signature replaces all six of the async
conventions Node accepts, which is why "Did you forget to signal async completion?" has no
counterpart here — a task that returns has finished.

If you need to bridge a callback-based API, see
[Async completion](getting-started/4-async-completion.md).

## Why is `?` not matching a single character in my glob?

Because it does not in gulp either. `is-glob` — the module gulp uses to decide where a glob's
non-magic prefix ends — only treats `?` as special when it follows `]`, `.`, `+` or `)`, where it is
an extglob quantifier. A bare `?` is a literal question mark.

This matters more than it looks: the non-magic prefix becomes the file's base, and the base decides
where `Dest()` writes. `src/?foo/*.js` has the base `src/?foo/`, not `src/`. See
[Explaining globs](getting-started/6-explaining-globs.md).

## Why is nothing happening when I call `Src`?

`Src` builds a pipeline; it does not run one. Nothing is read until you call `Run`, `Each`,
`Collect`, or hand the pipeline to a task with `AsTask`.

This is deliberate, and it matches a fix gulp made in 5.0.1 ("Avoid globbing before read stream is
opened"). A pipeline that is built during registration but run much later sees the files that exist
when it runs, not when it was built.

## Why does my watcher never stop?

Because you have not closed it. A `*Watcher` owns goroutines and operating-system handles, so it
must be closed:

```go
w, err := gulp.Watch([]string{"src/**/*"}, gulp.WatchOptions{}, rebuild)
if err != nil {
	return err
}
defer w.Close()
<-ctx.Done()
return nil
```

In JavaScript the process simply stays alive because chokidar keeps a handle open; Go needs you to
say when you are done.

## Why does the watcher not fire on a network drive?

Filesystem notifications often do not cross network mounts, container bind mounts, or virtual
machine shares. Set `UsePolling: true` and, if you need it, `Interval`:

```go
gulp.WatchOptions{UsePolling: true, Interval: gulp.Ptr(300 * time.Millisecond)}
```

The polling backend passes the same test suite as the notification backend.

## Why do I see two runs after one save?

You probably did not. Editors that save atomically write a temporary file, delete the original, and
rename the temporary into place. The `Atomic` option (100 ms by default) collapses that
delete-then-create pair into a single change, exactly as chokidar does.

If you genuinely see two runs, it is more likely `Delay` and `Queue` doing their job: changes during
a run are collapsed into one queued rerun.

## `gulp -v` says `Local version: v0.0.0`

That is Go reporting an untagged module. Once the module carries a semantic version tag, `go list`
returns it and the line fills in. It has no effect on behaviour.

## How do I pass arguments to a task?

Unrecognised flags are left alone, so read them yourself:

```bash
gulp build --env=production
```

```go
func build(ctx context.Context) error {
	env := os.Getenv("BUILD_ENV")
	if v, ok := flags["env"]; ok {
		env = v
	}
	...
}
```

The simplest approach is an ordinary Go `flag.FlagSet` parsed from `os.Args`, or environment
variables. Declaring the flag in `Task.Flags` makes it show up in `gulp --tasks`.

## Where are the recipes?

Most of gulp's recipes are about JavaScript tooling — browserify, watchify, rollup, grunt, swig —
and do not translate. [The recipes index](recipes/README.md) says which ones carry over and gives Go
versions of the ones that do.

## Something is missing from these docs

See [documentation-missing.md](documentation-missing.md).
