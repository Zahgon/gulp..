package vinyl

// SourceMap is the Source Map v3 object attached to a File by src() when the
// sourcemaps option is enabled, and consumed by dest() when writing.
//
// It mirrors the plain JS object that vinyl-sourcemap attaches to
// `file.sourceMap`, so the JSON field names are significant.
type SourceMap struct {
	Version        int      `json:"version"`
	File           string   `json:"file"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
	SourceRoot     string   `json:"sourceRoot,omitempty"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent,omitempty"`

	// Preexisting records whether the map was loaded from the file (inline or
	// external) rather than synthesised. dest() uses it only for diagnostics.
	Preexisting bool `json:"-"`
}

// Clone returns a deep copy, or nil if m is nil.
func (m *SourceMap) Clone() *SourceMap {
	if m == nil {
		return nil
	}
	cp := *m
	cp.Names = append([]string(nil), m.Names...)
	cp.Sources = append([]string(nil), m.Sources...)
	cp.SourcesContent = append([]string(nil), m.SourcesContent...)
	return &cp
}
