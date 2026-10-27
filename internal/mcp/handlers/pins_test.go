package handlers

import (
	"context"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePins records the calls it gets and answers with canned results.
type fakePins struct {
	outdated func(PinCall) (any, error)
	update   func(PinCall) (any, bool, error)
	calls    []PinCall
}

func (f *fakePins) Outdated(_ context.Context, call PinCall) (any, error) {
	f.calls = append(f.calls, call)
	return f.outdated(call)
}

func (f *fakePins) Update(_ context.Context, call PinCall) (any, bool, error) {
	f.calls = append(f.calls, call)
	return f.update(call)
}

func callPinHandler(t *testing.T, h func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error), args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := h(t.Context(), NewToolRequest(nil, args))
	require.NoError(t, err)
	return res
}

func TestPinHandlers_WithoutAnEngineReportItAsAnError(t *testing.T) {
	handlers := map[string]func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error){
		"lock_outdated": LockOutdatedHandler(nil), "update_plan": UpdatePlanHandler(nil), "update": UpdateHandler(nil),
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			res := callPinHandler(t, h, map[string]any{"working_directory": t.TempDir()})

			require.True(t, res.IsError)
			assert.Contains(t, textOf(t, res), "not available")
		})
	}
}

func TestPinHandlers_PassTheSelectorsToTheEngine(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(PinEngine) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
		wantDryRun bool
	}{
		{"update_plan plans", UpdatePlanHandler, true},
		{"update writes", UpdateHandler, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &fakePins{update: func(PinCall) (any, bool, error) {
				return map[string]any{"schema_version": 1, "dry_run": true, "updates": []any{}}, false, nil
			}}

			// Act
			res := callPinHandler(t, tt.handler(fake), map[string]any{"working_directory": t.TempDir(), "kind": "include", "names": []any{"shared", "other"}})

			// Assert
			require.False(t, res.IsError, textOf(t, res))
			require.Len(t, fake.calls, 1)
			assert.Equal(t, "include", fake.calls[0].Kind)
			assert.Equal(t, []string{"shared", "other"}, fake.calls[0].Names)
			assert.Equal(t, tt.wantDryRun, fake.calls[0].DryRun)
			assert.NotNil(t, fake.calls[0].Load)
		})
	}
}

func TestPinHandlers_RejectAnUnknownKindBeforeTheEngineRuns(t *testing.T) {
	fake := &fakePins{}

	for name, h := range map[string]func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error){
		"lock_outdated": LockOutdatedHandler(fake), "update_plan": UpdatePlanHandler(fake), "update": UpdateHandler(fake),
	} {
		t.Run(name, func(t *testing.T) {
			res := callPinHandler(t, h, map[string]any{"working_directory": t.TempDir(), "kind": "served"})

			require.True(t, res.IsError)
			assert.Contains(t, textOf(t, res), `unknown kind "served"`)
		})
	}
	assert.Empty(t, fake.calls)
}

func TestUpdateHandler_RefusalIsAnErrorResultCarryingTheDocument(t *testing.T) {
	// Arrange
	doc := map[string]any{"schema_version": 1, "dry_run": false, "updates": []any{}, "blocked": []any{map[string]any{"code": "AR732"}}}
	fake := &fakePins{update: func(PinCall) (any, bool, error) { return doc, true, nil }}

	// Act
	res := callPinHandler(t, UpdateHandler(fake), map[string]any{"working_directory": t.TempDir()})

	// Assert
	require.True(t, res.IsError)
	require.Len(t, res.Content, 2)
	assert.JSONEq(t, `{"schema_version":1,"dry_run":false,"updates":[],"blocked":[{"code":"AR732"}]}`, textOf(t, res))
	assert.Contains(t, res.Content[1].(*sdkmcp.TextContent).Text, "nothing was written")
}

func TestUpdateHandler_FailureIsAToolErrorAndSuccessNamesTheNextStep(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantError bool
		wantLines int
	}{
		{"a failure", errors.New("list the tags of include \"shared\": unreachable"), true, 1},
		{"a success", nil, false, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakePins{update: func(PinCall) (any, bool, error) {
				if tt.err != nil {
					return nil, false, tt.err
				}
				return map[string]any{"schema_version": 1, "dry_run": false, "updates": []any{}}, false, nil
			}}

			res := callPinHandler(t, UpdateHandler(fake), map[string]any{"working_directory": t.TempDir()})

			assert.Equal(t, tt.wantError, res.IsError)
			assert.Len(t, res.Content, tt.wantLines)
		})
	}
}

func TestLockOutdatedHandler_ReturnsTheEngineDocument(t *testing.T) {
	fake := &fakePins{outdated: func(PinCall) (any, error) {
		return map[string]any{"schema_version": 1, "sources": []any{}}, nil
	}}

	res := callPinHandler(t, LockOutdatedHandler(fake), map[string]any{"working_directory": t.TempDir()})

	require.False(t, res.IsError, textOf(t, res))
	assert.JSONEq(t, `{"schema_version":1,"sources":[]}`, textOf(t, res))
}

func TestLockDiffHandler_ProjectWithoutALockIsNotAnError(t *testing.T) {
	// Arrange: unlike lock_status, a project that was never locked has nothing to compare.
	dir := governanceProject(t, "")

	// Act
	res, doc := callGov(t, LockDiffHandler("test", nil), dir, nil)

	// Assert
	require.False(t, res.IsError, textOf(t, res))
	assert.Contains(t, doc, "in_sync")
}
