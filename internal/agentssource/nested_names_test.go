package agentssource_test

import (
	"os/exec"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func nestedNames(names []string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		if n != "agents" {
			out[n] = true
		}
	}
	return out
}

func TestNestedNamesAreStableWhenSlugsCollide(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory names with glob metacharacters are not valid on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	files := func(extra map[string]string) map[string]string {
		m := map[string]string{
			"a/b/AGENTS.md":    "# One\n",
			"a-b/AGENTS.md":    "# Two\n",
			"a b/AGENTS.md":    "# Three\n",
			"app[1]/AGENTS.md": "# Four\n",
			"app1/AGENTS.md":   "# Five\n",
			"svc/AGENTS.md":    "# Plain\n",
			"!!!/AGENTS.md":    "# Symbols\n",
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	hashed := regexp.MustCompile(`^agents-(a-b|app1|unnamed)-[0-9a-f]{8}$`)

	// Arrange: the same tree with and without a gitignore entry for colliding directories
	plain := nestedProject(t, files(nil))
	require.NoError(t, gitutil.CommandNoContext(plain, "init", "-q").Run())
	ignored := nestedProject(t, files(map[string]string{".gitignore": "/a-b/\n/app1/\n"}))
	require.NoError(t, gitutil.CommandNoContext(ignored, "init", "-q").Run())

	// Act
	withAll := nestedNames(names(load(t, plain).Content.Context))
	withIgnore := nestedNames(names(load(t, ignored).Content.Context))

	// Assert
	assert.Len(t, withAll, 7)
	assert.True(t, withAll["agents-svc"], "a name without a collision keeps the plain slug")
	hashes := 0
	for n := range withAll {
		if hashed.MatchString(n) {
			hashes++
		}
	}
	assert.Equal(t, 6, hashes, "every colliding or empty slug carries a hash: %v", withAll)
	for n := range withIgnore {
		assert.True(t, withAll[n], "%s is named the same with the gitignore entry", n)
	}
	assert.Len(t, withIgnore, 5, "the ignored directories are left out and nothing is renamed")
}
