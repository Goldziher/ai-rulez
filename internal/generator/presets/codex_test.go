package presets

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestCodexPresetGenerator_renderSkillFile_AlwaysIncludesDescription(t *testing.T) {
	g := &CodexPresetGenerator{}

	tests := []struct {
		name            string
		skill           config.ContentFile
		wantDescription string
	}{
		{
			name: "uses explicit description",
			skill: config.ContentFile{
				Name:    "core-workflows",
				Content: "# Core Workflows",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Handles core generation and validation workflows.",
					},
				},
			},
			wantDescription: "Handles core generation and validation workflows.",
		},
		{
			name: "falls back to short-description",
			skill: config.ContentFile{
				Name:    "release-and-distribution",
				Content: "# Release and Distribution",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"short-description": "Keeps release channels and versions aligned.",
					},
				},
			},
			wantDescription: "Keeps release channels and versions aligned.",
		},
		{
			name: "falls back to generated description",
			skill: config.ContentFile{
				Name:    "docs-and-site",
				Content: "# Docs and Site",
			},
			wantDescription: "Instructions for docs and site.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.renderSkillFile(tt.skill)
			expectedLine := `description: "` + tt.wantDescription + `"`
			if !strings.Contains(result, expectedLine) {
				t.Fatalf("expected %q in output, got:\n%s", expectedLine, result)
			}
		})
	}
}

func TestCodexPresetGenerator_renderSkillFile_PreservesShortDescriptionMetadata(t *testing.T) {
	g := &CodexPresetGenerator{}

	skill := config.ContentFile{
		Name:    "config-schema-maintainer",
		Content: "# Config Schema Maintainer",
		Metadata: &config.MetadataV3{
			Extra: map[string]string{
				"short-description": "Maintains config schema contracts.",
			},
		},
	}

	result := g.renderSkillFile(skill)
	if !strings.Contains(result, `metadata:`) {
		t.Fatalf("expected metadata block in output, got:\n%s", result)
	}
	if !strings.Contains(result, `short-description: "Maintains config schema contracts."`) {
		t.Fatalf("expected quoted short-description in output, got:\n%s", result)
	}
}
