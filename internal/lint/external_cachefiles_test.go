package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpreterScript(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"scan.py", "scan.js", "tools/check.sh"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644)) //nolint:gosec // a test file
	}
	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{"python script", []string{"python3", "scan.py"}, "scan.py"},
		{"versioned python with flags", []string{"/usr/bin/python3.12", "-u", "-W", "ignore", "scan.py"}, "scan.py"},
		{"windows spelling", []string{`C:\Python\python.EXE`, "scan.py"}, "scan.py"},
		{"node script", []string{"node", "--no-warnings", "scan.js", "{stage}"}, "scan.js"},
		{"uv run", []string{"uv", "run", "scan.py"}, "scan.py"},
		{"bash script in a subdirectory", []string{"bash", "tools/check.sh"}, "tools/check.sh"},
		{"an absolute script", []string{"sh", filepath.Join(root, "tools", "check.sh")}, "tools/check.sh"},
		{"a {project} path is the project's", []string{"sh", "{project}/tools/check.sh"}, "tools/check.sh"},
		{"inline code is not a file", []string{"sh", "-c", "echo hi"}, ""},
		{"a missing script", []string{"node", "--flag", "missing.js"}, ""},
		{"not an interpreter", []string{"scanner", "scan.py"}, ""},
		{"a placeholder is not a file", []string{"bash", "{stage}"}, ""},
		{"no arguments", []string{"python3"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := interpreterScript(tt.command, root)

			// Assert
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(tt.want)), got)
		})
	}
}

func TestCacheFilesProblems(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     string
	}{
		{"relative globs", []string{"rules/*.yaml", "config/**"}, ""},
		{"empty", []string{" "}, "empty"},
		{"absolute", []string{"/etc/passwd"}, "relative"},
		{"leaves the project", []string{"../secrets/*"}, "inside the project"},
		{"leaves the project in the middle", []string{"rules/../../x"}, "inside the project"},
		{"unterminated class", []string{"rules/[a"}, "not a valid glob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			problems := cacheFilesProblems(tt.patterns)

			// Assert
			if tt.want == "" {
				assert.Empty(t, problems)
				return
			}
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0], tt.want)
		})
	}
}

// interpreterProject is a staged scanner run through a copy of the test binary
// named node: the cached result must follow the script, which is the scanner.
func interpreterProject(t *testing.T, extraKeys string) (p *scannerProject, script, logFile string) {
	t.Helper()
	node := fakeScannerExe(t, "node")
	logFile = filepath.Join(t.TempDir(), "runs.log")
	script = filepath.Join(t.TempDir(), "scan.txt")
	require.NoError(t, os.WriteFile(script, []byte("first answer\n"), 0o644)) //nolint:gosec // a test file
	command := "command = " + tomlArray(fakeScannerArgv(node, script, "--log", logFile)...) + "\n"
	p = fakeProject(t, isolationNone, command+"inputs = [\"rules\"]\n"+extraKeys)
	return p, script, logFile
}

func TestCacheKeyFollowsAScriptRunThroughAnInterpreter(t *testing.T) {
	// Arrange
	p, script, logFile := interpreterProject(t, "")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	// Act
	first := scannerFindings(p.run(opts))
	cached := scannerFindings(p.run(opts))
	require.NoError(t, os.WriteFile(script, []byte("second answer\n"), 0o644)) //nolint:gosec // a test file
	changed := scannerFindings(p.run(opts))

	// Assert
	require.Len(t, first, 1)
	require.Len(t, cached, 1)
	require.Len(t, changed, 1)
	assert.Contains(t, first[0].Message, "first answer")
	assert.Equal(t, 2, logCount(t, logFile), "the unchanged script is served from the cache, the edited one is scanned again")
	assert.Contains(t, changed[0].Message, "second answer", "an edited script must not be answered from a stale cache entry")
}

func TestCacheFilesInvalidateTheCacheWhenAnyMatchChanges(t *testing.T) {
	// Arrange: the scanner loads rules/*.yaml, which only cache_files can tell the cache about.
	logFile := filepath.Join(t.TempDir(), "runs.log")
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--log", logFile)+"inputs = [\"rules\"]\ncache_files = [\"scan-rules/*.yaml\"]\n")
	p.write("scan-rules/a.yaml", "a: 1\n")
	p.write("scan-rules/b.yaml", "b: 1\n")
	p.write("scan-rules/notes.txt", "not matched\n")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	tests := []struct {
		name     string
		mutate   func()
		wantRuns int
	}{
		{"first run scans", func() {}, 1},
		{"unchanged files are cached", func() {}, 1},
		{"a file the glob does not match changes nothing", func() { p.write("scan-rules/notes.txt", "edited\n") }, 1},
		{"an edited match is scanned again", func() { p.write("scan-rules/b.yaml", "b: 2\n") }, 2},
		{"a new match is scanned again", func() { p.write("scan-rules/c.yaml", "c: 1\n") }, 3},
		{"a removed match is scanned again", func() { require.NoError(t, os.Remove(filepath.Join(p.root, "scan-rules", "a.yaml"))) }, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tt.mutate()
			p.run(opts)

			// Assert
			assert.Equal(t, tt.wantRuns, logCount(t, logFile))
		})
	}
}

func TestCacheFilesRejectsAPatternOutsideTheProject(t *testing.T) {
	// Arrange
	p := fakeProject(t, isolationNone, fakeScannerCommand(t)+"inputs = [\"rules\"]\ncache_files = [\"../outside/*\"]\n")

	// Act
	findings := p.run(Options{})

	// Assert
	invalid := ofCode(findings, CodeScannerConfigInvalid)
	require.Len(t, invalid, 1, dump(findings))
	assert.Contains(t, invalid[0].Message, "cache_files")
	assert.Empty(t, scannerFindings(findings))
}
