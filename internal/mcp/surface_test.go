package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func promptNames(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var names []string
	for p, err := range session.Prompts(context.Background(), nil) {
		require.NoError(t, err)
		names = append(names, p.Name)
	}
	return names
}

var readOnlyServeTools = []string{"find_skill", "load_skill", "find_context", "load_context"}

func TestInstructionsFollowTheRegisteredTools(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		build       func(t *testing.T) *Server
		contains    []string
		notContains []string
	}{
		{
			name:     "authoring server with every tool",
			build:    func(*testing.T) *Server { return NewServer("t", WithAnyDirectory()) },
			contains: []string{"CRUD tools", "generate_outputs", "validate_config"},
		},
		{
			name:        "read-only authoring server",
			build:       func(*testing.T) *Server { return NewServer("t", WithAnyDirectory(), WithReadOnly()) },
			contains:    []string{"read-only", "validate_config"},
			notContains: []string{"CRUD", "generate_outputs", "--read-only"},
		},
		{
			name: "merged server with every tool",
			build: func(t *testing.T) *Server {
				return NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory())
			},
			contains: []string{"CRUD tools", "generate_outputs", "find_skill", "--read-only"},
		},
		{
			name: "read-only merged server",
			build: func(t *testing.T) *Server {
				return NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithReadOnly())
			},
			contains:    []string{"read-only", "find_skill", "load_skill", "validate_config"},
			notContains: []string{"CRUD", "generate_outputs", "--read-only"},
		},
		{
			name: "merged server limited to the serving tools",
			build: func(t *testing.T) *Server {
				return NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithTools(readOnlyServeTools...))
			},
			contains:    []string{"find_skill", "load_skill"},
			notContains: []string{"CRUD", "generate_outputs", "validate_config", "search_skills", "list_skill_resources", "--read-only"},
		},
		{
			name: "skills-only server",
			build: func(t *testing.T) *Server {
				return NewSkillServer("t", loadCatalog(t))
			},
			contains:    []string{"find_skill", "load_skill", "cannot modify"},
			notContains: []string{"CRUD", "generate_outputs"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := connect(t, tc.build(t))

			got := session.InitializeResult().Instructions

			for _, s := range tc.contains {
				assert.Contains(t, got, s)
			}
			for _, s := range tc.notContains {
				assert.NotContains(t, got, s)
			}
		})
	}
}

func TestPromptsAreRegisteredOnlyWhenTheirToolsAre(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		build func(t *testing.T) *Server
		want  []string
	}{
		{"full authoring", func(*testing.T) *Server { return NewServer("t", WithAnyDirectory()) },
			[]string{"author-skill", "add-rule", "review-config", "trim-context"}},
		{"read-only authoring", func(*testing.T) *Server { return NewServer("t", WithAnyDirectory(), WithReadOnly()) },
			[]string{"review-config"}},
		{"read-only merged", func(t *testing.T) *Server {
			return NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithReadOnly())
		}, []string{"review-config"}},
		{"serving tools only", func(t *testing.T) *Server {
			return NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithTools(readOnlyServeTools...))
		}, nil},
		{"rule tools only", func(*testing.T) *Server {
			return NewServer("t", WithAnyDirectory(), WithTools("list_rules", "read_rule", "create_rule", "update_rule", "validate_config"))
		}, []string{"add-rule"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := connect(t, tc.build(t))

			got := promptNames(t, session)

			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

// The tools a prompt needs are read from its text, so the text and the rule that
// hides the prompt cannot drift apart.
func TestPromptNeedsAreTheToolsItsTextNames(t *testing.T) {
	t.Parallel()
	srv := NewServer("t", WithAnyDirectory())
	want := map[string][]string{
		"author-skill":  {"cost_report", "create_skill", "list_rules", "list_skills", "scan_content", "update_skill", "validate_config"},
		"add-rule":      {"create_rule", "list_rules", "read_rule", "update_rule", "validate_config"},
		"review-config": {"approvals_status", "cost_report", "doctor", "lock_status", "policy_show", "scan_content", "token_report", "validate_config"},
		"trim-context":  {"cost_report", "read_context", "read_rule", "read_skill", "token_report", "update_context", "update_rule", "update_skill"},
	}

	for _, spec := range authoringPrompts() {
		assert.Equal(t, want[spec.name], srv.promptNeeds(spec), "prompt %s", spec.name)
	}
}

func TestWithToolsRegistersOnlyTheNamedTools(t *testing.T) {
	t.Parallel()
	session := connect(t, NewMergedServer("t", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithTools(readOnlyServeTools...)))

	got := toolNames(t, session)

	// find_context and load_context need an indexed repository, which this catalog is not.
	assert.ElementsMatch(t, []string{"find_skill", "load_skill"}, got)
}

func TestKnownToolNamesCoverEveryRegistrableTool(t *testing.T) {
	t.Parallel()
	known := KnownToolNames()

	for _, want := range []string{"find_skill", "load_skill", "find_context", "load_context", "search_skills", "create_rule", "validate_config", "generate_outputs", "judge"} {
		assert.Contains(t, known, want)
	}
}

func TestValidateToolNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		names   []string
		wantErr string
	}{
		{"known names", []string{"find_skill", "load_context"}, ""},
		{"none", nil, ""},
		{"unknown name is named", []string{"find_skill", "find_skil"}, "find_skil"},
		{"valid names are listed", []string{"nope"}, "load_skill"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateToolNames(tc.names)

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
