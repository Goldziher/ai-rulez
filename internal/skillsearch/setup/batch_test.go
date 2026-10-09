package setup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolved_MaxBatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		llm  string
		want int
	}{
		{"liter-llm to gemini cannot batch", "provider = \"gemini\"\n", 1},
		{"liter-llm with a prefixed gemini model cannot batch", "", 1},
		{"liter-llm to another provider batches", "provider = \"openai\"\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			model := "base"
			if tt.name == "liter-llm with a prefixed gemini model cannot batch" {
				model = "gemini/gemini-embedding-001"
			}
			get := userConfig(t, "[llm]\nembedding_model = \""+model+"\"\n"+tt.llm)
			r, err := Resolve(projectConfig(nil), Options{Getenv: get})
			require.NoError(t, err)

			// Act
			got := r.maxBatch()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
