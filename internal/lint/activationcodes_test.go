package lint

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

func recordActivation(t *testing.T, key []byte, root, id string, act *evals.ActivationRecord, staleDigest bool) {
	t.Helper()
	cfgDir := filepath.Join(root, ".ai-rulez")
	digest, err := evals.SkillDigest(filepath.Join(cfgDir, "skills", id))
	if err != nil {
		t.Fatal(err)
	}
	if staleDigest {
		digest = "sha256:old"
	}
	act.Digest = digest
	store := evals.NewStore()
	store.SetKey(key)
	store.PutActivation(id, digest, act)
	if err := store.Save(evals.DefaultStorePath(cfgDir)); err != nil {
		t.Fatal(err)
	}
}

func fp(v float64) *float64 { return &v }

func TestActivationChecks(t *testing.T) {
	tests := []struct {
		name         string
		config       string
		act          *evals.ActivationRecord
		stale        bool
		forge        bool
		wantLow      int
		wantConfuse  int
		wantSeverity Severity
	}{
		{name: "off by default", config: "", act: &evals.ActivationRecord{Recall: fp(0.1), StolenBy: []evals.StolenBy{{Skill: "x", Prompts: 3, Share: 0.9}}}},
		{name: "recall below the floor", config: "[lint.evals]\nmin_activation_recall = 0.8\n", act: &evals.ActivationRecord{Recall: fp(0.5)}, wantLow: 1, wantSeverity: SeverityError},
		{name: "recall at the floor", config: "[lint.evals]\nmin_activation_recall = 0.5\n", act: &evals.ActivationRecord{Recall: fp(0.5)}},
		{name: "precision below the floor", config: "[lint.evals]\nmin_activation_precision = 0.9\n", act: &evals.ActivationRecord{Precision: fp(0.6)}, wantLow: 1, wantSeverity: SeverityError},
		{name: "nothing measured is no finding", config: "[lint.evals]\nmin_activation_recall = 0.8\n", act: &evals.ActivationRecord{}},
		{name: "sibling over the confusion threshold", config: "[lint.evals]\nconfusion_threshold = 0.25\n",
			act: &evals.ActivationRecord{StolenBy: []evals.StolenBy{{Skill: "rival", Prompts: 2, Share: 0.5}, {Skill: "minor", Prompts: 1, Share: 0.1}}}, wantConfuse: 1, wantSeverity: SeverityWarning},
		{name: "a record measured on an older skill is skipped", config: "[lint.evals]\nmin_activation_recall = 0.8\n", act: &evals.ActivationRecord{Recall: fp(0.1)}, stale: true},
		{name: "an unsigned record is flagged, not trusted", config: "[lint.evals]\nmin_activation_recall = 0.8\n", act: &evals.ActivationRecord{Recall: fp(0.1)}, forge: true, wantLow: 1, wantSeverity: SeverityError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			key := isolateEvalKey(t)
			if tt.forge {
				key = nil
			}
			writeFiles(t, root, evalRunnerFixture(tt.config))
			recordActivation(t, key, root, "good", tt.act, tt.stale)
			gitAdd(t, root)

			// Act
			findings := lintDir(t, root)

			// Assert
			if got := countCode(findings, CodeActivationLow); got != tt.wantLow {
				t.Errorf("AR9A1 count = %d, want %d: %v", got, tt.wantLow, findings)
			}
			if got := countCode(findings, CodeSkillConfusable); got != tt.wantConfuse {
				t.Errorf("AR9A2 count = %d, want %d: %v", got, tt.wantConfuse, findings)
			}
			for _, f := range findings {
				if (f.Code == CodeActivationLow || f.Code == CodeSkillConfusable) && f.Severity != tt.wantSeverity {
					t.Errorf("%s severity = %s, want %s", f.Code, f.Severity, tt.wantSeverity)
				}
				if f.Code == CodeEvalStale || f.Code == CodeEvalScoreLow {
					t.Errorf("an activation-only record must not raise %s: %v", f.Code, f)
				}
			}
		})
	}
}

func TestValidateEvalSettings_ActivationRanges(t *testing.T) {
	bad := &config.LintConfig{Evals: &config.LintEvals{MinActivationRecall: 1.5, MinActivationPrecision: -0.1, ConfusionThreshold: 2}}
	if problems := validateEvalSettings(bad); len(problems) != 3 {
		t.Errorf("want 3 problems, got %v", problems)
	}
	ok := &config.LintConfig{Evals: &config.LintEvals{MinActivationRecall: 0.8, MinActivationPrecision: 0.9, ConfusionThreshold: 0.25}}
	if problems := validateEvalSettings(ok); len(problems) != 0 {
		t.Errorf("valid settings rejected: %v", problems)
	}
}
