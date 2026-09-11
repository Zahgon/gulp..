<!--
name: dealing-with-streams
title: Dealing with Streams
-->

# Dealing with Streams

There are two kinds of stream in this port and it is worth keeping them apart.

* The **file stream** is the pipeline itself — a channel of vinyl files moving from `Src` to `Dest`.
* The **contents stream** is a single file's body when `Buffer: gulp.Value(false)` was used, so the bytes are read lazily instead of loaded into memory.

Most plugins only deal with the first. This page is about the second.

## Reading a contents stream

`vinyl.Stream` is an `io.ReadCloser`, so anything in the standard library works:

```go
func Count() pipeline.Transform {
	return pipeline.Map(func(ctx context.Context, f *vinyl.File) (*vinyl.File, error) {
		s, ok := f.Contents.(*vinyl.Stream)
		if !ok {
			return f, nil
		}
		defer s.Close()

		n, err := io.Copy(io.Discard, s)
		if err != nil {
			return nil, err
		}
		f.Set("lines", n)
		return f, nil
	})
}
```

A stream yields its bytes **once**. After that it is drained, and reading again returns nothing. Call `Reset()` to re-arm it if a later stage needs the same bytes:

```go
if err := s.Reset(); err != nil {
	return nil, err
}
```

`Dest` does this for you: after writing a streaming file it replaces `Contents` with a fresh stream over the file it just wrote, so a second `Dest` in the same pipeline works.

## Transforming without buffering

The point of a streaming file is that its body never has to fit in memory. Preserve that by wrapping the reader rather than draining it:

```go
func Upper() pipeline.Transform {
	return pipeline.Map(func(ctx context.Context, f *vinyl.File) (*vinyl.File, error) {
		s, ok := f.Contents.(*vinyl.Stream)
		if !ok {
			return f, nil
		}
		f.Contents = vinyl.NewStream(func() (io.ReadCloser, error) {
			if err := s.Reset(); err != nil {
				return nil, err
			}
			return upperReader{s}, nil
		})
		return f, nil
	})
}
```

`vinyl.NewStream` takes an *opener*, not a reader. Nothing is opened until a later stage actually reads, which is what keeps a pipeline lazy and what lets `Reset` work.

## Implementing Transform directly

`pipeline.Map` handles one file at a time. Implement `pipeline.Transform` when a plugin needs to emit a different number of files than it receives, or needs to interleave work:

```go
func Split() pipeline.Transform {
	return pipeline.TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for {
			f, ok, err := pipeline.Recv(ctx, in)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			head, tail := halve(f)
			if err := pipeline.Send(ctx, out, head); err != nil {
				return err
			}
			if err := pipeline.Send(ctx, out, tail); err != nil {
				return err
			}
		}
	})
}
```

Three rules apply and the runner enforces none of them for you:

* **Never close `out`.** The runner closes it after `Transform` returns. Closing it yourself panics the next stage.
* **Always use `Send` and `Recv`** rather than raw channel operations. They select on `ctx.Done()`, so a cancelled build unblocks immediately instead of deadlocking.
* **Return on the first error.** The runner cancels the shared context, which unblocks every other stage.

A source stage — one that produces files rather than transforming them — receives an already-closed `in` and simply ignores it.

## Back pressure

Stages are connected by buffered channels sized to match Node's object-mode high-water mark. A slow stage therefore stops the ones ahead of it rather than letting them read the whole tree into memory. This is automatic; a plugin gets it by using `Send`.

If a plugin *does* need the whole stream — a concatenator, a manifest writer — use `pipeline.Flush` and be explicit about it, rather than accumulating in a `Transform` and hoping the input is small.

---

Next: [Testing][testing]

[testing]: testing.md
[buffers]: using-buffers.md
