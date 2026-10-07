//go:build !windows

package runner

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
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
