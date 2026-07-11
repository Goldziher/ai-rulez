package plugin

import (
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddProvenanceDecoratesCommentSafeFilesAndTracksJSON(t *testing.T) {
	baseDir := t.TempDir()
	outputs := []config.OutputFile{
		{Path: baseDir + "/plugin.json", RawContent: []byte("{\"name\":\"example\"}\n")},
		{Path: baseDir + "/index.js", RawContent: []byte("export default async () => ({});\n")},
		{Path: baseDir + "/skills/example/SKILL.md", RawContent: []byte("---\nname: example\n---\n\n# Example\n")},
	}

	result, err := AddProvenance(outputs, baseDir)
	require.NoError(t, err)
	require.Len(t, result, 4)
	assert.Equal(t, outputs[0].RawContent, result[0].RawContent, "strict JSON must remain byte-identical")
	assert.Contains(t, string(result[1].RawContent), "// AI-RULEZ :: GENERATED FILE — DO NOT EDIT")
	assert.Contains(t, string(result[2].RawContent), "---\n\n<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT")

	var sidecar provenanceDocument
	require.NoError(t, json.Unmarshal(result[3].RawContent, &sidecar))
	assert.Equal(t, provenanceSchema, sidecar.SchemaVersion)
	assert.Len(t, sidecar.Outputs, 3)
	assert.NotContains(t, sidecar.Outputs, provenanceFileName)
}

func TestAddProvenanceIsDeterministic(t *testing.T) {
	baseDir := t.TempDir()
	outputs := []config.OutputFile{
		{Path: baseDir + "/b.json", RawContent: []byte("{}\n")},
		{Path: baseDir + "/a.js", RawContent: []byte("export {};\n")},
	}

	first, err := AddProvenance(outputs, baseDir)
	require.NoError(t, err)
	second, err := AddProvenance(outputs, baseDir)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
