package config

import "strings"

// SkillDescription extracts the explicit description from skill frontmatter.
func SkillDescription(metadata *Metadata) string {
	if metadata == nil || metadata.Extra == nil {
		return ""
	}

	return strings.TrimSpace(metadata.Extra["description"])
}

// SkillShortDescription extracts an optional short-description from skill frontmatter.
func SkillShortDescription(metadata *Metadata) string {
	if metadata == nil || metadata.Extra == nil {
		return ""
	}

	return strings.TrimSpace(metadata.Extra["short-description"])
}

// SkillDescriptionOrFallback returns an explicit description when present, otherwise falls back to the skill ID.
func SkillDescriptionOrFallback(description, skillID string) string {
	if desc := strings.TrimSpace(description); desc != "" {
		return desc
	}

	if id := strings.TrimSpace(skillID); id != "" {
		return id
	}

	return "skill"
}

// SkillID returns the canonical skill identifier from the path when available, otherwise the content name.
func SkillID(skill ContentFile) string {
	if skill.Path != "" {
		if id := strings.TrimSpace(extractSkillID(skill.Path)); id != "" && id != "." {
			return id
		}
	}

	return strings.TrimSpace(skill.Name)
}

// SkillDescriptionForContent returns the explicit skill description or falls back to the skill ID.
func SkillDescriptionForContent(skill ContentFile) string {
	return SkillDescriptionOrFallback(SkillDescription(skill.Metadata), SkillID(skill))
}
