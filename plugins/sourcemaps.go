package plugins

import (
	"context"

	"github.com/gulpjs/gulp-go/internal/sourcemap"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// SourcemapsInit attaches a source map to each file, loading an existing one
// from an inline comment or a neighbouring .map file when there is one and
// synthesising an identity map when there is not.
//
// It is the equivalent of gulp-sourcemaps.init, and of passing
// sourcemaps: true to src. Use it when the files did not come from src, or
// when a map has to be attached part way through a pipeline.
func SourcemapsInit() pipeline.Transform {
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if passthrough(f) {
			return f, nil
		}
		if err := sourcemap.Add(f); err != nil {
			return nil, pluginErr("sourcemaps", f, err)
		}
		return f, nil
	})
}

// SourcemapsWrite emits the accumulated source maps.
//
// An empty dir inlines each map as a base64 data URI comment; otherwise the
// map is written to a sibling file under dir and referenced by a relative
// sourceMappingURL comment. It is the equivalent of gulp-sourcemaps.write, and
// of passing sourcemaps to dest.
//
// The map file is emitted immediately after the file it belongs to, so a
// following dest() writes both.
func SourcemapsWrite(dir string) pipeline.Transform {
	return pipeline.TransformFunc(func(ctx context.Context, in <-chan *vinyl.File, out chan<- *vinyl.File) error {
		for {
			f, ok, err := pipeline.Recv(ctx, in)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			var mapFile *vinyl.File
			if !passthrough(f) && f.SourceMap != nil {
				mapFile, err = sourcemap.Write(f, dir)
				if err != nil {
					return pluginErr("sourcemaps", f, err)
				}
			}
			if err := pipeline.Send(ctx, out, f); err != nil {
				return err
			}
			if mapFile != nil {
				if err := pipeline.Send(ctx, out, mapFile); err != nil {
					return err
				}
			}
		}
	})
}
