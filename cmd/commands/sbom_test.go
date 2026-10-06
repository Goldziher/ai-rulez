package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// gitSpy puts a `git` wrapper first on PATH that records its arguments, and
// returns the log path.
func gitSpy(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + logPath + "'\nexec '" + real + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755)) //nolint:gosec // test stub
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestSBOMDoesNotTouchTheNetworkUnlessOnline(t *testing.T) {
	tests := []struct {
		name         string
		online       bool
		wantLsRemote bool
	}{
		{"offline by default", false, false},
		{"online allows ls-remote", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			logPath := gitSpy(t)
			cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"x\"\nsource = \"https://127.0.0.1:9/x/y.git\"\nref = \"main\"\n"
			sbomProject(t, cfg, nil)

			// Act
			var out, errOut bytes.Buffer
			code := runSBOM(&out, &errOut, sbomFlags{format: formatCycloneDX, online: tt.online}, false)

			// Assert
			require.Equal(t, 0, code, errOut.String())
			assert.Contains(t, out.String(), "ai-rulez:source:include:x")
			logged, _ := os.ReadFile(logPath) //nolint:errcheck // absent when git never ran
			assert.Equal(t, tt.wantLsRemote, strings.Contains(string(logged), "ls-remote"), string(logged))
		})
	}
}

const sbomBaseConfig = "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n"

// sbomProject writes a project, makes it the working directory and returns its root.
func sbomProject(t *testing.T, cfg string, files map[string]string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	conf := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(conf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(conf, "config.toml"), []byte(cfg), 0o600))
	for rel, content := range files {
		full := filepath.Join(conf, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}
	t.Chdir(root)
	return root
}

// writeLock pins the project in ai-rulez.lock.
func writeLock(t *testing.T, root string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal())
	require.NoError(t, err)
	snap, err := govview.Snapshot(cfg, "", true, "9.9.9")
	require.NoError(t, err)
	lock := &lockfile.File{}
	contentlock.Build(lock, snap)
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
}

func runSBOMWith(f sbomFlags) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = runSBOM(&o, &e, f, false)
	return code, o.String(), e.String()
}

func TestSBOMFormats(t *testing.T) {
	tests := []struct {
		name, format, wantKey string
	}{
		{"cyclonedx", "cyclonedx", `"bomFormat": "CycloneDX"`},
		{"spdx-json", "spdx-json", `"spdxVersion": "SPDX-2.3"`},
		{"spdx alias", "spdx", `"spdxVersion": "SPDX-2.3"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})

			// Act
			code, out, errOut := runSBOMWith(sbomFlags{format: tt.format})

			// Assert
			require.Equal(t, 0, code, errOut)
			assert.Contains(t, out, tt.wantKey)
		})
	}
}

func TestSBOMRejectsBadFlags(t *testing.T) {
	tests := []struct {
		name string
		f    sbomFlags
	}{
		{"unknown format", sbomFlags{format: "xml"}},
		{"unknown files mode", sbomFlags{format: "cyclonedx", files: "some"}},
		{"check needs output", sbomFlags{format: "cyclonedx", check: true}},
		{"unknown profile", sbomFlags{format: "cyclonedx", profile: "nope"}},
		{"unknown role", sbomFlags{format: "cyclonedx", role: "nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sbomProject(t, sbomBaseConfig, nil)

			// Act
			code, out, _ := runSBOMWith(tt.f)

			// Assert
			assert.Equal(t, 1, code)
			assert.Empty(t, out)
		})
	}
}

func TestSBOMRequireLock(t *testing.T) {
	tests := []struct {
		name     string
		lock     bool
		edit     bool
		wantCode int
	}{
		{"no lock", false, false, exitDrift},
		{"lock in sync", true, false, 0},
		{"lock stale", true, true, exitDrift},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})
			if tt.lock {
				writeLock(t, root)
			}
			if tt.edit {
				require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "r.md"), []byte("# changed\n"), 0o600))
			}

			// Act
			code, out, errOut := runSBOMWith(sbomFlags{format: "cyclonedx", requireLock: true})

			// Assert
			assert.Equal(t, tt.wantCode, code, errOut)
			if tt.wantCode == 0 {
				assert.Contains(t, out, "CycloneDX")
				return
			}
			assert.Empty(t, out, "nothing is written when the gate fails")
			assert.Contains(t, errOut, "AR752")
		})
	}
}

func TestSBOMStrictPins(t *testing.T) {
	tests := []struct {
		name               string
		servers            string
		wantCode           int
		wantCode2, wantNot string
	}{
		{"floating npm", "command = \"npx\"\nargs = [\"-y\", \"srv@latest\"]\n", exitDrift, "AR750", ""},
		{"pinned npm", "command = \"npx\"\nargs = [\"-y\", \"srv@1.2.3\"]\n", 0, "", "AR75"},
		{"unknown launcher", "command = \"/opt/srv\"\n", exitDrift, "AR751", ""},
		{"declared and pinned", "command = \"/opt/srv\"\npackage = \"pkg:npm/srv@1.2.3\"\n", 0, "", "AR75"},
		{"declared floating", "command = \"/opt/srv\"\npackage = \"pkg:npm/srv\"\n", exitDrift, "AR750", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sbomProject(t, sbomBaseConfig+"\n[[mcp_servers]]\nname = \"srv\"\n"+tt.servers, nil)

			// Act
			code, out, errOut := runSBOMWith(sbomFlags{format: "cyclonedx", strictPins: true})

			// Assert
			assert.Equal(t, tt.wantCode, code, errOut)
			if tt.wantCode == 0 {
				assert.NotEmpty(t, out)
				assert.NotContains(t, errOut, "AR75")
				return
			}
			assert.Empty(t, out)
			assert.Contains(t, errOut, tt.wantCode2)
		})
	}
}

func TestSBOMCheck(t *testing.T) {
	// Arrange
	root := sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})
	committed := filepath.Join(root, "sbom.cdx.json")
	check := sbomFlags{format: "cyclonedx", output: committed, check: true}

	// Act and assert, step by step: no file, written, in sync, drifted.
	code, _, errOut := runSBOMWith(check)
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, errOut, "AR753")
	assert.Contains(t, errOut, "no committed SBOM")

	code, _, errOut = runSBOMWith(sbomFlags{format: "cyclonedx", output: committed})
	require.Equal(t, 0, code, errOut)

	code, out, errOut := runSBOMWith(check)
	assert.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "up to date")

	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "r2.md"), []byte("# R2\n"), 0o600))
	code, _, errOut = runSBOMWith(check)
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, errOut, "AR753")
	assert.Contains(t, errOut, "added ai-rulez:item:rule::r2")
	onDisk, err := os.ReadFile(committed)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), "r2", "--check never rewrites the committed file")
}

func TestSBOMWritesOutputFile(t *testing.T) {
	// Arrange
	root := sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})
	target := filepath.Join(root, "out.spdx.json")

	// Act
	code, out, errOut := runSBOMWith(sbomFlags{format: "spdx-json", output: target})

	// Assert
	require.Equal(t, 0, code, errOut)
	assert.Empty(t, out)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "SPDX-2.3", doc["spdxVersion"])
}

func TestDocumentTime(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 600, time.UTC)
	tests := []struct {
		name    string
		flag    string
		set     bool
		env     string
		want    time.Time
		wantErr bool
	}{
		{"nothing", "", false, "", time.Time{}, false},
		{"now", "now", true, "", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), false},
		{"rfc3339", "2026-05-06T07:08:09+02:00", true, "", time.Date(2026, 5, 6, 5, 8, 9, 0, time.UTC), false},
		{"epoch", "", false, "1700000000", time.Unix(1700000000, 0).UTC(), false},
		{"flag wins over epoch", "now", true, "1700000000", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), false},
		{"bad flag", "yesterday", true, "", time.Time{}, true},
		{"bad epoch", "", false, "soon", time.Time{}, true},
		{"negative epoch", "", false, "-1", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := documentTime(tt.flag, tt.set, tt.env, now)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "%v != %v", tt.want, got)
		})
	}
}

func TestSBOMTimestampReachesBothFormats(t *testing.T) {
	// Arrange
	sbomProject(t, sbomBaseConfig, nil)
	var out, errOut bytes.Buffer

	// Act
	code := runSBOM(&out, &errOut, sbomFlags{format: "spdx-json", timestamp: "2026-05-06T07:08:09Z"}, true)

	// Assert
	require.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), `"created": "2026-05-06T07:08:09Z"`)
}
