package importer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestConvert_RedactedURLsValidate(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{".mcp.json": `{"mcpServers":{"s":{"type":"http","url":"https://user:pw1234@x.example/mcp?api_key=qq998877&v=1"}}}`})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, report.Validation.Errors, "%v", report.Validation.Messages)
	assert.True(t, report.Written)
	cfgText := snapshot(t, dir)[".ai-rulez/config.toml"]
	assert.NotContains(t, cfgText, "pw1234")
	assert.NotContains(t, cfgText, "qq998877")
}
