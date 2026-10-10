package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realFixtureDir returns a vendored real-world fixture under tests/fixtures,
// recorded in the fixture's README with its source repository, commit and license.
func realFixtureDir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{"..", "..", "tests", "fixtures"}, parts...)...)
	require.DirExists(t, dir, "the vendored fixture is missing; see tests/fixtures")
	return dir
}

// TestAPMPlan_RealOhMyPosh reads JanDeDobbeleer/oh-my-posh's committed apm.yml and
// apm.lock.yaml. The project is a consumer: it holds no .apm/ primitives, so the
// plan is remotes only, and this exercises the plural `targets:` list, the
// virtual-path dependencies and the commit the real lock pins them to.
func TestAPMPlan_RealOhMyPosh(t *testing.T) {
	// Arrange
	dir := realFixtureDir(t, "apm", "oh-my-posh", "in")
	fsys := os.DirFS(dir)

	// Act
	p := planOf(t, apmImporter{}, fsys, Options{})

	// Assert
	assert.Equal(t, []string{"apm.lock.yaml", "apm.yml"}, apmImporter{}.Detect(fsys))
	assert.Equal(t, []string{"claude", "copilot"}, p.Presets, "the plural targets: list")
	assert.NotNil(t, findingFor(p, StatusMapped, "apm.yml", "targets.claude"))
	assert.NotNil(t, findingFor(p, StatusMapped, "apm.yml", "targets.copilot"))
	assert.Nil(t, findingFor(p, StatusNeedsAction, "apm.lock.yaml", "dependencies.content_hash"),
		"the lock carries deployed_file_hashes, so there is nothing to act on")

	require.Len(t, p.Remotes, 10, "every dependency is a remote: nothing is on disk")
	assert.Equal(t, Remote{
		Kind: remotePackage, Origin: "apm.yml#dependencies.apm[0]",
		URL: "https://github.com/jandedobbeleer/agentic", Path: "instructions/golang.instructions.md",
		Commit: "e276d99d85e7f5951e45d524fbcfe7436ee78a37",
	}, p.Remotes[0])
	assert.Equal(t, Remote{
		Kind: remotePackage, Origin: "apm.yml#dependencies.apm[7]",
		URL: "https://github.com/jandedobbeleer/agentic", Path: "skills/writing-clearly-and-concisely",
		Commit: "e276d99d85e7f5951e45d524fbcfe7436ee78a37",
	}, p.Remotes[7])
	assert.Equal(t, Remote{
		Kind: remotePackage, Origin: "apm.yml#dependencies.apm[9]",
		URL: "https://github.com/ast-grep/agent-skill", Path: "ast-grep/skills/ast-grep",
		Commit: "affe2b9b7c608f4e354d7e83d0583ed35e845650",
	}, p.Remotes[9])
}

// TestAPMConvert_RealOhMyPoshExplainsFetch shows that the consumer project alone
// has nothing to write until its dependencies are fetched.
func TestAPMConvert_RealOhMyPoshExplainsFetch(t *testing.T) {
	dir := realFixtureDir(t, "apm", "oh-my-posh", "in")

	_, err := Convert(context.Background(), ConvertOptions{Source: dir})

	require.ErrorIs(t, err, ErrNothingToConvert)
	assert.Contains(t, err.Error(), "nothing to convert")
}

// TestTesslPlan_RealTutors reads tutors-sdk/tutors' committed tessl.json and its
// one vendored tile (.tessl/tiles/tessl/cli-setup). The tile is not listed in
// tessl.json and is still imported; the listed dependencies that are not on disk
// are reported needs-action.
func TestTesslPlan_RealTutors(t *testing.T) {
	// Arrange
	dir := realFixtureDir(t, "tessl", "tutors", "in")

	// Act
	p := planOf(t, tesslImporter{}, os.DirFS(dir), Options{})

	// Assert
	assert.Equal(t, []string{"rules/query_library_docs.md"}, itemRels(p), "the vendored tile's steering rule")
	assert.NotNil(t, findingFor(p, StatusDropped, ".tessl/RULES.md", ""), "RULES.md is generated, not source")
	assert.NotNil(t, findingFor(p, StatusDropped, ".tessl/tiles/tessl/cli-setup/tile.json", ""))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "tessl.json", "dependencies.tessl/npm-eslint"))

	var missing int
	for i := range p.Findings {
		f := p.Findings[i]
		if f.Status == StatusNeedsAction && f.Source == tesslManifest && strings.HasPrefix(f.Field, "dependencies.") {
			missing++
		}
	}
	assert.Equal(t, 20, missing, "all twenty listed tiles are absent from the vendored subset")
}

// TestConvert_GoldenRealFixtures converts the vendored real-world fixtures under
// tests/fixtures/<format>/<project>/in and compares the written tree and the
// findings with the sibling want/. Run with UPDATE_GOLDEN=1 to regenerate after an
// intended change. A fixture with no want/ (APM, which writes nothing until
// --fetch) is covered by its own plan test instead.
func TestConvert_GoldenRealFixtures(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("..", "..", "tests", "fixtures", "*", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, cases)
	for _, c := range cases {
		in, want := filepath.Join(c, "in"), filepath.Join(c, "want")
		if !isDir(in) || !isDir(want) {
			continue
		}
		t.Run(filepath.ToSlash(c), func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			copyDir(t, in, dir)

			// Act
			report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

			// Assert
			require.NoError(t, err)
			require.True(t, report.Written, "%+v", report.Security)
			got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
			delete(got, generator.ConvertRecordName) // a sidecar
			// The project name is the temp directory; pin it.
			got["config.toml"] = strings.ReplaceAll(got["config.toml"], "name = '"+filepath.Base(dir)+"'", "name = 'fixture'")
			var lines []string
			for _, f := range report.Findings {
				lines = append(lines, strings.Join([]string{string(f.Status), f.Source, f.Field, f.Target, f.Reason}, " | "))
			}
			got["report.findings"] = strings.Join(lines, "\n") + "\n"

			if os.Getenv("UPDATE_GOLDEN") != "" {
				require.NoError(t, os.RemoveAll(want))
				for p, content := range got {
					full := filepath.Join(want, filepath.FromSlash(p))
					require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
					require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
				}
			}
			assert.Equal(t, snapshot(t, want), got)
		})
	}
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
