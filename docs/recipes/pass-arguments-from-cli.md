<!-- front-matter
name: Pass arguments from the CLI
-->

# Pass arguments from the CLI

The CLI keeps every flag it does not recognise, so a task can read its own
options without any argument-parsing library.

```console
$ gulp build --env=production --minify
```

## Reading the flags

Unrecognised flags land in `Options.Extra`, keyed without the leading dashes.
A `--name=value` flag stores its value; a bare `--name` stores `"true"`; and
`--no-name` stores `"false"`.

```go
package main

import (
	"context"
	"os"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/internal/cli"
)

func main() {
	gulp.Task("build", build)
	gulp.Main()
}
```

`internal/cli` is not importable from outside this module, so read the flags
from `os.Args` in your own gulpfile:

```go
func flagValue(name, fallback string) string {
	prefix := "--" + name + "="
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--"+name:
			return "true"
		case arg == "--no-"+name:
			return "false"
		case strings.HasPrefix(arg, prefix):
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return fallback
}

func build(ctx context.Context) error {
	env := flagValue("env", "development")
	minify := flagValue("minify", "false") == "true"
	...
}
```

## Using the standard flag package

For anything more than a couple of options, parse into a struct once at startup
and let the tasks read the result. Register the flags on your own `FlagSet` so
they do not collide with the CLI's:

```go
type config struct {
	Env    string
	Minify bool
}

var cfg config

func parseConfig() {
	fs := flag.NewFlagSet("gulpfile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Env, "env", "development", "build environment")
	fs.BoolVar(&cfg.Minify, "minify", false, "minify output")

	// Ignore the error: the CLI's own flags and the task names are in here
	// too, and they are not this FlagSet's business.
	_ = fs.Parse(os.Args[1:])
}

func main() {
	parseConfig()
	gulp.Task("build", build)
	gulp.Main()
}
```

## Environment variables

A build environment is often better expressed as an environment variable,
because it applies to every task in the run rather than to one invocation:

```console
$ APP_ENV=production gulp build
```

```go
env := cmp.Or(os.Getenv("APP_ENV"), "development")
```

## Documenting the flags

Attach the flags to the task so `gulp --tasks` lists them:

```go
build := gulp.Task("build", build)
build.Description = "Compile the site"
build.Flags = map[string]string{
	"--env":    "Build environment (development or production)",
	"--minify": "Minify the output",
}
```

```console
$ gulp --tasks
Tasks for ~/site/gulpfile.go
└── build      Compile the site
    --env      …Build environment (development or production)
    --minify   …Minify the output
```

[cli]: ../CLI.md
[task]: ../api/task.md
