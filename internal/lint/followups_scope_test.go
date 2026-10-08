package lint

import (
	"path/filepath"
	"testing"
)

// A directory whose name merely starts with two dots is inside the config
// directory; only a real parent reference leaves it.
func TestScopeOfTreatsDotDotPrefixedNamesAsInside(t *testing.T) {
	configDir := filepath.Join(string(filepath.Separator), "proj", ".ai-rulez")
	tests := []struct {
		name string
		path string
		want string
	}{
		{"root content", filepath.Join(configDir, "rules", "a.md"), ""},
		{"directory named ..hidden", filepath.Join(configDir, "..hidden", "a.md"), ""},
		{"domain content", filepath.Join(configDir, "domains", "web", "rules", "a.md"), "web"},
		{"outside the config directory", filepath.Join(filepath.Dir(configDir), "other", "a.md"), "import"},
		{"parent of the config directory", filepath.Dir(configDir), "import"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopeOf(tt.path, configDir); got != tt.want {
				t.Errorf("scopeOf(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
