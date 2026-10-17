package approval

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestSubjectsOf_ListsMarketplacesAndPlugins(t *testing.T) {
	// Arrange
	lock := &lockfile.File{}
	lock.Set(lockfile.KindMarketplace, lockfile.Entry{Name: "acme", Digest: "sha256:m"})
	lock.Set(lockfile.KindPlugin, lockfile.Entry{Name: "tools", Digest: "sha256:p"})

	// Act
	subjects := SubjectsOf(lock, nil)

	// Assert
	refs := map[string]Subject{}
	for _, s := range subjects {
		refs[s.Ref()] = s
	}
	assert.Equal(t, "sha256:m", refs["marketplace:acme"].Digest)
	assert.Equal(t, ClassRemote, refs["marketplace:acme"].Class)
	assert.Equal(t, "sha256:p", refs["installed-plugin:tools"].Digest)
	assert.Equal(t, ClassRemote, refs["installed-plugin:tools"].Class)
}
