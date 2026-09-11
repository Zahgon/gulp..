package plugins

import (
	"context"
	"path/filepath"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Path is a file path split the way gulp-rename splits it, so that a rename
// can change one part without rebuilding the whole string.
//
// Dirname is relative to the file's base, which is the directory structure
// dest() will recreate. Basename excludes the extension and Extname includes
// its leading dot, so "src/js/app.min.js" with a base of "src" arrives as
// {Dirname: "js", Basename: "app.min", Extname: ".js"}.
type Path struct {
	Dirname  string
	Basename string
	Extname  string
}

// Rename rewrites each file's path through fn.
//
// This covers every form of gulp-rename's argument. A prefix or suffix is
// string concatenation, a directory move is an assignment, and flattening is
// setting Dirname to ".":
//
//	plugins.Rename(func(p *plugins.Path) {
//		p.Dirname = "."
//		p.Basename += ".min"
//	})
//
// Directories are renamed along with files, matching gulp-rename, so a
// pipeline that creates directory entries keeps them consistent.
func Rename(fn func(*Path)) pipeline.Transform {
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if fn == nil {
			return f, nil
		}
		return renameOne(f, fn, "rename")
	})
}

// RenameTo gives every file the same name, which is the string form of
// gulp-rename's argument. It is most useful after Concat.
func RenameTo(name string) pipeline.Transform {
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if err := f.SetPath(filepath.Join(f.Base(), name)); err != nil {
			return nil, pluginErr("rename", f, err)
		}
		return f, nil
	})
}
