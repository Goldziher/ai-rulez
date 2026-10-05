package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestStampSourceHashes(t *testing.T) {
	tests := []struct {
		name string
		in   config.OutputFile
		want string
	}{
		{"local-only output takes the local hash", config.OutputFile{LocalOnly: true}, "local-h"},
		{"local-only output replaces its own hash", config.OutputFile{LocalOnly: true, SourceHash: "own"}, "local-h"},
		{"shared output without a hash takes the baseline hash", config.OutputFile{}, "base-h"},
		{"shared output keeps its own hash", config.OutputFile{SourceHash: "own"}, "own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			plan := &localPlan{baselineHash: "base-h", localHash: "local-h"}
			outputs := []config.OutputFile{tt.in}

			// Act
			NewGenerator(&config.Config{}).stampSourceHashes(plan, outputs)

			// Assert
			assert.Equal(t, tt.want, outputs[0].SourceHash)
		})
	}
}
