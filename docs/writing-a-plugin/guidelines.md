<!-- front-matter
name: guidelines
-->

# Guidelines

Rules for a well-behaved plugin. The first group is enforced by the pipeline runner and breaking them causes visible failures; the rest are conventions that keep a plugin composable.

## Must

1. **Do not close the output channel.** The runner owns it and closes it when your `Transform` returns. Closing it yourself panics the next stage. This only applies to plugins that implement `pipeline.Transform` directly; `pipeline.Map` and friends handle it for you.

2. **Do not do work at construction time.** A pipeline may be built and never run, and gulp 5.0.1 fixed a real bug by making `Src` defer its globbing until the pipeline starts. A constructor should validate arguments and return; if validation fails, return a transform that reports the error when it runs rather than panicking.

3. **Respect cancellation.** Use `pipeline.Send` and `pipeline.Recv` rather than raw channel operations, and `exec.CommandContext` rather than `exec.Command`. A stage that ignores `ctx.Done()` keeps a failed build alive until it finishes.

4. **Pass null files and directories through untouched.**

   ```go
   if f.IsNull() || f.IsDirectory() {
       return f, nil
   }
   ```

   This is what makes `Read: gulp.Value(false)` pipelines and directory copies work.

5. **Return a typed error.**

   ```go
   return nil, &plugins.Error{Plugin: "banner", Path: f.Path(), Err: err}
   ```

   The runner's `pipeline stage 2: ...` prefix says where the failure happened but not to which file. Only the plugin knows that.

## Should

6. **Do one thing.** A plugin that minifies and also writes a sourcemap and also renames is three plugins. Composition is free here — `Pipe` costs a goroutine and a buffered channel.

7. **Wrap, do not reimplement.** If `esbuild`, `sass` or `terser` already does the work, `plugins.Exec` is the correct answer. A Go reimplementation of a JavaScript minifier will be wrong in ways that only show up in production.

8. **Do not buffer the stream unless buffering is the feature.** `pipeline.Flush` exists for `Concat` and its relatives. Everything else should stay one file at a time so that memory stays flat on a large glob.

9. **Leave paths alone unless renaming is the point.** Use `SetExtname`, `SetBasename` and `SetStem` rather than `SetPath`, so the file's history stays meaningful. Never touch `Base` — it is what `Dest()` uses to decide the output directory structure.

10. **Do not mutate contents in place.** `Clone` with `CloneOptions{Contents: gulp.Ptr(false)}` shares the underlying slice. Assign a new `vinyl.Buffer` instead.

11. **Keep no per-file state on the plugin value.** Stages run concurrently with the rest of the pipeline, and a shared counter or reused scratch buffer is a data race. If state is genuinely needed, guard it or use `pipeline.Flush`.

## Naming and packaging

12. **Name the package for what it does, not for gulp.** In JavaScript a plugin is `gulp-banner` because npm is a flat namespace and the prefix is how you find it. Go has import paths, so `example.com/build/banner` reads better at the call site:

    ```go
    Pipe(banner.Prepend(notice))
    ```

13. **Export a constructor returning `pipeline.Transform`**, not a struct. That keeps the implementation free to change and makes the call site read as a pipeline stage.

14. **Take options as a struct with useful zero values**, so `banner.Prepend(text)` works and `banner.With(banner.Options{...})` covers the rest. Follow `vfs.SrcOptions` if an option needs to vary per file: `gulp.Value` for a constant and `gulp.Func` for a function.

## Documentation

15. **Document the contents shapes you support.** Say plainly whether a streaming file is handled, buffered, or rejected. The bundled `plugins.Replace` buffers rather than failing, which is a deliberate departure from the JavaScript plugins that throw `Streaming not supported`.

16. **Document what the plugin does to paths**, because that determines where the files land.

---

Next: [Recommended modules][recommended-modules].

[recommended-modules]: recommended-modules.md
