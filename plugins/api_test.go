package plugins

import (
	"errors"
	"testing"

	"github.com/gulpjs/gulp-go/vinyl"
)

func TestFilterFuncKeepsMatchingFiles(t *testing.T) {
	source := []*vinyl.File{
		file(t, "keep.js", "a"),
		file(t, "drop.css", "b"),
		file(t, "also-keep.js", "c"),
	}

	got := run(t, source, FilterFunc(func(f *vinyl.File) bool {
		return f.Extname() == ".js"
	}))

	want := []string{"keep.js", "also-keep.js"}
	if !equal(relatives(t, got), want) {
		t.Errorf("kept %v, want %v", relatives(t, got), want)
	}
}

func TestFilterFuncWithoutPredicateKeepsEverything(t *testing.T) {
	source := []*vinyl.File{
		file(t, "one.js", "a"),
		file(t, "two.css", "b"),
	}

	got := run(t, source, FilterFunc(nil))

	want := []string{"one.js", "two.css"}
	if !equal(relatives(t, got), want) {
		t.Errorf("kept %v, want %v", relatives(t, got), want)
	}
}

func TestErrorUnwrapExposesTheCause(t *testing.T) {
	cause := errors.New("underlying")
	err := &Error{Plugin: "sass", Path: "a.scss", Err: cause}

	if !errors.Is(err, cause) {
		t.Error("errors.Is did not reach the cause")
	}
	if errors.Unwrap(err) != cause {
		t.Errorf("Unwrap() = %v, want %v", errors.Unwrap(err), cause)
	}
}

func TestErrorUnwrapIsNilWithoutACause(t *testing.T) {
	err := &Error{Plugin: "sass", Message: "bad syntax"}

	if errors.Unwrap(err) != nil {
		t.Errorf("Unwrap() = %v, want nil", errors.Unwrap(err))
	}
}
