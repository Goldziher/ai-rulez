package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/evals"
)

func evalRunnerFixture(extraConfig string) map[string]string {
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster.\n---\nbody\n"
	}
	return map[string]string{
		".ai-rulez/config.toml":                       baseConfig + extraConfig,
		".ai-rulez/skills/good/SKILL.md":              skill("good"),
		".ai-rulez/skills/good/evals/a.eval.yaml":     "prompt: hi\nexpect_trigger: true\nid: a\n",
		".ai-rulez/skills/bad/SKILL.md":               skill("bad"),
		".ai-rulez/skills/bad/evals/b.eval.yaml":      "cases:\n  - id: b\n    prompt: hi\n",
		".ai-rulez/skills/bad/evals/native/case.yaml": "prompt: not ours\n",
		".ai-rulez/evals/good/shared.eval.json":       "{\"cases\":[{\"id\":\"s\",\"prompt\":\"x\",\"expect_trigger\":false,\"bogus\":1}]}",
		".ai-rulez/skills/nocases/SKILL.md":           skill("nocases"),
		".ai-rulez/skills/nocases/evals/README.md":    "docs only",
	}
}

func TestEvalCaseInvalid(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, evalRunnerFixture(""))
	gitAdd(t, root)
	findings := lintDir(t, root)

	if !has(findings, CodeEvalCaseInvalid, "skills/bad/evals/b.eval.yaml", 2) {
		t.Errorf("expected AR996 on the malformed case at line 2, got %v", findings)
	}
	if !has(findings, CodeEvalCaseInvalid, "evals/good/shared.eval.json", 1) {
		t.Errorf("expected AR996 for the project-level JSON case, got %v", findings)
	}
	if n := countCode(findings, CodeEvalCaseInvalid); n != 2 {
		t.Errorf("want 2 AR996 findings, got %d: %v", n, findings)
	}
	for _, f := range findings {
		if f.Code == CodeEvalCaseInvalid && (filepath.Base(f.File) == "case.yaml" || filepath.Base(f.File) == "README.md") {
			t.Errorf("non-case files must be ignored: %v", f)
		}
		if f.Code == CodeEvalCaseInvalid && filepath.Base(f.File) == "a.eval.yaml" {
			t.Errorf("valid case flagged: %v", f)
		}
	}
	for _, f := range findings {
		if f.Code == CodeEvalCaseInvalid && f.Severity != SeverityError {
			t.Errorf("AR996 defaults to error, got %s", f.Severity)
		}
	}
}

func recordEval(t *testing.T, root, id string, passing bool, passRate float64, digestOverride string) {
	t.Helper()
	cfgDir := filepath.Join(root, ".ai-rulez")
	digest := digestOverride
	if digest == "" {
		var err error
		digest, err = evals.SkillDigest(filepath.Join(cfgDir, "skills", id))
		if err != nil {
			t.Fatal(err)
		}
	}
	store := evals.NewStore()
	store.Put(evals.SkillRecord{ID: id, Digest: digest, Passing: passing, Date: "2026-10-01",
		Score: evals.SkillScore{Scored: 4, PassRate: passRate}})
	if err := store.Save(evals.DefaultStorePath(cfgDir)); err != nil {
		t.Fatal(err)
	}
}

func TestEvalStaleAndScoreLow(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		passing   bool
		passRate  float64
		edit      bool
		wantStale bool
		wantLow   bool
		staleSev  Severity
	}{
		{name: "fresh and passing", config: "[lint.evals]\nrequire_fresh = \"error\"\nmin_pass_rate = 0.8\n", passing: true, passRate: 1},
		{name: "edited after pass is error", config: "[lint.evals]\nrequire_fresh = \"error\"\n", passing: true, passRate: 1, edit: true, wantStale: true, staleSev: SeverityError},
		{name: "edited after pass is warning", config: "[lint.evals]\nrequire_fresh = \"warn\"\n", passing: true, passRate: 1, edit: true, wantStale: true, staleSev: SeverityWarning},
		{name: "off by default", config: "", passing: true, passRate: 1, edit: true},
		{name: "require_fresh off", config: "[lint.evals]\nrequire_fresh = \"off\"\n", passing: true, passRate: 1, edit: true},
		{name: "score below floor", config: "[lint.evals]\nmin_pass_rate = 0.9\n", passing: false, passRate: 0.5, wantLow: true},
		{name: "score at floor", config: "[lint.evals]\nmin_pass_rate = 0.5\n", passing: false, passRate: 0.5},
		{name: "never passed is not stale", config: "[lint.evals]\nrequire_fresh = \"error\"\n", passing: false, passRate: 0.2, edit: true},
		{name: "severity override", config: "[lint.evals]\nmin_pass_rate = 0.9\n[lint.severity]\neval-score-low = \"warning\"\n", passing: false, passRate: 0.1, wantLow: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, evalRunnerFixture(tt.config))
			recordEval(t, root, "good", tt.passing, tt.passRate, "")
			if tt.edit {
				writeFiles(t, root, map[string]string{".ai-rulez/skills/good/SKILL.md": "---\nname: good\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster.\n---\nedited body\n"})
			}
			gitAdd(t, root)
			findings := lintDir(t, root)
			if got := has(findings, CodeEvalStale, "skills/good/SKILL.md", 0); got != tt.wantStale {
				t.Errorf("AR997 = %v, want %v: %v", got, tt.wantStale, findings)
			}
			if got := has(findings, CodeEvalScoreLow, "skills/good/SKILL.md", 0); got != tt.wantLow {
				t.Errorf("AR998 = %v, want %v: %v", got, tt.wantLow, findings)
			}
			if tt.wantStale {
				for _, f := range findings {
					if f.Code == CodeEvalStale && f.Severity != tt.staleSev {
						t.Errorf("AR997 severity = %s, want %s", f.Severity, tt.staleSev)
					}
				}
			}
		})
	}
}

func TestEvalStaleIgnoresEditedCases(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, evalRunnerFixture("[lint.evals]\nrequire_fresh = \"error\"\n"))
	recordEval(t, root, "good", true, 1, "")
	writeFiles(t, root, map[string]string{".ai-rulez/skills/good/evals/a.eval.yaml": "prompt: changed\nexpect_trigger: true\nid: a\n"})
	gitAdd(t, root)
	if findings := lintDir(t, root); countCode(findings, CodeEvalStale) != 0 {
		t.Errorf("editing a case must not make the skill stale: %v", findings)
	}
}

func TestEvalResultsInvalid(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, evalRunnerFixture(""))
	if err := os.WriteFile(filepath.Join(root, ".ai-rulez", "eval-results.json"), []byte(`{"schema_version": 42, "skills": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, root)
	findings := lintDir(t, root)
	if !has(findings, CodeEvalResultsInvalid, "eval-results.json", 1) {
		t.Errorf("expected AR9A0, got %v", findings)
	}
}

func TestValidateEvalSettings(t *testing.T) {
	ok := &config.LintConfig{Evals: &config.LintEvals{RequireFresh: "warn", MinPassRate: 0.5}}
	if problems := validateEvalSettings(ok); len(problems) != 0 {
		t.Errorf("valid settings rejected: %v", problems)
	}
	bad := &config.LintConfig{Evals: &config.LintEvals{RequireFresh: "sometimes", MinPassRate: 1.5}}
	if problems := validateEvalSettings(bad); len(problems) != 2 {
		t.Errorf("want 2 problems, got %v", problems)
	}
	if problems := ValidateSettings(bad); len(problems) < 2 {
		t.Errorf("ValidateSettings must include the eval checks: %v", problems)
	}
}
