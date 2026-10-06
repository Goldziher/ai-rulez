package generator

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timestampProject writes a project emitting both root instruction files —
// CLAUDE.md (claude) and AGENTS.md (codex) — from one rule. The two renders are
// byte-identical by construction: the minimal header carries no per-output field,
// and both files inline the same rule.
func timestampProject(t *testing.T, header string) string {
	t.Helper()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"4.0\"\nname = \"stamped\"\npresets = [\"claude\", \"codex\"]\ngitignore = false\n\n[rules]\nmode = \"inline\"\n"+header),
		0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"),
		[]byte("---\npriority: high\n---\n# Style\n\nUse tabs.\n"), 0o644))

	return tempDir
}

var generatedLine = regexp.MustCompile(`(?m)^Generated: .*$`)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// TestGenerator_OutputIsReproducibleAcrossRuns is the regression test for the
// non-reproducible-output defect: the same sources, generated from two clean
// checkouts, must produce byte-identical output. The `Generated:` header line
// made that impossible, so generated output could not be verified by content
// hash and every regeneration that touched anything rewrote every file with a
// fresh value.
//
// The runs are deliberately separated by more than a second. The header
// timestamp has one-second resolution, so without the wait two runs inside the
// same second would agree by luck and the test would pass against the defect.
func TestGenerator_OutputIsReproducibleAcrossRuns(t *testing.T) {
	t.Parallel()

	first := timestampProject(t, "")
	generateProfile(t, first, "default")
	firstClaude := readFile(t, filepath.Join(first, "CLAUDE.md"))

	time.Sleep(1100 * time.Millisecond)

	second := timestampProject(t, "")
	generateProfile(t, second, "default")

	assert.Equal(t, firstClaude, readFile(t, filepath.Join(second, "CLAUDE.md")),
		"identical sources must generate identical bytes regardless of when generate ran")
}

// TestGenerator_SiblingRootFilesAgree covers the second consequence: CLAUDE.md
// and AGENTS.md render from the same sources through the same minimal header, so
// they must not differ. They did whenever the two renders landed in different
// seconds, because each preset read its own clock.
func TestGenerator_SiblingRootFilesAgree(t *testing.T) {
	t.Parallel()

	tempDir := timestampProject(t, "")
	generateProfile(t, tempDir, "default")

	claude := readFile(t, filepath.Join(tempDir, "CLAUDE.md"))
	agents := readFile(t, filepath.Join(tempDir, "AGENTS.md"))

	assert.NotContains(t, claude, "Generated:",
		"the default header must carry no per-run value")
	assert.Equal(t, claude, agents,
		"the two root instruction files render from the same sources and must agree")
}

// TestGenerator_TimestampOptInSharedAcrossOutputs pins the opt-in path: with
// `[header] timestamp = true` every file a single run writes must carry the same
// value, taken from the run's timestamp rather than from a clock each renderer
// reads for itself.
func TestGenerator_TimestampOptInSharedAcrossOutputs(t *testing.T) {
	t.Parallel()

	tempDir := timestampProject(t, "\n[header]\ntimestamp = true\n")

	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	cfg.GeneratedAt = time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	claude := readFile(t, filepath.Join(tempDir, "CLAUDE.md"))
	agents := readFile(t, filepath.Join(tempDir, "AGENTS.md"))

	assert.Contains(t, claude, "Generated: 2024-03-04 05:06:07",
		"the run timestamp must be the only clock a renderer reads")
	assert.Equal(t, generatedLine.FindString(claude), generatedLine.FindString(agents),
		"every output of one run must carry the same Generated value")
}

// TestGenerator_SourceDateEpochPinsTimestamp covers the reproducible-builds
// convention: an opt-in timestamp is pinned by SOURCE_DATE_EPOCH, so a project
// that wants the header line can still verify its output by content hash.
func TestGenerator_SourceDateEpochPinsTimestamp(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	tempDir := timestampProject(t, "\n[header]\ntimestamp = true\n")
	generateProfile(t, tempDir, "default")

	want := "Generated: " + time.Unix(1700000000, 0).Format("2006-01-02 15:04:05")
	assert.Contains(t, readFile(t, filepath.Join(tempDir, "CLAUDE.md")), want)
}

// TestGenerator_SourceDateEpochIgnoredWhenUnparsable keeps a malformed value
// from failing generation: it falls back to the wall clock rather than erroring
// or stamping the epoch.
func TestGenerator_SourceDateEpochIgnoredWhenUnparsable(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "not-a-number")

	tempDir := timestampProject(t, "\n[header]\ntimestamp = true\n")
	generateProfile(t, tempDir, "default")

	line := generatedLine.FindString(readFile(t, filepath.Join(tempDir, "CLAUDE.md")))
	require.NotEmpty(t, line, "a malformed SOURCE_DATE_EPOCH must not suppress the header line")
	assert.NotContains(t, line, "1970-01-01",
		"a malformed value must fall back to the wall clock, not the epoch")
	year := strings.TrimPrefix(line, "Generated: ")[:4]
	assert.GreaterOrEqual(t, year, "2024", "expected a wall-clock year, got %q", line)
}

// TestGenerator_SharedAgentsMDIdenticalAcrossPresets pins that codex, opencode,
// xum and amp, which all write AGENTS.md, render it byte for byte alike, both
// alone and together (where the last writer wins), including a context entry
// that carries a summary.
func TestGenerator_SharedAgentsMDIdenticalAcrossPresets(t *testing.T) {
	t.Parallel()

	project := func(presets ...string) string {
		dir := t.TempDir()
		configDir := filepath.Join(dir, ".ai-rulez")
		require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(configDir, "context"), 0o755))
		list := "[\"" + strings.Join(presets, "\", \"") + "\"]"
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
			"version = \"4.0\"\nname = \"shared\"\npresets = "+list+"\ngitignore = false\n\n[rules]\nmode = \"inline\"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"),
			[]byte("---\npriority: high\n---\n# Style\n\nUse tabs.\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "context", "arch.md"),
			[]byte("---\nsummary: One line about the architecture\n---\n# Arch\n\nLayers.\n"), 0o644))
		generateProfile(t, dir, "default")
		// Source-Hash legitimately varies with the preset list.
		return regexp.MustCompile(`(?m)^Source-Hash: .*\n`).ReplaceAllString(readFile(t, filepath.Join(dir, "AGENTS.md")), "")
	}

	codex := project("codex")

	assert.NotContains(t, codex, "One line about the architecture")
	for _, preset := range []string{"opencode", "xum", "amp"} {
		assert.Equal(t, codex, project(preset), preset)
	}
	assert.Equal(t, codex, project("amp", "codex", "opencode", "xum"))
}
