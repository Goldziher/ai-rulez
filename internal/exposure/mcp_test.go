package exposure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpInventory() *Inventory {
	return &Inventory{Harness: HarnessClaude, Root: "/r", Artifacts: []Artifact{
		{Kind: KindClaudeMD, QualifiedName: "CLAUDE.md", Path: "CLAUDE.md", Scope: RootScope, Phase: PhaseStart, Tokens: 100},
		{Kind: KindMCPServer, QualifiedName: "a", Path: ".mcp.json", Scope: RootScope, Phase: PhaseStart, Tokens: 1},
		{Kind: KindMCPServer, QualifiedName: "b", Path: ".mcp.json", Scope: RootScope, Phase: PhaseStart, Tokens: 1},
		{Kind: KindMCPServer, QualifiedName: "c", Path: "svc/.mcp.json", Scope: "svc", Phase: PhaseOnDemand, Tokens: 1},
	}}
}

func TestNewReport_McpSchemasAreOffByDefault(t *testing.T) {
	rep := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000})

	assert.Nil(t, rep.McpSchemas)
	assert.Equal(t, 102, rep.StartTokens)
}

func TestNewReport_McpSchemasAreCountedPerStartServer(t *testing.T) {
	rep := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000, McpServerTokens: 300, StartBudget: 650})

	require.NotNil(t, rep.McpSchemas)
	assert.Equal(t, McpSchemas{Servers: 2, TokensPerServer: 300, Tokens: 600}, *rep.McpSchemas)
	assert.Equal(t, 702, rep.StartTokens, "102 from the files plus 600 estimated")
	assert.True(t, rep.OverStartBudget())
}

func TestNewReport_McpSchemasAreInTheByKindBreakdown(t *testing.T) {
	rep := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000, McpServerTokens: 300})

	assert.Equal(t, 600, rep.StartTokensByKind[KindMCPSchemas])
	sum := 0
	for _, n := range rep.StartTokensByKind {
		sum += n
	}
	assert.Equal(t, rep.StartTokens, sum)

	off := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000})
	assert.NotContains(t, off.StartTokensByKind, KindMCPSchemas)
}

func TestNewReport_McpSchemasAreInTheStartPhaseTokens(t *testing.T) {
	rep := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000, McpServerTokens: 300})

	assert.Equal(t, 702, rep.StartTokens)
	assert.Equal(t, rep.StartTokens, rep.TokensByPhase[PhaseStart])
	assert.Equal(t, 1, rep.TokensByPhase[PhaseOnDemand])

	off := NewReport(mcpInventory(), ReportOptions{ContextWindow: 200000})
	assert.Equal(t, off.StartTokens, off.TokensByPhase[PhaseStart])
}
