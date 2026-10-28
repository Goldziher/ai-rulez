package lint

import (
	"context"
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

// fakeExposure is an ExposureSource with a fixed start-of-session exposure that
// counts how often it is read.
type fakeExposure struct {
	noExposure
	start *ExposureStart
	calls *int
}

// noExposure is an ExposureSource with nothing to report, embedded by the
// fakes of each exposure rule family.
type noExposure struct{}

func (noExposure) StartExposure() *ExposureStart               { return nil }
func (noExposure) Collisions(float64) []ExposureCollision      { return nil }
func (noExposure) Leakage() (leakage ExposureLeakage, ok bool) { return ExposureLeakage{}, false }

func (f fakeExposure) StartExposure() *ExposureStart {
	*f.calls++
	return f.start
}

// lintExposure lints a project whose config is head plus table, handing the run
// the exposure start; it returns the findings and how often the exposure was
// read.
func lintExposure(t *testing.T, head, table string, start *ExposureStart) (fs []Finding, calls int) {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": head + "\n" + table})
	gitAdd(t, root)
	cfg, err := loadWithResolvers(context.Background(), root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree, WithExposure(fakeExposure{start: start, calls: &calls}))
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings, calls
}

// findingsWith keeps the findings of code whose message contains text.
func findingsWith(fs []Finding, code, text string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Code == code && strings.Contains(f.Message, text) {
			out = append(out, f)
		}
	}
	return out
}

func TestExposureStartBudget(t *testing.T) {
	over := &ExposureStart{ContextWindow: 200000, StartArtifacts: 4, StartTokens: 150, ListingBudgetChars: 8000}
	tests := []struct {
		name     string
		table    string
		start    *ExposureStart
		want     int
		sev      Severity
		measured bool
		inText   []string
	}{
		{name: "no exposure table is inert and never reads the exposure", table: "", start: over, want: 0},
		{name: "no start_budget is off", table: "[exposure]\ncontext_window = 200000\n", start: over, want: 0, measured: true},
		{name: "start_budget 0 is off", table: "[exposure]\nstart_budget = 0\n", start: over, want: 0, measured: true},
		{
			name: "over the budget", table: "[exposure]\nstart_budget = 100\n", start: over, want: 1, sev: SeverityError, measured: true,
			inText: []string{"about 150 estimated tokens (4 artifacts), 50 past the [exposure] start_budget of 100", "ai-rulez exposure"},
		},
		{
			name: "a severity override applies", table: "[exposure]\nstart_budget = 100\n[lint.severity]\nAR9W3 = \"warning\"\n",
			start: over, want: 1, sev: SeverityWarning, measured: true,
		},
		{name: "at the budget is clean", table: "[exposure]\nstart_budget = 150\n", start: over, want: 0, measured: true},
		{name: "an inventory that cannot be built is silent", table: "[exposure]\nstart_budget = 100\n", start: nil, want: 0, measured: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			fs, calls := lintExposure(t, baseConfig, tt.table, tt.start)

			// Assert
			if n := countCode(fs, CodeExposureStartBudget); n != tt.want {
				t.Fatalf("want %d AR9W3, got %d:\n%s", tt.want, n, dump(fs))
			}
			if (calls > 0) != tt.measured {
				t.Errorf("exposure read %d times, want read: %v", calls, tt.measured)
			}
			for _, f := range fs {
				if f.Code != CodeExposureStartBudget {
					continue
				}
				if a := AnalyzerFor(f.Code); f.Severity != tt.sev || a != (AnalyzerInfo{AnalyzerExposure, ScopeBundle}) {
					t.Errorf("AR9W3 classified as %s %v, want %s", f.Severity, a, tt.sev)
				}
				if !strings.HasSuffix(filepath.ToSlash(f.File), "/.ai-rulez/config.toml") || f.Line != 6 {
					t.Errorf("AR9W3 at %s:%d, want config.toml:6 (the start_budget line)", f.File, f.Line)
				}
				for _, w := range tt.inText {
					if !strings.Contains(f.Message, w) {
						t.Errorf("message %q lacks %q", f.Message, w)
					}
				}
			}
		})
	}
}

func TestClaudeSkillListingTotal(t *testing.T) {
	const marker = "skills a session lists at start"
	listing := func(chars int) *ExposureStart {
		return &ExposureStart{ContextWindow: 200000, StartTokens: 10, ListingSkills: 30, ListingChars: chars, ListingBudgetChars: 8000}
	}
	codexHead := "version = \"5.0\"\nname = \"t\"\npresets = [\"codex\"]\n"
	tests := []struct {
		name   string
		head   string
		table  string
		start  *ExposureStart
		want   int
		sev    Severity
		inText []string
	}{
		{name: "no exposure table is inert", head: baseConfig, start: listing(9000), want: 0},
		{
			name: "over the budget", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n", start: listing(8075), want: 1, sev: SeverityWarning,
			inText: []string{"the 30 skills a session lists at start add up to 8075 chars, past the 8000-chars skill listing budget", "1% of a 200000-token context window", "https://code.claude.com/docs/en/skills"},
		},
		{name: "within 10% is info", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n", start: listing(7200), want: 1, sev: SeverityInfo, inText: []string{"7200 of 8000 chars"}},
		{name: "well under is clean", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n", start: listing(7199), want: 0},
		{
			name: "the window scales the budget", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n", want: 0,
			start: &ExposureStart{ContextWindow: 1000000, ListingSkills: 30, ListingChars: 9000, ListingBudgetChars: 40000},
		},
		{
			name: "load_budgets overrides the scaled budget", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n[lint.load_budgets]\nclaude-skill-listing-total = 100\n",
			start: listing(150), want: 1, sev: SeverityWarning, inText: []string{"past the 100-chars", "[lint.load_budgets] claude-skill-listing-total"},
		},
		{name: "another harness is not checked", head: codexHead, table: "[exposure]\nplugins = \"enabled\"\n", start: listing(9000), want: 0},
		{name: "no measure is silent", head: baseConfig, table: "[exposure]\nplugins = \"enabled\"\n", start: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			fs, _ := lintExposure(t, tt.head, tt.table, tt.start)

			// Assert
			got := findingsWith(fs, CodeLoadBudget, marker)
			if len(got) != tt.want {
				t.Fatalf("want %d claude-skill-listing-total findings, got %d:\n%s", tt.want, len(got), dump(fs))
			}
			for _, f := range got {
				if f.Severity != tt.sev {
					t.Errorf("severity %s, want %s", f.Severity, tt.sev)
				}
				if f.Line != 3 {
					t.Errorf("line %d, want 3 (the presets line naming claude)", f.Line)
				}
				for _, w := range tt.inText {
					if !strings.Contains(f.Message, w) {
						t.Errorf("message %q lacks %q", f.Message, w)
					}
				}
			}
		})
	}
}
