package crud

import (
	"context"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
)

// InstallSkill adds a new installed skill entry to the config
func (op *OperatorImpl) InstallSkill(ctx context.Context, req *InstallSkillRequest) error {
	if err := validateInstallSkillRequest(req); err != nil {
		return err
	}

	if op.local {
		return op.installSkillLocal(ctx, req)
	}

	sourceType, err := validateSkillSource(op.baseDir, req.Source)
	if err != nil {
		return err
	}
	entry := req.entry()
	if err := validateInstalledSkillEntry(&entry); err != nil {
		return err
	}

	// Edited on the raw config.toml, like UninstallSkill: entries already there
	// are written back as the user spelled them (GitHub shorthand stays shorthand).
	err = op.editSharedConfig(func(cfg *config.Config) error {
		for i := range cfg.InstalledSkills {
			if cfg.InstalledSkills[i].Name == req.Name {
				return oops.
					With("name", req.Name).
					Hint("Choose a different name or remove the existing skill first").
					Errorf("skill '%s' is already installed", req.Name)
			}
		}
		cfg.InstalledSkills = append(cfg.InstalledSkills, entry)
		return nil
	})
	if err != nil {
		return err
	}

	op.logger().Info("Skill installed successfully",
		"name", req.Name,
		"source", incl.RedactURL(req.Source),
		"type", sourceType,
	)

	return nil
}

// UninstallSkill removes an installed skill from the config
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) UninstallSkill(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid skill name").
			Errorf("skill name is required")
	}

	if op.local {
		return op.uninstallSkillLocal(ctx, name)
	}

	// Edited on the raw config.toml, like RemoveInclude: a configuration the
	// loader refuses must not keep a skill from being uninstalled.
	err := op.editSharedConfig(func(cfg *config.Config) error {
		for i := range cfg.InstalledSkills {
			if cfg.InstalledSkills[i].Name == name {
				cfg.InstalledSkills = append(cfg.InstalledSkills[:i], cfg.InstalledSkills[i+1:]...)
				return nil
			}
		}
		return oops.
			With("name", name).
			Hint("Use 'skill list' to see installed skills").
			Errorf("installed skill '%s' not found", name)
	})
	if err != nil {
		return err
	}

	op.logger().Info("Skill uninstalled successfully", "name", name)
	return nil
}

// ListInstalledSkills returns all configured installed skills
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) ListInstalledSkills(ctx context.Context) ([]InstalledSkillInfo, error) {
	baseDir := op.baseDir

	cfg, err := op.load(config.WithUnresolvedIncludesTolerated(ctx), config.WithoutLocal())
	if err != nil {
		return nil, oops.With("base_dir", baseDir).Wrapf(err, "load config")
	}

	var infos []InstalledSkillInfo
	for i := range cfg.InstalledSkills {
		skill := &cfg.InstalledSkills[i]
		sourceType := skillSourceType(baseDir, skill.Source)

		infos = append(infos, InstalledSkillInfo{
			Name:   skill.Name,
			Source: skill.Source,
			Path:   skill.GetPath(),
			Ref:    skill.RequestedRef(),
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

	return rejectCredentialedSource("skill", req.Source)
}

// validateSkillSource validates that the skill source is accessible
func validateSkillSource(baseDir, source string) (string, error) {
	if isGitURL(source) {
		return sourceTypeGit, validateGitURL(source)
	}
	return skillSourceType(baseDir, source), nil
}

// skillSourceType is "git" for a source the resolver clones (an https or ssh URL,
// git@host:path, owner/repo or github:owner/repo, unless owner/repo names a
// directory below baseDir) and "local" otherwise.
func skillSourceType(baseDir, source string) string {
	entries := []config.InstalledSkillConfig{{Source: source}}
	config.NormalizeInstalledSkillSources(baseDir, entries)
	if incl.IsGitURL(entries[0].Source) {
		return sourceTypeGit
	}
	return sourceTypeLocal
}

// entry is the installed_skills entry the request describes.
func (req *InstallSkillRequest) entry() config.InstalledSkillConfig {
	return config.InstalledSkillConfig{
		Name: req.Name, Source: req.Source, Path: req.Path, Ref: req.Ref,
		Include: req.Include, Exclude: req.Exclude, NamePrefix: req.NamePrefix,
		Trust: req.Trust, Resources: req.Resources, Version: req.Version,
		Domain: req.Domain, Profiles: req.Profiles, LocalOverride: req.LocalOverride,
		TagPrefix: req.TagPrefix, IncludePrerelease: req.IncludePrerelease, MinReleaseAge: req.MinReleaseAge,
	}
}

// validateInstalledSkillEntry applies the checks config load applies to an
// entry, so a flag value the loader would refuse is refused before it is written.
func validateInstalledSkillEntry(entry *config.InstalledSkillConfig) error {
	if err := config.ValidateInstalledSkillFields(entry); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	return config.ValidateInstalledSkillVersion(entry) //nolint:wrapcheck // already contextual
}
