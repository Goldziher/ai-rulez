package commands

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoConfigHead = "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"

func repoConfigProject(t *testing.T, tail string) {
	t.Helper()
	judgeProject(t, map[string]string{
		".ai-rulez/config.toml":     repoConfigHead + tail,
		".claude/skills/a/SKILL.md": skillBody,
	})
}

type sarifNotes struct {
	Runs []struct {
		Invocations []struct {
			Notifications []struct {
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"toolExecutionNotifications"`
		} `json:"invocations"`
	} `json:"runs"`
}

func (s sarifNotes) texts() []string {
	var out []string
	for _, n := range s.Runs[0].Invocations[0].Notifications {
		out = append(out, n.Message.Text)
	}
	return out
}

func TestJudgeReportsRepoSettingsItIgnoresInEveryFormat(t *testing.T) {
	tests := []struct {
		name, tail string
		want       []string
	}{
		{"user-only keys in [judge]", "\n[judge]\nmodel = \"openai/gpt-4o\"\nbase_url = \"https://example.com\"\n",
			[]string{"[judge] table sets model", "[judge] table sets base_url", "[llm] base_url in your user config"}},
		{"provider settings in [llm]", "\n[llm]\nmodel = \"openai/gpt-4o\"\nbase_url = \"https://example.com\"\n",
			[]string{"[llm] table sets model, base_url"}},
	}
	for _, tc := range tests {
		for _, format := range []string{"text", "json", "sarif"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				// Arrange
				repoConfigProject(t, tc.tail)
				rig := newJudgeRig(t)

				// Act
				stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0", "--format", format)

				// Assert
				require.Equal(t, 0, code, rig.stderr.String())
				switch format {
				case "sarif":
					var doc sarifNotes
					require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
					all := ""
					for _, s := range doc.texts() {
						all += s + "\n"
					}
					for _, w := range tc.want {
						assert.Contains(t, all, w)
					}
				default:
					all := stdout + rig.stderr.String()
					for _, w := range tc.want {
						assert.Contains(t, all, w)
					}
				}
			})
		}
	}
}

func TestJudgeRejectsAnUnknownJudgeKeyLikeValidateDoes(t *testing.T) {
	// Arrange
	repoConfigProject(t, "\n[judge]\nfail_onn = [\"security:high\"]\n")
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, judgeRun...)

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "fail_onn")
}

func TestJudgeStaysQuietWhenTheRepoConfigSetsNothingItIgnores(t *testing.T) {
	// Arrange
	repoConfigProject(t, "\n[judge]\nmin_confidence = \"medium\"\n")
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, judgeRun...)

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Empty(t, parseJudgeDoc(t, stdout).Run.Notices)
}
