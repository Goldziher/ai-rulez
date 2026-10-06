package lint

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeReport is the shape `claude plugin validate --json` prints (captured
// from claude 2.1.285), with a failing manifest and a skill.
const claudeReport = `{
  "success": false,
  "strict": false,
  "target": "/stage",
  "manifest": {"file": "/stage", "type": "plugin", "errors": [{"path": "directory", "message": "No manifest found", "code": null}], "warnings": [], "notes": []},
  "contents": [
    {"file": "/stage/skills/bad/SKILL.md", "type": "skill", "errors": [],
     "warnings": [{"path": "description", "message": "No description in frontmatter.", "code": null}],
     "notes": [{"path": "name", "message": "Name looks fine", "code": "NAME_OK"}]}
  ]
}`

func TestParseClaudeValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []externalFinding
		wantErr string
	}{
		{
			name: "errors, warnings and notes map to levels", in: claudeReport,
			want: []externalFinding{
				{File: "/stage", Severity: "error", Rule: "plugin:directory", Message: "No manifest found"},
				{File: "/stage/skills/bad/SKILL.md", Severity: "warning", Rule: "skill:description", Message: "No description in frontmatter."},
				{File: "/stage/skills/bad/SKILL.md", Severity: "note", Rule: "skill:NAME_OK", Message: "Name looks fine"},
			},
		},
		{name: "clean", in: `{"success":true,"manifest":null,"contents":[]}`, want: nil},
		{name: "failure without an error is not clean", in: `{"success":false,"manifest":null,"contents":[]}`, wantErr: "without naming an error"},
		{name: "another document", in: `{"hello":"world"}`, wantErr: "no success field"},
		{name: "not json", in: `oops`, wantErr: "invalid claude-validate-json"},
		{name: "empty", in: "  ", wantErr: "no output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExternalKeep("adapter:claude-validate-json", []byte(tt.in), 1, false)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSnyk(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		exit    int
		want    []externalFinding
		wantErr string
	}{
		{
			name: "issues object", in: `{"issues":[{"id":"E001","severity":"high","message":"prompt injection","file":"skills/a/SKILL.md","line":3}]}`,
			want: []externalFinding{{File: "skills/a/SKILL.md", Line: 3, Severity: "high", Rule: "E001", Message: "prompt injection"}},
		},
		{
			name: "bare list with alternate spellings", in: `[{"code":"W1","level":"low","title":"odd","path":"a.md","start_line":9}]`,
			want: []externalFinding{{File: "a.md", Line: 9, Severity: "low", Rule: "W1", Message: "odd"}},
		},
		{
			name: "per file", in: `{"b.json":{"issues":[{"rule":"R","severity":"medium","description":"d"}]},"a.json":{"issues":[]}}`,
			want: []externalFinding{{File: "b.json", Severity: "medium", Rule: "R", Message: "d"}},
		},
		{name: "empty issues is clean", in: `{"issues":[]}`, want: nil},
		{name: "empty issues with a failing exit is not clean", in: `{"issues":[]}`, exit: 2, wantErr: "exited with status 2"},
		{name: "another document", in: `{"version":"1"}`, wantErr: "not a snyk-json report"},
		{name: "a string", in: `"x"`, wantErr: "invalid snyk-json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExternalKeep("adapter:snyk-json", []byte(tt.in), tt.exit, false)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAdapterCapsResults(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"issues":[`)
	for i := 0; i <= maxScannerResults; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":"R","message":"m"}`)
	}
	b.WriteString(`]}`)
	_, err := parseExternalKeep("adapter:snyk-json", []byte(b.String()), 0, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than")
}

func TestUnknownAdapterIsRejectedAtConfigTime(t *testing.T) {
	lc := &config.LintConfig{External: []config.LintExternal{{Name: "x", Command: []string{"x"}, Format: "adapter:nope"}}}
	problems := ValidateSettings(lc)
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "unknown format")
	lc.External[0].Format = "adapter:snyk-json"
	assert.Empty(t, ValidateSettings(lc))
}

// A staged scanner whose profile uses the claude adapter and the plugin
// layout: the fake claude prints what the real one prints, with the paths it
// finds under its working directory (the stage).
func TestClaudeValidateProfileEndToEnd(t *testing.T) {
	fakeBin(t, "claude", `case "$*" in *"plugin validate --json"*) ;; *) echo "unexpected: $*" >&2; exit 9;; esac
printf '{"success":true,"strict":false,"target":"%s","manifest":null,"contents":[{"file":"%s/skills/deploy/SKILL.md","type":"skill","errors":[],"warnings":[{"path":"description","message":"Vague description","code":null}],"notes":[]}]}' "$PWD" "$PWD"
`)
	p := policyProject(t, "\n[lint.scanner_policy]\npreset = \"baseline\"\n\n[plugin]\nname = \"p\"\nversion = \"1.0.0\"\n")
	findings := p.run(Options{})
	got := scannerFindings(findings)
	require.Len(t, got, 1, dump(findings))
	assert.Contains(t, got[0].Message, "[claude-plugin-validate]")
	assert.Contains(t, got[0].Message, "Vague description")
	assert.Equal(t, SeverityWarning, got[0].Severity)
	assert.Equal(t, ".ai-rulez/skills/deploy/SKILL.md", got[0].RepoPath(), "the stage path maps back to the source file")
	assert.Empty(t, ofCode(findings, CodeScannerOutOfScope), dump(findings))
}

// Contract test against the real binary, opt-in.
func TestClaudeValidateContractLive(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_SCANNERS") != "1" {
		t.Skip("set AI_RULEZ_LIVE_SCANNERS=1 to run the contract test against the installed claude")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not installed")
	}
	p := policyProject(t, "\n[lint.scanner_policy]\npreset = \"baseline\"\n\n[plugin]\nname = \"p\"\nversion = \"1.0.0\"\n")
	findings := p.run(Options{})
	assert.Empty(t, ofCode(findings, CodeScannerRunFailed), dump(findings))
	assert.Empty(t, ofCode(findings, CodeScannerOutOfScope), dump(findings))
	for _, f := range ofCode(findings, CodeScannerUnavailable) {
		assert.NotContains(t, f.Message, "[claude-plugin-validate]", dump(findings)) // agnix may be missing; claude is the one under test
	}
}
