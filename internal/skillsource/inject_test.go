package skillsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArg_RejectsGitOptions(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{
		"--upload-pack=touch /tmp/x;@h:p",
		"git+--upload-pack=touch /tmp/x;@h:p",
		"https://h/r@--upload-pack=x",
		"https://h/r@-x",
	} {
		_, err := ParseArg(arg)
		assert.Error(t, err, arg)
	}
}

func TestResolve_GitOptionInURLOrRefNeverReachesGit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	for _, spec := range []Spec{
		{Name: "evil", URL: "--upload-pack=touch " + marker + ";@h:p"},
		{Name: "evil", URL: "file:///nonexistent/r.git", Ref: "--upload-pack=touch " + marker},
	} {
		_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})
		require.Error(t, err)
		_, statErr := os.Stat(marker)
		assert.True(t, os.IsNotExist(statErr), "the option was executed by git")
	}
}

func TestResolve_ExtTransportIsNotAllowed(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	// A user@host:path remote whose "host" smuggles the ext:: transport.
	spec := Spec{Name: "evil", URL: "git@ext::sh -c 'touch " + marker + "' %S:x"}
	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})
	require.Error(t, err)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr))
}
