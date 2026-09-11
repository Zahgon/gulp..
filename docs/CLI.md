<!-- front-matter
id: CLI
title: CLI
hide_title: true
sidebar_label: CLI
-->

# CLI

The `gulp` command runs the tasks a gulpfile registers.

A gulpfile in this port is a compiled Go program, so there are two ways to run
one and they behave identically:

```sh
# With the launcher, from anywhere inside the project.
gulp build

# Without it. The launcher only finds the gulpfile and does this for you.
go run . build
```

The launcher is installed with:

```sh
go install github.com/gulpjs/gulp-go/cmd/gulp@latest
```

It searches the working directory and its parents for `gulpfile.go`,
`Gulpfile.go` or `gulpfile/main.go`, then runs `go run .` in that package's
directory with your arguments forwarded untouched. Everything below is handled
by the gulpfile itself, which is why the two invocations agree.

## Tasks

Tasks are given as bare arguments:

```sh
gulp build
```

With no arguments, `gulp` runs the task named `default`, and fails if there is
no such task.

Several tasks run **concurrently**, not in order:

```sh
# clean, styles and scripts all start at once
gulp clean styles scripts
```

Pass `--series` to run them one after another instead. If you need a fixed
order every time, compose the tasks in the gulpfile with
[`Series()`][series] rather than relying on the command line.

## Flags

gulp has a small set of flags. Any flag it does not recognise is left for your
own tasks to read, so `gulp build --env=prod` is yours to interpret.

| Flag | Short | Description |
|:-----|:------|:------------|
| `--help` | `-h` | Show this help. |
| `--version` | `-v` | Print the global and local gulp versions. |
| `--preload <path>` | | Preload a module before running the gulpfile. |
| `--gulpfile <path>` | `-f` | Manually set path of gulpfile. Also sets the working directory to its folder. |
| `--cwd <dir>` | | Manually set the working directory. |
| `--tasks` | `-T` | Print the task dependency tree for the loaded gulpfile. |
| `--tasks-simple` | | Print a plaintext list of tasks. |
| `--tasks-json [path]` | | Print the task dependency tree as JSON, to stdout or to a file. |
| `--tasks-depth <n>` | `--depth` | Limit the depth of the printed tree. |
| `--compact-tasks` | | Print only top-level tasks and their direct children. |
| `--sort-tasks` | | Sort the top-level tasks alphabetically. |
| `--color` | | Force colour output even when no colour support is detected. |
| `--no-color` | | Force plain output even when colour support is detected. |
| `--silent` | `-S` | Suppress all gulp logging. |
| `--continue` | | Continue running tasks after one fails. |
| `--series` | | Run the tasks given on the command line in series. |
| `-L` | | Set the log level. `-L` is least verbose, `-LLLL` is most. `-LLL` is the default. |

### `--preload` is inert in this port

`--preload` is accepted so that scripts written against gulp keep working, but
it does nothing. In JavaScript it exists to register a transpiler before the
gulpfile is read; a Go gulpfile is compiled, so there is nothing to register.

> Note: gulp's own JavaScript documentation calls this flag `--require`. It was
> renamed to `--preload` in gulp-cli 3, which gulp 5 depends on. This port
> accepts both spellings.

### Log levels

`-L` controls how much gulp itself prints. It does not affect what your tasks
print.

| Flag | Level | Shows |
|:-----|:------|:------|
| `-L` | 1 | errors |
| `-LL` | 2 | + warnings |
| `-LLL` | 3 | + task start and finish (default) |
| `-LLLL` | 4 | + `<series>` and `<parallel>` wrappers |

At the default level the composition wrappers are hidden, so a `Series` of two
tasks prints two starts and two finishes rather than four.

## Listing tasks

`gulp --tasks` prints the dependency tree. Task descriptions are right-aligned
in their own column, and flags declared on a task appear beneath it:

```sh
$ gulp --tasks
Tasks for ~/project/gulpfile.go
├── clean    Remove the build directory
│   --dry    …Report what would be removed
├── styles   Compile stylesheets
├── scripts
├─┬ build    Build everything
│ └─┬ <series>
│   ├── clean
│   └─┬ <parallel>
│     ├── styles
│     └── scripts
└─┬ default
  └─┬ build
```

> Note: gulp's own JavaScript documentation shows a `[20:58:55]` timestamp on
> each of these lines. gulp-cli 3 removed it deliberately, and so does this
> port.

Descriptions and flags come from the task itself:

```go
clean := gulp.Task("clean", cleanTask)
clean.Description = "Remove the build directory"
clean.Flags = map[string]string{"--dry": "Report what would be removed"}
```

`gulp --tasks-simple` prints one task name per line, in registration order,
with no tree and no colour — the form to pipe into other tools.

`gulp --tasks-json` prints the same tree as JSON. Give it a path to write a
file instead of printing:

```sh
gulp --tasks-json tasks.json
```

## Version

```sh
$ gulp -v
CLI version: 5.0.1
Local version: 5.0.1
```

In JavaScript these two numbers can differ, because the CLI is installed
globally and the library locally. A Go gulpfile compiles the library into the
binary, so there is only one version and it is reported twice for
compatibility with scripts that parse this output.

## `INIT_CWD`

gulp sets `INIT_CWD` to the directory it was invoked from, before applying
`--cwd` or `--gulpfile`. Tasks can read it to resolve paths relative to where
the user actually stood.

## Unimplemented

`--verify`, which checks dependencies against a plugin blacklist, has no
counterpart. The blacklist is a list of deprecated npm packages, and this port
has no npm dependencies.

[series]: api/series.md
