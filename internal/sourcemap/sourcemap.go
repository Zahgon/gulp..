// Package sourcemap is the Go port of the `vinyl-sourcemap` npm package. It
// attaches a Source Map v3 object to a vinyl file on the way in ([Add]) and
// serialises it back out, inline or as a sibling ".map" file, on the way out
// ([Write]).
package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gulpjs/gulp-go/vinyl"
)

// Errors reported with the same wording as the JavaScript implementation, so
// that user-facing messages do not change across the port.
var (
	ErrAddNotVinyl      = errors.New("vinyl-sourcemap-add: Not a vinyl file")
	ErrAddStreaming     = errors.New("vinyl-sourcemap-add: Streaming not supported")
	ErrWriteNotVinyl    = errors.New("vinyl-sourcemap-write: Not a vinyl file")
	ErrWriteStreaming   = errors.New("vinyl-sourcemap-write: Streaming not supported")
	ErrInvalidSourceMap = errors.New("vinyl-sourcemap: invalid source map")
)

// mapCommentRegex matches both the "//# sourceMappingURL=" and the
// "/*# sourceMappingURL= */" comment forms at the end of a line. It is the port
// of `convert-source-map`'s mapFileCommentRx, which vinyl-sourcemap relies on.
var mapCommentRegex = regexp.MustCompile(
	`(?m)(?:^[ \t]*//[@#][ \t]+sourceMappingURL=([^\s'"` + "`" + `]+?)[ \t]*$)` +
		`|(?:^[ \t]*/\*[@#][ \t]+sourceMappingURL=([^*]+?)[ \t]*\*/[ \t]*$)`)

// inlineDataRegex matches the payload of an inline data: source map URL.
var inlineDataRegex = regexp.MustCompile(
	`^data:application/json(?:;charset=[^;,]+)?(;base64)?,(.*)$`)

// remoteURLRegex matches source map URLs that point somewhere other than the
// local filesystem; vinyl-sourcemap leaves those alone.
var remoteURLRegex = regexp.MustCompile(`^(?:https?|webpack(?:-[^:]+)?)://`)

// Add attaches a source map to file, loading an existing inline or external map
// when one is referenced and synthesising an empty identity map otherwise. Any
// sourceMappingURL comment is removed from the contents, because the comment is
// re-emitted by [Write] once the final destination is known.
//
// Add is a no-op when the file already carries a source map, has no contents,
// or is a directory.
func Add(file *vinyl.File) error {
	if file == nil {
		return ErrAddNotVinyl
	}
	if file.IsStream() {
		return ErrAddStreaming
	}
	if file.SourceMap != nil {
		return nil
	}
	if file.IsNull() || file.IsDirectory() {
		return nil
	}

	raw, err := file.Bytes()
	if err != nil {
		return fmt.Errorf("reading %s: %w", file.Path(), err)
	}
	relative, err := relativeUnixPath(file)
	if err != nil {
		return err
	}
	contents := string(raw)

	sm := &vinyl.SourceMap{
		Version:        3,
		File:           relative,
		Names:          []string{},
		Mappings:       "",
		Sources:        []string{relative},
		SourcesContent: []string{contents},
	}

	url, match := findMapComment(contents)
	if match == "" {
		file.SourceMap = sm
		return nil
	}

	// The comment is always stripped, even when the map it points at cannot be
	// loaded; leaving a stale URL behind would break the emitted output.
	stripped := strings.Replace(contents, match, "", 1)
	stripped = strings.TrimRight(stripped, " \t")

	loaded, err := loadReferencedMap(file, url)
	if err != nil {
		return err
	}
	if loaded == nil {
		file.Contents = vinyl.Buffer(stripped)
		file.SourceMap = sm
		return nil
	}

	loaded.Preexisting = true
	if loaded.File == "" {
		loaded.File = relative
	}
	if loaded.Names == nil {
		loaded.Names = []string{}
	}
	fillSourcesContent(file, loaded)

	file.Contents = vinyl.Buffer(stripped)
	file.SourceMap = loaded
	return nil
}

// findMapComment returns the referenced URL and the full comment text of the
// last sourceMappingURL comment in contents, or empty strings when absent.
//
// The last comment wins: tools that append a map comment do so at the end, so a
// trailing comment supersedes anything earlier in the file.
func findMapComment(contents string) (url, comment string) {
	matches := mapCommentRegex.FindAllStringSubmatch(contents, -1)
	if len(matches) == 0 {
		return "", ""
	}
	last := matches[len(matches)-1]
	url = last[1]
	if url == "" {
		url = last[2]
	}
	return strings.TrimSpace(url), last[0]
}

// loadReferencedMap resolves a sourceMappingURL, returning nil when the map is
// remote or missing on disk. A malformed map is an error, since silently
// discarding it would hide a real build problem.
func loadReferencedMap(file *vinyl.File, url string) (*vinyl.SourceMap, error) {
	if url == "" || remoteURLRegex.MatchString(url) {
		return nil, nil
	}

	if payload := inlineDataRegex.FindStringSubmatch(url); payload != nil {
		raw := payload[2]
		var decoded []byte
		if payload[1] == ";base64" {
			var err error
			decoded, err = base64.StdEncoding.DecodeString(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrInvalidSourceMap, file.Path(), err)
			}
		} else {
			unescaped, err := unescapeDataURI(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrInvalidSourceMap, file.Path(), err)
			}
			decoded = []byte(unescaped)
		}
		return parseMap(file, decoded)
	}

	mapPath := filepath.Join(file.Dirname(), filepath.FromSlash(url))
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		// A dangling external map reference is tolerated, matching
		// vinyl-sourcemap, which falls back to an empty map.
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading source map %s: %w", mapPath, err)
	}
	return parseMap(file, raw)
}

// parseMap decodes source map JSON.
func parseMap(file *vinyl.File, raw []byte) (*vinyl.SourceMap, error) {
	var sm vinyl.SourceMap
	if err := json.Unmarshal(raw, &sm); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidSourceMap, file.Path(), err)
	}
	return &sm, nil
}

// fillSourcesContent loads any source listed in the map whose content is
// missing, so that downstream consumers get a self-contained map.
//
// Sources are resolved against sourceRoot when it is a local path, and against
// the file's own directory otherwise.
func fillSourcesContent(file *vinyl.File, sm *vinyl.SourceMap) {
	if len(sm.Sources) == 0 {
		return
	}

	base := file.Dirname()
	if sm.SourceRoot != "" && !remoteURLRegex.MatchString(sm.SourceRoot) {
		base = filepath.Join(base, filepath.FromSlash(sm.SourceRoot))
	}

	if sm.SourcesContent == nil {
		sm.SourcesContent = make([]string, len(sm.Sources))
	}
	for len(sm.SourcesContent) < len(sm.Sources) {
		sm.SourcesContent = append(sm.SourcesContent, "")
	}

	for i, source := range sm.Sources {
		if sm.SourcesContent[i] != "" || remoteURLRegex.MatchString(source) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(source)))
		if err != nil {
			// Missing sources are expected for generated or virtual files.
			continue
		}
		sm.SourcesContent[i] = string(content)
	}
}

// Write serialises file.SourceMap and appends a sourceMappingURL comment to the
// file's contents.
//
// When destPath is empty the map is embedded as a base64 data URI and the
// returned file is nil. Otherwise the map is returned as a separate vinyl file
// to be written alongside, at destPath relative to the file's base, and the
// comment points at it.
//
// Write is a no-op returning a nil file when there is no source map to write.
func Write(file *vinyl.File, destPath string) (*vinyl.File, error) {
	if file == nil {
		return nil, ErrWriteNotVinyl
	}
	if file.IsStream() {
		return nil, ErrWriteStreaming
	}
	if file.SourceMap == nil || file.IsNull() || file.IsDirectory() {
		return nil, nil
	}

	relative, err := relativeUnixPath(file)
	if err != nil {
		return nil, err
	}
	contents, err := file.Bytes()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file.Path(), err)
	}

	sm := file.SourceMap.Clone()
	sm.File = relative
	if sm.Version == 0 {
		sm.Version = 3
	}
	if sm.Names == nil {
		sm.Names = []string{}
	}
	if sm.Sources == nil {
		sm.Sources = []string{}
	}

	encoded, err := json.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("serialising source map for %s: %w", file.Path(), err)
	}

	if destPath == "" {
		comment := "//# sourceMappingURL=data:application/json;charset=utf-8;base64," +
			base64.StdEncoding.EncodeToString(encoded)
		file.Contents = vinyl.Buffer(appendComment(contents, comment))
		return nil, nil
	}

	mapFilePath := filepath.Join(file.Base(), destPath, filepath.FromSlash(relative)+".map")
	mapFile, err := vinyl.New(vinyl.Options{
		Cwd:      file.Cwd(),
		Base:     file.Base(),
		Path:     mapFilePath,
		Contents: vinyl.Buffer(encoded),
	})
	if err != nil {
		return nil, err
	}

	// The comment must reference the map relative to the file that carries it,
	// so that the pair keeps working wherever the output directory is served
	// from.
	rel, relErr := filepath.Rel(file.Dirname(), mapFilePath)
	if relErr != nil {
		rel = filepath.Base(mapFilePath)
	}
	comment := "//# sourceMappingURL=" + toUnixPath(rel)
	file.Contents = vinyl.Buffer(appendComment(contents, comment))

	return mapFile, nil
}

// appendComment adds comment on its own line at the end of contents.
func appendComment(contents []byte, comment string) []byte {
	out := make([]byte, 0, len(contents)+len(comment)+2)
	out = append(out, contents...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, comment...)
	out = append(out, '\n')
	return out
}

// unescapeDataURI decodes the percent-encoding used by non-base64 data URIs.
func unescapeDataURI(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("truncated escape sequence")
		}
		var v int
		if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &v); err != nil {
			return "", fmt.Errorf("invalid escape sequence %q", s[i:i+3])
		}
		b.WriteByte(byte(v))
		i += 2
	}
	return b.String(), nil
}

// toUnixPath normalises a path to forward slashes, which source maps require
// regardless of the host platform.
func toUnixPath(p string) string {
	return path.Clean(filepath.ToSlash(p))
}

// relativeUnixPath returns the file's base-relative path in source map form.
func relativeUnixPath(file *vinyl.File) (string, error) {
	rel, err := file.Relative()
	if err != nil {
		return "", fmt.Errorf("source map for %s: %w", file.Path(), err)
	}
	return toUnixPath(rel), nil
}
