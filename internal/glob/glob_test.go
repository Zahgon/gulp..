package glob

import "testing"

func TestIsGlob(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"/a/b/c.txt", false},
		{"./fixtures/stuff/run.dmc", false},
		{"", false},
		{"/a/*.txt", true},
		{"a*c", true},
		{"a[bc]d", true},
		{"a[bcd", false},       // no closing bracket: literal
		{"a{b,c}d", true},      // brace expansion
		{"a{b,c", false},       // no closing brace: literal
		{"!(a|b)", true},       // extglob
		{"+(a|b)", true},       // extglob
		{"@(a|b)", true},       // extglob
		{`path/\*/x`, false},   // escaped star is literal
		{`path/\*\*/x`, false}, // escaped globstar is literal
		{`path/\*/*`, true},    // one escaped, one real
		{"./fixtures/**/*.dmc", true},
		{"foo/**/bar", true},
		{"path.js", false},

		// The two rules that are easy to get backwards, each verified against
		// the real is-glob module rather than assumed.
		{"a?c", false},       // `?` alone is not a wildcard to is-glob
		{"path/?foo", false}, //   ... so this whole path is literal
		{"file?.txt", false}, //   ... and so is this
		{"(a|b)", true},      // a bare alternation group IS a glob
		{"a/(b|c)/d", true},  //   ... at any depth
		{"(a)", false},       //   ... but only when it alternates
		{"(?:a)", true},      // a regex-style group counts too
		{"a[bc]?", true},     // `?` after `]` IS the extglob quantifier
		{`a\@(b)`, false},    // escaped extglob prefix is literal
	}
	for _, tt := range tests {
		if got := IsGlob(tt.in); got != tt.want {
			t.Errorf("IsGlob(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestParent locks the glob-parent algorithm, because its result becomes each
// vinyl file's base and therefore decides the layout dest() writes.
func TestParent(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/src/js/**.js", "/src/js"},
		{"/src/js/**/*.js", "/src/js"},
		{"./fixtures/*.coffee", "./fixtures"},
		{"./fixtures/**/*.jade", "./fixtures"},
		{"./fixtures/**/*.dmc", "./fixtures"},
		{"./fixtures/stuff/run.dmc", "./fixtures/stuff"},
		{"./fixtures/stuff", "./fixtures"},
		{"path/foo[bar]/", "path"},
		{"path/{foo,bar}/", "path"},
		{"path/foo/bar/", "path/foo/bar"},
		{"path/*/foo.js", "path"},
		{"path/!(foo)/bar.js", "path"},
		{"path.js", "."},
		{"/", "/"},
		{"*", "."},
		{`path/\*/`, "path/*"},
		{`path/\*\*/`, "path/**"},
		{"foo/{bar,baz/qux}", "foo"},
	}
	for _, tt := range tests {
		if got := Parent(tt.in); got != tt.want {
			t.Errorf("Parent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		opts    Options
		want    bool
	}{
		{"*.coffee", "test.coffee", Options{}, true},
		{"*.coffee", "test.js", Options{}, false},
		{"**/*.dmc", "stuff/run.dmc", Options{}, true},
		{"**/*.dmc", "a/b/c/run.dmc", Options{}, true},
		{"{a,b}.txt", "a.txt", Options{}, true},
		{"{a,b}.txt", "a.txt", Options{NoBrace: true}, false},
		{"a[bc]d", "abd", Options{}, true},
		{"*.TXT", "a.txt", Options{NoCase: true}, true},
		{"*.TXT", "a.txt", Options{}, false},
		// Dotfiles are excluded unless the pattern names them or Dot is set.
		{"*", ".hidden", Options{}, false},
		{"*", ".hidden", Options{Dot: true}, true},
		{".*", ".hidden", Options{}, true},
		{"**/*.js", "node_modules/.bin/x.js", Options{}, false},
		// MatchBase lets a separator-free pattern match at any depth.
		{"*.dmc", "a/b/run.dmc", Options{MatchBase: true}, true},
		{"*.dmc", "a/b/run.dmc", Options{}, false},
		// NoGlobstar degrades ** to a single-segment *.
		{"**/x.js", "a/b/x.js", Options{NoGlobstar: true}, false},
		{"**/x.js", "a/x.js", Options{NoGlobstar: true}, true},
	}
	for _, tt := range tests {
		m, err := Compile(tt.pattern, tt.opts)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.pattern, err)
		}
		if got := m.Match(tt.subject); got != tt.want {
			t.Errorf("Compile(%q, %+v).Match(%q) = %v, want %v",
				tt.pattern, tt.opts, tt.subject, got, tt.want)
		}
	}
}

// TestMatchSetNegation reproduces gulp's own src test:
// src(['./fixtures/stuff/*.dmc', '!fixtures/stuff/test.dmc']) yields only
// run.dmc. Note the two patterns use different relative conventions, so they
// must be resolved against the same cwd before they can be compared.
func TestMatchSetNegation(t *testing.T) {
	const cwd = "/repo/test"
	raw := []string{"./fixtures/stuff/*.dmc", "!fixtures/stuff/test.dmc"}

	resolved := make([]string, len(raw))
	for i, p := range raw {
		resolved[i] = Resolve(p, cwd)
	}
	if want := "/repo/test/fixtures/stuff/*.dmc"; resolved[0] != want {
		t.Fatalf("Resolve positive = %q, want %q", resolved[0], want)
	}
	if want := "!/repo/test/fixtures/stuff/test.dmc"; resolved[1] != want {
		t.Fatalf("Resolve negative = %q, want %q", resolved[1], want)
	}

	set, err := CompileSet(resolved, Options{})
	if err != nil {
		t.Fatalf("CompileSet: %v", err)
	}
	if !set.Match("/repo/test/fixtures/stuff/run.dmc") {
		t.Error("run.dmc should be included")
	}
	if set.Match("/repo/test/fixtures/stuff/test.dmc") {
		t.Error("test.dmc should be excluded by the negation")
	}
}

// TestMatchSetNegationOrderIndependent documents that a negation applies to the
// whole set no matter where it appears, matching micromatch.
func TestMatchSetNegationOrderIndependent(t *testing.T) {
	for _, order := range [][]string{
		{"a/*.js", "!a/skip.js"},
		{"!a/skip.js", "a/*.js"},
	} {
		set, err := CompileSet(order, Options{})
		if err != nil {
			t.Fatalf("CompileSet(%v): %v", order, err)
		}
		if !set.Match("a/keep.js") {
			t.Errorf("%v: a/keep.js should match", order)
		}
		if set.Match("a/skip.js") {
			t.Errorf("%v: a/skip.js should be excluded", order)
		}
	}
}

func TestStripNegation(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		negated bool
	}{
		{"!a.js", "a.js", true},
		{`\!a.js`, "!a.js", false},
		{"a.js", "a.js", false},
	}
	for _, tt := range tests {
		if got := StripNegation(tt.in); got != tt.want {
			t.Errorf("StripNegation(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if got := IsNegative(tt.in); got != tt.negated {
			t.Errorf("IsNegative(%q) = %v, want %v", tt.in, got, tt.negated)
		}
	}
}
