package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

// pinEngineStub answers the pin tools with fixed documents.
type pinEngineStub struct {
	refuse bool
	calls  []handlers.PinCall
}

func (p *pinEngineStub) Outdated(_ context.Context, call handlers.PinCall) (any, error) {
	p.calls = append(p.calls, call)
	return map[string]any{"schema_version": 1, "sources": []any{}, "summary": map[string]any{"total": 0}}, nil
}

func (p *pinEngineStub) Update(_ context.Context, call handlers.PinCall) (any, bool, error) {
	p.calls = append(p.calls, call)
	doc := map[string]any{"schema_version": 1, "dry_run": call.DryRun, "updates": []any{}}
	if p.refuse {
		doc["blocked"] = []any{map[string]any{"kind": "include", "name": "shared", "code": "AR732"}}
	}
	return doc, p.refuse, nil
}

func TestPinToolsAreListedAsTheReadOnlySettingRequires(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []Option
		want map[string]bool
	}{
		{"read-only lists the three that do not write", []Option{WithReadOnly()},
			map[string]bool{"lock_outdated": true, "update_plan": true, "lock_diff": true, "update": false}},
		{"writable lists all four", nil,
			map[string]bool{"lock_outdated": true, "update_plan": true, "lock_diff": true, "update": true}},
		{"an allow-list that omits them lists none", []Option{WithTools("list_rules")},
			map[string]bool{"lock_outdated": false, "update_plan": false, "lock_diff": false, "update": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := connect(t, NewServer("test", append([]Option{WithAnyDirectory()}, tt.opts...)...))

			listed := map[string]bool{}
			for _, name := range toolNames(t, session) {
				listed[name] = true
			}

			for name, want := range tt.want {
				assert.Equal(t, want, listed[name], name)
			}
		})
	}
}

func TestPinToolsRunTheInjectedEngine(t *testing.T) {
	t.Parallel()
	dir := telemetryProject(t)
	stub := &pinEngineStub{refuse: true}
	session := connect(t, NewServer("test", WithAnyDirectory(), WithPinEngine(stub)))

	// A refusal is an error result that carries the document as structuredContent.
	res := call(t, session, "update", map[string]any{"working_directory": dir, "names": []string{"shared"}, "kind": "include"})
	require.True(t, res.IsError)
	data, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(data), "AR732")

	// The plan is a dry run and succeeds with the same engine.
	stub.refuse = false
	plan := call(t, session, "update_plan", map[string]any{"working_directory": dir})
	require.False(t, plan.IsError, textOfResult(t, plan))
	outdated := call(t, session, "lock_outdated", map[string]any{"working_directory": dir})
	require.False(t, outdated.IsError, textOfResult(t, outdated))

	require.Len(t, stub.calls, 3)
	assert.False(t, stub.calls[0].DryRun)
	assert.Equal(t, []string{"shared"}, stub.calls[0].Names)
	assert.Equal(t, "include", stub.calls[0].Kind)
	assert.True(t, stub.calls[1].DryRun)
}

func TestPinToolsWithoutAnEngineAreErrorsNotMissing(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))

	res := call(t, session, "lock_outdated", map[string]any{"working_directory": telemetryProject(t)})

	require.True(t, res.IsError)
	assert.Contains(t, textOfResult(t, res), "not available")
}

func TestPinToolsRejectAnUnknownKindInTheSchema(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory(), WithPinEngine(&pinEngineStub{})))

	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "update_plan", Arguments: map[string]any{"kind": "served"}})

	assert.True(t, err != nil || res.IsError, "the kind enum of the input schema refuses a served entry")
}
