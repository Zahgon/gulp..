<!--
name: templating-with-front-matter
title: Templating with front matter
-->

# Templating with front matter

Upstream gulp has a recipe for `gulp-front-matter` plus Swig. Go needs neither
package: `html/template` is in the standard library, and splitting a file at its
front matter delimiter is a few lines.

The goal is to turn this:

```markdown
---
title: Release notes
layout: page
---

# 5.0.1

Avoid globbing before the read stream is opened.
```

into a full HTML page, using a layout stored somewhere else.

## Splitting the front matter

Front matter is the block between the first two `---` lines. Parse it off the
front of the contents and keep the rest as the body.

```go
package frontmatter

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

var delimiter = []byte("---")

// Split separates a leading YAML front matter block from the body. A file
// without front matter is returned unchanged with empty metadata, because a
// plain page is not an error.
func Split(contents []byte) (meta map[string]any, body []byte, err error) {
	rest, ok := bytes.CutPrefix(contents, delimiter)
	if !ok {
		return nil, contents, nil
	}
	rest = bytes.TrimLeft(rest, "\r\n")

	end := bytes.Index(rest, append([]byte("\n"), delimiter...))
	if end < 0 {
		return nil, nil, fmt.Errorf("front matter is not closed")
	}

	if err := yaml.Unmarshal(rest[:end], &meta); err != nil {
		return nil, nil, fmt.Errorf("front matter: %w", err)
	}

	body = rest[end+len(delimiter)+1:]
	return meta, bytes.TrimLeft(body, "\r\n"), nil
}
```

> `gopkg.in/yaml.v3` is the one dependency worth adding here. If the metadata is
> only ever `key: value` pairs you can skip it and split each line on the first
> colon, but YAML front matter that grows a list will then fail silently.

## Storing metadata on the file

A vinyl file carries arbitrary custom properties, which is exactly what the
metadata is. Attach it in one stage so later stages can read it.

```go
func extractFrontMatter() gulp.Transform {
	return pipeline.Map(func(_ context.Context, f *gulp.File) (*gulp.File, error) {
		body, err := f.Bytes()
		if err != nil {
			return nil, err
		}

		meta, rest, err := frontmatter.Split(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Relative(), err)
		}

		f.Set("frontMatter", meta)
		f.Contents = vinyl.Buffer(rest)
		return f, nil
	})
}
```

`f.Set` stores the value under a custom property. `f.Get("frontMatter")` reads it
back, and `Clone()` carries it along.

## Rendering

Parse the layouts once, outside the per-file stage, so a hundred pages do not
re-parse the same template a hundred times.

```go
func render(layouts *template.Template) gulp.Transform {
	return pipeline.Map(func(_ context.Context, f *gulp.File) (*gulp.File, error) {
		body, err := f.Bytes()
		if err != nil {
			return nil, err
		}

		meta, _ := f.Get("frontMatter")
		data, _ := meta.(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		data["Content"] = template.HTML(body)

		name, _ := data["layout"].(string)
		if name == "" {
			name = "default"
		}

		var out bytes.Buffer
		if err := layouts.ExecuteTemplate(&out, name+".html", data); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Relative(), err)
		}

		f.Contents = vinyl.Buffer(out.Bytes())
		return f, nil
	})
}

func pages(ctx context.Context) error {
	layouts, err := template.ParseGlob("layouts/*.html")
	if err != nil {
		return err
	}

	return gulp.Src([]string{"content/**/*.md"}).
		Pipe(extractFrontMatter()).
		Pipe(render(layouts)).
		Pipe(plugins.Rename(func(p *plugins.Path) { p.Extname = ".html" })).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`template.HTML` marks the body as already-safe markup. Leave it as a plain
string and `html/template` will escape every tag, which is the right default for
untrusted input and the wrong one for your own content.

> `html/template` escapes by context — a value in an `href` is escaped
> differently from one in a `<script>`. That is why it is worth the extra import
> over `text/template` for anything that becomes a web page.

## Converting Markdown

The body above is still Markdown. Render it before handing it to the layout:

```go
import "github.com/yuin/goldmark"

var markdown = goldmark.New()

// inside the render stage, before ExecuteTemplate:
var html bytes.Buffer
if err := markdown.Convert(body, &html); err != nil {
	return nil, fmt.Errorf("%s: %w", f.Relative(), err)
}
data["Content"] = template.HTML(html.String())
```

A single `goldmark.Markdown` value is safe for concurrent use, so a package-level
instance is fine even though pipeline stages run in goroutines.

## Reloading templates while watching

Parsing the layouts inside the task rather than at startup means a layout edit is
picked up on the next run:

```go
gulp.Task("watch", func(ctx context.Context) error {
	w, err := gulp.Watch(
		[]string{"content/**/*.md", "layouts/**/*.html"},
		gulp.WatchOptions{},
		pages,
	)
	if err != nil {
		return err
	}
	defer w.Close()

	<-ctx.Done()
	return nil
})
```

Watching the layouts as well is the whole point: a change to `default.html` has
to rebuild every page, and it does, because `pages` reads them all.

[rename]: ../getting-started/7-using-plugins.md
[vinyl]: ../api/vinyl.md
[watch]: ../getting-started/8-watching-files.md
