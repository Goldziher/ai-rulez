package includes

import (
	"context"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// ResolveInstalledSkills resolves all configured installed skills and returns their content
func ResolveInstalledSkills(ctx context.Context, cfg *config.Config, accessToken string) ([]config.ContentFile, error) {
	logger.Debug("Resolving installed skills", "count", len(cfg.InstalledSkills))

	var skills []config.ContentFile

	for i := range cfg.InstalledSkills {
		skillConf := &cfg.InstalledSkills[i]

		contentFile, err := resolveInstalledSkill(ctx, cfg.BaseDir, skillConf, accessToken)
		if err != nil {
			logger.Warn("Failed to resolve installed skill", "name", skillConf.Name, "error", err)
			continue
		}

		skills = append(skills, contentFile)
		logger.Debug("Successfully resolved installed skill", "name", skillConf.Name)
	}

	return skills, nil
}

// resolveInstalledSkill resolves a single installed skill
func resolveInstalledSkill(ctx context.Context, baseDir string, skillConf *config.InstalledSkillConfig, accessToken string) (config.ContentFile, error) {
	// Check for local override first
	if skillConf.LocalOverride != "" {
		localDir := resolveSkillLocalOverride(baseDir, skillConf)
		if localDir != "" {
			logger.Info("Using local override for installed skill", "name", skillConf.Name, "path", localDir)
			return ScanInstalledSkillDir(localDir, skillConf.Name)
		}
		logger.Info("Skipping installed skill (local_override path not found)", "name", skillConf.Name, "local_override", skillConf.LocalOverride)
		return config.ContentFile{}, oops.Errorf("local override path not found for skill '%s'", skillConf.Name)
	}

	sourceType := DetectSourceType(skillConf.Source)
	skillPath := skillConf.GetPath()

	switch sourceType {
	case SourceTypeGit:
		source, err := NewSkillGitSource(skillConf.Name, skillConf.Source, skillPath, skillConf.Ref, accessToken)
		if err != nil {
			return config.ContentFile{}, oops.Wrapf(err, "failed to create git source for skill '%s'", skillConf.Name)
		}
		return source.Fetch(ctx)

	case SourceTypeLocal:
		localPath := skillConf.Source
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(baseDir, localPath)
		}
		localPath = filepath.Clean(localPath)

		// Append the skill path within the local source
		skillDir := filepath.Join(localPath, skillPath)
		if !hasSkillMarker(skillDir) {
			return config.ContentFile{}, oops.
				With("path", skillDir).
				Errorf("no SKILL.md found at local path for skill '%s'", skillConf.Name)
		}
		return ScanInstalledSkillDir(skillDir, skillConf.Name)

	default:
		return config.ContentFile{}, oops.Errorf("unknown source type for skill '%s': %s", skillConf.Name, sourceType)
	}
}

// resolveSkillLocalOverride resolves a local_override path for an installed skill.
// Returns the resolved path if it exists and contains SKILL.md, or empty string.
func resolveSkillLocalOverride(baseDir string, skillConf *config.InstalledSkillConfig) string {
	overridePath := skillConf.LocalOverride

	if !filepath.IsAbs(overridePath) {
		overridePath = filepath.Join(baseDir, overridePath)
	}
	overridePath = filepath.Clean(overridePath)

	// Append the skill path within the override
	skillPath := skillConf.GetPath()
	skillDir := filepath.Join(overridePath, skillPath)

	if hasSkillMarker(skillDir) {
		return skillDir
	}

	return ""
}
