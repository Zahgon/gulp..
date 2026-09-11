<!--
id: using-plugins
title: Using Plugins
hide_title: true
sidebar_label: Using Plugins
-->

# Using Plugins

gulp plugins are transforms. A transform receives files, does something with them, and passes them on. Chaining transforms between `Src()` and `Dest()` is how a build is assembled.

In JavaScript a plugin is an npm package that returns a Node transform stream. That ecosystem does not exist in Go, and pretending otherwise would be dishonest. This port replaces it with three things:

1. `pipeline.Transform`, the interface any function can satisfy.
2. `plugins.Exec`, which turns any command-line tool into a transform.
3. A small `plugins` package covering the handful of operations that are effectively part of gulp's vocabulary.

Most builds need only the second and third.

## Using a transform

Every transform goes between `Src` and `Dest` with `Pipe`:

```go
package main

import (
	"context"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/plugins"
)

func scripts(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.js"}).
		Pipe(plugins.Replace("__VERSION__", "1.4.0")).
		Pipe(plugins.Concat("bundle.js")).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`Pipe` returns the pipeline, so the calls chain. Nothing executes until a terminal method — `Run`, `Collect`, `Each` — is called.

## The bundled plugins

| Plugin | Purpose |
| :----- | :------ |
| `plugins.Concat(name)` | Join every file into one, separated by newlines |
| `plugins.ConcatSeparated(name, sep)` | The same, with an explicit separator |
| `plugins.Rename(fn)` | Rewrite a file's directory, basename or extension |
| `plugins.RenameTo(name)` | Rename every file to a fixed name |
| `plugins.Replace(old, new)` | Literal string replacement |
| `plugins.ReplaceRegexp(re, repl)` | Regular-expression replacement with `$1` expansion |
| `plugins.ReplaceFunc(re, fn)` | Replacement computed per match |
| `plugins.Filter(globs)` | Keep only files matching the globs |
| `plugins.FilterFunc(keep)` | Keep only files a predicate accepts |
| `plugins.If(cond, then, otherwise)` | Route files down one of two branches |
| `plugins.SourcemapsInit()` | Attach a source map to each file |
| `plugins.SourcemapsWrite(dir)` | Emit the map inline or as a sibling file |
| `plugins.Exec(opts)` | Run an external command over each file |

### Renaming

`Rename` hands you the three parts of a path, and whatever you leave alone stays as it was:

```go
gulp.Src([]string{"src/**/*.css"}).
	Pipe(plugins.Rename(func(p *plugins.Path) {
		p.Basename += ".min"
	})).
	Pipe(gulp.Dest("dist"))
```

`src/site/home.css` becomes `dist/site/home.min.css`. Setting `p.Dirname = "."` flattens the output into a single directory.

### Conditional branches

`If` sends each file to one of two transforms based on a predicate. Either branch may be `nil`, which passes the file through untouched:

```go
isVendor := func(f *gulp.File) bool {
	rel, err := f.Relative()
	return err == nil && strings.HasPrefix(rel, "vendor/")
}

gulp.Src([]string{"src/**/*.js"}).
	Pipe(plugins.If(isVendor, nil, plugins.Replace("__DEV__", "false"))).
	Pipe(gulp.Dest("dist"))
```

> **Note:** `If` runs both branches concurrently, so the order files leave it is not the order they entered. This matches `gulp-if`. Add a `Flush`-based transform such as `Concat` afterwards if order matters.

## Wrapping a command-line tool

`plugins.Exec` is how you use tools that were never written for gulp. It runs a command once per file, writes the file's contents to the command's stdin, and replaces them with what the command writes to stdout:

```go
var minify = plugins.Exec(plugins.ExecOptions{
	Name:    "terser",
	Command: "terser",
	Args: func(f *gulp.File) []string {
		return []string{"--compress", "--mangle"}
	},
})
```

Some tools refuse to read stdin and demand a real path. Set `TempFile` and use the `{}` placeholder, which is substituted with the path of a temporary copy:

```go
var sass = plugins.Exec(plugins.ExecOptions{
	Name:       "sass",
	Command:    "sass",
	TempFile:   true,
	TempSuffix: ".scss",
	Args: func(f *gulp.File) []string {
		return []string{"--no-source-map", plugins.Placeholder}
	},
	Rename: func(p *plugins.Path) {
		p.Extname = ".css"
	},
})
```

`Rename` adjusts the output path, so `src/site/home.scss` is written as `dist/site/home.css`.

If the command exits non-zero, its stderr is folded into the returned error alongside the file's path, so the failure names both the tool and the file that broke it. Set `IgnoreStderr` for tools that write progress information to stderr on success.

Other options: `Env` adds environment variables, `Dir` overrides the working directory (which otherwise defaults to the file's `Cwd()`).

## Writing your own

A transform is anything with a `Transform` method, but the helpers in `pipeline` cover almost every case. `pipeline.Map` handles one file at a time:

```go
var banner = pipeline.Map(func(ctx context.Context, f *gulp.File) (*gulp.File, error) {
	body, err := f.Bytes()
	if err != nil {
		return nil, err
	}
	f.Contents = vinyl.Buffer(append([]byte("/* built by gulp */\n"), body...))
	return f, nil
})
```

Returning `nil` drops the file. `pipeline.Filter`, `pipeline.Tap` and `pipeline.Flush` cover filtering, side effects and whole-stream operations respectively.

[Writing a plugin][writing-a-plugin] covers this in depth, including how to handle streaming contents and how to test a transform.

## What replaced the npm ecosystem

If you are porting a JavaScript build, the usual answer is one of:

| JavaScript plugin | Go replacement |
| :---------------- | :------------- |
| `gulp-concat` | `plugins.Concat` |
| `gulp-rename` | `plugins.Rename` |
| `gulp-replace` | `plugins.Replace` |
| `gulp-filter` | `plugins.Filter` |
| `gulp-if` | `plugins.If` |
| `gulp-sourcemaps` | `plugins.SourcemapsInit` / `plugins.SourcemapsWrite` |
| `gulp-sass`, `gulp-terser`, `gulp-postcss`, … | `plugins.Exec` around the tool's own CLI |
| `gulp-uglify`, `gulp-babel`, … | `plugins.Exec` around `esbuild`, `swc`, … |
| Anything else | `pipeline.Map` |

The `Exec` route is usually better than it sounds: `esbuild`, `sass`, `terser` and `postcss` all ship real command-line interfaces, and running them directly avoids a layer of wrapper that in JavaScript is frequently the thing that breaks.

---

Next: [Watching files][watching-files]

[writing-a-plugin]: ../writing-a-plugin/README.md
[watching-files]: 8-watching-files.md
