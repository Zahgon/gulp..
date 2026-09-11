package plugins

import (
	"bytes"
	"context"
	"regexp"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Replace substitutes every occurrence of search with replacement, the string
// form of gulp-replace.
func Replace(search, replacement string) pipeline.Transform {
	from, to := []byte(search), []byte(replacement)
	return rewrite("replace", func(body []byte, _ *vinyl.File) ([]byte, error) {
		return bytes.ReplaceAll(body, from, to), nil
	})
}

// ReplaceRegexp substitutes every match of re. The replacement may reference
// capture groups as $1 or ${name}, following [regexp.Regexp.ReplaceAll].
func ReplaceRegexp(re *regexp.Regexp, replacement string) pipeline.Transform {
	repl := []byte(replacement)
	return rewrite("replace", func(body []byte, f *vinyl.File) ([]byte, error) {
		if re == nil {
			return nil, pluginErrf("replace", f, "nil regexp")
		}
		return re.ReplaceAll(body, repl), nil
	})
}

// ReplaceFunc substitutes every match of re with the result of fn, the
// function form of gulp-replace. fn receives the whole match.
func ReplaceFunc(re *regexp.Regexp, fn func(match string) string) pipeline.Transform {
	return rewrite("replace", func(body []byte, f *vinyl.File) ([]byte, error) {
		if re == nil || fn == nil {
			return nil, pluginErrf("replace", f, "nil regexp or function")
		}
		return re.ReplaceAllFunc(body, func(m []byte) []byte {
			return []byte(fn(string(m)))
		}), nil
	})
}

// rewrite builds a transform that replaces a file's contents wholesale.
//
// Buffering a streaming file here rather than refusing it is a deliberate
// departure from the JavaScript plugins, which error on buffer: false. A
// content rewrite cannot be done without seeing the whole file anyway, so the
// choice is between doing it and failing, and gulp-replace's own streaming
// support does exactly this.
func rewrite(name string, fn func([]byte, *vinyl.File) ([]byte, error)) pipeline.Transform {
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if passthrough(f) {
			return f, nil
		}
		body, err := f.Bytes()
		if err != nil {
			return nil, pluginErr(name, f, err)
		}
		out, err := fn(body, f)
		if err != nil {
			return nil, err
		}
		f.Contents = vinyl.Buffer(out)
		return f, nil
	})
}
