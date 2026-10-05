package providers

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestApplyRefSyntax_Dollar_OnlyWholeValueRefs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"whole ref becomes $NAME", "${TOKEN}", "$TOKEN"},
		{"suffix would bind to another variable", "${TOKEN}_v2", "resolved"},
		{"prefix embedded keeps the resolved value", "Bearer ${TOKEN}", "resolved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			server := &config.MCPServer{Env: map[string]string{"K": "resolved"}, EnvRefs: map[string]string{"K": tt.raw}}
			entry := map[string]any{"env": server.Env}

			// Act
			applyRefSyntax(entry, server, EnvRefSyntaxDollar)

			// Assert
			assert.Equal(t, map[string]string{"K": tt.want}, entry["env"])
		})
	}
}

func TestApplyRefSyntax_EnvPrefix_RewritesEmbeddedRefs(t *testing.T) {
	server := &config.MCPServer{Env: map[string]string{"K": "resolved"}, EnvRefs: map[string]string{"K": "a-${T}-b"}}
	entry := map[string]any{"env": server.Env}

	applyRefSyntax(entry, server, EnvRefSyntaxEnvPrefix)

	assert.Equal(t, map[string]string{"K": "a-${env:T}-b"}, entry["env"])
}

func TestBodySpecRewrite_SinglePassLongestKeyFirst(t *testing.T) {
	tests := []struct {
		name    string
		replace map[string]string
		content string
		want    string
		flag    bool
	}{
		{"longest key wins over its prefix", map[string]string{"$ARG": "A", "$ARGUMENTS": "ALL"}, "run $ARGUMENTS and $ARG", "run ALL and A", true},
		{"a replacement is never rewritten again", map[string]string{"$A": "$B", "$B": "x"}, "$A $B", "$B x", true},
		{"no match leaves content and spec alone", map[string]string{"$X": "y"}, "plain", "plain", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			spec := &BodySpec{Replace: tt.replace, ReplaceFlag: "flagged"}

			// Act
			got, fm := spec.rewrite(config.ContentFile{Content: tt.content}, nil)

			// Assert
			assert.Equal(t, tt.want, got.Content)
			assert.Equal(t, tt.flag, fm != nil)
		})
	}
}
