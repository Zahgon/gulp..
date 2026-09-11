# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog][keepachangelog], and versions follow [Semantic Versioning][semver].

Because this is a port, each entry also records which gulp release the
behaviour was matched against.

## [Unreleased]

### Added

- Initial Go port of [gulp][gulp] 5.0.1 (commit `61f22dc`) and
  [gulp-cli][gulp-cli] 3.1.0.
- `gulp` — the façade: `Src`, `Dest`, `Symlink`, `Watch`, `Task`, `Series`,
  `Parallel`, `Tree`, `LastRun`, `Registry`, plus a package-level singleton
  matching `module.exports = new Gulp()`.
- `vinyl` — the file object, ported from vinyl 3.0.0, including buffer, stream
  and null contents, history, custom properties and `Clone`.
- `pipeline` — the replacement for Node streams: `Transform` values joined by
  `Pipe`, with bounded buffering, context cancellation, panic recovery and
  per-stage error attribution. Construction does no work, preserving gulp
  5.0.1's *Avoid globbing before read stream is opened* fix.
- `vfs` — `Src`/`Dest`/`Symlink`, ported from vinyl-fs 4.0.2, including
  encodings, BOM handling, sourcemaps, and fd-based permission, time and owner
  synchronisation guarded by an ownership check.
- `undertaker` — the task registry, ported from undertaker 2.0.0: named tasks,
  forward references, `Series`/`Parallel` compositions, the `Registry`
  extension point, `LastRun` and the task tree.
- `watch` — file watching, ported from glob-watcher 6.0.0, with delay and queue
  semantics, atomic-save collapsing, an fsnotify backend and a polling backend.
- `plugins` — `Concat`, `Rename`, `Replace`, `Filter`, `If`, `Sourcemaps` and
  `Exec`, the last of which turns any command-line tool into a pipeline stage.
- `cmd/gulp` — the command-line launcher, and `gulp.Main` for a gulpfile's own
  `main`. All 17 gulp-cli flags, the box-drawing task tree, and log output
  matching gulp's wording, colours and duration formatting.
- Differential test suite comparing the port against real gulp 5.0.1 running
  in a Node subprocess: `src` contents and emission order across eight glob
  shapes, `dest` output trees, task tree JSON and `--tasks-simple`.
- Documentation ported and adapted to Go: getting started, API reference, CLI
  reference, plugin authoring, custom registries, recipes and FAQ.
- [MIGRATION.md][migration] cataloguing every divergence from the JavaScript
  original, and the bugs found in the port while proving equivalence.

### Deliberate divergences

These are the differences a JavaScript user will notice. The full list, with
reasoning, is in [MIGRATION.md][migration].

- A task is `func(context.Context) error`. Node's six async conventions collapse
  to one, so *Did you forget to signal async completion?* cannot happen.
- A gulpfile is a compiled Go program. There is no transpiler layer, and the
  `gulp` binary is a convenience over `go run .`.
- npm plugins cannot be loaded. `plugins.Exec` wraps command-line tools and
  `pipeline.Map` covers the rest.
- `Parallel` cancels its remaining siblings on the first error, which JavaScript
  cannot do. `SettleParallel` and `--continue` opt out.
- Passing a task name where a function is expected is a compile error rather
  than a runtime one, so several of gulp's validation messages are unreachable.
- `useJunctions`, `persistent`, `alwaysStat` and `binaryInterval` are accepted
  and inert; each has no counterpart in Go's standard library.

[keepachangelog]: https://keepachangelog.com/en/1.1.0/
[semver]: https://semver.org/spec/v2.0.0.html
[gulp]: https://github.com/gulpjs/gulp
[gulp-cli]: https://github.com/gulpjs/gulp-cli
[migration]: MIGRATION.md
