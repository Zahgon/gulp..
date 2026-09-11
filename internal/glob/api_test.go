package glob

import "testing"

func TestMatcherAccessors(t *testing.T) {
	opts := Options{Dot: true, NoCase: true}
	m, err := Compile("SRC/**/*.JS", opts)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := m.Pattern(); got != "SRC/**/*.JS" {
		t.Errorf("Pattern() = %q, want the original pattern", got)
	}
	if got := m.Compiled(); got != "src/**/*.js" {
		t.Errorf("Compiled() = %q, want the case-folded rewrite", got)
	}
	if m.AllowsDot() {
		t.Error("AllowsDot() = true, want false: the pattern names no dot segment")
	}
	if got := m.Opts(); got != opts {
		t.Errorf("Opts() = %+v, want %+v", got, opts)
	}
}

func TestMatcherCompiledEqualsPatternWithoutRewrites(t *testing.T) {
	m, err := Compile("src/*.js", Options{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if m.Pattern() != m.Compiled() {
		t.Errorf("Compiled() = %q, want it unchanged from %q", m.Compiled(), m.Pattern())
	}
	if m.AllowsDot() {
		t.Error("AllowsDot() = true, want false without Options{Dot}")
	}
}

// TestMatcherAllowsDotFollowsThePattern pins what AllowsDot actually reports.
// It is not the Dot option: it says whether the pattern itself names a
// dot-prefixed segment, which is what lets the walker prune hidden
// directories instead of descending into every .git in the tree.
func TestMatcherAllowsDotFollowsThePattern(t *testing.T) {
	named, err := Compile(".config/*", Options{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !named.AllowsDot() {
		t.Error("AllowsDot() = false for .config/*, want true")
	}

	plain, err := Compile("src/**/*.js", Options{Dot: true})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if plain.AllowsDot() {
		t.Error("AllowsDot() = true for src/**/*.js, want false even with Options{Dot}")
	}
}

func TestMatchSetSplitsPositivesAndNegatives(t *testing.T) {
	set, err := CompileSet([]string{"src/**/*.js", "!src/vendor/**", "docs/*.md"}, Options{})
	if err != nil {
		t.Fatalf("CompileSet: %v", err)
	}

	positives := set.Positives()
	if len(positives) != 2 {
		t.Fatalf("Positives() has %d entries, want 2", len(positives))
	}
	if positives[0].Pattern() != "src/**/*.js" || positives[1].Pattern() != "docs/*.md" {
		t.Errorf("Positives() = %q, %q, want the two non-negated patterns",
			positives[0].Pattern(), positives[1].Pattern())
	}

	negatives := set.Negatives()
	if len(negatives) != 1 {
		t.Fatalf("Negatives() has %d entries, want 1", len(negatives))
	}
	if negatives[0].Pattern() != "src/vendor/**" {
		t.Errorf("Negatives()[0] = %q, want the pattern with the ! stripped", negatives[0].Pattern())
	}
}

func TestMatchSetWithoutNegatives(t *testing.T) {
	set, err := CompileSet([]string{"*.go"}, Options{})
	if err != nil {
		t.Fatalf("CompileSet: %v", err)
	}
	if len(set.Positives()) != 1 {
		t.Errorf("Positives() has %d entries, want 1", len(set.Positives()))
	}
	if len(set.Negatives()) != 0 {
		t.Errorf("Negatives() has %d entries, want 0", len(set.Negatives()))
	}
}
