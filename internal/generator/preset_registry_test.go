package generator

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
)

func TestPresetRegistry_NamesAndGeneratorsAgree(t *testing.T) {
	// Arrange
	valid := config.AllPresetNames()
	registered := make([]string, 0, len(config.PresetRegistry))
	for name := range config.PresetRegistry {
		registered = append(registered, name)
	}
	sort.Strings(registered)

	// Act + Assert: every valid name has a generator and every generator a valid name.
	assert.Equal(t, valid, registered,
		"config.builtInPresets and the registered generators drifted: add the Go constant to builtInPresets, "+
			"or check that the embedded spec name matches its file name")
}

func TestPresetRegistry_EveryEmbeddedSpecIsAValidPreset(t *testing.T) {
	// Arrange
	names, err := providers.BuiltinNames()
	require.NoError(t, err)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			// Act
			p := config.Preset{BuiltIn: name}

			// Assert
			assert.True(t, p.IsValid(), "embedded spec %q is not accepted as a built-in preset", name)
			gen, err := config.GetPresetGenerator(name)
			require.NoError(t, err)
			assert.Equal(t, name, gen.GetName())
		})
	}
}

func TestPresetRegistry_SpecRootFilesJoinTheOwnersTable(t *testing.T) {
	// Arrange
	names, err := providers.BuiltinNames()
	require.NoError(t, err)

	for _, name := range names {
		gen, err := providers.LoadBuiltin(name)
		require.NoError(t, err)
		if gen.Spec.Root == nil || gen.Spec.Root.File == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			// Act
			owners := rulefiles.RootOwners(gen.Spec.Root.File)

			// Assert
			assert.Contains(t, owners, name)
		})
	}
}

func TestGitignoreTables_IncludeSpecDerivedEntries(t *testing.T) {
	// Arrange
	hintFiles, hintDirs := providers.GitignoreHints()

	// Act
	roots := gitignoreRootFiles()
	dirs := gitignoreAssistantDirs()

	// Assert
	for _, f := range hintFiles {
		assert.Contains(t, roots, f)
	}
	for _, d := range hintDirs {
		assert.Contains(t, dirs, d)
	}
	for _, d := range generatedAssistantDirs {
		assert.Contains(t, dirs, d, "static assistant dirs must survive the merge")
	}
	assert.NotContains(t, dirs, ".github/")
}

func TestIsMCPConfigOutput_CoversSpecSidecars(t *testing.T) {
	// Arrange
	paths := providers.MCPConfigPaths()
	require.NotEmpty(t, paths)

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			// Act + Assert
			assert.True(t, isMCPConfigOutput(p))
			assert.True(t, isMCPConfigOutputIn("sub/"+p, []string{"sub"}), "a scope root holds the same layout")
			assert.False(t, isMCPConfigOutput("sub/"+p), "arbitrary nesting is not a config file of this project")
			assert.False(t, isMCPConfigOutputIn("other/"+p, []string{"sub"}))
			assert.False(t, isMCPConfigOutput("a/b/"+p))
		})
	}
	assert.False(t, isMCPConfigOutput("AGENTS.md"))
}
