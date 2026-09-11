package sourcemap

import "testing"

func TestUnescapeDataURI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"version":3}`, `{"version":3}`},
		{"braces", `%7B%22version%22%3A3%7D`, `{"version":3}`},
		{"space in a source name", `{"sources":["a%20b.js"]}`, `{"sources":["a b.js"]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unescapeDataURI(tc.in)
			if err != nil {
				t.Fatalf("unescapeDataURI(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("unescapeDataURI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestUnescapeDataURIRejectsATruncatedEscape(t *testing.T) {
	if _, err := unescapeDataURI("%zz"); err == nil {
		t.Fatal("unescapeDataURI(%zz) succeeded, want an error")
	}
}
