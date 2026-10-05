package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TestSharedOutputConsumers_MatchSpecs keeps the participation table in
// config.sharedOutputConsumers honest against the declarative specs: the
// skills directory and root file a consumer drops must be the ones its spec
// writes, and its rules folder kind must follow from the spec's rules output.
func TestSharedOutputConsumers_MatchSpecs(t *testing.T) {
	t.Parallel()

	for _, spec := range loadBuiltinSpecs() {
		consumer, ok := config.SharedOutputConsumerFor(spec.Name)
		if !ok {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			skills := spec.Outputs["skills"]
			rules := spec.Outputs["rules"]

			// Assert: the skills directory the shared tree replaces.
			if consumer.OwnSkillsDir != "" {
				if assert.NotNil(t, skills, "OwnSkillsDir set but the spec has no skills output") {
					assert.Equal(t, consumer.OwnSkillsDir, skills.Dir)
				}
				assert.True(t, consumer.Reads(config.SharedAgentSkills), "OwnSkillsDir is moot unless .agents/skills is read")
			}
			if consumer.Reads(config.SharedAgentSkills) && skills != nil && skills.Dir != ".agents/skills" {
				assert.Equal(t, skills.Dir, consumer.OwnSkillsDir, "a skills dir other than .agents/skills must be dropped")
			}

			// Assert: the root file the shared AGENTS.md replaces.
			if consumer.OwnRootFile != "" && spec.Root != nil {
				assert.Equal(t, consumer.OwnRootFile, spec.Root.File)
			}
			if consumer.Reads(config.SharedAgentsMD) && spec.Root != nil && spec.Root.File != "AGENTS.md" {
				assert.Equal(t, spec.Root.File, consumer.OwnRootFile, "a root file other than AGENTS.md must be dropped")
			}

			// Assert: the rules folder kind.
			assert.Equal(t, wantFolderKind(rules), consumer.Folder)
		})
	}
}

func wantFolderKind(rules *OutputSpec) config.RulesFolderKind {
	switch {
	case rules == nil:
		return config.RulesFolderNone
	case rules.AlwaysFiles:
		return config.RulesFolderAlways
	case rules.InlineUnscoped:
		return config.RulesFolderScopedOnly
	case rules.Split && rules.InlineFilter == InlineFilterPathScoped:
		return config.RulesFolderScopedInInline
	case rules.Split:
		return config.RulesFolderSplitOnly
	}
	return config.RulesFolderNone
}
