package builtins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid universal", "ai-governance", true},
		{"valid language", "rust", true},
		{"valid binding", "pyo3", true},
		{"valid with exclusion prefix", "!ai-governance", true},
		{"valid polyglot bindings", "polyglot-bindings", true},
		{"invalid name", "nonexistent", false},
		{"empty string", "", false},
		{"valid default-commands", "default-commands", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, IsValid(tt.input))
		})
	}
}

func TestIsAutoInclude(t *testing.T) {
	t.Parallel()

	assert.True(t, IsAutoInclude("ai-governance"))
	assert.True(t, IsAutoInclude("security"))
	assert.True(t, IsAutoInclude("code-quality"))
	assert.True(t, IsAutoInclude("testing"))
	assert.True(t, IsAutoInclude("git-workflow"))
	assert.True(t, IsAutoInclude("token-efficiency"))
	assert.False(t, IsAutoInclude("rust"))
	assert.False(t, IsAutoInclude("docker"))
	assert.False(t, IsAutoInclude("cicd"))
}

func TestResolveBuiltins(t *testing.T) {
	t.Parallel()

	t.Run("empty list still includes auto-includes", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{})
		assert.Contains(t, result, "ai-governance")
		assert.Contains(t, result, "agent-delegation")
		assert.Contains(t, result, "code-quality")
		assert.Contains(t, result, "testing")
		assert.Contains(t, result, "git-workflow")
		assert.Contains(t, result, "security")
		assert.Contains(t, result, "token-efficiency")
	})

	t.Run("explicit exclusion removes auto-include", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{"!ai-governance"})
		assert.NotContains(t, result, "ai-governance")
	})

	t.Run("explicit includes are added", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{"rust", "python", "pyo3"})
		assert.Contains(t, result, "rust")
		assert.Contains(t, result, "python")
		assert.Contains(t, result, "pyo3")
		assert.Contains(t, result, "ai-governance") // auto-include
	})

	t.Run("exclude and include together", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{"!ai-governance", "rust", "security"})
		assert.NotContains(t, result, "ai-governance")
		assert.Contains(t, result, "rust")
		assert.Contains(t, result, "security")
	})

	t.Run("result is sorted", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{"python", "rust", "go"})
		// Should be sorted and include all auto-includes + explicit
		assert.Equal(t, []string{
			"agent-delegation", "ai-governance", "code-quality",
			"git-workflow", "go", "python", "rust",
			"security", "testing", "token-efficiency",
		}, result)
	})

	t.Run("default-commands included when requested", func(t *testing.T) {
		t.Parallel()
		result := ResolveBuiltins([]string{"default-commands"})
		assert.Contains(t, result, "default-commands")
		assert.Contains(t, result, "ai-governance")
	})
}

func TestList(t *testing.T) {
	t.Parallel()

	domains := List()
	assert.GreaterOrEqual(t, len(domains), 24) // 8 universal + default-commands + agent-delegation + 9 language + 6 binding

	// Check categories are grouped
	var lastCategory Category
	for _, d := range domains {
		if d.Category != lastCategory {
			if lastCategory != "" {
				assert.Less(t, string(lastCategory), string(d.Category),
					"categories should be sorted")
			}
			lastCategory = d.Category
		}
	}
}

func TestLoadDomainContent(t *testing.T) {
	t.Parallel()

	t.Run("loads ai-governance domain", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("ai-governance")
		require.NoError(t, err)
		assert.Len(t, entries, 11) // 9 rules + 2 agents

		ruleCount := 0
		agentCount := 0
		for _, e := range entries {
			assert.NotEmpty(t, e.Content)
			switch e.Type {
			case "rules":
				ruleCount++
			case "agents":
				agentCount++
			}
		}
		assert.Equal(t, 9, ruleCount)
		assert.Equal(t, 2, agentCount)
	})

	t.Run("loads security domain with rules and context", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("security")
		require.NoError(t, err)
		assert.Len(t, entries, 6) // 4 rules + 1 context + 1 agent

		ruleCount := 0
		contextCount := 0
		for _, e := range entries {
			switch e.Type {
			case "rules":
				ruleCount++
			case "context":
				contextCount++
			}
		}
		assert.Equal(t, 4, ruleCount)
		assert.Equal(t, 1, contextCount)
	})

	t.Run("loads language builtin", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("rust")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("loads binding builtin", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("pyo3")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("nonexistent domain returns nil", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("nonexistent")
		require.NoError(t, err)
		assert.Nil(t, entries)
	})

	t.Run("loads default-commands domain", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("default-commands")
		require.NoError(t, err)
		assert.Len(t, entries, 2) // iterate + parallelize

		commandCount := 0
		for _, e := range entries {
			if e.Type == "commands" {
				commandCount++
			}
		}
		assert.Equal(t, 2, commandCount)
	})

	t.Run("loads polyglot-bindings domain", func(t *testing.T) {
		t.Parallel()
		entries, err := LoadDomainContent("polyglot-bindings")
		require.NoError(t, err)
		assert.Len(t, entries, 5) // 3 rules + 2 agents

		ruleCount := 0
		agentCount := 0
		for _, e := range entries {
			switch e.Type {
			case "rules":
				ruleCount++
			case "agents":
				agentCount++
			}
		}
		assert.Equal(t, 3, ruleCount)
		assert.Equal(t, 2, agentCount)
	})
}
