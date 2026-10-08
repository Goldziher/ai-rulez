package runner

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRunLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the commands are POSIX shell")
	}
	tests := []struct {
		name       string
		script     string
		stopAt     string
		timeout    time.Duration
		wantStatus Status
		wantLines  []string
		wantFast   bool
	}{
		{name: "runs to the end", script: "echo a; echo b", wantStatus: StatusOK, wantLines: []string{"a", "b"}},
		{name: "stops the command at the line it asked for", script: "echo a; echo b; sleep 30; echo c", stopAt: "b",
			wantStatus: StatusOK, wantLines: []string{"a", "b"}, wantFast: true},
		{name: "a non-zero exit is reported", script: "echo a; exit 3", wantStatus: StatusExit, wantLines: []string{"a"}},
		{name: "a timeout kills it", script: "echo a; sleep 30", timeout: 3 * time.Second, wantStatus: StatusTimeout, wantLines: []string{"a"}, wantFast: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var lines []string
			onLine := func(line []byte) bool {
				lines = append(lines, string(line))
				return tt.stopAt != "" && string(line) == tt.stopAt
			}
			started := time.Now()

			// Act
			res := Exec{}.RunLines(context.Background(), Spec{Argv: []string{"sh", "-c", tt.script}, Env: []string{"PATH=" + "/usr/bin:/bin"}, Timeout: tt.timeout}, onLine)

			// Assert
			assert.Equal(t, tt.wantStatus, res.Status, "%v", res.Err)
			assert.Equal(t, tt.wantLines, lines)
			if tt.wantFast {
				assert.Less(t, time.Since(started), 30*time.Second, "the process group was killed, not waited for")
			}
		})
	}
}

func TestExecRunLines_UnavailableAndEmpty(t *testing.T) {
	res := Exec{}.RunLines(context.Background(), Spec{Argv: []string{"definitely-not-a-binary-xyz"}}, func([]byte) bool { return false })
	assert.Equal(t, StatusUnavailable, res.Status)

	res = Exec{}.RunLines(context.Background(), Spec{}, func([]byte) bool { return false })
	assert.Equal(t, StatusError, res.Status)
}

func TestExecRunLines_StderrIsCaptured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is POSIX shell")
	}
	res := Exec{}.RunLines(context.Background(), Spec{Argv: []string{"sh", "-c", "echo oops >&2; echo out"}, Env: []string{"PATH=/usr/bin:/bin"}}, func([]byte) bool { return false })

	require.Equal(t, StatusOK, res.Status)
	assert.Contains(t, string(res.Stderr), "oops")
}

func TestAsLineRunner(t *testing.T) {
	lr, ok := AsLineRunner(nil)
	assert.True(t, ok)
	assert.IsType(t, Exec{}, lr)

	lr, ok = AsLineRunner(Deny{})
	assert.True(t, ok)
	res := lr.RunLines(context.Background(), Spec{Argv: []string{"x"}}, func([]byte) bool { return false })
	assert.Equal(t, StatusUnavailable, res.Status, "a denying runner starts nothing")

	_, ok = AsLineRunner(Func(func(context.Context, Spec) Result { return Result{} }))
	assert.False(t, ok, "a runner that cannot stream is reported, not silently buffered")
}
