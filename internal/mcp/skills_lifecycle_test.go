package mcp

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolVersionsCompareAsDates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version string
		want    bool
	}{
		{"2026-07-28", true},
		{"2026-12-01", true},
		{"2027-01-01", true},
		{"2026-07-27", false},
		{"2025-11-25", false},
		{"", false},
		{"latest", false},
		{"2026-7-28", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.want, protocolAtLeast(tt.version, sessionlessProtocol))
		})
	}
}

// A client of the sessionless protocol (2026-07-28) sends no initialize: every
// request carries its protocol version in _meta. The SDK serves it, so the guard
// and the tolerant-initialize middleware must let it through, and keep refusing
// anything that does not name that version. This also pins sessionlessProtocol
// to the SDK's behaviour.
func TestSessionlessRequestsNeedNoInitialize(t *testing.T) {
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	meta := func(version string) map[string]any {
		return map[string]any{"_meta": map[string]any{
			sdkmcp.MetaKeyProtocolVersion:                version,
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}}
	}

	for _, method := range []string{"tools/list", "skills/list", "resources/list"} {
		t.Run(method+" with the sessionless version", func(t *testing.T) {
			p, _ := startUninitializedSkillServer(t, cat, ServeOptions{})

			resp := p.call(method, meta(sessionlessProtocol))

			assert.Nil(t, resp["error"], "a sessionless request needs no initialize: %v", resp)
			assert.NotNil(t, resp["result"])
		})
		t.Run(method+" with an older version", func(t *testing.T) {
			p, _ := startUninitializedSkillServer(t, cat, ServeOptions{})

			resp := p.call(method, meta("2025-11-25"))

			require.NotNil(t, resp["error"], "an older client must initialize first")
		})
	}
}

// The skills extension runs through the SDK, so a malformed parameter is the
// SDK's invalid-params error rather than a hand-written one.
func TestSkillsExtensionRejectsMalformedParams(t *testing.T) {
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})

	for name, params := range map[string]map[string]any{
		"a number for the uri": {"uri": 7},
		"an unknown skill":     {"uri": "skill://nope/SKILL.md"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := p.call("skills/get", params)

			rpcErr, ok := resp["error"].(map[string]any)
			require.True(t, ok, "got %v", resp)
			assert.EqualValues(t, -32602, rpcErr["code"])
		})
	}
}
