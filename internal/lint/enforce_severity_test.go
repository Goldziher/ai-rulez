package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestEnforceRaisesUnpinnedRemoteAndMCPToErrors(t *testing.T) {
	sev := func(cfg *config.Config) (Severity, Severity) {
		r := &runner{cfg: cfg}
		if cfg.Lint != nil {
			r.lc = *cfg.Lint
		}
		r.resolveSettings()
		return r.sev[CodeUnpinnedRemote], r.sev[CodeMCPUnpinned]
	}
	on, off := true, false

	remote, mcp := sev(&config.Config{Lock: &config.LockConfig{Enforce: &on}})
	assert.Equal(t, SeverityError, remote)
	assert.Equal(t, SeverityError, mcp)

	remote, mcp = sev(&config.Config{})
	assert.Equal(t, SeverityWarning, remote, "validate's default")
	assert.Equal(t, SeverityWarning, mcp, "validate's default")

	remote, mcp = sev(&config.Config{Lock: &config.LockConfig{Enforce: &off}})
	assert.Equal(t, SeverityWarning, remote)
	assert.Equal(t, SeverityWarning, mcp)

	// an explicit [lint.severity] still wins
	remote, _ = sev(&config.Config{Lock: &config.LockConfig{Enforce: &on}, Lint: &config.LintConfig{Severity: map[string]string{"AR010": "warning"}}})
	assert.Equal(t, SeverityWarning, remote)
}
