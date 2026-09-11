# Contributing

This is a port of [gulp][gulp] 5.0.1 to Go. That single fact decides most of
what a contribution should look like: the goal is not to design a build system,
it is to reproduce one that already exists, and to be honest in writing wherever
Go cannot.

## Before you start

Read [MIGRATION.md][migration]. It records what was ported, what deliberately
diverges, and why. A change that contradicts something written there needs to
update that document in the same commit, or it is not finished.

## Getting set up

```sh
git clone https://github.com/gulpjs/gulp-go
cd gulp-go
make check
```

`make check` runs formatting, `go vet`, the linter and the full test suite. It
is the same gate CI applies, so a green `make check` locally means a green CI
run.

The linter is optional locally — `make lint` skips itself with a notice when
`golangci-lint` is not installed. Install it if you are touching more than a
line or two:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Running the tests

```sh
make test         # -race, with coverage
make test-short   # skips the watcher and subprocess tests
make cover        # opens the HTML coverage report
```

Always use `-race`. Pipeline stages, watchers and the task runner are all
goroutines, and a bug in any of them is invisible without it. `make test`
already passes `-race`; if you run `go test` by hand, pass it yourself.

Filesystem and watcher tests need `-count=1` when you re-run them, because a
cached pass tells you nothing about the state of the disk.

## The differential suite

The most valuable tests in this repository compare the port against the real
thing. They run gulp 5.0.1 in a Node subprocess and byte-compare the results.

```sh
npm install gulp@5.0.1 --prefix /tmp/gulp-js
GULP_JS_REPO=/tmp/gulp-js/node_modules/gulp make differential
```

Without `GULP_JS_REPO` the live cases skip and only the golden vectors run, so
a plain `make check` never fails for want of Node.

**If you change globbing, path handling, `Src`, `Dest` or the task tree, run
the differential suite.** Every serious bug found during this port was found
this way and by nothing else — including a glob-parsing divergence that quietly
sent `Dest()` output to the wrong directory.

## What a change should include

**Cite the source.** When you fix a behavioural difference, say which upstream
file you read. `vinyl-fs/lib/file-operations.js` is an argument; "this seems
more correct" is not. The upstream packages are on npm and are small enough to
read:

```sh
npm pack undertaker@2.0.0 && tar -xzf undertaker-2.0.0.tgz
```

**Add a test that would have caught it.** Prefer a differential case over a
hand-written assertion, because a hand-written assertion only encodes what you
already believed. Several assertions in this repository were wrong in exactly
that way and were corrected once the real gulp was consulted.

**Match gulp's wording exactly.** Error strings, log lines and CLI output are
reproduced character for character, including capitalisation that Go's linters
dislike — `staticcheck`'s ST1005 is suppressed for that reason. Do not tidy
them.

**Document divergences where they are visible.** A note in MIGRATION.md, and a
blockquote on the relevant page under `docs/`, so the reader meets it where the
question arises rather than in an appendix.

## Style

The [`programming` conventions][programming] this repository follows in short:

- Strict types. No `any` where a real type will do.
- Errors are values, wrapped with `%w`, matched with `errors.Is`/`errors.As`.
- No panics in library code. A constructor that cannot fail early returns a
  transform that reports the error when the pipeline runs.
- Comments explain *why*, not *what*. Most of the comments here record an
  upstream contract that the signature does not convey, or a concurrency
  invariant that is easy to break during a refactor. Add one only when it is
  carrying that kind of weight.
- Exported symbols get doc comments. `revive`'s `exported` rule is enabled.

Run `make fmt` before committing.

## Commit messages

Match the existing history: a short imperative summary, and a body explaining
why when the change is not self-evident. If you fixed a divergence, name the
upstream file in the body.

## Reporting a bug

The most useful bug report is a difference. Tell us what gulp does and what
this does, ideally as a failing case in `differential_test.go`. The second most
useful is a small `gulpfile.go` that reproduces the problem, plus the output
of `gulp -LLLL <task>`.

Security issues go through [SECURITY.md][security] instead.

## License

Contributions are accepted under the [MIT license][license], the same license
gulp uses.

[gulp]: https://github.com/gulpjs/gulp
[migration]: MIGRATION.md
[security]: .github/SECURITY.md
[license]: LICENSE
[programming]: docs/writing-a-plugin/guidelines.md
