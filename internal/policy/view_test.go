package policy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// threeLayers is the documented example: an org baseline, a team file and a CI flag.
func threeLayers() []Layer {
	managed := Layer{Origin: OriginManaged, Path: "/etc/ai-rulez/policy.toml", Name: "example-org baseline", Digest: "sha256:5c1e00000000000000000000000000000000000000000000000000000000aaaa", Policy: Policy{
		Sources:   Sources{Allowed: List{Set: true, Items: []string{"*.example.org", "github.com/example-org"}}, RequirePinned: true},
		Lint:      Lint{RequiredCodes: []string{"AR001", "AR005"}, SeverityFloor: map[string]string{"AR005": "error", "AR008": "warning"}},
		Telemetry: Network{Disabled: true},
	}}
	env := Layer{Origin: OriginEnv, Path: "/ci/team.toml", Name: "team", Digest: "sha256:a90b00000000000000000000000000000000000000000000000000000000bbbb", Policy: Policy{
		Lint: Lint{SeverityFloor: map[string]string{"AR008": "error"}, Security: Security{ScanImports: "error"}},
		Lock: Lock{Enforce: true},
	}}
	flag := Layer{Origin: OriginFlag, Path: "./extra.toml", Digest: "sha256:0c3d00000000000000000000000000000000000000000000000000000000cccc", Policy: Policy{
		Guard: Guard{Generated: true},
	}}
	return []Layer{flag, env, managed}
}

func goldenCheck(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), "run UPDATE_GOLDEN=1 go test ./internal/policy to refresh")
}

func showReport(t *testing.T) Report {
	t.Helper()
	cfg := &config.Config{ConfigDir: t.TempDir(), ConfigFile: "config.toml", Lint: &config.LintConfig{
		Severity: map[string]string{"AR008": "warning"},
		Security: &config.LintSecurity{ScanImports: "warn"},
	}}
	res := Resolve(threeLayers()).Apply(cfg)
	rep := BuildReport(Resolve(threeLayers()), &res)
	for i := range rep.Violations { // the temp dir differs per run
		rep.Violations[i].File = "config.toml"
	}
	return rep
}

func TestShowPolicyGolden(t *testing.T) {
	// Arrange
	rep := showReport(t)
	var text, js bytes.Buffer
	// Act
	rep.WriteText(&text)
	require.NoError(t, rep.WriteJSON(&js))
	// Assert
	goldenCheck(t, "show_text.golden", text.Bytes())
	goldenCheck(t, "show_json.golden", js.Bytes())
}

func TestShowPolicyWithoutPolicy(t *testing.T) {
	var text, js bytes.Buffer
	rep := BuildReport(nil, nil)
	rep.WriteText(&text)
	require.NoError(t, rep.WriteJSON(&js))
	assert.Contains(t, text.String(), "policy: none")
	var back map[string]any
	require.NoError(t, json.Unmarshal(js.Bytes(), &back))
	assert.Equal(t, []any{}, back["layers"])
	assert.Equal(t, map[string]any{}, back["effective"])
}
