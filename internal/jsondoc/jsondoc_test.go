package jsondoc_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name string
		doc  any
		want string
	}{
		{"object gets a leading version", map[string]any{"a": 1}, "{\n  \"schema_version\": 1,\n  \"a\": 1\n}\n"},
		{"empty object", struct{}{}, "{\n  \"schema_version\": 1\n}\n"},
		{"own version is kept", map[string]any{"schema_version": 7, "a": 1}, "{\n  \"a\": 1,\n  \"schema_version\": 7\n}\n"},
		{"array is wrapped", []int{1, 2}, "{\n  \"schema_version\": 1,\n  \"items\": [\n    1,\n    2\n  ]\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsondoc.Marshal(tt.doc)

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			assert.True(t, json.Valid(got))
		})
	}
}

func TestMarshalRejectsScalars(t *testing.T) {
	_, err := jsondoc.Marshal("text")

	require.Error(t, err)
}

func TestWrite(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, jsondoc.Write(&out, map[string]string{"k": "v"}))

	assert.Contains(t, out.String(), `"schema_version": 1`)
}
