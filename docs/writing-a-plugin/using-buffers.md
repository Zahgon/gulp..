<!--
name: using-buffers
title: Using buffers
hide_title: true
sidebar_label: Using buffers
-->

# Using buffers

A file read with the default options carries its contents as a `vinyl.Buffer`,
which is a `[]byte`. This is the case a plugin should handle first, because it
is what almost every pipeline produces.

## Reading

`vinyl.Buffer` is a defined slice type, so it converts directly:

```go
body, ok := f.Contents.(vinyl.Buffer)
if !ok {
	return nil, errors.New("expected buffered contents")
}
fmt.Println(len(body))
```

Prefer `f.Bytes()` when the plugin should work regardless of how the file was
read. It returns the buffer as-is, drains a stream into memory, and returns an
empty slice for a null file:

```go
body, err := f.Bytes()
if err != nil {
	return nil, err
}
```

## Writing

Assign a new `vinyl.Buffer`. Do not mutate the existing one in place: `Clone`
copies the buffer by default but shares it when called with
`CloneOptions{Contents: gulp.Ptr(false)}`, so an in-place edit can reach a file
another branch of the build is still holding.

```go
f.Contents = vinyl.Buffer(append([]byte("/* built */\n"), body...))
```

There is no size field to update. `f.Stat.ByteSize` still describes the file on
disk, and `Dest()` refreshes the whole `Stat` from the destination after
writing.

## The three shapes

Every plugin has to decide what to do with each of them:

| Shape | Test | Usual handling |
| :---- | :--- | :------------- |
| Buffer | `f.IsBuffer()` | Transform it. |
| Stream | `f.IsStream()` | Transform incrementally, or buffer it — see [dealing with streams][streams]. |
| Null | `f.IsNull()` | Pass through untouched. |

A directory also has null contents, and `f.IsDirectory()` distinguishes it. The
bundled plugins share one predicate for both:

```go
func passthrough(f *vinyl.File) bool {
	return f.IsNull() || f.IsDirectory()
}
```

Passing null files through matters more than it looks. `gulp.Src` with
`Read: gulp.Value(false)` produces a stream of paths with no contents, which is
how a build deletes or lists files without reading them. A plugin that errors
on null contents breaks that pattern.

## Buffering a stream

If the operation genuinely needs the whole file — a regular-expression rewrite,
a checksum, a parser — buffer it rather than refusing:

```go
body, err := f.Bytes()
if err != nil {
	return nil, err
}
f.Contents = vinyl.Buffer(transform(body))
```

This is what `plugins.Replace` does. The equivalent JavaScript plugins throw
`Streaming not supported`; buffering is friendlier and costs the same memory
the caller would have spent by not passing `Buffer: gulp.Value(false)`.

Do not buffer when the plugin could stream. A file large enough to matter is
exactly the file the caller opted out of buffering for.

## Encodings

`Src` decodes to UTF-8 and strips a UTF-8 BOM before a plugin sees the
contents, so a buffer is plain UTF-8 unless `Encoding` was changed or set to
`gulp.Value("")` to disable decoding. `Dest` re-encodes on the way out. A
plugin should not do its own encoding work.

## Next

Read [dealing with streams][streams] for the incremental case.

[streams]: dealing-with-streams.md
