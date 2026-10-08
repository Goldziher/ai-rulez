package safefs

import (
	"path/filepath"
	"testing"
)

func TestRelEscapes(t *testing.T) {
	sep := string(filepath.Separator)
	cases := map[string]bool{
		"..":                         true,
		".." + sep + "x":             true,
		".." + sep + "..":            true,
		".":                          false,
		"x":                          false,
		"..foo":                      false,
		"...":                        false,
		"..hidden" + sep + "x":       false,
		"a" + sep + ".." + sep + "b": false, // Rel output is already cleaned; not a leading ..
		"":                           false,
	}
	for rel, want := range cases {
		if got := RelEscapes(rel); got != want {
			t.Errorf("RelEscapes(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestWithin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "proj")
	cases := []struct {
		target string
		want   bool
	}{
		{root, true},
		{filepath.Join(root, "a", "b"), true},
		{filepath.Join(root, "..foo"), true}, // a directory named "..foo" is inside
		{filepath.Join(root, "..foo", "x"), true},
		{filepath.Dir(root), false},
		{filepath.Join(filepath.Dir(root), "other"), false},
		{filepath.Join(filepath.Dir(root), "proj2"), false},
	}
	for _, c := range cases {
		if got := Within(root, c.target); got != c.want {
			t.Errorf("Within(%q, %q) = %v, want %v", root, c.target, got, c.want)
		}
	}
	if Within("rel", filepath.Join(root, "a")) {
		t.Error("mixed relative and absolute paths must not be within")
	}
}
