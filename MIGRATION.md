# Migrating gulp from JavaScript to Go

This document records what the port does, how each JavaScript package was
replaced, and every place where the Go behaviour deliberately differs from
gulp 5.0.1. It is the reference for anyone asking "is this the same program?"

The short answer: yes for observable behaviour, no for the plugin ecosystem.
The long answer is below.

## 1. Source of truth

The port targets **gulp 5.0.1** at commit `61f22dc`, with **gulp-cli 3.1.0**
for command-line behaviour. Where the two disagree with `docs/`, the source
wins; §9 lists the places gulp's own documentation is stale.

The original `index.js` is 61 lines. Almost all of gulp's behaviour lives in
its dependencies, so the port is really a port of the stack beneath it.

## 2. Package mapping

| JavaScript | Go | Notes |
| --- | --- | --- |
| `gulp` (index.js) | `gulp` (root) | `Gulp` type plus a package-level singleton |
| `gulp` (index.mjs) | `default.go` | The ten named exports as package functions |
| `undertaker` | `undertaker/` | Public: custom registries need its types |
| `bach` | `internal/bach` | series/parallel/settleSeries/settleParallel |
| `async-done` | `internal/asyncdone` | Completion adapters |
| `last-run` | `internal/lastrun` | Capture/Release/Truncate |
| `vinyl` | `vinyl/` | File, Stat, Contents, SourceMap |
| `vinyl-fs` | `vfs/` | Src, Dest, Symlink |
| `glob-stream` | `internal/globstream` | Glob expansion into vinyl files |
| `glob-parent`, `is-glob`, `micromatch` | `internal/glob` | Backed by `doublestar` |
| `vinyl-sourcemap` | `internal/sourcemap` | Add/Write, inline and external |
| `remove-bom-buffer`, `remove-bom-stream`, `iconv-lite` | `vfs/encoding.go` | Backed by `golang.org/x/text` |
| `resolve-options`, `value-or-function` | `vfs/options.go` | Generic `Option[T]` |
| `glob-watcher` | `watch/` | Debounce, queue, glob filtering |
| `chokidar` | `internal/fswatch` | fsnotify backend plus a polling backend |
| `gulp-cli`, `liftoff`, `interpret`, `rechoir` | `internal/cli`, `cmd/gulp` | See §6 |
| `fancy-log` | `internal/cli/logger.go` | Timestamps, levels, stream routing |
| `pretty-hrtime` | `internal/cli/duration.go` | gulp-cli 3 ships its own; see §9 |
| `chalk`, `string-width`, `strip-ansi` | `internal/cli/color.go`, `width.go` | |
| Node object-mode streams | `pipeline/` | See §3 |
| npm plugin ecosystem | `plugins/` + `pipeline.Transform` | See §5 |

## 3. Streams became channels

Node's object-mode streams are replaced by
`<-chan *vinyl.File` plus `context.Context`:

```go
type Transform interface {
	Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error
}
```

`Pipeline` is the `.pipe()` equivalent. It runs one goroutine per stage with a
16-element buffer between them, which is the same value as Node's objectMode
`highWaterMark`, so backpressure behaves the same way.

Three properties are preserved deliberately:

- **Laziness.** Nothing runs until a terminal call (`Run`, `Collect`, `Each`).
  This is what keeps gulp 5.0.1's fix *"Avoid globbing before read stream is
  opened"*: a `Src` that is built but never started performs no I/O, so the
  file list reflects the moment the build actually begins.
- **Error propagation.** The first stage error cancels a shared derived
  context and is delivered once, wrapped as `pipeline stage N: ...`. Use
  `errors.Is`, not string comparison.
- **Panics become errors**, so one bad transform cannot take the build down.

A stage must never close `out`; the runner owns that. `plugins.If` is the
place this matters most, since both branches write into the same channel.

## 4. Tasks

### 4.1 One signature instead of six

Node accepts callbacks, streams, promises, event emitters, child processes and
observables as task completion signals. Go has one:

```go
type TaskFunc func(ctx context.Context) error
```

`internal/asyncdone` provides adapters — `FromCallback`, `FromChannel`,
`FromCmd`, `FromPipeline` — so the other shapes are one wrapper away. Unlike
JavaScript, calling `done()` twice is silently ignored rather than crashing
the process.

Synchronous tasks are unsupported in both languages, but for opposite reasons:
gulp cannot detect completion, whereas in Go a function that returns is done.

### 4.2 Forward references still work

`gulp.Name("build")` resolves at run time, not registration time, so tasks may
reference each other in any order exactly as in JavaScript. An unresolved name
produces `Task never defined: <name>`, verbatim.

### 4.3 Parallel cancels — a deliberate divergence

JavaScript's `bach.parallel` cannot cancel work already in flight, so a failing
task lets its siblings run to completion. `bach.Parallel` in Go cancels the
context the siblings received on the first error.

This is strictly better and is observable, so it is called out here. Tasks that
ignore `ctx` behave exactly as they do in JavaScript. `SettleSeries` and
`SettleParallel` back the `--continue` flag and deliberately do **not** cancel,
so every failure is still reported.

Undertaker's `UNDERTAKER_SETTLE=true` environment variable is honoured. Setting
it makes `Series`, `Parallel` and `Run` use the settle variants for the whole
instance, exactly as the JavaScript constructor does. The choice is made when a
composed task **runs**, not when it is composed, because a Go gulpfile calls
`gulp.Series` during `main()` — before the CLI has parsed `--continue`. JavaScript
gets the same reach for free: `gulp-cli` exports `UNDERTAKER_SETTLE` before the
gulpfile is required. `Undertaker.SetSettle` is what `--continue` calls.

### 4.3.1 Parallel start order — a deliberate divergence

JavaScript's `async-done` defers every task body to `process.nextTick`, so
`gulp one two three` prints all three `Starting` lines in reference order and
only then runs the bodies. Go starts each task in its own goroutine, and the
scheduler runs the most recently created one first, so the same command prints
`Starting 'three'` and that task's output before `Starting 'one'`.

Every task still runs, the exit code is unchanged, and nothing observable but
the interleaving of the log lines differs. Reproducing the JavaScript order
would mean splitting "emit the start event" from "run the body" throughout the
task contract, which buys log cosmetics at the cost of the task signature.

### 4.4 Task events

`Event` carries `UID`, `Name`, `Kind`, `Time`, `Duration`, `Branch` and `Err`,
which is the payload undertaker emits. `UID` numbers one *execution*, not one
task, so a listener can pair a stop with the start it belongs to when the same
task runs more than once.

### 4.5 Registries

The documented four-method contract is unchanged:

```go
type Registry interface {
	Get(name string) (*Task, bool)
	Set(name string, t *Task) *Task
	Init(u *Undertaker)
	Tasks() map[string]*Task
}
```

Two differences:

- JavaScript validates registry *shape* at run time and throws
  ``Custom registry must have `get` function.`` and friends. Go's type system
  makes those states unrepresentable, so those five errors are unreachable.
  Only a nil registry is rejected.
- JavaScript object keys preserve insertion order; Go maps do not. Task
  listing order is observable through `--tasks`, `--tasks-simple` and
  `--sort-tasks`, so an optional interface restores it:

  ```go
  type OrderedRegistry interface{ Names() []string }
  ```

  `DefaultRegistry` implements it. Registries that do not are listed
  alphabetically. `SetRegistry` transfers the already-registered tasks in the
  same order before calling `Init`, so swapping a registry does not silently
  reorder `--tasks`.

### 4.6 Tree JSON

`tree()` output is compared byte for byte against gulp in the differential
suite, which forced two Go-specific decisions:

- A shallow tree's nodes are bare JSON strings in JavaScript
  (`{"label":"Tasks","nodes":["a","b"]}`), because `tree.js` maps each task to
  `meta.tree.label`. `Node.MarshalJSON` reproduces this: a node carrying only a
  label encodes as a string, while a deep node — which always has a `type` —
  encodes as an object.
- A deep leaf must emit `"nodes": []`. Go's `omitempty` cannot distinguish a nil
  slice from an empty one, so the marshaller uses a `*[]*Node` shadow field and
  every deep node is built with a non-nil slice.

Use `undertaker.MarshalTree`, not `json.Marshal`. `encoding/json` re-compacts a
nested `MarshalJSON` result with HTML escaping switched back on, which turns
`<series>` into `\u003cseries\u003e`. `MarshalTree` encodes with escaping
disabled throughout, and the CLI uses it for `--tasks-json`.

## 5. Plugins: the one thing that could not be ported

gulp's value is largely its npm plugin ecosystem, and JavaScript packages
cannot be translated into Go. Pretending otherwise would be the dishonest part
of a migration like this, so the replacement is explicit and three-layered:

1. **`pipeline.Transform`** is the extension point. Writing a plugin means
   writing a function; there is no registration or wrapper library.
2. **`plugins.Exec`** turns any command-line tool into a transform, piping
   contents through stdin/stdout or writing a temp file for tools that need a
   real path. This covers esbuild, sass, terser, imagemin and most of what
   people actually reach for.
3. **Reference plugins** for the handful that are effectively part of gulp's
   vocabulary: `Concat`, `Rename`, `Replace`, `Filter`, `If`,
   `SourcemapsInit`/`SourcemapsWrite`.

`plugins.Error` carries `Plugin`, `Path` and the wrapped cause, matching the
role of `plugin-error`.

One behavioural difference: `Replace*` buffers a streaming file rather than
rejecting it. The JS plugins refuse `buffer: false`, but a whole-content
rewrite needs the whole content anyway, so refusing adds nothing.

## 6. The gulpfile is a Go program

A JavaScript gulpfile is interpreted, which is why gulp-cli carries Liftoff,
interpret and rechoir to locate a gulpfile, pick a transpiler and require it. A
Go gulpfile is compiled, so that machinery has no counterpart.

```go
func main() {
	gulp.Task("build", build)
	gulp.Main()
}
```

`gulp.Main()` is the whole CLI — flag parsing, task listing, logging, exit
codes — running inside your own binary. `go run .` is therefore a complete
replacement for the `gulp` command.

The `cmd/gulp` binary exists for familiarity: it finds `gulpfile.go` by walking
up from the working directory and shells out to `go run`, forwarding argv. It
is entirely optional.

Consequences:

- `--preload` / `--require` is accepted and recorded but cannot load code into
  a compiled binary. Import the package instead.
- `--verify` (the npm plugin blacklist) has no meaning and is not implemented.
- `-v` prints the same version twice; JavaScript distinguishes a global CLI
  from a locally installed library, and a Go gulpfile has only one.

Everything else matches, including `INIT_CWD`, `--cwd` overriding the directory
implied by `--gulpfile`, concurrent execution of `gulp a b c`, and exit code 1
on failure.

## 7. Filesystem

- `Src` glob semantics — argument order, negation, base calculation,
  `allowEmpty`, `uniqueBy`, `since`, BOM removal, encodings, buffer vs stream —
  all match. `internal/glob.Parent` is a faithful port of `glob-parent`,
  including its unusual `endsInEnclosureWithSeparator` and `"a"`-append steps.
- `Dest` preserves the "null contents are never written" rule and re-arms a
  streaming `contents` so it can be read again downstream.
- Metadata sync is a full port of `vinyl-fs/lib/file-operations.js`. Mode,
  times and ownership are compared against the file just written and applied on
  the **open descriptor** (`fchmod`, `futimes`, `fchown`) rather than by path,
  so nothing can be swapped underneath between write and adjustment. All three
  are gated behind `isOwner`, which passes only for root or the file's own
  owner — matching JavaScript, which returns early on the same condition. Every
  error is deliberately swallowed, as upstream does.
- `vinyl.Stat` therefore carries `UID` and `GID`, read from `syscall.Stat_t`
  behind a `unix` build tag. `vinyl.InvalidID` (-1) and `vinyl.ValidID` port
  `isValidUnixId`, giving Go the three-state answer JavaScript gets from
  `undefined`. On Windows there are no ids and the whole sync is disabled,
  which is also what vinyl-fs does.
- **`useJunctions` is a no-op.** Go's `os.Symlink` cannot choose a Windows link
  type, so dir/junction selection is unavailable. Everything else about
  `Symlink` matches, including `relativeSymlinks`, which anchors both the base
  and the target to the file's cwd before relativising so it behaves like
  Node's `path.relative` even when one side is already relative.
- `Clone(CloneOptions{Contents: &false})` **shares** the buffer rather than
  nulling contents, which is what vinyl documents `contents: false` to mean.

## 8. Watching

`watch/` reproduces glob-watcher's semantics on top of `internal/fswatch`:
`ignoreInitial` defaults to true, `delay` to 200ms, `queue` to true with at
most one queued run, `events` to `add`/`change`/`unlink`, and `atomic` to 100ms
so an unlink followed by a re-add collapses into a single change.

`internal/fswatch` supplies two interchangeable backends — fsnotify with
manual recursion, and stat polling for `usePolling` — and is tested against
both.

Two notes:

- **macOS Unicode.** APFS stores filenames NFD while glob patterns are usually
  written NFC, so paths and patterns are normalised to NFC on darwin. gulp's
  own test watching `フォルダ/*` is ported and passes.
- **`persistent`, `alwaysStat` and `binaryInterval` are accepted but inert.**
  A Go program's lifetime is not tied to an event loop with pending handles.

Passing a task **name** to `Watch` throws at run time in JavaScript:

```
watching empty.txt: watch task has to be a function
(optionally generated by using gulp.parallel or gulp.series)
```

In Go the parameter is typed `TaskFunc`, so this is a compile error. gulp's
watch tests 12 and 13 are therefore unreachable and were not ported; the
mistake they guard against cannot be made.

## 9. Where gulp's own docs are wrong

Found while porting, verified against gulp-cli 3.1.0 source:

- `docs/CLI.md` documents `--require`. gulp-cli 3 renamed it to `--preload`.
  Both are accepted here.
- `docs/CLI.md` shows `[20:58:55] ├── one` for `--tasks`. gulp-cli 3 prints the
  tree with `console.log` and no timestamps. The Go output matches the code.
- Durations are **not** formatted by `pretty-hrtime`. gulp-cli 3 ships its own
  `format-hrtime`, which produces `21 ms`, `37 μs`, `5.69 ms`, `1.5 s`. That is
  what `internal/cli.FormatDuration` reproduces, digit for digit.
- `--tasks-json` uses `JSON.stringify`, which does not escape `<` and `>`. Go's
  `json.Marshal` does, so the encoder runs with `SetEscapeHTML(false)` and
  `<series>` survives intact.

## 10. Bugs found and fixed during the port

Recorded because they are the strongest evidence the tests do real work:

| Where | Bug |
| --- | --- |
| `internal/cli/options.go` | `--tasks-json out.json` dropped its path and printed to stdout |
| `internal/sourcemap` | Inline comment emitted `charset=utf8`; convert-source-map emits `charset=utf-8` |
| `internal/fswatch/polling.go` | Data race: the poll loop aliased the live state map that `Remove` mutated |
| `internal/fswatch/fswatch.go` | `Op.String()` reported only the first set bit of a combined flag |
| `undertaker` | Task listing was always sorted, making `--sort-tasks` a no-op |
| `undertaker/tree.go` | A deep-tree leaf omitted `nodes` entirely; gulp emits `"nodes": []`. Go's `omitempty` cannot tell a nil slice from an empty one, so `Node` grew a `MarshalJSON` using a `*[]*Node` shadow field |
| `undertaker/tree.go` | The new `MarshalJSON` re-escaped `<series>` as `\u003cseries\u003e`, because a `MarshalJSON` result is copied through verbatim and the outer `SetEscapeHTML(false)` cannot undo it. Fixed by encoding inside `MarshalJSON` with its own escape-disabled encoder |
| `internal/glob/isglob.go` | The hand-rolled `IsGlob` got two rules wrong: it treated a bare `?` as a wildcard, and refused a bare `(a\|b)` group. Rewritten as a statement-for-statement port of `is-glob`'s `strictCheck` plus `is-extglob` |
| `internal/cli/tasks.go` | `copyTree` rebuilt each node without a `Nodes` slice, so `--tasks-json` leaves lost `"nodes": []` again downstream of the `undertaker` fix above. Found by an external audit, then locked with a byte comparison against real gulp |
| `undertaker/tree.go` | A shallow tree encoded its nodes as objects; JavaScript encodes them as bare strings |

The `IsGlob` one was the most consequential. It feeds `glob.Parent`, which
computes every vinyl `base`, which decides every path `dest()` writes — so
`gulp.src('path/?foo/*.js').pipe(gulp.dest('out'))` was writing to the wrong
directory. It surfaced only once the differential suite ran `glob-parent`
itself and compared, which is the argument for having built that suite.

## 11. Verification

Every gulp test file has a counterpart:

| gulp | Go |
| --- | --- |
| `test/index.test.js` | `index_test.go` (exported surface, real gulpfile subprocess) |
| `test/src.js` | `gulp_test.go`, `vfs/src_test.go`, `internal/globstream` |
| `test/dest.js` | `gulp_test.go`, `vfs/dest_test.go` |
| `test/watch.js` | `gulp_test.go`, `watch/watch_test.go` (17 cases) |

`test/fixtures/` is copied byte for byte from the JS repo. Note that gulp's own
assertions compare against `'this is a test'` while the fixture on disk ends
with a newline; the Go assertions use the real bytes.

`differential_test.go` holds golden vectors extracted from the JavaScript
sources — glob-parent outputs, `formatHrTime` outputs, tree rendering, message
wording — and always runs. Setting `GULP_JS_REPO` to a gulp checkout with
`node_modules` installed additionally drives real gulp through `node` and
compares, file by file and byte for byte:

```
npm install gulp@5.0.1 --prefix /tmp/gulp-js
GULP_JS_REPO=/tmp/gulp-js/node_modules/gulp go test -run Differential -v .
```

That run covers `src` contents and emission order across eight glob shapes
(flat, deep, deeper, multiple globs, negation, `read: false`, directories), the
full `dest` output tree, the `tree()` shape including `label`/`type`/`branch`,
the shallow and deep `tree()` JSON **byte for byte**, and `--tasks-simple`. It
passes against gulp 5.0.1.

The byte comparison is deliberate. An earlier version decoded both sides and
compared the structures, which hid exactly the differences that mattered —
a missing `"nodes": []`, an object where gulp emits a string, an escaped
`<series>`. Comparing the encoded bytes is the only assertion that catches
them.

```
go test ./...           # everything
go test -race ./...     # what CI runs
go test -short ./...    # skips subprocess and filesystem-timing tests
```
