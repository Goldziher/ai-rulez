package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/contentlock"
)

func lockOutputDigests(t *testing.T, headerBlock string) map[string]string {
	t.Helper()
	dir := hashesProject(t, headerBlock)
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	outputs, err := NewGenerator(cfg).LockOutputs("default")
	require.NoError(t, err)
	require.NotEmpty(t, outputs)
	snap, err := contentlock.Compute(cfg, contentlock.Options{IncludeOutputs: true, Outputs: outputs})
	require.NoError(t, err)
	got := map[string]string{}
	for _, o := range snap.Outputs {
		got[o.Path] = o.Digest
	}
	return got
}

func TestLockOutputsAreStableAcrossHeaderModesAndStamps(t *testing.T) {
	full := lockOutputDigests(t, "")
	assert.Equal(t, full, lockOutputDigests(t, "header:\n  hashes: content\n"))
	assert.Equal(t, full, lockOutputDigests(t, "header:\n  hashes: none\n"))
	assert.Equal(t, full, lockOutputDigests(t, "header:\n  timestamp: true\n"), "the Generated stamp is not part of the digest")
	assert.Contains(t, full, "CLAUDE.md")
	for path := range full {
		assert.False(t, strings.HasSuffix(path, "settings.json"), path)
	}
}

func TestStripGeneratedStamp(t *testing.T) {
	stamped := "<!-- AI-RULEZ GENERATED | Generated: 2026-01-02T03:04:05Z -->\n\n# Body\nGenerated: keep this line\n"
	got := stripGeneratedStamp(stamped, "CLAUDE.md")
	assert.NotContains(t, got, "2026-01-02")
	assert.Contains(t, got, "Generated: keep this line", "body lines are untouched")
}
