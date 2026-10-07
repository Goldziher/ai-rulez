package lint

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deliveryFixture(extraConfig string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":           baseConfig + extraConfig,
		".ai-rulez/skills/heavy/SKILL.md": "---\nname: heavy\ndescription: Heavy served skill for big migrations. Use when planning a schema migration.\ndelivery: served\n---\nHEAVY body.\n",
		".ai-rulez/skills/core/SKILL.md": "---\nname: core\ndescription: Core conventions kept static. Use when writing any code in this repository.\n---\n" +
			"Before a migration run the `heavy` skill.\n",
		".ai-rulez/skills/kept/SKILL.md": "---\nname: kept\ndescription: Both kinds of delivery. Use when checking that delivery both is static too.\ndelivery: both\n---\nKept body.\n",
		".ai-rulez/rules/style.md":       "# Style\n\nUse the `heavy` skill for migrations. See the `kept` skill for the rest.\n",
		".ai-rulez/skills/typo/SKILL.md": "---\nname: typo\ndescription: Has a typo in its delivery. Use when testing AR994 for a wrong value.\ndelivery: lazy\n---\nBody.\n",
	}
}

func TestStrict_ServedSkillReferencedStatically(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, deliveryFixture(""))
	gitAdd(t, root)
	fs := lintDir(t, root)

	assert.True(t, has(fs, CodeServedReferencedStatically, "skills/core/SKILL.md", 0), "a static skill naming a served skill: %v", fs)
	assert.True(t, has(fs, CodeServedReferencedStatically, "rules/style.md", 0), "a static rule naming a served skill")
	assert.False(t, has(fs, CodeServedReferencedStatically, "skills/heavy/SKILL.md", 0), "a served skill's own body is not static")
	assert.Equal(t, 2, countCode(fs, CodeServedReferencedStatically), "delivery: both is static too, so naming it is fine")
	for _, f := range fs {
		if f.Code == CodeServedReferencedStatically {
			assert.Equal(t, SeverityWarning, f.Severity)
		}
	}
}

func TestStrict_ServedSkillPreloadedByAgent(t *testing.T) {
	root := t.TempDir()
	files := deliveryFixture("")
	files[".ai-rulez/agents/pre.md"] = "---\nname: pre\ndescription: Preloads skills. Use when testing agent skills.\nskills:\n  - heavy\n  - kept\n---\nBody.\n"
	writeFiles(t, root, files)
	gitAdd(t, root)
	fs := lintDir(t, root)
	assert.True(t, has(fs, CodeServedReferencedStatically, "agents/pre.md", 5), "a preloaded served skill is not on disk: %v", fs)
	assert.Equal(t, 3, countCode(fs, CodeServedReferencedStatically), "the skill, the rule and the agent entry; delivery: both is on disk, so preloading it is fine")
	assert.Zero(t, countCode(fs, CodeFrontmatterSkill))
}

func TestStrict_DeliveryInvalid(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, deliveryFixture(""))
	gitAdd(t, root)
	fs := lintDir(t, root)
	assert.True(t, has(fs, CodeDeliveryInvalid, "skills/typo/SKILL.md", 4), "the finding points at the delivery line: %v", fs)
	assert.Equal(t, 1, countCode(fs, CodeDeliveryInvalid))
	assert.NotContains(t, messages(fs, "AR303"), "delivery", "delivery is a known frontmatter key")
}

func messages(fs []Finding, code string) string {
	out := ""
	for _, f := range fs {
		if f.Code == code {
			out += f.Message + "\n"
		}
	}
	return out
}

func TestStrict_NothingServedIsSilent(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":       baseConfig,
		".ai-rulez/skills/a/SKILL.md": "---\nname: a\ndescription: Plain static skill one. Use when testing that nothing fires.\n---\nSee skill `b`.\n",
		".ai-rulez/skills/b/SKILL.md": "---\nname: b\ndescription: Plain static skill two. Use when testing that nothing fires.\n---\nBody.\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)
	for _, c := range []string{CodeServedReferencedStatically, CodeDeliveryInvalid, CodeDeliveryStubMissing, CodeDeliveryStaticFallback} {
		assert.Zero(t, countCode(fs, c), c)
	}
}

func TestStrict_UnpinnedSkillSourceIsAR010(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml": baseConfig + "\n[[skill_sources]]\nname = \"team\"\nurl = \"https://example.com/org/skills.git\"\nref = \"main\"\n" +
			"\n[[skill_sources]]\nname = \"pinned\"\nurl = \"https://example.com/org/other.git\"\nref = \"" + "0123456789abcdef0123456789abcdef01234567" + "\"\n" +
			"\n[[skill_sources]]\nname = \"local\"\nurl = \"./vendor/skills\"\n",
		".ai-rulez/skills/a/SKILL.md": "---\nname: a\ndescription: Plain static skill. Use when testing source pinning findings.\n---\nBody.\n",
	}
	writeFiles(t, root, files)
	gitAdd(t, root)
	fs := lintDir(t, root)
	require.Equal(t, 1, countCode(fs, CodeUnpinnedRemote), "%v", fs)
	for _, f := range fs {
		if f.Code == CodeUnpinnedRemote {
			assert.Contains(t, f.Message, `skill source "team"`)
			assert.Equal(t, SeverityWarning, f.Severity)
		}
	}
}

func TestStrict_VersionSkillSourceCoveredByLockIsNotAR010(t *testing.T) {
	// Arrange: a source following a version range, with a lock that covers it.
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":       baseConfig + "\n[[skill_sources]]\nname = \"ranged\"\nurl = \"https://example.com/org/skills.git\"\nversion = \"^1.2\"\n",
		".ai-rulez/skills/a/SKILL.md": "---\nname: a\ndescription: Plain static skill. Use when testing source pinning findings.\n---\nBody.\n",
	})
	lock := &lockfile.File{}
	lock.Set(lockfile.KindSource, lockfile.Entry{
		Name: "ranged", Source: "https://example.com/org/skills.git", Ref: "^1.2",
		Commit: "0123456789abcdef0123456789abcdef01234567", Tag: "v1.2.3",
	})
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), lock))
	gitAdd(t, root)

	// Act
	fs := lintDir(t, root)

	// Assert
	assert.Zero(t, countCode(fs, CodeUnpinnedRemote), "%v", fs)
}

func TestDeliveryRulesAreRegistered(t *testing.T) {
	t.Parallel()
	want := map[string]Severity{
		CodeServedReferencedStatically: SeverityWarning, CodeDeliveryStubMissing: SeverityError, CodeDeliveryStaticFallback: SeverityWarning,
		CodeServedNoServer: SeverityWarning, CodeDeliveryInvalid: SeverityError, CodeServedLockMismatch: SeverityError,
	}
	for code, sev := range want {
		rule, ok := lookupRule(code)
		require.True(t, ok, code)
		assert.Equal(t, sev, rule.Default, code)
	}
	codes := map[string]bool{}
	for _, r := range Rules() {
		assert.False(t, codes[r.Code], "duplicate code %s", r.Code)
		codes[r.Code] = true
	}
}

func loadCfg(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

func TestWithDeliverySuppliesFindingsAtTheirSeverity(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, deliveryFixture(""))
	gitAdd(t, root)
	cfg := loadCfg(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	rep, err := Run(cfg, tree, WithDelivery([]DeliveryFinding{
		{Code: CodeDeliveryStubMissing, Message: "stub missing for claude"},
		{Code: CodeDeliveryStaticFallback, Message: "cline falls back"},
	}))
	require.NoError(t, err)
	assert.Equal(t, 1, countCode(rep.Findings, CodeDeliveryStubMissing))
	assert.Equal(t, 1, countCode(rep.Findings, CodeDeliveryStaticFallback))
	for _, f := range rep.Findings {
		if f.Code == CodeDeliveryStubMissing {
			assert.Equal(t, SeverityError, f.Severity)
			assert.Contains(t, f.File, "config.toml")
		}
	}
}
