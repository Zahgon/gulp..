<!--
name: bundling-with-esbuild
title: Bundling with esbuild
-->

# Bundling with esbuild

Upstream gulp has five browserify recipes and one for rollup. All of them
solve the same problem: a bundler wants to own the whole build, and gulp wants
to stream files, so the recipes are mostly plumbing to make the two coexist.

esbuild is written in Go and publishes a real Go API, so there is no plumbing.
Call it.

## Bundling with the Go API

```go
import "github.com/evanw/esbuild/pkg/api"

func scripts(ctx context.Context) error {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{"src/js/main.js"},
		Outfile:     "dist/js/main.js",
		Bundle:      true,
		Write:       true,
		Sourcemap:   api.SourceMapLinked,
		Target:      api.ES2020,
	})

	if len(result.Errors) > 0 {
		return fmt.Errorf("esbuild: %s", result.Errors[0].Text)
	}
	return nil
}
```

Add it with `go get github.com/evanw/esbuild`. It brings no transitive
dependencies.

This task never touches `gulp.Src` or `gulp.Dest`, and that is the point. A
bundler already reads the entry point, follows the imports and writes the
output. Feeding it files one at a time through a pipeline would only take that
work away from it and do it worse.

## Bundling into a pipeline

Use `Write: false` when the bundle still has stages to go through — a banner, a
hash in the filename, a manifest:

```go
func scripts(ctx context.Context) error {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{"src/js/main.js"},
		Bundle:      true,
		Write:       false,
		Outfile:     "main.js",
	})
	if len(result.Errors) > 0 {
		return fmt.Errorf("esbuild: %s", result.Errors[0].Text)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	files := make([]*gulp.File, 0, len(result.OutputFiles))
	for _, out := range result.OutputFiles {
		file, err := vinyl.New(vinyl.Options{
			Cwd:      cwd,
			Base:     filepath.Join(cwd, "dist"),
			Path:     out.Path,
			Contents: vinyl.Buffer(out.Contents),
		})
		if err != nil {
			return err
		}
		files = append(files, file)
	}

	return pipeline.New(pipeline.From(files...)).
		Pipe(banner.Prepend("/* built " + time.Now().Format(time.DateOnly) + " */\n")).
		Pipe(gulp.Dest("dist/js")).
		Run(ctx)
}
```

`result.OutputFiles[i].Path` is already absolute, because esbuild resolves
`Outfile` against the working directory. Set `Base` to the directory you want
the output tree to hang from — see [make a vinyl file from memory][memory] for
why `Base` matters as much as `Path`.

## Using the binary instead

If you would rather not add the Go module, or you need a bundler with no Go
API, wrap the binary with `plugins.Exec`:

```go
func scripts(ctx context.Context) error {
	return gulp.Src([]string{"src/js/*.entry.js"}).
		Pipe(plugins.Exec(plugins.ExecOptions{
			Name:     "esbuild",
			Command:  "esbuild",
			TempFile: true,
			Args: func(f *gulp.File) []string {
				return []string{plugins.Placeholder, "--bundle", "--minify"}
			},
			Rename: func(p *plugins.Path) {
				p.Basename = strings.TrimSuffix(p.Basename, ".entry")
			},
		})).
		Pipe(gulp.Dest("dist/js")).
		Run(ctx)
}
```

`TempFile: true` writes each file to a real path and substitutes
`plugins.Placeholder` (`{}`) with it, because a bundler must resolve imports
relative to the entry point and cannot do that reading stdin. The bundle comes
back on stdout.

> The same shape wraps `rollup`, `swc`, `parcel` or anything else with a
> command line. See [using plugins][plugins].

## Watching

esbuild has its own watch mode, which is faster than rebuilding from scratch
because it keeps the module graph in memory. Use it when the bundle is the only
thing you are watching:

```go
func watchScripts(ctx context.Context) error {
	build, err := api.Context(api.BuildOptions{
		EntryPoints: []string{"src/js/main.js"},
		Outfile:     "dist/js/main.js",
		Bundle:      true,
		Write:       true,
	})
	if err != nil {
		return err
	}
	defer build.Dispose()

	if err := build.Watch(api.WatchOptions{}); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
```

Use `gulp.Watch` instead when the rebuild involves more than the bundler —
copying assets, regenerating templates, running tests. Mixing both is fine:
give esbuild its own task and run the two under `gulp.Parallel`.

[memory]: make-stream-from-buffer.md
[plugins]: ../getting-started/7-using-plugins.md
