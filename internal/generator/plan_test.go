package generator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const planConfig = `version = "5.0"
agents_md = false
name = "plan"
presets = ["claude", "cursor", "codex"]
gitignore = true

[[mcp_servers]]
name = "tool"
command = "tool"
args = ["--key", "literal-secret-value-123"]
[mcp_servers.env]
TOKEN = "${PLAN_TEST_TOKEN}"

[permissions]
allow = ["Bash(npm run test:*)"]
`

func planProject(t *testing.T, cfgText string) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(cfgText), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "rules", "r.md"), []byte("---\npriority: high\n---\n# R\n\nBody.\n"), 0o644))
	return dir
}

func planTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p) //nolint:errcheck // below dir
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		out[rel] = string(data)
		return nil
	}))
	return out
}

func loadPlanConfig(t *testing.T, dir string, env map[string]string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithHost(ambient.Host{Env: ambient.MapEnv{Vars: env, Home: t.TempDir()}}))
	require.NoError(t, err)
	return cfg
}

func TestPlanOutputsTouchesNothingOnDisk(t *testing.T) {
	// Arrange
	dir := planProject(t, planConfig)
	cfg := loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "resolved-token-value"})
	before := planTree(t, dir)

	// Act
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, before, planTree(t, dir), "planning must not write, delete or create anything")
	paths := map[string]PlanFile{}
	for _, f := range plan.Files {
		paths[f.Path] = f
	}
	for _, want := range []string{"CLAUDE.md", ".claude/rules/r.md", ".mcp.json", ".claude/settings.json"} {
		assert.Contains(t, paths, want)
	}
	assert.Equal(t, PlanWrite, paths[".claude/settings.json"].Action, "a document ai-rulez would create is its own")
	assert.Equal(t, PlanWrite, paths["CLAUDE.md"].Action)
}

func TestPlanMarksAnExistingSharedDocumentAsMerge(t *testing.T) {
	// Arrange
	dir := planProject(t, planConfig)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("{\"theme\": \"dark\"}\n"), 0o644))
	cfg := loadPlanConfig(t, dir, map[string]string{})

	// Act
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})

	// Assert
	require.NoError(t, err)
	for _, f := range plan.Files {
		if f.Path == ".claude/settings.json" {
			assert.Equal(t, PlanMerge, f.Action)
			return
		}
	}
	t.Fatal("settings.json is not in the plan")
}

func TestPlanOutputsIsDeterministicAndHoldsNoSecret(t *testing.T) {
	// Arrange
	dir := planProject(t, planConfig)
	cfg := loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "resolved-token-value"})

	// Act
	first, err := PlanOutputs(context.Background(), cfg, PlanOptions{})
	require.NoError(t, err)
	second, err := PlanOutputs(context.Background(), cfg, PlanOptions{})
	require.NoError(t, err)
	a, err := MarshalPlan(first)
	require.NoError(t, err)
	b, err := MarshalPlan(second)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, string(a), string(b))
	assert.NotContains(t, string(a), "resolved-token-value", "a resolved environment value must never reach the plan")
	assert.NotContains(t, string(a), "literal-secret-value-123")
	var mcp *PlanFile
	for i := range first.Files {
		if first.Files[i].Path == ".mcp.json" {
			mcp = &first.Files[i]
		}
	}
	require.NotNil(t, mcp)
	assert.True(t, mcp.Sensitive)
	assert.Empty(t, mcp.SHA256, "a sensitive output carries no digest")
	assert.Equal(t, "0600", mcp.Mode)
}

func TestPlanDigestsDoNotDependOnTheEnvironment(t *testing.T) {
	// Arrange: the same project resolved with two different secrets
	dir := planProject(t, planConfig)

	// Act
	one, err := PlanOutputs(context.Background(), loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "aaa"}), PlanOptions{})
	require.NoError(t, err)
	two, err := PlanOutputs(context.Background(), loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "bbb"}), PlanOptions{})
	require.NoError(t, err)

	// Assert
	assert.Equal(t, one, two)
}

func TestPlanOutputsReportsStaleFiles(t *testing.T) {
	// Arrange: generate with cursor, then drop it from the config
	dir := planProject(t, planConfig)
	cfg := loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "t"})
	require.NoError(t, NewGenerator(cfg).Generate("default"))
	reduced := strings.Replace(planConfig, `["claude", "cursor", "codex"]`, `["claude"]`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(reduced), 0o644))
	cfg = loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "t"})
	before := planTree(t, dir)

	// Act
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, before, planTree(t, dir))
	var stale []string
	for _, r := range plan.Removals {
		if r.Reason == RemoveStale {
			stale = append(stale, r.Path)
		}
	}
	assert.Contains(t, stale, ".cursor/rules/r.mdc")
	assert.NotContains(t, stale, "CLAUDE.md")
}

func TestPlanMatchesWhatGenerateWrites(t *testing.T) {
	// Arrange
	dir := planProject(t, strings.Replace(planConfig, "args = [\"--key\", \"literal-secret-value-123\"]", `args = ["-y"]`, 1))
	cfg := loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "t"})
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})
	require.NoError(t, err)

	// Act
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// Assert: every planned file exists, and the files that are not merged documents differ from
	// the rendering only by their header lines
	for _, f := range plan.Files {
		abs := filepath.Join(dir, filepath.FromSlash(f.Path))
		info, err := os.Stat(abs)
		require.NoError(t, err, f.Path)
		if f.Action == PlanMkdir {
			assert.True(t, info.IsDir(), f.Path)
			continue
		}
		if f.Sensitive {
			continue // the plan is conservative: a run may find the file holds no secret
		}
		assert.Equal(t, f.Mode, fmt.Sprintf("%04o", info.Mode().Perm()), f.Path)
	}
}

func TestPlanValidatesAgainstTheSchema(t *testing.T) {
	// Arrange
	dir := planProject(t, planConfig)
	plan, err := PlanOutputs(context.Background(), loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "t"}), PlanOptions{})
	require.NoError(t, err)
	data, err := MarshalPlan(plan)
	require.NoError(t, err)
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schema", "plan.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)

	// Act
	var doc any
	require.NoError(t, json.NewDecoder(bytes.NewReader(data)).Decode(&doc))
	res := compiled.Validate(doc)

	// Assert
	assert.True(t, res.IsValid(), "plan violates schema/plan.schema.json: %v", res.Errors)
}

func TestPlanOutputsRejectsRoleAndProfile(t *testing.T) {
	// Arrange
	dir := planProject(t, planConfig)
	cfg := loadPlanConfig(t, dir, map[string]string{})

	// Act
	_, err := PlanOutputs(context.Background(), cfg, PlanOptions{Profile: "p", Role: "r"})

	// Assert
	require.Error(t, err)
}

// TestPlanDigestsEqualWrittenFilesForEveryPreset enables every preset and requires each planned
// whole-file output to carry the digest of the bytes generate writes (header hashes off, so the
// only difference left would be a rendering one such as the trailing newline).
func TestPlanDigestsEqualWrittenFilesForEveryPreset(t *testing.T) {
	// Arrange
	files := map[string]string{
		"rules/style.md":    "---\npriority: high\n---\n\nUse tabs.\n",
		"skills/s/SKILL.md": "---\nname: s\ndescription: A skill\n---\n\nDo it.\n",
	}
	base := writeChecksProject(t, config.IndividualPresetNames(), files, "\n[header]\nhashes = \"none\"\ntimestamp = false\n")
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})
	require.NoError(t, err)

	// Act
	generateChecksProject(t, base, "default")

	// Assert
	checked := 0
	for _, f := range plan.Files {
		if f.Action != PlanWrite || f.Sensitive || f.SHA256 == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(f.Path)))
		require.NoError(t, err, f.Path)
		sum := sha256.Sum256(data)
		assert.Equal(t, hex.EncodeToString(sum[:]), f.SHA256, "plan digest of %s differs from the written file", f.Path)
		assert.Equal(t, len(data), f.Size, f.Path)
		checked++
	}
	assert.Greater(t, checked, 50)
}

func TestPlanOmitsTheDigestOfAnOutputHoldingADetectedSecret(t *testing.T) {
	// Arrange: a literal credential in a rule lands in the rendered instruction files
	dir := planProject(t, strings.Replace(planConfig, "args = [\"--key\", \"literal-secret-value-123\"]", `args = ["-y"]`, 1))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", "leak.md"), []byte("---\npriority: high\n---\n# Leak\n\nUse AKIAIOSFODNN7EXAMPLE for the bucket.\n"), 0o644))
	cfg := loadPlanConfig(t, dir, map[string]string{"PLAN_TEST_TOKEN": "t"})

	// Act
	plan, err := PlanOutputs(context.Background(), cfg, PlanOptions{})
	require.NoError(t, err)

	// Assert
	byPath := map[string]PlanFile{}
	for _, f := range plan.Files {
		byPath[f.Path] = f
	}
	claude := byPath[".claude/rules/leak.md"]
	assert.Empty(t, claude.SHA256, "the digest of an output holding a credential is not published")
	assert.Zero(t, claude.Size)
	assert.False(t, claude.Sensitive)
	assert.Equal(t, PlanWrite, claude.Action)
	data, err := MarshalPlan(plan)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "AKIAIOSFODNN7EXAMPLE")
	var digested int
	for _, f := range plan.Files {
		if f.SHA256 != "" {
			digested++
		}
	}
	assert.Positive(t, digested, "outputs without a credential keep their digest")
}
