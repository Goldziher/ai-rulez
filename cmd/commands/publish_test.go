package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const publishProjectConfig = `version = "4.0"
name = "acme"
presets = ["claude"]

[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["claude"]

[plugin.author]
name = "Jane"
`

func resetPublishFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		publishTo, publishDist, publishTag, publishRepo, publishFormat = "", "dist", "", "", ""
		publishDryRun, publishExecute, publishYes, publishForce, publishAllowDirty = false, false, false, false, false
		publishTemplates, publishRunner, profile = nil, nil, ""
	}
	reset()
	t.Cleanup(reset)
}

func publishGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)
	res := gitutil.New(nil).Exec(context.Background(), "", nil, full...)
	require.Equal(t, runner.StatusOK, res.Status, string(res.Stderr))
}

// publishProject is a committed, locked project with a generated plugin bundle.
func publishProject(t *testing.T) string {
	t.Helper()
	resetPublishFlags(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SOURCE_DATE_EPOCH", "")
	includes.Mode, includes.SkipFetch = includes.LockAuto, false
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), publishProjectConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Use when deploying the service to production; not for local runs.\n---\n\n# Deploy\n\nRun the pipeline.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "care.md"), "# Care\n\nBe careful.\n")
	chdir(t, root)
	require.Equal(t, 0, runRecursiveGenerate(), "generate")
	cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).GeneratePlugin(""))
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	publishGit(t, root, "init", "-q")
	publishGit(t, root, "add", "-A")
	publishGit(t, root, "commit", "-q", "-m", "init")
	return root
}

func readDist(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path) //nolint:errcheck // below dir
		data, rerr := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = string(data)
		return rerr
	}))
	return out
}

func runPublishCapture(t *testing.T) (string, error) {
	t.Helper()
	var out bytes.Buffer
	var err error
	_, _ = capture(t, func() { err = runPublish(context.Background(), &out) })
	return out.String(), err
}

func requirePublishError(t *testing.T, err error, code string, exit int) {
	t.Helper()
	var pe *publish.Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, code, pe.Code, pe.Error())
	assert.Equal(t, exit, pe.Exit)
}

func TestPublish_WritesAVerifiableDist(t *testing.T) {
	// Arrange
	root := publishProject(t)

	// Act
	out, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	assert.Contains(t, out, "acme-1.4.0.tar.gz")
	for _, name := range []string{"acme-1.4.0.tar.gz", "acme-1.4.0.manifest.json", "ai-rulez.lock", "SHA256SUMS", "RELEASE_NOTES.md", "publish-plan.json"} {
		assert.Contains(t, dist, name)
	}
	var manifest publish.Manifest
	require.NoError(t, json.Unmarshal([]byte(dist["acme-1.4.0.manifest.json"]), &manifest))
	assert.Equal(t, "https://github.com/acme/skills", manifest.Source.Repo)
	assert.False(t, manifest.Source.Dirty)
	assert.Len(t, manifest.Source.Commit, 40)
	assert.Equal(t, []string{"claude"}, manifest.Runtimes)
	lockOnDisk, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "ai-rulez.lock"))
	require.NoError(t, err)
	assert.Equal(t, string(lockOnDisk), dist["ai-rulez.lock"])

	var verifyOut bytes.Buffer
	publishFormat = ""
	_, _ = capture(t, func() { err = runPublishVerify(&verifyOut, filepath.Join(root, "dist")) })
	require.NoError(t, err)
	assert.Contains(t, verifyOut.String(), "verified acme 1.4.0")
}

func TestPublish_IsByteIdenticalAcrossRunsAndUmasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("umask is not available")
	}
	// Arrange
	publishProject(t)
	outside := t.TempDir()
	run := func(umask int, dist string) map[string]string {
		old := syscall.Umask(umask)
		defer syscall.Umask(old)
		publishDist = filepath.Join(outside, dist)
		_, err := runPublishCapture(t)
		require.NoError(t, err)
		return readDist(t, publishDist)
	}

	// Act
	first := run(0o022, "dist-a")
	second := run(0o077, "dist-b")

	// Assert
	assert.Equal(t, first, second)
}

func TestPublish_SourceDateEpochSetsTheArchiveTime(t *testing.T) {
	// Arrange
	root := publishProject(t)
	t.Setenv("SOURCE_DATE_EPOCH", "1234567890")

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	f, oerr := os.Open(filepath.Join(root, "dist", "acme-1.4.0.tar.gz"))
	require.NoError(t, oerr)
	defer f.Close()
	zr, zerr := gzip.NewReader(f)
	require.NoError(t, zerr)
	hdr, terr := tar.NewReader(zr).Next()
	require.NoError(t, terr)
	assert.Equal(t, int64(1234567890), hdr.ModTime.Unix())
}

func TestPublish_InvalidSourceDateEpoch(t *testing.T) {
	publishProject(t)
	t.Setenv("SOURCE_DATE_EPOCH", "yesterday")

	_, err := runPublishCapture(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SOURCE_DATE_EPOCH")
}

func TestPublish_DryRunWritesNothing(t *testing.T) {
	// Arrange
	root := publishProject(t)
	publishDryRun, publishTo = true, publish.TargetGitHubRelease

	// Act
	out, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(root, "dist"))
	assert.Contains(t, out, "would write")
	assert.Contains(t, out, "would run   gh release create v1.4.0 --repo acme/skills --title acme 1.4.0 --notes-file RELEASE_NOTES.md --verify-tag")
}

func TestPublish_JSONFormatPrintsThePlan(t *testing.T) {
	// Arrange
	root := publishProject(t)
	publishFormat = formatJSON

	// Act
	out, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	written, rerr := os.ReadFile(filepath.Join(root, "dist", "publish-plan.json"))
	require.NoError(t, rerr)
	assert.Equal(t, string(written), out)
}

func TestPublish_TheDistDirectoryDoesNotMakeTheTreeDirty(t *testing.T) {
	publishProject(t)

	for i := 0; i < 2; i++ {
		_, err := runPublishCapture(t)
		require.NoError(t, err, "run %d", i+1)
	}
}

func TestPublish_Gates(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, root string)
		allow    bool
		wantCode string
		wantExit int
		wantMsg  string
	}{
		{"dirty tree", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, "notes.txt"), "uncommitted\n")
		}, false, publish.CodeSource, publish.ExitGate, "dirty"},
		{"stale plugin output", func(t *testing.T, root string) {
			appendFile(t, filepath.Join(root, "skills", "deploy", "SKILL.md"), "\nedited by hand\n")
			publishGit(t, root, "add", "-A")
			publishGit(t, root, "commit", "-q", "-m", "edit")
		}, false, publish.CodePreflight, publish.ExitGate, "verify --plugin failed"},
		{"source drift is caught by the strict gate", func(t *testing.T, root string) {
			appendFile(t, filepath.Join(root, ".ai-rulez", "rules", "care.md"), "\nMore.\n")
			publishGit(t, root, "add", "-A")
			publishGit(t, root, "commit", "-q", "-m", "edit")
		}, false, publish.CodePreflight, publish.ExitGate, "validate --strict reported findings"},
		{"missing lock", func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "ai-rulez.lock")))
			publishGit(t, root, "add", "-A")
			publishGit(t, root, "commit", "-q", "-m", "drop lock")
		}, false, publish.CodePreflight, publish.ExitFailed, "lock --check failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := publishProject(t)
			tt.mutate(t, root)
			publishAllowDirty = tt.allow

			// Act
			_, err := runPublishCapture(t)

			// Assert
			requirePublishError(t, err, tt.wantCode, tt.wantExit)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NoDirExists(t, filepath.Join(root, "dist"), "a failed gate writes nothing")
		})
	}
}

func TestPublish_AllowDirtyWaivesOnlyTheTree(t *testing.T) {
	// Arrange
	root := publishProject(t)
	writeFile(t, filepath.Join(root, "notes.txt"), "uncommitted\n")
	publishAllowDirty = true

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	var manifest publish.Manifest
	data, rerr := os.ReadFile(filepath.Join(root, "dist", "acme-1.4.0.manifest.json"))
	require.NoError(t, rerr)
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.True(t, manifest.Source.Dirty)
}

func TestPublish_NeedsAPluginAndAVersion(t *testing.T) {
	tests := []struct {
		name string
		edit func(string) string
		want string
	}{
		{"no plugin block", func(s string) string { return strings.SplitN(s, "[plugin]", 2)[0] }, "no [plugin] block"},
		{"no version", func(s string) string { return strings.Replace(s, "version = \"1.4.0\"\n", "", 1) }, "version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := publishProject(t)
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), tt.edit(publishProjectConfig))

			_, err := runPublishCapture(t)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSecretGate(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"clean", "# Deploy\nRun the pipeline.\n", false},
		{"aws key", "export KEY=AKIAIOSFODNN7EXAMPLE\n", true},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := secretGate([]generator.PluginFile{{Path: "hooks/run.sh", Data: []byte(tt.data)}})

			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			requirePublishError(t, err, publish.CodeSecret, publish.ExitGate)
			assert.Contains(t, err.Error(), "hooks/run.sh")
			assert.NotContains(t, err.Error(), "AKIAIOSFODNN7EXAMPLE", "the value is never printed")
		})
	}
}

func TestPublish_FlagValidation(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
		want  string
	}{
		{"unknown target", func() { publishTo = "npm" }, "unknown --to"},
		{"execute without target", func() { publishExecute, publishYes = true, true }, "needs --to"},
		{"execute without yes", func() { publishExecute, publishTo = true, "github-release" }, "needs --yes"},
		{"execute with dry-run", func() { publishExecute, publishDryRun, publishYes, publishTo = true, true, true, "github-release" }, "cannot be combined"},
		{"force without execute", func() { publishForce = true }, "--force only applies"},
		{"tag without target", func() { publishTag = "v1" }, "need --to"},
		{"bad format", func() { publishFormat = "xml" }, "format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPublishFlags(t)
			tt.setup()

			err := checkPublishFlags()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPublish_RejectsAnUnsafeTagOrRepo(t *testing.T) {
	tests := []struct {
		name      string
		tag, repo string
	}{
		{"option-looking tag", "--title", "acme/skills"},
		{"traversing tag", "v1/../x", "acme/skills"},
		{"shell-looking repo", "v1", "acme/skills;rm -rf"},
		{"url repo", "v1", "https://evil.example/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publishProject(t)
			publishTo, publishTag, publishRepo, publishDryRun = publish.TargetGitHubRelease, tt.tag, tt.repo, true

			_, err := runPublishCapture(t)

			requirePublishError(t, err, publish.CodeTarget, publish.ExitFailed)
		})
	}
}

func TestPublish_TemplatesRenderIntoEmit(t *testing.T) {
	// Arrange
	root := publishProject(t)
	tpl := filepath.Join(t.TempDir(), "catalog.json.tmpl")
	require.NoError(t, os.WriteFile(tpl, []byte(`{"id":{{json .Name}},"version":{{json .Version}},"sha":"{{.BundleDigest}}"}`), 0o600))
	publishTemplates = []string{tpl}

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	body, rerr := os.ReadFile(filepath.Join(root, "dist", "emit", "catalog.json"))
	require.NoError(t, rerr)
	assert.Contains(t, string(body), `"id":"acme"`)
	res, verr := publish.Verify(filepath.Join(root, "dist"))
	require.NoError(t, verr)
	assert.True(t, res.OK(), "%v", res.Problems)
}

// fakeGH puts a gh script first on PATH. It logs its argv and working
// directory, and answers `release view` from the "mode" file ("exists" or not).
func fakeGH(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
d=$(dirname "$0")
echo "$@" >> "$d/gh.log"
pwd >> "$d/gh.cwd"
echo "${GH_TOKEN:-unset}:${AI_RULEZ_SECRET:-unset}" >> "$d/gh.env"
case "$2" in
  view) if [ "$(cat "$d/mode")" = exists ]; then exit 0; fi; echo "release not found" >&2; exit 1;;
  *) echo "https://github.com/acme/skills/releases/tag/v1.4.0"; exit 0;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755)) //nolint:gosec // test stub
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mode"), []byte(mode), 0o600))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestPublish_ExecuteRunsGhWithTheFixedArgv(t *testing.T) {
	// Arrange
	root := publishProject(t)
	gh := fakeGH(t, "new")
	t.Setenv("GH_TOKEN", "token-from-env")
	t.Setenv("AI_RULEZ_SECRET", "must-not-reach-gh")
	publishTo, publishExecute, publishYes = publish.TargetGitHubRelease, true, true

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	logged, rerr := os.ReadFile(filepath.Join(gh, "gh.log"))
	require.NoError(t, rerr)
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "release view v1.4.0 --repo acme/skills", lines[0])
	assert.Equal(t, "release create v1.4.0 --repo acme/skills --title acme 1.4.0 --notes-file RELEASE_NOTES.md --verify-tag acme-1.4.0.tar.gz acme-1.4.0.manifest.json ai-rulez.lock SHA256SUMS", lines[1])
	cwd, _ := os.ReadFile(filepath.Join(gh, "gh.cwd"))                //nolint:errcheck // asserted below
	resolved, _ := filepath.EvalSymlinks(filepath.Join(root, "dist")) //nolint:errcheck // asserted below
	assert.Equal(t, resolved, strings.Split(strings.TrimSpace(string(cwd)), "\n")[0])
	env, _ := os.ReadFile(filepath.Join(gh, "gh.env")) //nolint:errcheck // asserted below
	assert.Equal(t, "token-from-env:unset", strings.Split(strings.TrimSpace(string(env)), "\n")[0], "gh gets its own credentials and nothing else")
}

func TestPublish_ExecuteRefusesAnExistingRelease(t *testing.T) {
	// Arrange
	root := publishProject(t)
	gh := fakeGH(t, "exists")
	publishTo, publishExecute, publishYes = publish.TargetGitHubRelease, true, true

	// Act
	_, err := runPublishCapture(t)

	// Assert
	requirePublishError(t, err, publish.CodeTarget, publish.ExitFailed)
	assert.Contains(t, err.Error(), "already exists")
	logged, _ := os.ReadFile(filepath.Join(gh, "gh.log")) //nolint:errcheck // asserted below
	assert.NotContains(t, string(logged), "release create")
	assert.FileExists(t, filepath.Join(root, "dist", "SHA256SUMS"), "the dist is built before the upload step")
}

func TestPublish_ExecuteForceReplacesAssets(t *testing.T) {
	// Arrange
	publishProject(t)
	gh := fakeGH(t, "exists")
	publishTo, publishExecute, publishYes, publishForce = publish.TargetGitHubRelease, true, true, true

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	logged, _ := os.ReadFile(filepath.Join(gh, "gh.log")) //nolint:errcheck // asserted below
	assert.Contains(t, string(logged), "release upload v1.4.0 --repo acme/skills --clobber acme-1.4.0.tar.gz")
}

func TestPublish_ExecuteWithoutGhIsAClearError(t *testing.T) {
	// Arrange
	publishProject(t)
	t.Setenv("PATH", t.TempDir()) // no gh, no git: git ran during setup
	publishTo, publishExecute, publishYes = publish.TargetGitHubRelease, true, true
	publishAllowDirty = true // git is unavailable, so the tree cannot be proven clean

	// Act
	_, err := runPublishCapture(t)

	// Assert
	requirePublishError(t, err, publish.CodeTarget, publish.ExitFailed)
	assert.Contains(t, err.Error(), "gh was not found")
}

func TestPublishVerify_ReportsATamperedDist(t *testing.T) {
	// Arrange
	root := publishProject(t)
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	appendFile(t, filepath.Join(root, "dist", "ai-rulez.lock"), "# edited\n")
	publishFormat = formatJSON

	// Act
	var out bytes.Buffer
	_, _ = capture(t, func() { err = runPublishVerify(&out, filepath.Join(root, "dist")) })

	// Assert
	requirePublishError(t, err, publish.CodeVerify, publish.ExitGate)
	var res publish.VerifyResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &res))
	require.NotEmpty(t, res.Problems)
	assert.Equal(t, "ai-rulez.lock", res.Problems[0].Path)
}

func TestPublishVerify_MissingDistIsExit1(t *testing.T) {
	resetPublishFlags(t)

	err := runPublishVerify(&bytes.Buffer{}, t.TempDir())

	requirePublishError(t, err, publish.CodeVerify, publish.ExitFailed)
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString(s)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
