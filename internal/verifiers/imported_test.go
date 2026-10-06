package verifiers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const (
	importedRegex = "[[verifiers]]\nid = \"imp\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.forbid]\nregex = \"DROP\"\n"
	importedCmd   = "[[verifiers]]\nid = \"impcmd\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.command]\nargv = [\"make\"]\n"
)

func importProject(t *testing.T, include, toml string) *config.Config {
	t.Helper()
	cfg := specProject(t, map[string]string{"a.sql": "DROP TABLE t;\n"}, "")
	cfg.Includes = []config.IncludeConfig{{Name: include, Source: "https://example.com/org/shared.git"}}
	cfg.Content.ImportedVerifiers = []config.ImportedVerifierFile{{Include: include, Name: "shared.toml", Data: toml}}
	return cfg
}

func pinInclude(t *testing.T, cfg *config.Config, f lockfile.File) {
	t.Helper()
	f.Version = lockfile.Version
	require.NoError(t, lockfile.Save(cfg.ConfigDir, &f))
}

func TestLoadSpecs_ImportedVerifier(t *testing.T) {
	// Arrange
	cfg := importProject(t, "shared", importedRegex)

	// Act
	specs, problems := LoadSpecs(cfg)
	rep := Run(context.Background(), cfg, Options{})

	// Assert
	require.Empty(t, problems)
	require.Len(t, specs, 1)
	assert.Equal(t, "shared", specs[0].origin)
	assert.Equal(t, "include:shared/verifiers/shared.toml", specs[0].Source())
	require.Len(t, rep.Results, 1)
	assert.Equal(t, StatusFail, rep.Results[0].Status, "an imported non-executing verifier runs like a local one")
}

func TestLoadSpecs_ImportedCommandIsRefusedUnlessTrustedAndPinned(t *testing.T) {
	remotePin := lockfile.File{Include: []lockfile.Entry{{Name: "shared", Source: "https://example.com/org/shared.git", Commit: "abc", Digest: "sha256:00"}}}
	localPin := lockfile.File{Item: []lockfile.Item{{Kind: "local-include", ID: "shared", Digest: "sha256:00"}}}
	tests := []struct {
		name     string
		settings *config.VerifiersSettings
		lock     *lockfile.File
		want     string // problem substring; "" means it loads
	}{
		{"default refuses", nil, nil, "may not: list the include in [verifiers_settings] trust_exec_from"},
		{"trusted but unpinned", &config.VerifiersSettings{TrustExecFrom: []string{"shared"}}, nil, "not pinned in ai-rulez.lock"},
		{"trusted for another include", &config.VerifiersSettings{TrustExecFrom: []string{"other"}}, &remotePin, "may not"},
		{"trusted and pinned remote", &config.VerifiersSettings{TrustExecFrom: []string{"shared"}}, &remotePin, ""},
		{"trusted and pinned local", &config.VerifiersSettings{TrustExecFrom: []string{"shared"}}, &localPin, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := importProject(t, "shared", importedCmd)
			cfg.VerifiersSettings = tt.settings
			if tt.lock != nil {
				pinInclude(t, cfg, *tt.lock)
			}

			// Act
			specs, problems := LoadSpecs(cfg)

			// Assert
			if tt.want == "" {
				assert.Empty(t, problems)
				assert.Len(t, specs, 1)
				return
			}
			assert.Empty(t, specs)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, tt.want)
			assert.Equal(t, "include:shared/verifiers/shared.toml", problems[0].File)
		})
	}
}

func TestRun_ImportedCommandNeverStartsAProcessWhenRefused(t *testing.T) {
	cfg := importProject(t, "shared", importedCmd)
	fake := &runner.Fake{}

	rep := Run(context.Background(), cfg, Options{AllowExec: true, Runner: fake})

	assert.Empty(t, fake.Calls(), "--allow-exec does not lift the import restriction")
	require.Len(t, rep.Results, 1)
	assert.Equal(t, StatusError, rep.Results[0].Status)
	assert.Equal(t, CodeVerifierInvalid, rep.Results[0].Code)
}

func TestRun_TrustedPinnedImportRunsItsCommand(t *testing.T) {
	cfg := importProject(t, "shared", importedCmd)
	cfg.VerifiersSettings = &config.VerifiersSettings{TrustExecFrom: []string{"shared"}}
	pinInclude(t, cfg, lockfile.File{Include: []lockfile.Entry{{Name: "shared", Source: "https://example.com/org/shared.git", Commit: "abc", Digest: "sha256:00"}}})
	fake := &runner.Fake{}

	rep := Run(context.Background(), cfg, Options{AllowExec: true, Runner: fake})

	require.Len(t, fake.Calls(), 1)
	assert.Equal(t, StatusPass, rep.Results[0].Status)
}

func TestLoadSpecs_ImportedCombinatorsAreScannedForCommands(t *testing.T) {
	toml := "[[verifiers]]\nid = \"nested\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n" +
		"[[verifiers.require.all]]\n[verifiers.require.all.regex]\nregex = \"x\"\n" +
		"[[verifiers.require.all]]\n[verifiers.require.all.not.command]\nargv = [\"make\"]\n"
	cfg := importProject(t, "shared", toml)

	specs, problems := LoadSpecs(cfg)

	assert.Empty(t, specs)
	require.Len(t, problems, 1, "a command hidden under all/not is still a command")
}

func TestLoadSpecs_ImportedDuplicateIDAndBadFile(t *testing.T) {
	cfg := importProject(t, "shared", importedRegex)
	cfg.Verifiers = []config.VerifierConfig{{Name: "imp", Type: "file_exists", Path: "a"}}
	cfg.Content.ImportedVerifiers = append(cfg.Content.ImportedVerifiers, config.ImportedVerifierFile{Include: "shared", Name: "bad.toml", Data: "[[verifiers]]\nbogus = 1\n"})

	specs, problems := LoadSpecs(cfg)

	assert.Empty(t, specs)
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0].Message, "duplicate")
	assert.Contains(t, problems[1].Message, "invalid TOML")
}
