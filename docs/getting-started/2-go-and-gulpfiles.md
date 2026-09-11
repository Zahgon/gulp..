<!-- front-matter
id: go-and-gulpfiles
title: Go and Gulpfiles
hide_title: true
sidebar_label: Go and Gulpfiles
-->

# Go and Gulpfiles

> The JavaScript edition of this page is called *JavaScript and Gulpfiles* and
> spends most of its length on transpilers, `.babelrc` files and the difference
> between CommonJS and ES modules. None of that applies here. A Go gulpfile is
> compiled by the Go toolchain, so there is no loader, no transpiler and no
> module-format question to answer.

## Gulpfile explained

Your gulpfile lives in your project root. It is `package main`, it imports
`gulp`, and it ends by calling `gulp.Main()`:

```go
package main

import gulp "github.com/gulpjs/gulp-go"

func main() {
	// register tasks here
	gulp.Main()
}
```

`gulp.Main()` parses the command line, runs whichever tasks were asked for, and
exits with a non-zero status if any of them failed. Everything before it is
registration.

The import is aliased to `gulp` because the module path ends in `gulp-go` while
the package is named `gulp`. Go can work this out on its own, but the explicit
alias makes the file readable to someone who has never seen it.

## Splitting the gulpfile

A gulpfile is a Go package, so it can be more than one file. Any `.go` file in
the same directory is part of the same package:

```
gulpfile.go
tasks_styles.go
tasks_scripts.go
```

```go
// tasks_styles.go
package main

import (
	"context"

	gulp "github.com/gulpjs/gulp-go"
)

func styles(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.css"}).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

```go
// gulpfile.go
package main

import gulp "github.com/gulpjs/gulp-go"

func main() {
	gulp.Task("styles", styles)
	gulp.Task("scripts", scripts)
	gulp.Main()
}
```

If you prefer to keep the project root clean, put the whole thing in a
`gulpfile/` directory with a `main.go` inside. The launcher looks there too.

## Sharing tasks between projects

Because tasks are ordinary Go functions, a shared build can be an ordinary Go
module:

```go
import "example.com/buildkit"

func main() {
	buildkit.Register()   // registers its tasks on gulp.Default
	gulp.Main()
}
```

A library that does not want to touch the shared registry can create its own
with `gulp.New()` and hand it back to the caller.

## The default instance

`gulp.Task`, `gulp.Src` and the rest are package-level functions that operate
on `gulp.Default`, a shared instance created at init time. This mirrors the
JavaScript module, which exports one `new Gulp()`.

When you need isolation — most often in tests — build your own:

```go
g := gulp.New()
g.Task("build", build)
err := g.Run(ctx, gulp.Name("build"))
```

## Compilation is part of the build

Running `go run .` compiles the gulpfile before executing it. A typo in a task
is a compile error, not a crash halfway through a deploy. This is the single
biggest practical difference from the JavaScript gulp, and it is worth leaning
on: prefer real function references over strings where you can, since
`gulp.Name("bulid")` is only caught at run time while a misspelled Go
identifier is caught immediately.

## Next

Continue to [Creating Tasks][creating-tasks].

[creating-tasks]: 3-creating-tasks.md
