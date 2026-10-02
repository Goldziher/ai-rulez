package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// url.Parse errors quote the whole URL, userinfo included.
func TestValidateGitURL_ParseErrorDoesNotLeakUserinfo(t *testing.T) {
	err := validateGitURL("https://user:s3cretpw@host:badport/repo.git")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cretpw")
}
