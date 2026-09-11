package plugins

import (
	"context"

	"github.com/gulpjs/gulp-go/internal/glob"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Filter keeps only the files whose path relative to their base matches one of
// the patterns, the equivalent of gulp-filter.
//
// Patterns use the same syntax as src globs, including "!" for negation, so
// a pipeline can narrow itself part way through:
//
//	Pipe(plugins.Filter([]string{"**/*.js", "!**/*.min.js"}))
//
// Matching is on the relative path rather than the absolute one so that the
// patterns read the same as the src globs that produced the files.
func Filter(patterns []string) pipeline.Transform {
	set, err := glob.CompileSet(patterns, glob.Options{Dot: true})
	if err != nil {
		return failed(pluginErr("filter", nil, err))
	}
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		rel, err := f.Relative()
		if err != nil {
			return nil, pluginErr("filter", f, err)
		}
		if !set.Match(rel) {
			return nil, nil
		}
		return f, nil
	})
}

// FilterFunc keeps only the files for which keep returns true.
func FilterFunc(keep func(*vinyl.File) bool) pipeline.Transform {
	if keep == nil {
		return pipeline.Filter(func(*vinyl.File) bool { return true })
	}
	return pipeline.Filter(keep)
}

// failed produces a transform that reports err as soon as the pipeline runs.
//
// Compiling a bad pattern has to be reported somewhere, and returning an error
// from Filter would force every caller to handle one, breaking the fluent
// chaining that makes a pipeline readable. Deferring it to run time matches
// how a JavaScript plugin would emit the error on its stream.
func failed(err error) pipeline.Transform {
	return pipeline.TransformFunc(func(context.Context, <-chan *vinyl.File, chan<- *vinyl.File) error {
		return err
	})
}
