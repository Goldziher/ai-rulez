package lint

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExposureMapInvalid(t *testing.T) {
	tests := []struct {
		name   string
		table  string
		files  map[string]string
		want   int
		inText string
		file   string
		line   int
	}{
		{name: "no exposure table is inert", table: "", want: 0},
		{name: "a table without areas is clean", table: "[exposure]\ncontext_window = 100000\n", want: 0},
		{
			name:  "valid inline map is clean",
			table: "[exposure]\n[[exposure.areas]]\nname = \"billing\"\npaths = [\"services/billing\"]\nterms = [\"invoice\"]\n[exposure.skill_areas]\ndeploy = \"billing\"\n",
			want:  0,
		},
		{
			name:   "duplicate inline names",
			table:  "[exposure]\n[[exposure.areas]]\nname = \"billing\"\n[[exposure.areas]]\nname = \"billing\"\n",
			want:   1,
			inText: `duplicate area name "billing"`,
			file:   ".ai-rulez/config.toml",
			line:   9,
		},
		{
			name:   "skill_areas value naming no area",
			table:  "[exposure]\n[[exposure.areas]]\nname = \"billing\"\nterms = [\"ops\"]\n[exposure.skill_areas]\ndeploy = \"ops\"\n",
			want:   1,
			inText: `"deploy" to "ops", which names no area`,
			line:   10,
		},
		{
			name:   "bad scalar key",
			table:  "[exposure]\nplugins = \"some\"\n",
			want:   1,
			inText: `exposure.plugins "some"`,
		},
		{
			name:   "areas file and inline areas",
			table:  "[exposure]\nareas_file = \"areas.toml\"\n[[exposure.areas]]\nname = \"billing\"\n",
			files:  map[string]string{"areas.toml": "[[area]]\nname = \"search\"\n"},
			want:   1,
			inText: "keep the area map in one place",
		},
		{
			name:   "missing areas file",
			table:  "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			want:   1,
			inText: "areas file cannot be read",
		},
		{
			name:   "areas file outside the project",
			table:  "[exposure]\nareas_file = \"../areas.toml\"\n",
			want:   1,
			inText: "must stay inside the project root",
		},
		{
			name:  "valid areas file is clean",
			table: "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			files: map[string]string{"tools/areas.toml": "[[area]]\nname = \"search\"\npaths = [\"products/search\"]\n[skill_areas]\nranker = \"search\"\n"},
			want:  0,
		},
		{
			name:   "bad path claim in the areas file, on its line",
			table:  "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			files:  map[string]string{"tools/areas.toml": "[[area]]\nname = \"search\"\npaths = [\"products/search/\"]\n"},
			want:   1,
			inText: `path "products/search/"`,
			file:   "tools/areas.toml",
			line:   3,
		},
		{
			name:   "unknown key in the areas file",
			table:  "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			files:  map[string]string{"tools/areas.toml": "[[area]]\nname = \"search\"\ncross_cutting = true\n"},
			want:   1,
			inText: `unknown key "area.cross_cutting"`,
			line:   3,
		},
		{
			name:   "duplicate name in the areas file, on the second table",
			table:  "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			files:  map[string]string{"tools/areas.toml": "[[area]]\nname = \"search\"\n[[area]]\nname = \"search\"\n"},
			want:   1,
			inText: `duplicate area name "search"`,
			file:   "tools/areas.toml",
			line:   4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := map[string]string{".ai-rulez/config.toml": baseConfig + "\n" + tt.table}
			for name, body := range tt.files {
				files[name] = body
			}
			writeFiles(t, root, files)
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			if n := countCode(fs, CodeExposureMapInvalid); n != tt.want {
				t.Fatalf("want %d AR9W5, got %d:\n%s", tt.want, n, dump(fs))
			}
			for _, f := range fs {
				if f.Code != CodeExposureMapInvalid {
					continue
				}
				if a := AnalyzerFor(f.Code); f.Severity != SeverityError || a != (AnalyzerInfo{AnalyzerExposure, ScopeBundle}) {
					t.Errorf("AR9W5 classified as %s %v", f.Severity, a)
				}
				if !strings.Contains(f.Message, tt.inText) {
					t.Errorf("message %q lacks %q", f.Message, tt.inText)
				}
				if tt.file != "" && !strings.HasSuffix(filepath.ToSlash(f.File), "/"+tt.file) {
					t.Errorf("file = %s, want %s", f.File, tt.file)
				}
				if tt.line != 0 && f.Line != tt.line {
					t.Errorf("line = %d, want %d", f.Line, tt.line)
				}
			}
		})
	}
}
