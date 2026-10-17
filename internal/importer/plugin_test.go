package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPluginHooks(t *testing.T) {
	tests := []struct {
		name      string
		harness   string
		doc       string
		wantEvent []string
		wantTgt   []string
		problem   bool
	}{
		{"claude wrapped table", config.HarnessClaude,
			`{"description":"d","hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"fmt"}]}]}}`,
			[]string{"PostToolUse"}, []string{config.HarnessClaude}, false},
		{"bare event table", config.HarnessCodex,
			`{"SessionStart":[{"hooks":[{"type":"command","command":"hi"}]}]}`, []string{"SessionStart"}, []string{config.HarnessCodex}, false},
		{"cursor camel case events", config.HarnessCursor,
			`{"hooks":{"preToolUse":[{"command":"check"}]}}`, []string{"PreToolUse"}, []string{config.HarnessCursor}, false},
		{"gemini events and millisecond timeouts", config.HarnessGemini,
			`{"hooks":{"BeforeTool":[{"hooks":[{"type":"command","command":"x","timeout":2500}]}]}}`, []string{"PreToolUse"}, []string{config.HarnessGemini}, false},
		{"unknown event is reported", config.HarnessClaude, `{"hooks":{"Nope":[{"hooks":[{"type":"command","command":"x"}]}]}}`, nil, nil, true},
		{"prompt hook is not a command", config.HarnessClaude, `{"hooks":{"Stop":[{"hooks":[{"type":"prompt","prompt":"p"}]}]}}`, nil, nil, true},
		{"not json", config.HarnessClaude, `{`, nil, nil, true},
		{"hooks of the wrong type", config.HarnessClaude, `{"hooks":[]}`, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			groups, findings := PluginHooks("hooks.json", []byte(tt.doc), tt.harness)

			// Assert
			var events []string
			for _, g := range groups {
				events = append(events, g.Event)
				assert.Equal(t, tt.wantTgt, g.Targets)
			}
			assert.Equal(t, tt.wantEvent, events)
			if tt.problem {
				assert.NotEmpty(t, findings)
			}
		})
	}
}

func TestPluginHooks_GeminiTimeoutIsInSeconds(t *testing.T) {
	// Act
	groups, _ := PluginHooks("hooks.json", []byte(`{"hooks":{"BeforeTool":[{"hooks":[{"type":"command","command":"x","timeout":2500}]}]}}`), config.HarnessGemini)

	// Assert
	require.Len(t, groups, 1)
	assert.Equal(t, 3, groups[0].Hooks[0].Timeout)
}

func TestPluginMCP(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		names []string
	}{
		{"wrapped", `{"mcpServers":{"b":{"command":"x"},"a":{"url":"https://e.com/mcp"}}}`, []string{"a", "b"}},
		{"bare table", `{"s":{"command":"x","args":["-y","pkg"]}}`, []string{"s"}},
		{"servers key", `{"servers":{"v":{"command":"c"}}}`, []string{"v"}},
		{"server without command or url", `{"mcpServers":{"x":{}}}`, nil},
		{"comments are tolerated", "// c\n{\"mcpServers\":{\"s\":{\"command\":\"x\"}}}", []string{"s"}},
		{"table of the wrong type", `{"mcpServers":[]}`, nil},
		{"not json", `{`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			servers, _ := PluginMCP(".mcp.json", []byte(tt.doc))

			// Assert
			var got []string
			for _, s := range servers {
				got = append(got, s.Name)
			}
			assert.Equal(t, tt.names, got)
		})
	}
}
