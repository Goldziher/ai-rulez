package lint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestARDCodesMatchPackage(t *testing.T) {
	assert.Equal(t, ard.CodeSchema, CodeARDSchema)
	assert.Equal(t, ard.CodeIdentifier, CodeARDIdentifier)
	assert.Equal(t, ard.CodeEntry, CodeARDEntry)
	assert.Equal(t, ard.CodeQueries, CodeARDQueries)
	assert.Equal(t, ard.CodeNotDeclared, CodeARDNotDeclared)
}

func lintARD(t *testing.T, mutate func(*config.Config), opts ...Option) (*config.Config, []Finding) {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, policyFixture(""))
	gitAdd(t, root)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	mutate(cfg)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	rep, err := Run(cfg, tree, opts...)
	require.NoError(t, err)
	return cfg, rep.Findings
}

func TestStrictReportsARDFindingsOnTheConfigFile(t *testing.T) {
	findings := []ard.Finding{
		{Rule: ard.RuleSchema, Severity: ard.SeverityError, Identifier: "urn:air:acme.test:tools:a", Path: "/entries/0/displayName", Message: "missing"},
		{Rule: ard.RuleIdentifier, Severity: ard.SeverityError, Message: "skill \"a b\": bad name"},
		{Rule: ard.RuleEntry, Severity: ard.SeverityError, Identifier: "urn:air:acme.test:tools:b", Message: "both url and data"},
		{Rule: ard.RuleQueries, Severity: ard.SeverityWarning, Identifier: "urn:air:acme.test:tools:c", Message: "no representativeQueries"},
	}

	cfg, got := lintARD(t, func(c *config.Config) { c.ARD = &config.ARDConfig{Publisher: "acme.test", Namespace: "tools"} }, WithARD(findings))

	codes := map[string]Severity{}
	for _, f := range got {
		if strings.HasPrefix(f.Code, "AR9S") {
			codes[f.Code] = f.Severity
			assert.Equal(t, filepath.ToSlash(filepath.Join(cfg.ConfigDir, cfg.ConfigFile)), f.File)
		}
	}
	assert.Equal(t, map[string]Severity{
		CodeARDSchema: SeverityError, CodeARDIdentifier: SeverityError, CodeARDEntry: SeverityError, CodeARDQueries: SeverityWarning,
	}, codes)
}

func TestStrictIsQuietWithoutARD(t *testing.T) {
	_, got := lintARD(t, func(*config.Config) {})
	for _, f := range got {
		assert.NotContains(t, f.Code, "AR9S", "%v", f)
	}
}

func TestStrictReportsAnARDEmitterWithoutATable(t *testing.T) {
	_, got := lintARD(t, func(c *config.Config) {
		c.Publish = &config.PublishConfig{Emitters: []config.PublishEmitter{{Name: config.PublishEmitterARD}}}
	})

	var codes []string
	for _, f := range got {
		codes = append(codes, f.Code)
	}
	assert.Contains(t, codes, CodeARDNotDeclared)
}

func TestExplainResolvesTheARDCodes(t *testing.T) {
	for _, code := range []string{CodeARDSchema, CodeARDIdentifier, CodeARDEntry, CodeARDQueries, CodeARDNotDeclared} {
		ex, ok := Explain(code)
		require.True(t, ok, code)
		assert.NotEmpty(t, ex.Why, code)
	}
}

func TestStrictARDFindingNamesTheEntry(t *testing.T) {
	findings := []ard.Finding{
		{Rule: ard.RuleQueries, Severity: ard.SeverityWarning, Identifier: "urn:air:acme.test:tools:c", Message: "no representativeQueries"},
	}
	_, got := lintARD(t, func(c *config.Config) { c.ARD = &config.ARDConfig{Publisher: "acme.test", Namespace: "tools"} }, WithARD(findings))
	var msgs []string
	for _, f := range got {
		if f.Code == CodeARDQueries {
			msgs = append(msgs, f.Message)
		}
	}
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "urn:air:acme.test:tools:c")
}
