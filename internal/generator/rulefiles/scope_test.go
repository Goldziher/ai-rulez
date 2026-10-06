package rulefiles

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

func TestScopeHelpers(t *testing.T) {
	scope := &config.ScopeRun{Path: "packages/api/", Slug: "packages-api", RootDir: filepath.FromSlash("/repo")}
	tests := []struct {
		name       string
		cfg        *config.Config
		wantInfo   ScopeInfo
		wantInside bool
		wantRoot   string
		wantFile   string
	}{
		{
			name: "nil config is the project root", cfg: nil,
			wantRoot: "/repo/packages/api", wantFile: "/repo/packages/api/.claude/rules/x.md",
		},
		{
			name: "root run", cfg: &config.Config{},
			wantRoot: "/repo/packages/api", wantFile: "/repo/packages/api/.claude/rules/x.md",
		},
		{
			name: "scope run writes under the project root", cfg: &config.Config{Run: &config.RunState{Scope: scope}},
			wantInfo:   ScopeInfo{Slug: "packages-api", Prefix: "packages/api"},
			wantInside: true, wantRoot: "/repo", wantFile: "/repo/.claude/rules/x.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			target := Target{Dir: ".claude/rules"}
			baseDir := filepath.FromSlash("/repo/packages/api")

			// Act
			info, inside := ScopeOf(tt.cfg), InScope(tt.cfg)
			root := RulesRoot(tt.cfg, baseDir)
			file := RulesDirPath(tt.cfg, baseDir, target, "x.md")

			// Assert
			if info != tt.wantInfo || inside != tt.wantInside {
				t.Errorf("ScopeOf/InScope = %+v/%v, want %+v/%v", info, inside, tt.wantInfo, tt.wantInside)
			}
			if root != filepath.FromSlash(tt.wantRoot) {
				t.Errorf("RulesRoot = %q, want %q", root, tt.wantRoot)
			}
			if file != filepath.FromSlash(tt.wantFile) {
				t.Errorf("RulesDirPath = %q, want %q", file, tt.wantFile)
			}
		})
	}
}

func TestRegistryFor_SharedPerPreset(t *testing.T) {
	// Arrange
	cfg := &config.Config{Run: config.NewRunState()}

	// Act
	a1, a2 := RegistryFor(cfg, "claude"), RegistryFor(cfg, "claude")
	b := RegistryFor(cfg, "cursor")
	loose1, loose2 := RegistryFor(&config.Config{}, "claude"), RegistryFor(&config.Config{}, "claude")

	// Assert
	if a1.claims != a2.claims {
		t.Error("same preset must share one registry")
	}
	if a1.claims == b.claims {
		t.Error("presets must not share a registry")
	}
	if loose1.claims == loose2.claims {
		t.Error("without a generation each call gets its own registry")
	}
}

func TestWarnUnreadScopeFile(t *testing.T) {
	scopeCfg := &config.Config{Run: &config.RunState{Scope: &config.ScopeRun{Path: "packages/api", Slug: "packages-api"}}}
	rules := []config.ContentFile{{Name: "style"}}
	ctx := []config.ContentFile{{Name: "notes"}}
	tests := []struct {
		name     string
		cfg      *config.Config
		rules    []config.ContentFile
		ctx      []config.ContentFile
		wantWarn bool
	}{
		{"root run is silent", &config.Config{}, rules, ctx, false},
		{"scope without inline items is silent", scopeCfg, nil, nil, false},
		{"scope with inline items warns", scopeCfg, rules, ctx, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var args [][]any
			cfg := *tt.cfg
			cfg.Diag = diag.New(func(_ string, a ...any) { args = append(args, a) })

			// Act
			WarnUnreadScopeFile(&cfg, "copilot", ".github/copilot-instructions.md", tt.rules, tt.ctx)

			// Assert
			if !tt.wantWarn {
				if len(args) != 0 {
					t.Fatalf("unexpected warning %v", args)
				}
				return
			}
			if len(args) != 1 || args[0][3] != "rule style, context notes" {
				t.Fatalf("warning args = %v", args)
			}
		})
	}
}
