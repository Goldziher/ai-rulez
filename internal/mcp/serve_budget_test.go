package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every way to read skill bytes draws on the one session budget.
func TestSessionBudget_CoversEveryReadPath(t *testing.T) {
	t.Parallel()
	cat := loadCatalog(t)
	forms := "skill://pdf-processing/references/FORMS.md"
	p, srv := startSkillServerWith(t, cat, ServeOptions{BudgetBytes: 2500})

	for range 2 {
		_, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": forms})
		require.False(t, isErr, text)
	}
	assert.Equal(t, 2000, srv.Used(""))

	resp := p.call("resources/read", map[string]any{"uri": forms})
	assert.NotNil(t, resp["error"], "the third 1000 bytes exceed the cap; resources/read is charged like the tools")
	assert.Equal(t, 2000, srv.Used(""))
}

func TestSessionBudget_RefusesPastTheCapOnEveryPath(t *testing.T) {
	t.Parallel()
	cat := loadCatalog(t)
	forms := "skill://pdf-processing/references/FORMS.md"
	p, srv := startSkillServerWith(t, cat, ServeOptions{BudgetBytes: 1500})

	_, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": forms})
	require.False(t, isErr, text)

	_, isErr, text = callTool(t, p, "read_skill_file", map[string]any{"uri": forms})
	assert.True(t, isErr)
	assert.Contains(t, text, "session budget exhausted")

	resp := p.call("resources/read", map[string]any{"uri": forms})
	assert.NotNil(t, resp["error"], "resources/read is refused past the cap")

	_, isErr, text = callTool(t, p, "get_skill", map[string]any{"name": "pdf-processing"})
	if isErr {
		assert.Contains(t, text, "session budget exhausted")
	}
	_, isErr, _ = callTool(t, p, "load_skill", map[string]any{"name": "pdf-processing", "path": "references/FORMS.md"})
	assert.True(t, isErr, "load_skill shares the same budget")
	assert.LessOrEqual(t, srv.Used(""), 1500)
}

func TestSessionBudget_TracksABoundedNumberOfSessions(t *testing.T) {
	t.Parallel()
	st := newServeState(ServeOptions{BudgetBytes: 100})
	for i := range maxTrackedSessions + 50 {
		_, ok := st.charge("s"+string(rune('a'+i%26))+string(rune(i)), 1)
		require.True(t, ok)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	assert.LessOrEqual(t, len(st.used), maxTrackedSessions)
}
