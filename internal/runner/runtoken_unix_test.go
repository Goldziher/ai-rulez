//go:build !windows

package runner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWithRunToken(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"adds the token", []string{"A=1"}, RunTokenEnv + "=new"},
		{"appends to an outer run's token", []string{RunTokenEnv + "=outer", "A=1"}, RunTokenEnv + "=outer:new"},
		{"replaces an empty value", []string{RunTokenEnv + "=", "A=1"}, RunTokenEnv + "=new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := withRunToken(tt.env, "new")
			// Assert
			if got[len(got)-1] != tt.want || !slices.Contains(got, "A=1") || len(got) != 2 {
				t.Fatalf("withRunToken(%q) = %q, want A=1 and %q", tt.env, got, tt.want)
			}
			if !hasRunToken(got, "new") {
				t.Fatalf("hasRunToken(%q, new) = false", got)
			}
		})
	}
}

func TestHasRunToken(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want bool
	}{
		{"single token", []string{RunTokenEnv + "=abc"}, true},
		{"nested tokens", []string{RunTokenEnv + "=outer:abc:inner"}, true},
		{"prefix only", []string{RunTokenEnv + "=abcd"}, false},
		{"other variable", []string{"X_" + RunTokenEnv + "=abc", "Y=abc"}, false},
		{"none", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasRunToken(tt.env, "abc"); got != tt.want {
				t.Fatalf("hasRunToken(%q) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestRunTagsTheChildWithARunToken(t *testing.T) {
	// Arrange
	spec := Spec{Argv: []string{"/bin/sh", "-c", "printf %s \"$" + RunTokenEnv + "\""}, Env: []string{"PATH=/usr/bin:/bin"}}
	// Act
	first := Run(context.Background(), spec)
	second := Run(context.Background(), spec)
	// Assert
	a, b := string(first.Stdout), string(second.Stdout)
	if len(a) != 32 || len(b) != 32 || a == b {
		t.Fatalf("run tokens %q and %q: want two distinct 32-hex tokens", a, b)
	}
}

// TestRunKillsADetachedSystemBinary is the RV-SEC-2 replay: a double fork made
// of system binaries (perl, /bin/sh) whose middle exits at once. macOS
// withholds the environment of such binaries, so the run token alone cannot
// find the grandchild there; the marker descriptor does.
func TestRunKillsADetachedSystemBinary(t *testing.T) {
	if _, err := LookPath("perl"); err != nil {
		t.Skip("perl is needed for the setsid double fork")
	}
	tests := []struct {
		name    string
		timeout time.Duration
		hold    string
		want    Status
	}{
		{"at the timeout", time.Second, "30", StatusTimeout},
		{"after a clean exit", 10 * time.Second, "0.5", StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			marker := filepath.Join(t.TempDir(), "alive")
			spawn := "perl -e 'use POSIX; fork and exit; POSIX::setsid(); " +
				"exec(\"/bin/sh\",\"-c\",\"sleep 3; : > " + marker + "\")' & sleep " + tt.hold
			// Act
			res := Run(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", spawn}, InheritEnv: true, Timeout: tt.timeout})
			// Assert
			if res.Status != tt.want {
				t.Fatalf("status = %s (%v), want %s", res.Status, res.Err, tt.want)
			}
			time.Sleep(4 * time.Second)
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("the detached grandchild outlived the run")
			}
		})
	}
}

func TestProcessEnvReadsOwnProcess(t *testing.T) {
	// Act
	env, err := processEnv(os.Getpid())
	// Assert
	if err != nil {
		t.Skipf("process environment not readable here: %v", err)
	}
	if !slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "PATH=") }) {
		t.Fatalf("own environment has no PATH: %d entries", len(env))
	}
}

// The sweep after each run must not adopt a process another, concurrent run
// has forked but not yet exec'd: that process holds a copy of this run's marker
// read end until its exec closes it, and it is not this run's.
func TestConcurrentRunsDoNotKillEachOther(t *testing.T) {
	// Arrange
	const workers, runs = 32, 20
	errs := make(chan string, workers*runs)
	done := make(chan struct{})

	// Act
	for range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			for range runs {
				res := Run(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", "exit 0"}, Env: []string{"PATH=/usr/bin:/bin"}, Timeout: 30 * time.Second})
				if res.Status != StatusOK {
					errs <- string(res.Status) + ": " + string(res.Stderr)
				}
			}
		}()
	}
	for range workers {
		<-done
	}
	close(errs)

	// Assert
	for e := range errs {
		t.Errorf("a concurrent run did not finish cleanly: %s", e)
	}
}
