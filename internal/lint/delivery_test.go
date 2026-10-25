package lint

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deliveryFixture(extraConfig string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":             baseConfig + extraConfig,
		".ai-rulez/skills/heavy/SKILL.md":   "---\nname: heavy\ndescription: Heavy served skill for big migrations. Use when planning a schema migration.\ndelivery: served\n---\nHEAVY body.\nSee the `sibling` skill for the client.\n",
		".ai-rulez/skills/sibling/SKILL.md": "---\nname: sibling\ndescription: Served sibling of heavy. Use when a served skill names another served skill.\ndelivery: served\n---\nSIBLING body.\n",
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

	assert.True(t, has(fs, CodeServedReferencedStatically, "rules/style.md", 0), "an always-loaded rule naming a served skill: %v", fs)
	assert.False(t, has(fs, CodeServedReferencedStatically, "skills/core/SKILL.md", 0), "a skill is loaded on demand, so its body is not always-loaded content")
	assert.False(t, has(fs, CodeServedReferencedStatically, "skills/heavy/SKILL.md", 0), "a served skill naming a served sibling is fine")
	assert.Equal(t, 1, countCode(fs, CodeServedReferencedStatically), "only the rule fires; delivery: both is static too, so naming it is fine")
	for _, f := range fs {
		if f.Code == CodeServedReferencedStatically {
			assert.Equal(t, SeverityWarning, f.Severity)
			assert.Contains(t, f.Message, "transitive fix", "the message names the transitive fix: %q", f.Message)
			assert.Contains(t, f.Message, "delivery: both")
		}
	}
}

// AR990 detects every pointer form AR301 does, plus the file-path forms, so a
// non-backticked pointer to a served skill is reported (#335).
func TestStrict_ServedSkillReferencedByPointerForms(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"backticked name then kind", "Run the `heavy` skill."},
		{"kind then backticked name", "Run skill `heavy`."},
		{"skill call", "Run Skill(heavy)."},
		{"labeled skill pointer", "Procedure/reference: skill heavy"},
		{"skills directory", "Read skills/heavy for the migration steps."},
		{"harness skill file", "Edit .claude/skills/heavy/SKILL.md by hand."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":           baseConfig,
				".ai-rulez/skills/heavy/SKILL.md": "---\nname: heavy\ndescription: Heavy served skill for big migrations. Use when planning a schema migration.\ndelivery: served\n---\nHEAVY body.\n",
				".ai-rulez/rules/use.md":          "# Use\n\n" + tt.body + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "%q must raise AR990:\n%s", tt.body, dump(fs))
		})
	}
}

// AR990 reads a skills MCP call that names a served skill the same way as a bare
// backticked name, so one form is not silent while the other fires for the same
// skill (#343).
func TestStrict_ServedSkillReferencedByMCPCall(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"quoted load_skill name", `Run load_skill "heavy" for the steps.`},
		{"load_skill name argument", `Run load_skill(name: "heavy") for the steps.`},
		{"get_skill call", "Call get_skill(heavy) first."},
		{"bare hyphenated load_skill call", "Run load_skill dev-cycle-procedure for the steps."},
		{"bare backticked name", "Run the `heavy` skill."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":                         baseConfig,
				".ai-rulez/skills/heavy/SKILL.md":               "---\nname: heavy\ndescription: Heavy served skill for big migrations. Use when planning a schema migration.\ndelivery: served\n---\nHEAVY body.\n",
				".ai-rulez/skills/dev-cycle-procedure/SKILL.md": "---\nname: dev-cycle-procedure\ndescription: The development cycle. Use when following the project workflow.\ndelivery: served\n---\nBODY.\n",
				".ai-rulez/rules/use.md":                        "# Use\n\n" + tt.body + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "%q must raise AR990:\n%s", tt.body, dump(fs))
			assert.Contains(t, messages(fs, CodeServedReferencedStatically), "load_skill", "the message names the accepted pointer form")
		})
	}
}

// A single code span `load_skill <name>` is the form AR990 itself recommends, so
// it stays quiet, while a bare backticked name or a source path on the same line
// is still reported (#377).
func TestStrict_ServedSkillLoadSkillCodeSpanStaysQuiet(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"single code span", "For heavy work, call `load_skill heavy`.", false},
		{"single code span quoted name", "For heavy work, call `load_skill \"heavy\"`.", false},
		{"two code spans", "For heavy work, call `load_skill` `heavy`.", false},
		{"bare name beside the call", "Run `load_skill heavy`, or the `heavy` skill.", true},
		{"source path beside the call", "Run `load_skill heavy`, not .ai-rulez/skills/heavy/SKILL.md.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":           baseConfig,
				".ai-rulez/skills/heavy/SKILL.md": "---\nname: heavy\ndescription: Heavy served skill for big migrations. Use when planning a schema migration.\ndelivery: served\n---\nHEAVY body.\n",
				".ai-rulez/rules/use.md":          "# Use\n\n" + tt.body + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.Equal(t, tt.want, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "%q:\n%s", tt.body, dump(fs))
		})
	}
}

// A bare tool name followed by prose is not a pointer: "load_skill returns" must
// not read "returns" as a skill (#343).
func TestReferenceUnknown_ProseAfterMCPToolNameStaysQuiet(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, precisionTree("load_skill returns the file; list_skill_resources lists the rest.", "", nil))
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.Zero(t, countCode(fs, CodeReferenceUnknown), "%s", dump(fs))
}

// A served domain whose skills name each other must not raise AR990: the whole
// domain stays served, and only content that is always loaded is reported
// (#319).
func TestStrict_ServedSiblingsDoNotFireAR990(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":                   baseConfig + "\n[domains.api]\ndelivery = \"served\"\n",
		".ai-rulez/domains/api/skills/a/SKILL.md": "---\nname: a\ndescription: First served skill of the domain. Use when testing a served sibling pair.\n---\nSee the `b` skill.\n",
		".ai-rulez/domains/api/skills/b/SKILL.md": "---\nname: b\ndescription: Second served skill of the domain. Use when testing a served sibling pair.\n---\nSee the `a` skill.\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)
	assert.False(t, has(fs, CodeServedReferencedStatically, "skills/a/SKILL.md", 0), "a served sibling does not fire: %v", fs)
	assert.False(t, has(fs, CodeServedReferencedStatically, "skills/b/SKILL.md", 0), "a served sibling does not fire")

	// The same domain's served delivery, named from an always-loaded rule, is
	// reported and the message names the setting that made the skill served.
	writeFiles(t, root, map[string]string{".ai-rulez/rules/use.md": "# Use\n\nRun the `a` skill.\n"})
	gitAdd(t, root)
	fs = lintDir(t, root)
	assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "a rule naming a served skill fires: %v", fs)
	assert.Contains(t, messages(fs, CodeServedReferencedStatically), `[domains.api] delivery = "served"`, "the message names the transitive source")
}

func TestStrict_ServedSkillPreloadedByAgent(t *testing.T) {
	root := t.TempDir()
	files := deliveryFixture("")
	files[".ai-rulez/agents/pre.md"] = "---\nname: pre\ndescription: Preloads skills. Use when testing agent skills.\nskills:\n  - heavy\n  - kept\n---\nBody.\n"
	writeFiles(t, root, files)
	gitAdd(t, root)
	fs := lintDir(t, root)
	assert.True(t, has(fs, CodeServedReferencedStatically, "agents/pre.md", 5), "a preloaded served skill is not on disk: %v", fs)
	assert.Equal(t, 2, countCode(fs, CodeServedReferencedStatically), "the rule and the agent entry; delivery: both is on disk, so preloading it is fine")
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
		CodeServedBundledFiles: SeverityWarning, CodeBothDMIPointer: SeverityWarning, CodeServedDMIUnreachable: SeverityWarning,
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

// AR9R0: a served skill that ships scripts/ or assets/ has no file on disk for a
// relative path into it to resolve. A static skill and a served skill that ships
// only references/ do not warn.
func TestStrict_ServedSkillBundledFilesWarn(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".ai-rulez/skills/runner/SKILL.md": "---\nname: runner\ndescription: Runs a bundled migration script. Use when testing served bundled files.\ndelivery: served\n---\n" +
			"Run `scripts/migrate.py`.\n",
		".ai-rulez/skills/runner/scripts/migrate.py": "print('hi')\n",
		".ai-rulez/skills/reader/SKILL.md": "---\nname: reader\ndescription: Serves only references, not scripts. Use when testing that references do not warn.\ndelivery: served\n---\n" +
			"See `references/api.md`.\n",
		".ai-rulez/skills/reader/references/api.md": "# API\n",
		".ai-rulez/skills/static-runner/SKILL.md": "---\nname: static-runner\ndescription: A static skill with a script on disk. Use when testing that static skills do not warn.\n---\n" +
			"Run `scripts/migrate.py`.\n",
		".ai-rulez/skills/static-runner/scripts/migrate.py": "print('hi')\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)

	assert.True(t, has(fs, CodeServedBundledFiles, "skills/runner/SKILL.md", 0), "a served skill with a script warns: %v", fs)
	assert.Equal(t, 1, countCode(fs, CodeServedBundledFiles), "only the served skill with a script warns: %v", fs)
	for _, f := range fs {
		if f.Code == CodeServedBundledFiles {
			assert.Equal(t, SeverityWarning, f.Severity)
			assert.Contains(t, f.Message, "scripts")
			assert.Contains(t, f.Message, "delivery: both")
		}
	}
}

// AR990 names the missing file when the pointer is a file path, not a bare name.
func TestStrict_ServedSkillReferencedByFilePath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":           baseConfig,
		".ai-rulez/skills/heavy/SKILL.md": "---\nname: heavy\ndescription: Heavy served skill for migrations. Use when testing a served file path.\ndelivery: served\n---\nHEAVY body.\n",
		".ai-rulez/rules/use.md":          "# Use\n\nEdit .claude/skills/heavy/SKILL.md by hand.\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)

	assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "a file-path pointer to a served skill warns: %v", fs)
	assert.Contains(t, messages(fs, CodeServedReferencedStatically), "the file will not be there")
}

// AR9R1: a bare pointer to a both + disable-model-invocation skill is refused by
// the Skill tool; a file path still works, and a load_skill pointer works only
// when [mcp] serve_user_invoked_skills serves the skill to the model.
func TestStrict_BothDMIPointer(t *testing.T) {
	const dmi = "---\nname: deploy\ndescription: Deploy the service. Use when shipping a release.\ndelivery: both\ndisable-model-invocation: true\n---\nDeploy body.\n"
	tests := []struct {
		name   string
		body   string
		skill  string
		config string
		want   bool
	}{
		{"backticked name then kind", "Run the `deploy` skill.", dmi, "", true},
		{"skill call", "Run Skill(deploy).", dmi, "", true},
		{"labeled pointer", "Procedure/reference: skill deploy", dmi, "", true},
		{"file path pointer", "Edit .claude/skills/deploy/SKILL.md by hand.", dmi, "", false},
		{"skills directory pointer", "Read skills/deploy for the steps.", dmi, "", false},
		{"load_skill pointer fires while the server hides user-only skills", "Run the `deploy` skill via load_skill.", dmi, "", true},
		{"find_skill pointer fires while the server hides user-only skills", "Run the `deploy` skill via find_skill.", dmi, "", true},
		{"load_skill pointer when user-invoked skills are served", "Run the `deploy` skill via load_skill.", dmi, "\n[mcp]\nserve_user_invoked_skills = true\n", false},
		{"a bare pointer still fires when user-invoked skills are served", "Run the `deploy` skill.", dmi, "\n[mcp]\nserve_user_invoked_skills = true\n", true},
		{"underscore spelling with a load_skill pointer is not hidden by the server", "Run the `deploy` skill via load_skill.", strings.Replace(dmi, "disable-model-invocation", "disable_model_invocation", 1), "", false},
		{"underscore spelling still fires", "Run the `deploy` skill.", strings.Replace(dmi, "disable-model-invocation", "disable_model_invocation", 1), "", true},
		{"a both skill without dmi does not fire", "Run the `deploy` skill.",
			"---\nname: deploy\ndescription: Deploy the service. Use when shipping a release.\ndelivery: both\n---\nDeploy body.\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":            baseConfig + tt.config,
				".ai-rulez/skills/deploy/SKILL.md": tt.skill,
				".ai-rulez/rules/use.md":           "# Use\n\n" + tt.body + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.Equal(t, tt.want, has(fs, CodeBothDMIPointer, "rules/use.md", 0), "%q:\n%s", tt.body, dump(fs))
		})
	}
}

// A served + disable-model-invocation skill named statically is AR990's case, not
// AR9R1's: it is not a both skill.
func TestStrict_ServedDMIIsNotABothPointer(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":            baseConfig,
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploy the service. Use when shipping a release.\ndelivery: served\ndisable-model-invocation: true\n---\nDeploy body.\n",
		".ai-rulez/rules/use.md":           "# Use\n\nRun the `deploy` skill.\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)

	assert.Zero(t, countCode(fs, CodeBothDMIPointer), "a served skill is AR990's case: %v", fs)
	assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "AR990 still fires: %v", fs)
}

// AR9R2: a served skill with disable-model-invocation is written nowhere (no slash
// command) and hidden from the model; opting in to serve user-only skills quiets it.
func TestStrict_ServedDMIUnreachable(t *testing.T) {
	const front = "---\nname: deploy\ndescription: Deploy the service. Use when shipping a release.\n%sdisable-model-invocation: %s\n---\nDeploy body.\n"
	tests := []struct {
		name   string
		skill  string
		config string
		want   bool
	}{
		{"served", fmt.Sprintf(front, "delivery: served\n", "true"), "", true},
		{"served, underscore spelling", strings.Replace(fmt.Sprintf(front, "delivery: served\n", "true"), "disable-model-invocation", "disable_model_invocation", 1), "", false}, // only the hyphenated key is honored
		{"served, flag off", fmt.Sprintf(front, "delivery: served\n", "false"), "", false},
		{"both is on disk for the slash command", fmt.Sprintf(front, "delivery: both\n", "true"), "", false},
		{"static is on disk for the slash command", fmt.Sprintf(front, "delivery: static\n", "true"), "", false},
		{"served by the skills default", fmt.Sprintf(front, "", "true"), "\n[skills]\ndelivery = \"served\"\n", true},
		{"served, user-invoked skills are served", fmt.Sprintf(front, "delivery: served\n", "true"), "\n[mcp]\nserve_user_invoked_skills = true\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":            baseConfig + tt.config,
				".ai-rulez/skills/deploy/SKILL.md": tt.skill,
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.Equal(t, tt.want, countCode(fs, CodeServedDMIUnreachable) > 0, dump(fs))
		})
	}
}
