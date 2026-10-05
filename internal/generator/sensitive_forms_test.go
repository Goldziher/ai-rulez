package generator

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TestOutputContainsAny_EscapedForms: a secret holding quotes, backslashes or
// angle brackets is written escaped by JSON, TOML and YAML encoders, so the raw
// substring is absent from the document yet the file is still sensitive.
func TestOutputContainsAny_EscapedForms(t *testing.T) {
	t.Parallel()

	secret := "pa\"ss\\w<o>rd&it's"
	jsonBytes, err := json.Marshal(secret)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		content string
	}{
		{"json", `{"k": ` + string(jsonBytes) + `}`},
		{"toml basic string", "k = " + strconv.Quote(secret)},
		{"yaml single quoted", "k: 'pa\"ss\\w<o>rd&it''s'"},
		{"raw", "k = " + secret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := &config.OutputFile{Content: tt.content}
			assert.True(t, outputContainsAny(o, []string{secret}))
			assert.False(t, outputContainsAny(o, []string{"unrelated-value"}))
		})
	}
}
