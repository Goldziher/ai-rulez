package crud

import (
	"context"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// InstallSkill adds a new installed skill entry to the config
func (op *OperatorImpl) InstallSkill(ctx context.Context, req *InstallSkillRequest) error {
	if err := validateInstallSkillRequest(req); err != nil {
		return err
	}

	baseDir := filepath.Dir(op.aiRulezDir)

	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.With("base_dir", baseDir).Wrapf(err, "load config")
	}

	// Check if skill is already installed
	for i := range cfg.InstalledSkills {
		if cfg.InstalledSkills[i].Name == req.Name {
			return oops.
				With("name", req.Name).
				Hint("Choose a different name or remove the existing skill first").
				Errorf("skill '%s' is already installed", req.Name)
		}
	}

	// Validate the source is accessible
	sourceType, err := validateSkillSource(req.Source)
	if err != nil {
		return err
	}

	// Create the installed skill config entry
	skillConfig := config.InstalledSkillConfig{
		Name:   req.Name,
		Source: req.Source,
		Path:   req.Path,
		Ref:    req.Ref,
	}

	cfg.InstalledSkills = append(cfg.InstalledSkills, skillConfig)

	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.With("config_dir", op.aiRulezDir).Wrapf(err, "save config")
	}

	logger.Info("Skill installed successfully",
		"name", req.Name,
		"source", req.Source,
		"type", sourceType,
	)

	return nil
}

// UninstallSkill removes an installed skill from the config
func (op *OperatorImpl) UninstallSkill(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid skill name").
			Errorf("skill name is required")
	}

	baseDir := filepath.Dir(op.aiRulezDir)

	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.With("base_dir", baseDir).Wrapf(err, "load config")
	}

	foundIdx := -1
	for i := range cfg.InstalledSkills {
		if cfg.InstalledSkills[i].Name == name {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return oops.
			With("name", name).
			Hint("Use 'skill list' to see installed skills").
			Errorf("installed skill '%s' not found", name)
	}

	cfg.InstalledSkills = append(cfg.InstalledSkills[:foundIdx], cfg.InstalledSkills[foundIdx+1:]...)

	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.With("config_dir", op.aiRulezDir).Wrapf(err, "save config")
	}

	logger.Info("Skill uninstalled successfully", "name", name)
	return nil
}

// ListInstalledSkills returns all configured installed skills
func (op *OperatorImpl) ListInstalledSkills(ctx context.Context) ([]InstalledSkillInfo, error) {
	baseDir := filepath.Dir(op.aiRulezDir)

	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return nil, oops.With("base_dir", baseDir).Wrapf(err, "load config")
	}

	var infos []InstalledSkillInfo
	for i := range cfg.InstalledSkills {
		skill := &cfg.InstalledSkills[i]
		sourceType := sourceTypeLocal
		if isGitURL(skill.Source) {
			sourceType = sourceTypeGit
		}

		infos = append(infos, InstalledSkillInfo{
			Name:   skill.Name,
			Source: skill.Source,
			Path:   skill.GetPath(),
			Ref:    skill.Ref,
			Type:   sourceType,
		})
	}

	return infos, nil
}

// validateInstallSkillRequest validates the install skill request
func validateInstallSkillRequest(req *InstallSkillRequest) error {
	if req == nil {
		return oops.Hint("Provide a valid InstallSkillRequest").Errorf("request is nil")
	}

	if req.Name == "" {
		return oops.
			Hint("Skill name must be alphanumeric with hyphens/underscores").
			Errorf("skill name is required")
	}

	if req.Source == "" {
		return oops.
			Hint("Provide a git URL (https:// or git@) or local path").
			Errorf("skill source is required")
	}

	return nil
}

// validateSkillSource validates that the skill source is accessible
func validateSkillSource(source string) (string, error) {
	if isGitURL(source) {
		return sourceTypeGit, validateGitURL(source)
	}
	return sourceTypeLocal, nil
}
