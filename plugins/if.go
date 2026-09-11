package plugins

import (
	"context"
	"sync"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// If routes each file through then when cond returns true and through
// otherwise when it does not, the equivalent of gulp-if.
//
// Either branch may be nil, in which case the files it would have received
// pass straight through:
//
//	Pipe(plugins.If(isProduction, plugins.Exec(minify), nil))
//
// Both branches run concurrently and their outputs are merged as they arrive,
// so the order files leave If is not the order they entered. gulp-if has the
// same property, and a pipeline that depends on file order should use Flush or
// Concat, which see the whole stream at once.
func If(cond func(*vinyl.File) bool, then, otherwise pipeline.Transform) pipeline.Transform {
	return pipeline.TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
		if cond == nil {
			return forward(ctx, in, out)
		}

		yes := newBranch(ctx, then, out)
		no := newBranch(ctx, otherwise, out)

		var err error
		for {
			f, ok, recvErr := pipeline.Recv(ctx, in)
			if recvErr != nil {
				err = recvErr
				break
			}
			if !ok {
				break
			}
			target := no
			if cond(f) {
				target = yes
			}
			if sendErr := target.send(f); sendErr != nil {
				err = sendErr
				break
			}
		}

		if closeErr := yes.close(); err == nil {
			err = closeErr
		}
		if closeErr := no.close(); err == nil {
			err = closeErr
		}
		return err
	})
}

// branch is one side of an If: a transform running in its own goroutine, fed
// by a channel and writing into the shared output.
type branch struct {
	ctx  context.Context
	in   chan *vinyl.File
	done chan error
	once sync.Once
}

// newBranch starts t, or a pass-through when t is nil, forwarding into out.
//
// out is shared with the sibling branch, so neither goroutine may close it;
// the pipeline runner owns that. Each branch therefore signals completion on
// its own done channel instead.
func newBranch(ctx context.Context, t pipeline.Transform, out chan<- *vinyl.File) *branch {
	b := &branch{
		ctx:  ctx,
		in:   make(chan *vinyl.File),
		done: make(chan error, 1),
	}
	go func() {
		defer close(b.done)
		if t == nil {
			b.done <- forward(ctx, b.in, out)
			return
		}
		b.done <- t.Transform(ctx, b.in, out)
	}()
	return b
}

func (b *branch) send(f *vinyl.File) error {
	select {
	case b.in <- f:
		return nil
	case err := <-b.done:
		if err != nil {
			return err
		}
		return context.Canceled
	case <-b.ctx.Done():
		return b.ctx.Err()
	}
}

func (b *branch) close() error {
	b.once.Do(func() { close(b.in) })
	return <-b.done
}

func forward(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
	for {
		f, ok, err := pipeline.Recv(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := pipeline.Send(ctx, out, f); err != nil {
			return err
		}
	}
}
