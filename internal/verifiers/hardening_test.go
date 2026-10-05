package verifiers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runOne(t *testing.T, root string, v config.VerifierConfig) Result {
	t.Helper()
	v.Name = "v"
	rep := Run(context.Background(), &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{v}}, Options{})
	require.Len(t, rep.Results, 1)
	return rep.Results[0]
}

func TestForbid_CapsHitsAndNamesLinesIncrementally(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("TODO\n")
	}
	root := writeFiles(t, map[string]string{"a.txt": sb.String()})

	res := runOne(t, root, config.VerifierConfig{Type: "forbid", Glob: "*.txt", Pattern: "TODO"})

	assert.Equal(t, StatusFail, res.Status)
	assert.Contains(t, res.Message, "a.txt:1,")
	assert.Contains(t, res.Message, "a.txt:10 and more")
	assert.NotContains(t, res.Message, "a.txt:11")
}

func TestForbid_LineNumbersAcrossMatches(t *testing.T) {
	root := writeFiles(t, map[string]string{"a.txt": "x\nTODO\ny\ny\nTODO\n"})

	res := runOne(t, root, config.VerifierConfig{Type: "forbid", Glob: "a.txt", Pattern: "TODO"})

	assert.Contains(t, res.Message, "a.txt:2, a.txt:5")
}

func TestContentPredicates_UncheckableFilesAreNotAPass(t *testing.T) {
	big := strings.Repeat("a", maxFileBytes) + "\nTODO\n" // the hit sits past the 5 MiB read limit
	root := writeFiles(t, map[string]string{
		"big.txt":  big,
		"bin.dat":  "abc\x00TODO",
		"ok.txt":   "fine\n",
		"only.txt": "fine\n",
	})
	tests := []struct {
		name       string
		v          config.VerifierConfig
		want       Status
		wantInMsg  string
		notInMsg   string
		wantPassOf string
	}{
		{"forbid with truncated file", config.VerifierConfig{Type: "forbid", Glob: "big.txt", Pattern: "TODO"}, StatusError, "big.txt", "", ""},
		{"forbid with binary file", config.VerifierConfig{Type: "forbid", Glob: "bin.dat", Pattern: "TODO"}, StatusError, "bin.dat", "", ""},
		{"forbid exclude binary passes", config.VerifierConfig{Type: "forbid", Glob: "*", Exclude: []string{"big.txt", "bin.dat"}, Pattern: "TODO"}, StatusPass, "2 file(s)", "", ""},
		{"regex on binary", config.VerifierConfig{Type: "regex", Glob: "bin.dat", Pattern: "zzz"}, StatusError, "bin.dat", "", ""},
		{"regex matched in truncated prefix passes", config.VerifierConfig{Type: "regex", Glob: "big.txt", Pattern: "^a"}, StatusPass, "1 file(s)", "", ""},
		{"regex not found in truncated file", config.VerifierConfig{Type: "regex", Glob: "big.txt", Pattern: "TODO"}, StatusError, "big.txt", "", ""},
		{"regex fail still reported", config.VerifierConfig{Type: "regex", Glob: "ok.txt", Pattern: "zzz"}, StatusFail, "ok.txt", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runOne(t, root, tt.v)

			assert.Equal(t, tt.want, res.Status, res.Message)
			assert.Contains(t, res.Message, tt.wantInMsg)
		})
	}
}

func TestForbid_HitInReadablePartPlusSkippedFileFails(t *testing.T) {
	root := writeFiles(t, map[string]string{"a.txt": "TODO\n", "b.dat": "\x00"})

	res := runOne(t, root, config.VerifierConfig{Type: "forbid", Glob: "*", Pattern: "TODO"})

	assert.Equal(t, StatusFail, res.Status)
	assert.Contains(t, res.Message, "not fully checked")
}

func TestKeyEquals_ValueSemantics(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"p.json": `{"big":12345678901234567890,"f":1.0,"nul":null,"empty":"","dependencies":{"lodash.merge":"4.0.0"},"a":{"b":[ {"c":"x"} ]}}`,
		"c.yaml": "nul: null\nempty: ''\n1: one\nwhen: 2024-01-02T03:04:05Z\ntilde: ~\n",
		"c.toml": "when = 2024-01-02T03:04:05Z\nlocal = 2024-01-02\nn = 7\n",
	})
	tests := []struct {
		name string
		path string
		key  string
		eq   string
		want Status
		msg  string
	}{
		{"big integer exact", "p.json", "big", "12345678901234567890", StatusPass, ""},
		{"float spelling kept", "p.json", "f", "1.0", StatusPass, ""},
		{"null is not empty string", "p.json", "nul", "", StatusFail, "null"},
		{"null is not the word null", "p.json", "nul", "null", StatusFail, "null"},
		{"empty string equals empty", "p.json", "empty", "", StatusPass, ""},
		{"missing key is distinct", "p.json", "nope", "", StatusFail, "no key"},
		{"escaped dot", "p.json", `dependencies.lodash\.merge`, "4.0.0", StatusPass, ""},
		{"bracket syntax", "p.json", `dependencies["lodash.merge"]`, "4.0.0", StatusPass, ""},
		{"unescaped dot cannot reach", "p.json", `dependencies.lodash.merge`, "4.0.0", StatusFail, "no key"},
		{"bracket index", "p.json", `a.b[0].c`, "x", StatusPass, ""},
		{"yaml null", "c.yaml", "nul", "", StatusFail, "null"},
		{"yaml tilde null", "c.yaml", "tilde", "", StatusFail, "null"},
		{"yaml empty", "c.yaml", "empty", "", StatusPass, ""},
		{"yaml non-string key", "c.yaml", "1", "one", StatusPass, ""},
		{"yaml timestamp", "c.yaml", "when", "2024-01-02T03:04:05Z", StatusPass, ""},
		{"toml datetime", "c.toml", "when", "2024-01-02T03:04:05Z", StatusPass, ""},
		{"toml local date", "c.toml", "local", "2024-01-02", StatusPass, ""},
		{"toml int", "c.toml", "n", "7", StatusPass, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq := tt.eq

			res := runOne(t, root, config.VerifierConfig{Type: "key_equals", Path: tt.path, Key: tt.key, Equals: &eq})

			assert.Equal(t, tt.want, res.Status, res.Message)
			assert.Contains(t, res.Message, tt.msg)
		})
	}
}

func TestFileExists_SymlinkSemantics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	root := writeFiles(t, map[string]string{"real.txt": "x", "dir/in.txt": "y"})
	for link, target := range map[string]string{
		"ok-link":     "real.txt",
		"dangling":    "nowhere",
		"out-link":    filepath.Join(outside, "secret"),
		"out-dir":     outside,
		"in-dir-link": "dir",
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(root, link)))
	}
	tests := []struct {
		name string
		typ  string
		path string
		want Status
	}{
		{"live symlink exists", "file_exists", "ok-link", StatusPass},
		{"live symlink is not absent", "file_absent", "ok-link", StatusFail},
		{"dangling symlink does not exist", "file_exists", "dangling", StatusFail},
		{"dangling symlink counts as absent", "file_absent", "dangling", StatusPass},
		{"symlink out of the project is an error", "file_exists", "out-link", StatusError},
		{"intermediate symlink out of the project is an error", "file_exists", "out-dir/secret", StatusError},
		{"intermediate symlink probe of a missing file does not leak", "file_absent", "out-dir/missing", StatusError},
		{"intermediate symlink inside is followed", "file_exists", "in-dir-link/in.txt", StatusPass},
		{"file as directory is absent", "file_absent", "real.txt/x", StatusPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runOne(t, root, config.VerifierConfig{Type: tt.typ, Path: tt.path})

			assert.Equal(t, tt.want, res.Status, res.Message)
		})
	}
}

func TestFileExists_MissingIsNotAnErrorEvenWhenRootIsASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	real := writeFiles(t, map[string]string{"a.txt": "x"})
	link := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink(real, link))

	res := runOne(t, link, config.VerifierConfig{Type: "file_exists", Path: "a.txt"})

	assert.Equal(t, StatusPass, res.Status, res.Message)
}

func TestRun_CanceledContextStopsVerifiers(t *testing.T) {
	root := writeFiles(t, map[string]string{"a.txt": "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{
		{Name: "a", Type: "file_exists", Path: "a.txt"},
		{Name: "b", Type: "forbid", Glob: "*", Pattern: "x"},
	}}

	rep := Run(ctx, cfg, Options{})

	for _, res := range rep.Results {
		assert.Equal(t, StatusError, res.Status, res.Name)
	}
	assert.True(t, rep.CannotRun())
}

func TestGlobVerifiers_UnreadableDirectoryIsAFinding(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs permission bits that bind the current user")
	}
	root := writeFiles(t, map[string]string{"a.txt": "x", "locked/b.txt": "y", "p.txt": "x"})
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	cfg := &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{
		{Name: "count", Type: "glob_count", Glob: "*.txt", Max: intp(5)},
		{Name: "excluded", Type: "glob_count", Glob: "*.txt", Exclude: []string{"locked/**"}, Max: intp(5)},
		{Name: "exists", Type: "file_exists", Path: "a.txt"},
	}}

	rep := Run(context.Background(), cfg, Options{})

	require.Len(t, rep.Results, 3, "one unreadable directory does not abort the run")
	assert.Equal(t, StatusFail, rep.Results[0].Status)
	assert.Contains(t, rep.Results[0].Message, "locked")
	assert.Equal(t, StatusPass, rep.Results[1].Status, rep.Results[1].Message)
	assert.Equal(t, StatusPass, rep.Results[2].Status)
}

func TestMessagesAreSanitized(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file names cannot hold control characters on windows")
	}
	name := "evil\x1b[31mred\nline.txt"
	root := writeFiles(t, map[string]string{name: "TODO\n"})
	cfg := &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{
		{Name: "f", Type: "forbid", Glob: "*.txt", Pattern: "TODO"},
	}}

	rep := Run(context.Background(), cfg, Options{})

	msg := rep.Results[0].Message
	assert.NotContains(t, msg, "\x1b")
	assert.NotContains(t, msg, "\n")
	assert.Contains(t, msg, `\x1b`)
	var out bytes.Buffer
	require.NoError(t, WriteText(&out, rep))
	assert.NotContains(t, out.String(), "\x1b")
}

func TestDefaultDrift_DetectsStaleOutput(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"), 0o600))
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	drift, err := DefaultDrift(cfg, "")

	require.NoError(t, err)
	assert.NotEmpty(t, drift, "outputs were never generated, so they differ from a fresh render: %v", drift)
}
