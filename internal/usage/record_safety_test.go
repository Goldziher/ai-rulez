package usage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const skillEvent = `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"deploy"},"session_id":"s1"}`

func TestAppendLine_RefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on windows")
	}
	tests := []struct {
		name  string
		build func(t *testing.T, root string) (logPath, victim string)
	}{
		{"log file is a symlink", func(t *testing.T, root string) (string, string) {
			victim := filepath.Join(root, "victim.txt")
			require.NoError(t, os.WriteFile(victim, []byte("keep\n"), 0o600))
			local := filepath.Join(root, ".ai-rulez", "local")
			require.NoError(t, os.MkdirAll(local, 0o750))
			require.NoError(t, os.Symlink(victim, filepath.Join(local, "usage.jsonl")))
			return filepath.Join(local, "usage.jsonl"), victim
		}},
		{"local directory is a symlink", func(t *testing.T, root string) (string, string) {
			target := filepath.Join(root, "elsewhere")
			require.NoError(t, os.MkdirAll(target, 0o750))
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
			require.NoError(t, os.Symlink(target, filepath.Join(root, ".ai-rulez", "local")))
			return filepath.Join(root, ".ai-rulez", "local", "usage.jsonl"), filepath.Join(target, "usage.jsonl")
		}},
		{"dangling symlink to a new file", func(t *testing.T, root string) (string, string) {
			local := filepath.Join(root, ".ai-rulez", "local")
			require.NoError(t, os.MkdirAll(local, 0o750))
			victim := filepath.Join(root, "created-by-attacker")
			require.NoError(t, os.Symlink(victim, filepath.Join(local, "usage.jsonl")))
			return filepath.Join(local, "usage.jsonl"), victim
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			logPath, victim := tt.build(t, t.TempDir())
			before, _ := os.ReadFile(victim) //nolint:errcheck // may not exist

			// Act
			err := appendLine(logPath, []byte(`{"x":1}`+"\n"))

			// Assert
			require.ErrorContains(t, err, "refusing")
			after, _ := os.ReadFile(victim) //nolint:errcheck // may not exist
			assert.Equal(t, string(before), string(after), "the link target must be untouched")
		})
	}
}

func TestAppendLine_AppendsToRegularFileAndCreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "usage.jsonl")
	require.NoError(t, appendLine(path, []byte("one\n")))
	require.NoError(t, appendLine(path, []byte("two\n")))
	data, err := os.ReadFile(path) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\n", string(data))
}

func TestSink_TimeoutKillsTheTreeAndKeepsTheEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sink tests use sh")
	}
	old := sinkTimeout
	sinkTimeout = 300 * time.Millisecond
	t.Cleanup(func() { sinkTimeout = old })
	marker := filepath.Join(t.TempDir(), "alive")
	// A background grandchild that would write the marker if it survived the kill.
	sink := "(sleep 2; touch " + marker + ") & sleep 30"

	start := time.Now()
	entry, err := Record(strings.NewReader(skillEvent), RecordOptions{SinkCommand: sink, Now: fixedClock, SaltPath: filepath.Join(t.TempDir(), "salt")})

	require.ErrorContains(t, err, "timed out")
	require.NotNil(t, entry, "a sink error must not drop the entry")
	assert.Equal(t, "deploy", entry.ID)
	assert.Less(t, time.Since(start), 5*time.Second)
	time.Sleep(2500 * time.Millisecond)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "the whole process group must be killed")
}

func TestSink_OutputIsCappedAndFailureKeepsTheEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sink tests use sh")
	}
	logPath := filepath.Join(t.TempDir(), "usage.jsonl")
	// ~1 MiB of output, then a failure.
	sink := "head -c 1048576 /dev/zero | tr '\\0' x; exit 3"
	entry, err := Record(strings.NewReader(skillEvent), RecordOptions{LogPath: logPath, SinkCommand: sink, Now: fixedClock})

	require.Error(t, err)
	assert.Less(t, len(err.Error()), 200<<10, "the error must not carry unbounded sink output")
	require.NotNil(t, entry)
	data, readErr := os.ReadFile(logPath) //nolint:gosec // test path
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"id":"deploy"`, "the file sink still got the line")
}

func TestSink_ReceivesTheLineOnStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sink tests use sh")
	}
	out := filepath.Join(t.TempDir(), "got")
	_, err := Record(strings.NewReader(skillEvent), RecordOptions{SinkCommand: "cat > '" + out + "'", Now: fixedClock})
	require.NoError(t, err)
	data, readErr := os.ReadFile(out) //nolint:gosec // test path
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"skill":"deploy"`)
}

func TestShellQuoting(t *testing.T) {
	tests := []struct {
		name, in, want string
		fn             func(string) string
	}{
		{"plain path stays double-quoted", "/tmp/u.jsonl", `"/tmp/u.jsonl"`, shellQuote},
		{"project dir stays live", "${CLAUDE_PROJECT_DIR}/.ai-rulez/local/usage.jsonl", `"${CLAUDE_PROJECT_DIR}/.ai-rulez/local/usage.jsonl"`, shellQuote},
		{"command substitution stays literal", "/tmp/$(touch pwned)/x", `'/tmp/$(touch pwned)/x'`, shellQuote},
		{"backtick stays literal", "/tmp/`id`", "'/tmp/`id`'", shellQuote},
		{"other variable stays literal", "$HOME/x", `'$HOME/x'`, shellQuote},
		{"project dir plus substitution", "${CLAUDE_PROJECT_DIR}/$(id)", `"${CLAUDE_PROJECT_DIR}"'/$(id)'`, shellQuote},
		{"single quote escaped", "a'b$", `'a'\''b$'`, shellQuote},
		{"plain executable unchanged", "ai-rulez", "ai-rulez", ShellWord},
		{"executable path unchanged", "/opt/bin/ai-rulez", "/opt/bin/ai-rulez", ShellWord},
		{"executable with space quoted", "/opt/my tools/ai-rulez", `'/opt/my tools/ai-rulez'`, ShellWord},
		{"executable with substitution quoted", "/x/$(id)/ai-rulez", `'/x/$(id)/ai-rulez'`, ShellWord},
		{"executable with semicolon quoted", "ai-rulez; rm -rf /", `'ai-rulez; rm -rf /'`, ShellWord},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.fn(tt.in))
		})
	}
}

func TestRecordCommand_QuotesTheExecutable(t *testing.T) {
	got := recordCommand(&HookTemplateOptions{Executable: "/Users/a b/bin/ai-rulez"}, HarnessClaude)
	assert.True(t, strings.HasPrefix(got, `'/Users/a b/bin/ai-rulez' usage record`), got)
}

func TestLoadSalt_IgnoresASymlinkedSaltFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on windows")
	}
	t.Setenv(SaltEnv, "")
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("planted-salt\n"), 0o644))
	path := filepath.Join(dir, "usage.salt")
	require.NoError(t, os.Symlink(victim, path))

	salt := loadSalt(path)

	assert.NotEqual(t, "planted-salt", salt, "a symlinked salt must not be read")
	info, err := os.Stat(victim)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "the target's mode must not be changed")
}

func TestLoadSalt_DoesNotCreateAFileThroughASymlinkedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on windows")
	}
	t.Setenv(SaltEnv, "")
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	require.NoError(t, os.MkdirAll(target, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
	require.NoError(t, os.Symlink(target, filepath.Join(root, ".ai-rulez", "local")))

	salt := loadSalt(filepath.Join(root, ".ai-rulez", "local", "usage.salt"))

	assert.Empty(t, salt)
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing may be created behind the link")
}
