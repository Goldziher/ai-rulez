package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncludeCacheLabel_IsIndependentOfTheHomeDirectory(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{"linux home", "/home/ci/.cache/ai-rulez/includes/shared-0123456789ab/modules/core/rules/a.md", "include shared:modules/core/rules/a.md", true},
		{"macos home", "/Users/dev/.cache/ai-rulez/includes/shared-0123456789ab/modules/core/rules/a.md", "include shared:modules/core/rules/a.md", true},
		{"dashed include name", "/r/.cache/ai-rulez/includes/my-rules-0123456789ab/x.md", "include my-rules:x.md", true},
		{"project file", "/work/proj/.ai-rulez/rules/a.md", "", false},
		{"builtin", "builtin://rules/a.md", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, ok := includeCacheLabel(tt.path)

			// Assert
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAssignIdentity_CachedIncludeFingerprintIgnoresTheHomeDirectory(t *testing.T) {
	// Arrange
	at := func(home string) Finding {
		return Finding{Code: CodeDuplicateCollapsed, File: home + "/.cache/ai-rulez/includes/shared-0123456789ab/rules/a.md", Line: 1, Message: "rule \"a\" is defined twice"}
	}
	a, b := []Finding{at("/home/ci")}, []Finding{at("/Users/dev")}

	// Act
	assignIdentity(a, nil, "/work")
	assignIdentity(b, nil, "/work")

	// Assert
	assert.Equal(t, a[0].meta().Fingerprint, b[0].meta().Fingerprint)
}
