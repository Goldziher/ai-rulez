package config

import "testing"

func TestSkillDescriptionOrFallback(t *testing.T) {
	tests := []struct {
		name        string
		description string
		skillID     string
		want        string
	}{
		{
			name:        "uses explicit description",
			description: "Project-wide standards.",
			skillID:     "core-principles",
			want:        "Project-wide standards.",
		},
		{
			name:        "falls back to skill id when description is empty",
			description: "   ",
			skillID:     "core-principles",
			want:        "core-principles",
		},
		{
			name:        "falls back to generic label when both are empty",
			description: "",
			skillID:     "",
			want:        "skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SkillDescriptionOrFallback(tt.description, tt.skillID)
			if got != tt.want {
				t.Fatalf("SkillDescriptionOrFallback(%q, %q) = %q, want %q", tt.description, tt.skillID, got, tt.want)
			}
		})
	}
}

func TestSkillDescriptionForContent(t *testing.T) {
	t.Run("uses frontmatter description", func(t *testing.T) {
		skill := ContentFile{
			Name: "core-principles",
			Path: "/tmp/.ai-rulez/skills/core-principles/SKILL.md",
			Metadata: &MetadataV3{
				Extra: map[string]string{
					"description": "Project-wide standards.",
				},
			},
		}

		if got := SkillDescriptionForContent(skill); got != "Project-wide standards." {
			t.Fatalf("SkillDescriptionForContent() = %q, want %q", got, "Project-wide standards.")
		}
	})

	t.Run("falls back to skill path id", func(t *testing.T) {
		skill := ContentFile{
			Name: "display-name",
			Path: "/tmp/.ai-rulez/skills/core-principles/SKILL.md",
		}

		if got := SkillDescriptionForContent(skill); got != "core-principles" {
			t.Fatalf("SkillDescriptionForContent() = %q, want %q", got, "core-principles")
		}
	})
}
