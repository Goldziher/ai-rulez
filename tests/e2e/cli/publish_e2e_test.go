package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const publishConfig = `version = "5.0"
name = "acme"
presets = ["claude"]
gitignore = false

[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["claude"]

[plugin.author]
name = "Jane"
`

// publishableProject is a committed project with a generated plugin bundle and
// a lock: everything publish's preflight asks for.
func publishableProject(t *testing.T, env *isoEnv) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".ai-rulez/config.toml":            publishConfig,
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Use when deploying the service to production; not for local runs.\n---\n\n# Deploy\n\nRun the pipeline.\n",
		".ai-rulez/rules/care.md":          "# Care\n\nBe careful.\n",
	})
	for _, args := range [][]string{{"generate", "--yes"}, {"generate", "--plugin"}, {"lock"}} {
		res := env.run(root, args...)
		require.Equal(t, 0, res.ExitCode, "%v: %s", args, res.Stderr)
	}
	env.commitAll(root, "init")
	return root
}

// A fake gh that logs its argv and environment, and answers "release view"
// with "not found" so publish creates the release.
const fakeGHScript = `#!/bin/sh
d=$(dirname "$0")
echo "$@" >> "$d/gh.log"
echo "${GH_TOKEN:-unset}:${E2E_UNRELATED_SECRET:-unset}" >> "$d/gh.env"
case "$2" in
  view) echo "release not found" >&2; exit 1;;
  *) echo "https://github.com/acme/skills/releases/tag/v1.4.0"; exit 0;;
esac
`

// A fake npm: "config list" names the public registry, "view" says the version
// is new, "pack" writes an empty tarball (npm may leave files out) where
// --pack-destination says, and every call is logged.
const fakeNPMScript = `#!/bin/sh
d=$(dirname "$0")
echo "$@" >> "$d/npm.log"
echo "${NPM_TOKEN:-unset}:${E2E_UNRELATED_SECRET:-unset}" >> "$d/npm.env"
case "$1" in
  config) echo '{"registry":"https://registry.npmjs.org/"}';;
  view) echo "npm error code E404" >&2; echo "npm error 404 Not Found" >&2; exit 1;;
  pack)
    dest=""; prev=""; last=""
    for a in "$@"; do
      if [ "$prev" = "--pack-destination" ]; then dest="$a"; fi
      prev="$a"; last="$a"
    done
    name=$(sed -n 's/.*"name": *"\([^"]*\)".*/\1/p' "$last/package.json" | head -1 | sed 's/^@//; s#/#-#')
    ver=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$last/package.json" | head -1)
    mkdir -p "$d/empty"
    COPYFILE_DISABLE=1 tar -czf "$dest/$name-$ver.tgz" -C "$d/empty" .
    echo "$name-$ver.tgz";;
esac
exit 0
`

func readLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test file
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestPublishE2E(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		mutate   func(t *testing.T, env *isoEnv, root string)
		wantExit int
		wantDist bool
		wantGH   []string
	}{
		{name: "a dry run writes no dist and runs nothing", args: []string{"publish", "--dry-run", "--format", "json"}},
		{name: "a build writes the dist", args: []string{"publish", "--format", "json"}, wantDist: true},
		{
			name: "--execute --yes runs gh with fixed arguments", args: []string{"publish", "--to", "github-release", "--execute", "--yes", "--format", "json"},
			wantDist: true,
			wantGH: []string{
				"release view v1.4.0 --repo acme/skills",
				"release create v1.4.0 --repo acme/skills --title acme 1.4.0 --notes-file RELEASE_NOTES.md --verify-tag acme-1.4.0.tar.gz acme-1.4.0.manifest.json ai-rulez.lock SHA256SUMS",
			},
		},
		{name: "--execute without --to cannot run", args: []string{"publish", "--execute", "--yes"}, wantExit: 1},
		{
			name: "a stale lock fails the preflight gate", args: []string{"publish", "--format", "json"},
			mutate: func(t *testing.T, env *isoEnv, root string) {
				writeTree(t, root, map[string]string{".ai-rulez/rules/care.md": "# Care\n\nChanged.\n"})
				env.commitAll(root, "edit")
			},
			wantExit: 2,
		},
		{
			name: "an uncommitted tree fails the AR9N3 gate", args: []string{"publish"},
			mutate: func(t *testing.T, _ *isoEnv, root string) {
				writeTree(t, root, map[string]string{"notes.txt": "dirty\n"})
			},
			wantExit: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			env.set("GH_TOKEN", "gh-token-from-env").set("E2E_UNRELATED_SECRET", "must-not-leak")
			fakes := env.fakeTool("gh", fakeGHScript)
			env.fakeTool("npm", fakeNPMScript)
			root := publishableProject(t, env)
			if tt.mutate != nil {
				tt.mutate(t, env, root)
			}

			// Act
			res := env.run(root, tt.args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if containsArg(tt.args, "json") && tt.wantExit == 0 {
				requireJSONDoc(t, res)
			}
			_, err := os.Stat(filepath.Join(root, "dist", "SHA256SUMS"))
			assert.Equal(t, tt.wantDist, err == nil, "dist")
			assert.Equal(t, tt.wantGH, readLog(t, filepath.Join(fakes, "gh.log")))
			if tt.wantGH != nil {
				assert.Equal(t, []string{"gh-token-from-env:unset", "gh-token-from-env:unset"}, readLog(t, filepath.Join(fakes, "gh.env")),
					"gh gets its own credentials and nothing else")
			}
			assert.Nil(t, readLog(t, filepath.Join(fakes, "npm.log")), "npm is never run for another target")
		})
	}
}

func TestPublishNPME2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	env.set("NPM_TOKEN", "npm-token").set("E2E_UNRELATED_SECRET", "must-not-leak")
	fakes := env.fakeTool("npm", fakeNPMScript)
	env.fakeTool("gh", fakeGHScript)
	root := publishableProject(t, env)

	// Act
	res := env.run(root, "publish", "--to", "npm", "--npm-scope", "@acme", "--execute", "--yes", "--format", "json")

	// Assert
	require.Equal(t, 0, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	requireJSONDoc(t, res)
	calls := readLog(t, filepath.Join(fakes, "npm.log"))
	verbs := make([]string, 0, len(calls))
	for _, c := range calls {
		verbs = append(verbs, strings.Fields(c)[0])
		assert.Contains(t, c, "empty.npmrc --globalconfig", "every npm call ignores a .npmrc the repository could plant: %s", c)
	}
	assert.Equal(t, []string{"config", "view", "pack", "publish"}, verbs, "registry check, free-version check, pack, publish: %v", calls)
	assert.Contains(t, calls[1], "view @acme/acme@1.4.0 version --json")
	assert.Contains(t, calls[3], "--access restricted --ignore-scripts", "the default access is restricted and scripts never run")
	for _, line := range readLog(t, filepath.Join(fakes, "npm.env")) {
		assert.Equal(t, "npm-token:unset", line, "npm gets its own credentials and nothing else")
	}
	assert.Nil(t, readLog(t, filepath.Join(fakes, "gh.log")))
}

// TestPublishVerifyE2E: the dist a build writes verifies, and a tampered
// artifact is a gate failure (exit 2, AR9N5).
func TestPublishVerifyE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root := publishableProject(t, env)
	require.Equal(t, 0, env.run(root, "publish").ExitCode)

	// Act
	ok := env.run(root, "publish", "verify", "dist", "--format", "json")
	unsigned := env.run(root, "publish", "verify", "dist", "--require-signature")
	writeTree(t, root, map[string]string{"dist/RELEASE_NOTES.md": "tampered\n"})
	bad := env.run(root, "publish", "verify", "dist", "--format", "json")

	// Assert
	require.Equal(t, 0, ok.ExitCode, ok.Stderr)
	assert.Equal(t, []any{}, requireJSONDoc(t, ok)["problems"])
	assert.Equal(t, 2, unsigned.ExitCode, unsigned.Stdout+unsigned.Stderr)
	assert.Equal(t, 2, bad.ExitCode, bad.Stdout+bad.Stderr)
	assert.Contains(t, bad.Stdout+bad.Stderr, "RELEASE_NOTES.md")
}
