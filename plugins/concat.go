package plugins

import (
	"bytes"
	"context"
	"path/filepath"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// DefaultSeparator joins concatenated files, matching gulp-concat's newLine.
const DefaultSeparator = "\n"

// Concat joins every incoming file into one named file, separated by newlines.
//
// The result inherits its cwd and base from the first file, so a later dest()
// writes it beside where those files came from. Nothing is emitted when the
// stream is empty, which is how gulp-concat behaves and what keeps an empty
// glob from producing a stray empty file.
func Concat(name string) pipeline.Transform {
	return ConcatSeparated(name, DefaultSeparator)
}

// ConcatSeparated is Concat with an explicit separator. Pass "" to join the
// files with nothing between them.
func ConcatSeparated(name, separator string) pipeline.Transform {
	return pipeline.Flush(func(_ context.Context, files []*vinyl.File) ([]*vinyl.File, error) {
		var (
			joined bytes.Buffer
			first  *vinyl.File
			n      int
		)
		for _, f := range files {
			if passthrough(f) {
				continue
			}
			body, err := f.Bytes()
			if err != nil {
				return nil, pluginErr("concat", f, err)
			}
			if n > 0 {
				joined.WriteString(separator)
			}
			joined.Write(body)
			if first == nil {
				first = f
			}
			n++
		}
		if first == nil {
			return nil, nil
		}
		out, err := vinyl.New(vinyl.Options{
			Cwd:      first.Cwd(),
			Base:     first.Base(),
			Path:     filepath.Join(first.Base(), name),
			Contents: vinyl.Buffer(joined.Bytes()),
		})
		if err != nil {
			return nil, pluginErr("concat", first, err)
		}
		return []*vinyl.File{out}, nil
	})
}
