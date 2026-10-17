package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func manySkills(n int) map[string]string {
	files := map[string]string{}
	for i := range n {
		files[fmt.Sprintf(".claude/skills/s%03d/SKILL.md", i)] = skillBody
	}
	return files
}

func TestJudgeEstimateCoversMoreUnitsThanARunMayJudge(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantNote bool
		wantErr  string
	}{
		{"estimate", []string{"--estimate", "--max-cost", "0", "--format", "json"}, 0, true, ""},
		{"real run", []string{"--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0", "--format", "json"}, 1, false, "the limit is 5 per run"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, manySkills(6))
			rig := newJudgeRig(t)
			rig.maxUnits = 5

			// Act
			stdout, code := rig.run(t, tc.args...)

			// Assert
			assert.Equal(t, tc.wantCode, code, rig.stderr.String())
			assert.Contains(t, rig.stderr.String(), tc.wantErr)
			if tc.wantNote {
				doc := parseJudgeDoc(t, stdout)
				assert.Equal(t, 6, doc.Run.Estimate.Units)
				require.NotEmpty(t, doc.Run.Notices)
				assert.Contains(t, doc.Run.Notices[0], "6 units match; a real run is limited to 5 units and would be refused")
			}
		})
	}
}

func TestJudgeEstimateExitsNonZeroOnlyWhenOverTheCap(t *testing.T) {
	tests := []struct {
		name     string
		maxCost  string
		wantCode int
		wantWarn bool
	}{
		{"cap far above the estimate", "100", 0, false},
		{"no cap", "0", 0, false},
		{"cap below the estimate", "0.001", 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{".claude/skills/a/SKILL.md": skillBody})
			rig := newJudgeRig(t)

			// Act
			_, code := rig.run(t, "--estimate", "--max-cost", tc.maxCost, "--model", judgeTestModel)

			// Assert
			assert.Equal(t, tc.wantCode, code, rig.stderr.String())
			if tc.wantWarn {
				assert.Contains(t, rig.stderr.String(), "exceeds --max-cost $0.0010")
			} else {
				assert.NotContains(t, rig.stderr.String(), "exceeds --max-cost")
			}
		})
	}
}

func TestJudgeEstimateCountsPlannedCallsOnceAndSaysHowManyFit(t *testing.T) {
	// Arrange
	judgeProject(t, manySkills(5))
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--estimate", "--max-cost", "0.01", "--model", judgeTestModel, "--format", "json")

	// Assert
	assert.Equal(t, 1, code, "the estimate is over the cap")
	var doc struct {
		Run struct {
			Estimate struct {
				Calls    int `json:"calls"`
				CallsFit int `json:"calls_fit"`
			} `json:"estimate"`
			Notices []string `json:"notices"`
		} `json:"run"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	e := doc.Run.Estimate
	assert.Positive(t, e.Calls)
	assert.Less(t, e.CallsFit, e.Calls)
	assert.Contains(t, fmt.Sprint(doc.Run.Notices), fmt.Sprintf("%d of the %d planned calls fit under the caps", e.CallsFit, e.Calls))
}

func TestJudgeRefusesANamedPathThroughAnInRepoSymlink(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantErr bool
	}{
		{"symlink to a directory", "linkdir", true},
		{"file below a symlinked directory", "linkdir/AGENTS.md", true},
		{"symlink to a file", "link.md", true},
		{"the real directory", "real", false},
		{"the real file", "real/AGENTS.md", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			dir := judgeProject(t, map[string]string{"real/AGENTS.md": "# a\nRun tests.\n"})
			testutil.SymlinkOrSkip(t, filepath.Join(dir, "real"), filepath.Join(dir, "linkdir"))
			testutil.SymlinkOrSkip(t, filepath.Join(dir, "real", "AGENTS.md"), filepath.Join(dir, "link.md"))
			rig := newJudgeRig(t)

			// Act
			stdout, code := rig.run(t, "--estimate", "--max-cost", "0", "--format", "json", tc.arg)

			// Assert
			if tc.wantErr {
				assert.Equal(t, 1, code)
				assert.Contains(t, rig.stderr.String(), "symlink")
				return
			}
			require.Equal(t, 0, code, rig.stderr.String())
			assert.Equal(t, []string{"real/AGENTS.md"}, parseJudgeDoc(t, stdout).paths())
		})
	}
}

func TestJudgeUnitLimit(t *testing.T) {
	tests := []struct {
		name string
		deps judgeDeps
		want int
	}{
		{"the default limit", judgeDeps{}, 200},
		{"a lowered limit", judgeDeps{maxUnits: 5}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.deps.unitLimit())
		})
	}
}
