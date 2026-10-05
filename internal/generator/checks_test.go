package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
)

var checkPresets = []string{"cursor", "kilo", "qwen", "factory", "amp", "rovodev", "augment", "gitlab-duo"}

// checkOutputs maps each preset to the file its checks land in.
var checkOutputs = map[string]string{
	"cursor":     ".cursor/BUGBOT.md",
	"kilo":       "REVIEW.md",
	"qwen":       ".qwen/review-rules.md",
	"factory":    ".factory/skills/review-guidelines/SKILL.md",
	"amp":        ".agents/checks/security.md",
	"rovodev":    ".rovodev/.review-agent.md",
	"augment":    ".augment/code_review_guidelines.yaml",
	"gitlab-duo": ".gitlab/duo/mr-review-instructions.yaml",
}

func writeChecksProject(t *testing.T, presets []string, files map[string]string, extraConfig string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = `"` + p + `"`
	}
	cfg := "version = \"4.0\"\nname = \"checks\"\ngitignore = false\npresets = [" + strings.Join(quoted, ",") + "]\n" + extraConfig
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600))
	return base
}

func generateChecksProject(t *testing.T, base, profile string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(profile))
}

func readCheckOutput(t *testing.T, base, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	return string(data)
}

const securityCheck = "---\ndescription: Security issues\nseverity: critical\ntools: [Read, Grep]\n---\n\nFlag injection.\n"

func TestChecks_RenderPerPreset(t *testing.T) {
	for _, preset := range checkPresets {
		t.Run(preset, func(t *testing.T) {
			// Arrange
			base := writeChecksProject(t, []string{preset}, map[string]string{"checks/security.md": securityCheck}, "")

			// Act
			generateChecksProject(t, base, "")

			// Assert
			got := readCheckOutput(t, base, checkOutputs[preset])
			assert.Contains(t, got, "Flag injection.")
			assert.Contains(t, got, "security")
			switch preset {
			case "amp":
				assert.Contains(t, got, "severity-default: critical")
				assert.Contains(t, got, "name: security")
				assert.Contains(t, got, "description: Security issues")
				assert.Contains(t, got, "- Read")
			case "factory":
				assert.Contains(t, got, "name: review-guidelines")
				assert.Contains(t, got, "<!-- ai-rulez:check:security -->")
			case "augment":
				assert.Contains(t, got, "severity: high", "critical is written as high")
				assert.Contains(t, got, "id: security")
				assert.Contains(t, got, "'**'")
			case "gitlab-duo":
				assert.Contains(t, got, "name: security")
				assert.Contains(t, got, "instructions: Flag injection.")
			default:
				assert.Contains(t, got, "<!-- ai-rulez:check:security -->")
				assert.Contains(t, got, "## security")
			}
		})
	}
}

func TestChecks_NoChecksWritesNothing(t *testing.T) {
	base := writeChecksProject(t, checkPresets, map[string]string{"rules/r.md": "Rule.\n"}, "")

	generateChecksProject(t, base, "")

	for preset, rel := range checkOutputs {
		assert.NoFileExists(t, filepath.Join(base, filepath.FromSlash(rel)), preset)
	}
}

func TestChecks_TargetsSelectPresets(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, checkPresets, map[string]string{
		"checks/security.md": "---\ntargets: [kilo, augment]\n---\n\nOnly some.\n",
	}, "")

	// Act
	generateChecksProject(t, base, "")

	// Assert
	assert.Contains(t, readCheckOutput(t, base, "REVIEW.md"), "Only some.")
	assert.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "Only some.")
	for _, preset := range []string{"cursor", "qwen", "factory", "amp", "rovodev", "gitlab-duo"} {
		assert.NoFileExists(t, filepath.Join(base, filepath.FromSlash(checkOutputs[preset])), preset)
	}
}

func TestChecks_DomainsFollowProfiles(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"kilo"}, map[string]string{
		"checks/root.md":            "Root check.\n",
		"domains/api/checks/api.md": "API check.\n",
		"domains/web/checks/web.md": "Web check.\n",
	}, "\n[profiles]\napi = [\"api\"]\nweb = [\"web\"]\n")

	// Act
	generateChecksProject(t, base, "api")
	got := readCheckOutput(t, base, "REVIEW.md")

	// Assert
	assert.Contains(t, got, "Root check.")
	assert.Contains(t, got, "API check.")
	assert.NotContains(t, got, "Web check.")
}

func TestChecks_MultipleSourcesAreDeduplicatedByName(t *testing.T) {
	base := writeChecksProject(t, []string{"kilo"}, map[string]string{
		"checks/dup.md":             "Root wins.\n",
		"domains/api/checks/dup.md": "Domain loses.\n",
	}, "")

	generateChecksProject(t, base, "")
	got := readCheckOutput(t, base, "REVIEW.md")

	assert.Contains(t, got, "Root wins.")
	assert.NotContains(t, got, "Domain loses.")
	assert.Equal(t, 1, strings.Count(got, "ai-rulez:check:dup"))
}

func TestChecks_YAMLMergePreservesHandwrittenContentAndRemovesStale(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"augment", "gitlab-duo"}, map[string]string{
		"checks/security.md": securityCheck,
		"checks/perf.md":     "Perf.\n",
	}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	augmentPath := filepath.Join(base, ".augment", "code_review_guidelines.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(augmentPath), 0o755))
	require.NoError(t, os.WriteFile(gitlabPath, []byte("instructions:\n  - name: mine\n    instructions: hand written\nother: 1\n"), 0o600))
	require.NoError(t, os.WriteFile(augmentPath, []byte("areas:\n  mine:\n    description: d\n    globs: [\"**\"]\n    rules: []\nfile_paths_to_ignore: [a]\n"), 0o600))

	// Act: first run, then drop one check and edit the other
	generateChecksProject(t, base, "")
	first := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")
	require.NoError(t, os.Remove(filepath.Join(base, ".ai-rulez", "checks", "perf.md")))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "checks", "security.md"), []byte("Changed text.\n"), 0o600))
	generateChecksProject(t, base, "")
	gitlab := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")
	augment := readCheckOutput(t, base, ".augment/code_review_guidelines.yaml")

	// Assert
	assert.Contains(t, first, "name: perf")
	assert.Contains(t, gitlab, "hand written")
	assert.Contains(t, gitlab, "other: 1")
	assert.Contains(t, gitlab, "Changed text.")
	assert.NotContains(t, gitlab, "Flag injection.", "the edited group is replaced, not duplicated")
	assert.NotContains(t, gitlab, "name: perf", "a removed check's group is dropped")
	assert.Equal(t, 1, strings.Count(gitlab, "name: security"))
	assert.Contains(t, augment, "mine:")
	assert.Contains(t, augment, "file_paths_to_ignore")
	assert.Contains(t, augment, "security:")
	assert.NotContains(t, augment, "perf:")
}

func TestChecks_CleanRemovesOutputsAndKeepsHandwrittenYAML(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, checkPresets, map[string]string{"checks/security.md": securityCheck}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	require.NoError(t, os.WriteFile(gitlabPath, []byte("instructions:\n  - name: mine\n    instructions: hand\n"), 0o600))
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate(""))
	for preset, rel := range checkOutputs {
		require.FileExists(t, filepath.Join(base, filepath.FromSlash(rel)), preset)
	}

	// Act
	_, err = gen.Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	for _, preset := range []string{"cursor", "kilo", "qwen", "factory", "amp", "rovodev"} {
		assert.NoFileExists(t, filepath.Join(base, filepath.FromSlash(checkOutputs[preset])), preset)
	}
	got := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")
	assert.Contains(t, got, "name: mine")
	assert.NotContains(t, got, "security")
}

func TestChecks_FactoryAggregateCollidesWithUserSkill(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"factory"}, map[string]string{
		"checks/security.md":                "---\ndescription: Security issues\n---\n\nFlag injection.\n",
		"skills/review-guidelines/SKILL.md": "---\ndescription: Mine\n---\n\nMy guidelines.\n",
	}, "")
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)

	// Act
	err = NewGenerator(cfg).Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.FromSlash(".factory/skills/review-guidelines/SKILL.md"))
}

func TestChecks_OutputsAreNotGitignored(t *testing.T) {
	// Arrange: a stale entry from an earlier run must be dropped from the block.
	base := writeChecksProject(t, checkPresets, map[string]string{
		"checks/security.md": "---\ndescription: Security issues\n---\n\nFlag injection.\n",
	}, "")
	cfgPath := filepath.Join(base, ".ai-rulez", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(string(raw), "gitignore = false", "gitignore = true", 1)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".gitignore"),
		[]byte(gitignore.BeginMarker+"\nREVIEW.md\n.cursor/BUGBOT.md\n"+gitignore.EndMarker+"\n"), 0o600))

	// Act
	generateChecksProject(t, base, "")

	// Assert
	gi, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	require.NoError(t, err)
	for preset, rel := range checkOutputs {
		assert.NotContains(t, string(gi), rel, preset)
	}
	assert.NotContains(t, string(gi), "REVIEW.md")
}

func rewriteChecksConfigPresets(t *testing.T, base string, presets ...string) {
	t.Helper()
	cfgPath := filepath.Join(base, ".ai-rulez", "config.toml")
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = `"` + p + `"`
	}
	cfg := "version = \"4.0\"\nname = \"checks\"\ngitignore = false\npresets = [" + strings.Join(quoted, ",") + "]\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o600))
}

func TestChecks_HandwrittenFileIsMergedNotClobbered(t *testing.T) {
	for _, preset := range []string{"kilo", "qwen", "rovodev", "factory", "cursor"} {
		t.Run(preset, func(t *testing.T) {
			// Arrange
			rel := checkOutputs[preset]
			base := writeChecksProject(t, []string{preset}, map[string]string{"checks/security.md": securityCheck}, "")
			path := filepath.Join(base, filepath.FromSlash(rel))
			handWritten := "# Our own review rules\n\nNever approve a PR without tests.\n"
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(handWritten), 0o600))
			cfg, err := config.LoadConfig(context.Background(), base)
			require.NoError(t, err)
			gen := NewGenerator(cfg)

			// Act
			require.NoError(t, gen.Generate(""))
			first := readCheckOutput(t, base, rel)
			require.NoError(t, gen.Generate(""))
			second := readCheckOutput(t, base, rel)

			// Assert: the user's text stays, the block is added once and is stable.
			assert.True(t, strings.HasPrefix(first, handWritten), first)
			assert.Contains(t, first, "<!-- ai-rulez:checks:begin -->")
			assert.Contains(t, first, "Flag injection.")
			assert.Equal(t, 1, strings.Count(first, "ai-rulez:checks:begin"))
			assert.Equal(t, first, second, "regenerating changes nothing")

			// Act: clean takes back only the block.
			_, err = gen.Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, handWritten, readCheckOutput(t, base, rel), "clean gives the hand-written file back")
		})
	}
}

func TestChecks_UserTextAroundTheBlockIsKeptAcrossRegeneration(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"kilo"}, map[string]string{"checks/security.md": securityCheck}, "")
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate(""))
	path := filepath.Join(base, "REVIEW.md")
	generated := readCheckOutput(t, base, "REVIEW.md")
	edited := "# Added later\n\n" + generated + "\n## Footer\n\nMy footer.\n"
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "checks", "security.md"), []byte("Rewritten.\n"), 0o600))

	// Act
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert
	got := readCheckOutput(t, base, "REVIEW.md")
	assert.True(t, strings.HasPrefix(got, "# Added later\n\n"))
	assert.True(t, strings.HasSuffix(got, "\n## Footer\n\nMy footer.\n"))
	assert.Contains(t, got, "Rewritten.")
	assert.NotContains(t, got, "Flag injection.")
}

func TestChecks_RemovingAllChecksTakesTheBlockBack(t *testing.T) {
	// Arrange: one file ai-rulez created, one hand-written.
	base := writeChecksProject(t, []string{"kilo", "qwen"}, map[string]string{"checks/security.md": securityCheck}, "")
	handWritten := "# Mine\n"
	require.NoError(t, os.WriteFile(filepath.Join(base, "REVIEW.md"), []byte(handWritten), 0o600))
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate(""))
	require.FileExists(t, filepath.Join(base, ".qwen", "review-rules.md"))

	// Act
	require.NoError(t, os.Remove(filepath.Join(base, ".ai-rulez", "checks", "security.md")))
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert
	assert.Equal(t, handWritten, readCheckOutput(t, base, "REVIEW.md"))
	assert.NoFileExists(t, filepath.Join(base, ".qwen", "review-rules.md"), "a file ai-rulez created goes with its last check")
}

func mustLoad(t *testing.T, base string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	return cfg
}

func TestChecks_CleanAfterTheChecksWereRemovedFromTheConfig(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, checkPresets, map[string]string{"checks/security.md": securityCheck}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	augmentPath := filepath.Join(base, ".augment", "code_review_guidelines.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(augmentPath), 0o755))
	require.NoError(t, os.WriteFile(gitlabPath, []byte("instructions:\n  - name: mine\n    instructions: hand\n"), 0o600))
	require.NoError(t, os.WriteFile(augmentPath, []byte("areas:\n  mine:\n    description: d\n    globs: [\"**\"]\n    rules: []\n"), 0o600))
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))
	require.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "security:")
	require.NoError(t, os.Remove(filepath.Join(base, ".ai-rulez", "checks", "security.md")))

	// Act: clean runs against a config that no longer has the check.
	_, err := NewGenerator(mustLoad(t, base)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	for _, preset := range []string{"cursor", "kilo", "qwen", "factory", "amp", "rovodev"} {
		assert.NoFileExists(t, filepath.Join(base, filepath.FromSlash(checkOutputs[preset])), preset)
	}
	assert.Contains(t, readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml"), "name: mine")
	assert.NotContains(t, readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml"), "security")
	assert.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "mine:")
	assert.NotContains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "security")
}

func TestChecks_TargetFilteredChecksFollowTheirTargetsOnRegenerateAndClean(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"kilo", "qwen", "augment"}, map[string]string{
		"checks/security.md": "---\ntargets: [kilo, augment]\n---\n\nOnly some.\n",
	}, "")
	gen := NewGenerator(mustLoad(t, base))
	require.NoError(t, gen.Generate(""))
	require.FileExists(t, filepath.Join(base, "REVIEW.md"))
	require.NoFileExists(t, filepath.Join(base, ".qwen", "review-rules.md"))

	// Act: retarget the check from kilo and augment to qwen.
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "checks", "security.md"),
		[]byte("---\ntargets: [qwen]\n---\n\nOnly some.\n"), 0o600))
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert
	assert.NoFileExists(t, filepath.Join(base, "REVIEW.md"), "kilo no longer gets the check")
	assert.NoFileExists(t, filepath.Join(base, ".augment", "code_review_guidelines.yaml"))
	assert.Contains(t, readCheckOutput(t, base, ".qwen/review-rules.md"), "Only some.")

	// Act: clean removes what is there now.
	_, err := NewGenerator(mustLoad(t, base)).Clean("", CleanOptions{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(base, ".qwen", "review-rules.md"))
}

func TestChecks_RemovingAPresetDropsItsStaleYAMLEntries(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"augment", "gitlab-duo"}, map[string]string{"checks/security.md": securityCheck}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	augmentPath := filepath.Join(base, ".augment", "code_review_guidelines.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(augmentPath), 0o755))
	require.NoError(t, os.WriteFile(gitlabPath, []byte("instructions:\n  - name: mine\n    instructions: hand\n"), 0o600))
	require.NoError(t, os.WriteFile(augmentPath, []byte("areas:\n  mine:\n    description: d\n    globs: [\"**\"]\n    rules: []\n"), 0o600))
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Act
	rewriteChecksConfigPresets(t, base, "kilo")
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert
	assert.NotContains(t, readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml"), "security")
	assert.Contains(t, readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml"), "name: mine")
	assert.NotContains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "security")
	assert.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "mine:")
}

func TestChecks_YAMLMerge_UnclaimedUserEntryWithTheSameNameIsKept(t *testing.T) {
	// Arrange: the user's own "security" area and group, never written by ai-rulez.
	base := writeChecksProject(t, []string{"augment", "gitlab-duo"}, map[string]string{
		"checks/security.md": securityCheck,
		"checks/perf.md":     "Perf.\n",
	}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	augmentPath := filepath.Join(base, ".augment", "code_review_guidelines.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(augmentPath), 0o755))
	require.NoError(t, os.WriteFile(gitlabPath, []byte("instructions:\n  - name: security\n    instructions: my own security rules\n"), 0o600))
	require.NoError(t, os.WriteFile(augmentPath,
		[]byte("areas:\n  security:\n    description: my area\n    globs: [\"src/**\"]\n    rules: []\n"), 0o600))
	var warnings []string
	restore := rulefiles.SetWarnSink(func(msg string, _ ...any) { warnings = append(warnings, msg) })
	defer restore()

	// Act
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert: theirs is untouched, ours of another name is added, and the skip is reported.
	gitlab := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")
	assert.Contains(t, gitlab, "my own security rules")
	assert.NotContains(t, gitlab, "Flag injection.")
	assert.Equal(t, 1, strings.Count(gitlab, "name: security"))
	assert.Contains(t, gitlab, "name: perf")
	augment := readCheckOutput(t, base, ".augment/code_review_guidelines.yaml")
	assert.Contains(t, augment, "my area")
	assert.Contains(t, augment, "src/**")
	assert.NotContains(t, augment, "Flag injection.")
	assert.Contains(t, augment, "perf:")
	joined := strings.Join(warnings, "\n")
	assert.Contains(t, joined, "security")

	// A second run keeps it that way: the user's entry never became ours.
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))
	assert.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "my area")

	// And clean does not delete it.
	_, err := NewGenerator(mustLoad(t, base)).Clean("", CleanOptions{})
	require.NoError(t, err)
	assert.Contains(t, readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml"), "my own security rules")
	assert.Contains(t, readCheckOutput(t, base, ".augment/code_review_guidelines.yaml"), "my area")
}

func TestChecks_GitLabKeepsCommentsAndKeyOrderOfTheUsersGroups(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"gitlab-duo"}, map[string]string{"checks/security.md": securityCheck}, "")
	gitlabPath := filepath.Join(base, ".gitlab", "duo", "mr-review-instructions.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gitlabPath), 0o755))
	userDoc := "# Team review instructions\n" +
		"instructions:\n" +
		"  # keep this group first\n" +
		"  - name: Team style   # trailing note\n" +
		"    instructions: |\n" +
		"      Prefer small PRs.\n" +
		"\n" +
		"    fileFilters: [\"!**/*.md\"]   # skip docs\n" +
		"\n" +
		"  # second group: instructions before name on purpose\n" +
		"  - instructions: Keep tests fast.\n" +
		"    name: Tests\n" +
		"other: 1\n"
	require.NoError(t, os.WriteFile(gitlabPath, []byte(userDoc), 0o600))

	// Act
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))
	first := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "checks", "security.md"), []byte("Changed.\n"), 0o600))
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))
	second := readCheckOutput(t, base, ".gitlab/duo/mr-review-instructions.yaml")

	// Assert: the user's text, comments and key order survive byte for byte.
	userBody := strings.TrimSuffix(strings.SplitN(userDoc, "other: 1\n", 2)[0], "")
	assert.Contains(t, first, userBody)
	assert.Contains(t, second, userBody)
	assert.Contains(t, first, "other: 1\n")
	assert.Contains(t, second, "Changed.")
	assert.NotContains(t, second, "Flag injection.")
	assert.Equal(t, 1, strings.Count(second, "name: security"))
}

func TestChecks_YAMLMerge_AnAreaTheUserEditedAfterGenerationStaysTheirs(t *testing.T) {
	// Arrange
	base := writeChecksProject(t, []string{"augment"}, map[string]string{"checks/security.md": securityCheck}, "")
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))
	path := filepath.Join(base, ".augment", "code_review_guidelines.yaml")
	generated := readCheckOutput(t, base, ".augment/code_review_guidelines.yaml")
	edited := strings.Replace(generated, "Security issues", "Our tuned description", 1)
	require.NotEqual(t, generated, edited)
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "checks", "security.md"), []byte("Changed.\n"), 0o600))

	// Act
	require.NoError(t, NewGenerator(mustLoad(t, base)).Generate(""))

	// Assert
	got := readCheckOutput(t, base, ".augment/code_review_guidelines.yaml")
	assert.Contains(t, got, "Our tuned description")
	assert.NotContains(t, got, "Changed.")
}

func TestChecks_LocalChecksGetMachineLocalPerItemFiles(t *testing.T) {
	// Arrange: amp writes one file per check, so a local check has a file of its own.
	base := writeChecksProject(t, []string{"amp"}, map[string]string{
		"checks/shared.md":               "Shared check.\n",
		"local/checks/mine.md":           "Local check.\n",
		"local/domains/me/checks/dom.md": "Local domain check.\n",
	}, "\n[profiles]\nme = [\"me\"]\n")

	// Act
	generateChecksProject(t, base, "me")

	// Assert
	assert.FileExists(t, filepath.Join(base, ".agents", "checks", "shared.md"))
	assert.Contains(t, readCheckOutput(t, base, ".agents/checks/mine.md"), "Local check.")
	assert.Contains(t, readCheckOutput(t, base, ".agents/checks/dom.md"), "Local domain check.")
	var local struct {
		Files []string `json:"files"`
	}
	data, err := os.ReadFile(filepath.Join(base, ".ai-rulez", ".generated-manifest.local.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &local))
	assert.Contains(t, local.Files, ".agents/checks/mine.md", "the local check file is recorded as machine-local")
	assert.NotContains(t, local.Files, ".agents/checks/shared.md")
}

func TestChecks_LocalCheckCannotReplaceASharedOne(t *testing.T) {
	base := writeChecksProject(t, []string{"amp"}, map[string]string{
		"checks/security.md":       "Shared.\n",
		"local/checks/security.md": "Local.\n",
	}, "")
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)

	err = NewGenerator(cfg).Generate("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "security")
}

func TestHasDomainContent_CountsChecks(t *testing.T) {
	tree := &config.ContentTree{Domains: map[string]*config.Domain{"d": {Name: "d", Checks: []config.ContentFile{{Name: "c"}}}}}

	assert.True(t, hasDomainContent(tree))
}
