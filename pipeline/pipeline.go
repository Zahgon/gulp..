// Package pipeline provides the streaming primitive that replaces Node's
// object-mode streams.
//
// In the JS implementation a gulp task looks like:
//
//	src('in/*.js').pipe(pluginA()).pipe(pluginB()).pipe(dest('out/'))
//
// where each stage is a Node Transform stream carrying vinyl objects, with
// backpressure handled by the stream machinery and failures surfaced as
// 'error' events.
//
// The Go port models a stage as a Transform reading from and writing to
// channels of *vinyl.File. Backpressure comes from the bounded channels,
// cancellation from context.Context, and errors are returned rather than
// emitted -- the first stage to fail cancels the whole pipeline.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gulpjs/gulp-go/vinyl"
)

// bufferSize matches Node's default object-mode highWaterMark of 16, so the
// amount of in-flight work between stages is comparable to the JS original.
const bufferSize = 16

// Transform is a single stage of a pipeline.
//
// Implementations read files from in until it is closed, write results to out,
// and return. The pipeline runner closes out on the implementation's behalf,
// so a Transform must never close out itself.
//
// A source stage (such as vfs.Src) receives an in channel that is already
// closed and simply produces files.
type Transform interface {
	Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error
}

// TransformFunc adapts a plain function to the Transform interface.
type TransformFunc func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error

// Transform implements Transform.
func (f TransformFunc) Transform(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	return f(ctx, in, out)
}

// Pipeline is a lazily-started chain of Transforms.
//
// Nothing runs until one of the terminal methods (Run, Collect, Each, Start)
// is called. This laziness is deliberate: gulp 5.0.1 fixed a bug where
// globbing began before the read stream was opened, and the Go port preserves
// the corrected behaviour.
type Pipeline struct {
	stages []Transform
	// err records a construction-time failure (for example an invalid glob
	// or an invalid dest folder) so it can be reported when the pipeline is
	// finally run, mirroring how JS constructors emit deferred errors.
	err error
}

// New starts a pipeline from a source Transform.
func New(source Transform) *Pipeline {
	return &Pipeline{stages: []Transform{source}}
}

// Failed returns a pipeline that reports err from every terminal method. It
// lets constructors defer reporting instead of panicking.
func Failed(err error) *Pipeline {
	return &Pipeline{err: err}
}

// Pipe appends a stage, mirroring JS `.pipe(...)`.
func (p *Pipeline) Pipe(t Transform) *Pipeline {
	if p.err != nil {
		return p
	}
	if t == nil {
		p.err = errors.New("pipeline: nil transform passed to Pipe")
		return p
	}
	p.stages = append(p.stages, t)
	return p
}

// PipeAll appends several stages in order.
func (p *Pipeline) PipeAll(ts ...Transform) *Pipeline {
	for _, t := range ts {
		p.Pipe(t)
	}
	return p
}

// Err returns any construction-time error recorded so far.
func (p *Pipeline) Err() error { return p.err }

// Start runs the pipeline and returns the output channel of the final stage
// plus a channel that yields the terminal error (nil on success) exactly once
// after the output channel is closed.
//
// The caller must drain the returned file channel; failing to do so blocks the
// pipeline. Run, Collect and Each handle this automatically.
func (p *Pipeline) Start(ctx context.Context) (<-chan *vinyl.File, <-chan error) {
	errCh := make(chan error, 1)
	if p.err != nil {
		out := make(chan *vinyl.File)
		close(out)
		errCh <- p.err
		close(errCh)
		return out, errCh
	}
	if len(p.stages) == 0 {
		out := make(chan *vinyl.File)
		close(out)
		errCh <- nil
		close(errCh)
		return out, errCh
	}

	// Each stage gets its own cancellable context derived from a shared one,
	// so the first failure tears down every other stage promptly.
	runCtx, cancel := context.WithCancel(ctx)

	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	record := func(stageIdx int, err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = fmt.Errorf("pipeline stage %d: %w", stageIdx, err)
			cancel()
		}
	}

	// The source stage reads from an already-closed channel.
	closed := make(chan *vinyl.File)
	close(closed)

	var in <-chan *vinyl.File = closed
	for i, stage := range p.stages {
		out := make(chan *vinyl.File, bufferSize)
		wg.Add(1)
		go func(idx int, st Transform, src <-chan *vinyl.File, dst chan<- *vinyl.File) {
			defer wg.Done()
			defer close(dst)
			defer func() {
				// A panicking plugin must not take the process down; convert
				// it into a pipeline error the way a thrown exception in a JS
				// transform becomes an 'error' event.
				if r := recover(); r != nil {
					record(idx, fmt.Errorf("panic: %v", r))
				}
			}()
			record(idx, st.Transform(runCtx, src, dst))
			// Drain any remaining input so upstream stages are never left
			// blocked on a send after this stage returns early.
			for range src { //nolint:revive // intentional drain
			}
		}(i, stage, in, out)
		in = out
	}

	final := in
	go func() {
		wg.Wait()
		cancel()
		mu.Lock()
		err := firstErr
		mu.Unlock()
		if err == nil {
			// Surface cancellation from the caller's context.
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
		}
		errCh <- err
		close(errCh)
	}()

	return final, errCh
}

// Run executes the pipeline, discarding the files produced by the final stage.
// This is the terminal method a gulp task uses when the last stage is dest().
func (p *Pipeline) Run(ctx context.Context) error {
	return p.Each(ctx, nil)
}

// Each executes the pipeline, invoking fn for every file emitted by the final
// stage. Returning an error from fn aborts the pipeline. A nil fn discards.
func (p *Pipeline) Each(ctx context.Context, fn func(*vinyl.File) error) error {
	files, errCh := p.Start(ctx)
	var cbErr error
	for f := range files {
		if fn == nil || cbErr != nil {
			continue // keep draining so upstream stages can finish
		}
		if err := fn(f); err != nil {
			cbErr = err
		}
	}
	err := <-errCh
	if cbErr != nil {
		return cbErr
	}
	return err
}

// Collect executes the pipeline and returns every file the final stage
// emitted, in order. Primarily used by tests and by plugins that need the
// whole set (for example a concat plugin).
func (p *Pipeline) Collect(ctx context.Context) ([]*vinyl.File, error) {
	var out []*vinyl.File
	err := p.Each(ctx, func(f *vinyl.File) error {
		out = append(out, f)
		return nil
	})
	return out, err
}

// AsTask adapts the pipeline to a gulp task function, so a pipeline can be
// registered directly with Task(), Series() or Parallel().
func (p *Pipeline) AsTask() func(context.Context) error {
	return p.Run
}
