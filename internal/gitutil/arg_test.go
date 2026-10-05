package gitutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckArg(t *testing.T) {
	for _, ok := range []string{"https://github.com/o/r.git", "git@github.com:o/r.git", "file:///tmp/r.git", "v1.0.0", "feature/x", ""} {
		require.NoError(t, CheckArg("url", ok), ok)
	}
	for _, bad := range []string{"--upload-pack=touch /tmp/x;@h:p", "-c", " --upload-pack=x", "a\nb", "a\x00b", "a\tb"} {
		assert.Error(t, CheckArg("url", bad), bad)
	}
}

func TestHardenedConfigBlocksExtAndEnvRestrictsProtocols(t *testing.T) {
	cfg := HardenedConfig()
	assert.Contains(t, cfg, "protocol.ext.allow=never")
	assert.Contains(t, HardenedEnv(nil), "GIT_ALLOW_PROTOCOL=file:https:ssh")
}
