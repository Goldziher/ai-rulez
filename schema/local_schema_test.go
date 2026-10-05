package schema_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestLocalSchemaInSync(t *testing.T) {
	// Arrange
	main, err := os.ReadFile("ai-rules.schema.json")
	require.NoError(t, err)

	// Act
	want, err := schema.DeriveLocalSchema(main)
	require.NoError(t, err)
	if os.Getenv("UPDATE_LOCAL_SCHEMA") == "1" {
		require.NoError(t, os.WriteFile("ai-rules-local.schema.json", want, 0o600))
	}
	got, err := os.ReadFile("ai-rules-local.schema.json")
	require.NoError(t, err)

	// Assert: compared as JSON documents, because the committed file is
	// reformatted by the repository formatter after generation.
	var wantDoc, gotDoc any
	require.NoError(t, json.Unmarshal(want, &wantDoc))
	require.NoError(t, json.Unmarshal(got, &gotDoc))
	assert.Equal(t, wantDoc, gotDoc,
		"regenerate with: UPDATE_LOCAL_SCHEMA=1 go test ./schema -run TestLocalSchemaInSync, then poly fmt --fix")
}

func TestValidateLocalFile(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "partial overlay without required keys", body: "default = \"dev\"\n[profiles]\ndev = [\"backend\"]"},
		{name: "remove and preset drop accepted", body: "presets = [\"!cursor\", \"codex\"]\n[[includes]]\nname = \"shared\"\nremove = true"},
		{name: "mcp server override without command", body: "[[mcp_servers]]\nname = \"a\"\nenabled = false"},
		{name: "named entry without name rejected", body: "[[mcp_servers]]\ncommand = \"x\"", wantErr: true},
		{name: "scope identified by path accepted", body: "[[scopes]]\npath = \"svc\"\nprofile = \"p\""},
		{name: "scope identified by name accepted", body: "[[scopes]]\nname = \"api\"\nremove = true"},
		{name: "scope without name or path rejected", body: "[[scopes]]\nprofile = \"p\"", wantErr: true},
		{name: "unknown key rejected", body: "bogus = 1", wantErr: true},
		{name: "remove must be boolean", body: "[[includes]]\nname = \"a\"\nremove = \"yes\"", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeTOML(t, tt.body)

			// Act
			err := schema.ValidateLocalFile(path)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
