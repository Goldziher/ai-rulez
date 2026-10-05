package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePruneBundle(t *testing.T, root string) []config.OutputFile {
	t.Helper()
	outputs, err := AddProvenance([]config.OutputFile{
		{Path: filepath.Join(root, "skills/old/SKILL.md"), RawContent: []byte("---\nname: old\n---\n\nbody\n")},
		{Path: filepath.Join(root, "skills/old/asset.bin"), RawContent: []byte{0, 1, 2}},
	}, root)
	require.NoError(t, err)
	for _, output := range outputs {
		require.NoError(t, os.MkdirAll(filepath.Dir(output.Path), 0o750))
		require.NoError(t, os.WriteFile(output.Path, output.RawContent, 0o600))
	}
	return []config.OutputFile{{Path: filepath.Join(root, provenanceFileName), PluginInventory: true}}
}

func TestPlanStaleOutputsSafety(t *testing.T) {
	for _, scenario := range []string{"modified-header", "symlink-file", "symlink-parent", "traversal", "absolute", "directory", "unsupported-schema", "invalid-json", "missing-inventory", "missing-entry"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			outputs := writePruneBundle(t, root)
			target := filepath.Join(root, "skills/old/SKILL.md")
			original, err := os.ReadFile(target)
			require.NoError(t, err)
			outside := t.TempDir()
			external := filepath.Join(outside, "SKILL.md")
			require.NoError(t, os.WriteFile(external, original, 0o600))
			sidecar := filepath.Join(root, provenanceFileName)
			switch scenario {
			case "modified-header":
				require.NoError(t, os.WriteFile(target, append([]byte("\n"), original...), 0o600))
			case "symlink-file":
				require.NoError(t, os.Remove(target))
				require.NoError(t, os.Symlink(external, target))
			case "symlink-parent":
				require.NoError(t, os.RemoveAll(filepath.Dir(target)))
				require.NoError(t, os.Symlink(outside, filepath.Dir(target)))
			case "directory":
				require.NoError(t, os.Remove(target))
				require.NoError(t, os.Mkdir(target, 0o750))
			case "invalid-json":
				require.NoError(t, os.WriteFile(sidecar, []byte("broken"), 0o600))
			default:
				data, err := os.ReadFile(sidecar)
				require.NoError(t, err)
				var doc provenanceDocument
				require.NoError(t, json.Unmarshal(data, &doc))
				switch scenario {
				case "traversal":
					doc.Outputs["../outside"] = provenanceOutput{ContentHash: hashBytes(original)}
				case "absolute":
					doc.Outputs[external] = provenanceOutput{ContentHash: hashBytes(original)}
				case "unsupported-schema":
					doc.SchemaVersion = "v99"
				case "missing-inventory":
					doc.Outputs = nil
				case "missing-entry":
					delete(doc.Outputs, "skills/old/SKILL.md")
				}
				data, err = json.Marshal(doc)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(sidecar, data, 0o600))
			}
			_, err = PlanStaleOutputs(outputs)
			require.Error(t, err)
			data, err := os.ReadFile(external)
			require.NoError(t, err)
			assert.Equal(t, original, data)
		})
	}
}

func TestRemoveStaleOutputsBinaryMissingAndUntracked(t *testing.T) {
	root := t.TempDir()
	outputs := writePruneBundle(t, root)
	require.NoError(t, os.Remove(filepath.Join(root, "skills/old/SKILL.md")))
	manual := filepath.Join(root, "skills/old/manual.md")
	require.NoError(t, os.WriteFile(manual, []byte("manual"), 0o600))
	stale, err := PlanStaleOutputs(outputs)
	require.NoError(t, err)
	require.Len(t, stale, 1)
	require.NoError(t, RemoveStaleOutputs(stale))
	assert.NoFileExists(t, filepath.Join(root, "skills/old/asset.bin"))
	assert.FileExists(t, manual)
	assert.FileExists(t, outputs[0].Path)
}

func TestRemoveStaleOutputsRechecksBytes(t *testing.T) {
	root := t.TempDir()
	outputs := writePruneBundle(t, root)
	stale, err := PlanStaleOutputs(outputs)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stale[0].Path, []byte("edited"), 0o600))
	require.ErrorContains(t, RemoveStaleOutputs(stale), "changed during generation")
	data, err := os.ReadFile(stale[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "edited", string(data))
}
