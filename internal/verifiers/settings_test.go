package verifiers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const (
	forbidSpec = "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.forbid]\nregex = \"DROP\"\n"
	domainSpec = "[[verifiers]]\nid = \"dv\"\nrule = \"db/dbrule\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.forbid]\nregex = \"DROP\"\n"
)

func withDomainRule(cfg *config.Config) *config.Config {
	cfg.Content.Domains = map[string]*config.Domain{"db": {Name: "db", Rules: []config.ContentFile{{Name: "dbrule", Content: "# DB\n"}}}}
	return cfg
}

func TestRun_MaxFileBytesSetting(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		wantNote string
	}{
		{"default reads small files", 0, ""},
		{"a low limit skips the file with a note", 4, "larger than 4 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := specProject(t, map[string]string{"a.sql": "DROP TABLE t;\n"}, forbidSpec)
			if tt.limit > 0 {
				cfg.VerifiersSettings = &config.VerifiersSettings{MaxFileBytes: tt.limit}
			}

			// Act
			rep := Run(context.Background(), cfg, Options{})

			// Assert
			require.Len(t, rep.Results, 1)
			res := rep.Results[0]
			if tt.wantNote == "" {
				assert.Equal(t, StatusFail, res.Status)
				return
			}
			assert.Equal(t, StatusPass, res.Status, "an unread file cannot be flagged")
			assert.Contains(t, strings.Join(res.Notes, " "), tt.wantNote)
		})
	}
}

func TestRun_WarnDeadSetting(t *testing.T) {
	spec := "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"nothing/**\"]\n[verifiers.require.forbid]\nregex = \"x\"\n"
	cfg := specProject(t, map[string]string{"a.sql": "x"}, spec)

	plain := Run(context.Background(), cfg, Options{})
	cfg.VerifiersSettings = &config.VerifiersSettings{WarnDead: true}
	warned := Run(context.Background(), cfg, Options{})

	assert.Equal(t, StatusNotApplicable, plain.Results[0].Status)
	assert.Equal(t, StatusFail, warned.Results[0].Status)
	assert.Equal(t, CodeVerifierDeadScope, warned.Results[0].Code)
}

func TestRun_RequireExamples(t *testing.T) {
	withExample := forbidSpec + "[[verifiers.examples]]\nname = \"e\"\nexpect = \"pass\"\nfiles = { \"a.sql\" = \"ok\" }\nchanged = [\"a.sql\"]\n"
	tests := []struct {
		name    string
		toml    string
		require bool
		want    int
	}{
		{"off by default", forbidSpec, false, 1},
		{"missing examples reported", forbidSpec, true, 2},
		{"examples present", withExample, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := specProject(t, map[string]string{"a.sql": "ok"}, tt.toml)
			cfg.VerifiersSettings = &config.VerifiersSettings{RequireExamples: tt.require}

			rep := Run(context.Background(), cfg, Options{})

			require.Len(t, rep.Results, tt.want)
			if tt.want == 2 {
				extra := rep.Results[1]
				assert.Equal(t, CodeVerifierNoExamples, extra.Code)
				assert.Equal(t, StatusFail, extra.Status)
				assert.Equal(t, severityWarning, extra.Severity)
				assert.False(t, rep.FailedAt(severityError), "AR9H6 is a warning")
				assert.True(t, rep.FailedAt(severityWarning))
			}
		})
	}
}

func TestRun_InactiveOutsideTheActiveProfile(t *testing.T) {
	tests := []struct {
		name     string
		profiles map[string][]string
		opts     Options
		want     Status
	}{
		{"no profiles defined: every domain is active", nil, Options{}, StatusFail},
		{"profiles without default: domain rules are outside it", map[string][]string{"backend": {"db"}}, Options{}, StatusInactive},
		{"named profile includes the domain", map[string][]string{"backend": {"db"}}, Options{Profile: "backend"}, StatusFail},
		{"named profile without the domain", map[string][]string{"backend": {"db"}, "web": {}}, Options{Profile: "web"}, StatusInactive},
		{"explicit default profile", map[string][]string{"default": {"db"}}, Options{}, StatusFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := withDomainRule(specProject(t, map[string]string{"a.sql": "DROP TABLE t;\n"}, domainSpec))
			cfg.Profiles = tt.profiles

			// Act
			rep := Run(context.Background(), cfg, tt.opts)

			// Assert
			require.NoError(t, rep.Err)
			require.Len(t, rep.Results, 1)
			assert.Equal(t, tt.want, rep.Results[0].Status, rep.Results[0].Message)
			if tt.want == StatusInactive {
				assert.Contains(t, rep.Results[0].Message, "is not in the active profile")
				assert.False(t, rep.FailedAt(severityWarning), "an inactive verifier never fails the run")
				assert.False(t, rep.CannotRun())
			}
		})
	}
}

func TestRun_UnknownProfileAndRoleCannotRun(t *testing.T) {
	cfg := specProject(t, nil, forbidSpec)
	cfg.Profiles = map[string][]string{"a": {}}

	byProfile := Run(context.Background(), cfg, Options{Profile: "ghost"})
	byRole := Run(context.Background(), cfg, Options{Role: "ghost"})

	require.Error(t, byProfile.Err)
	assert.Contains(t, byProfile.Err.Error(), "not defined")
	require.Error(t, byRole.Err)
	assert.Contains(t, byRole.Err.Error(), "role")
}

func TestReports_InactiveAndSkippedAreVisibleButNeverFail(t *testing.T) {
	rep := &Report{Results: []Result{
		{Name: "a", Type: "forbid", Severity: "error", Status: StatusInactive, Message: "rule \"x\" is not in the active profile \"web\""},
		{Name: "b", Type: "llm", Severity: "warning", Status: StatusSkipped, Code: CodeVerifierLLMSkipped, Message: "LLM use is off"},
	}}

	var text, sarif, junit, js bytes.Buffer
	require.NoError(t, WriteText(&text, rep))
	require.NoError(t, WriteSARIF(&sarif, rep, "test"))
	require.NoError(t, WriteJUnit(&junit, rep, "warning"))
	require.NoError(t, WriteJSON(&js, rep))

	assert.Contains(t, text.String(), "1 skipped, 1 inactive")
	assert.Contains(t, sarif.String(), "AR9H4 b: LLM use is off")
	assert.Contains(t, sarif.String(), `"executionSuccessful": true`)
	assert.Contains(t, junit.String(), "<skipped")
	assert.Contains(t, js.String(), `"inactive": 1`)
	assert.False(t, rep.FailedAt("info"))
	assert.False(t, rep.CannotRun())
}

func TestReports_RefusedCommandIsAnErrorResult(t *testing.T) {
	rep := &Report{Results: []Result{{Name: "c", Type: "command", Severity: "error", Status: StatusError, Code: CodeVerifierCommand, Message: "needs --allow-exec", Fix: "pass --allow-exec"}}}

	var text, sarif bytes.Buffer
	require.NoError(t, WriteText(&text, rep))
	require.NoError(t, WriteSARIF(&sarif, rep, "test"))

	assert.Contains(t, text.String(), "AR9H3 c")
	assert.Contains(t, sarif.String(), `"ruleId": "AR9H3/c"`)
	assert.True(t, rep.CannotRun())
}
