package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sessionStartBase = "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\ngitignore = false\n"

func lookOnly(present ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, p := range present {
			if p == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func sessionFindings(t *testing.T, dir string, opts Options) []Finding {
	t.Helper()
	opts.Load = loader(dir)
	if opts.LookPath == nil {
		opts.LookPath = lookOnly("ai-rulez", "npx", "uvx")
	}
	return byCheck(Run(context.Background(), opts), CheckSessionStart)
}

func TestCheckSessionStart_ListsTheLauncherOrder(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase, ".ai-rulez/rules/r.md": "# R\n\nbody\n"})
	generate(t, dir)

	got := sessionFindings(t, dir, Options{})

	require.Len(t, got, 1)
	assert.Equal(t, SeverityInfo, got[0].Severity)
	assert.Equal(t, "the SessionStart hook tries, in order: ai-rulez session-start; then "+
		"npx -y ai-rulez@latest session-start; then uvx --from ai-rulez@latest ai-rulez session-start", got[0].Message)
}

func TestCheckSessionStart_PinnedVersionIsShown(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase + "\n[session_start]\nversion = \"5.4.1\"\n"})
	generate(t, dir)

	got := sessionFindings(t, dir, Options{})

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Message, "npx -y ai-rulez@5.4.1 session-start")
	assert.Contains(t, got[0].Message, "uvx --from ai-rulez==5.4.1 ai-rulez session-start")
}

func TestCheckSessionStart_OffMeansSilent(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase + "\n[session_start]\ngenerate = false\n"})
	generate(t, dir)

	assert.Empty(t, sessionFindings(t, dir, Options{}))
}

func TestCheckSessionStart_SettingsWithoutTheHook(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase})
	// Not generated: a fresh clone has no .claude/settings.json.

	got := sessionFindings(t, dir, Options{})

	require.Len(t, got, 2)
	assert.Equal(t, SeverityWarning, got[0].Severity, "most serious first")
	assert.Equal(t, ".claude/settings.json", got[0].Path)
	assert.Contains(t, got[0].Message, "does not regenerate its outputs")
	assert.Equal(t, "run `ai-rulez generate` and commit .claude/settings.json", got[0].Hint)
}

func TestCheckSessionStart_SettingsIgnoredByGit(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase, ".gitignore": ".claude/\n"})
	gitInit(t, dir)
	generate(t, dir)

	got := sessionFindings(t, dir, Options{})

	var warnings []Finding
	for _, f := range got {
		if f.Severity == SeverityWarning {
			warnings = append(warnings, f)
		}
	}
	require.Len(t, warnings, 1, "%+v", got)
	assert.Contains(t, warnings[0].Message, ".claude/settings.json is ignored by git")
	assert.Contains(t, warnings[0].Hint, "do not ignore .claude/settings.json")
}

func TestCheckSessionStart_RemoteSession(t *testing.T) {
	const registry = "registry.npmjs.org"
	tests := []struct {
		name        string
		remote      string
		present     []string
		probeStatus int
		probeErr    error
		wantProbe   bool
		wantWarning string
	}{
		{name: "not remote: no environment checks", present: nil},
		{name: "ai-rulez installed: nothing to fetch", remote: "true", present: []string{"ai-rulez"}},
		{name: "no launcher at all", remote: "true", wantWarning: "none of ai-rulez, npx and uvx is on PATH"},
		{name: "uvx only: no registry probe", remote: "true", present: []string{"uvx"}},
		{name: "npx and a reachable registry", remote: "true", present: []string{"npx"}, probeStatus: 200, wantProbe: true},
		{name: "registry denied by the egress policy", remote: "true", present: []string{"npx"}, probeStatus: 403, wantProbe: true,
			wantWarning: "the network policy denied " + registry + " (HTTP 403)"},
		{name: "proxy authentication required", remote: "1", present: []string{"npx"}, probeStatus: 407, wantProbe: true,
			wantWarning: "the network policy denied " + registry + " (HTTP 407)"},
		{name: "registry unreachable", remote: "true", present: []string{"npx"}, probeErr: errors.New("dial tcp: i/o timeout\nmore"), wantProbe: true,
			wantWarning: registry + " is not reachable (dial tcp: i/o timeout)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase})
			generate(t, dir)
			t.Setenv("CLAUDE_CODE_REMOTE", tt.remote)
			probed := ""
			opts := Options{
				LookPath: lookOnly(tt.present...),
				Probe: func(_ context.Context, url string) (int, error) {
					probed = url
					return tt.probeStatus, tt.probeErr
				},
			}

			got := sessionFindings(t, dir, opts)

			var warnings []string
			for _, f := range got {
				if f.Severity == SeverityWarning {
					warnings = append(warnings, f.Message)
				}
			}
			if tt.wantWarning == "" {
				assert.Empty(t, warnings)
			} else {
				require.Len(t, warnings, 1)
				assert.True(t, strings.Contains(warnings[0], tt.wantWarning), warnings[0])
			}
			if tt.wantProbe {
				assert.Equal(t, "https://registry.npmjs.org/ai-rulez", probed)
			} else {
				assert.Empty(t, probed)
			}
		})
	}
}

func TestCheckSessionStart_CommittedSettingsFileIsRead(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": sessionStartBase})
	generate(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("{}\n"), 0o644))

	got := sessionFindings(t, dir, Options{})

	require.GreaterOrEqual(t, len(got), 1)
	assert.Equal(t, SeverityWarning, got[0].Severity)
}
