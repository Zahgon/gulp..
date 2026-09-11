package pipeline

import (
	"context"

	"github.com/gulpjs/gulp-go/vinyl"
)

// Send writes f to out, aborting if ctx is cancelled. Transform
// implementations should use it instead of a bare channel send so that a
// failure elsewhere in the pipeline can unblock them.
func Send(ctx context.Context, out chan<- *vinyl.File, f *vinyl.File) error {
	select {
	case out <- f:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recv reads the next file from in, aborting if ctx is cancelled. The second
// return value is false when the input has been exhausted.
func Recv(ctx context.Context, in <-chan *vinyl.File) (*vinyl.File, bool, error) {
	select {
	case f, ok := <-in:
		return f, ok, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

// Map builds a Transform that applies fn to each file in order.
//
// Returning a nil file drops it from the stream, which is how a JS transform
// signals "filtered out" by calling the callback with no value.
func Map(fn func(ctx context.Context, f *vinyl.File) (*vinyl.File, error)) Transform {
	return TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for {
			f, ok, err := Recv(ctx, in)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			res, err := fn(ctx, f)
			if err != nil {
				return err
			}
			if res == nil {
				continue
			}
			if err := Send(ctx, out, res); err != nil {
				return err
			}
		}
	})
}

// Filter builds a Transform that forwards only the files for which keep
// returns true.
func Filter(keep func(*vinyl.File) bool) Transform {
	return Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if keep(f) {
			return f, nil
		}
		return nil, nil
	})
}

// Tap builds a Transform that observes each file and forwards it unchanged.
// Useful for logging and for assertions in tests.
func Tap(fn func(*vinyl.File) error) Transform {
	return Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if err := fn(f); err != nil {
			return nil, err
		}
		return f, nil
	})
}

// Flush builds a Transform that buffers the whole stream, then calls fn once
// with every file and emits whatever fn returns. This is the analogue of a JS
// through2 transform that does its work in the flush callback -- concat
// plugins are the canonical example.
func Flush(fn func(ctx context.Context, files []*vinyl.File) ([]*vinyl.File, error)) Transform {
	return TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
		var buf []*vinyl.File
		for {
			f, ok, err := Recv(ctx, in)
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			buf = append(buf, f)
		}
		res, err := fn(ctx, buf)
		if err != nil {
			return err
		}
		for _, f := range res {
			if err := Send(ctx, out, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// From builds a source Transform that emits a fixed set of files. It is the
// counterpart of vinyl-source-stream / gulp.src for synthesised files.
func From(files ...*vinyl.File) Transform {
	return TransformFunc(func(ctx context.Context, _ <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for _, f := range files {
			if err := Send(ctx, out, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// Discard builds a terminal Transform that consumes every file and emits
// nothing.
func Discard() Transform {
	return TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, _ chan<- *vinyl.File) error {
		for {
			_, ok, err := Recv(ctx, in)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
	})
}
