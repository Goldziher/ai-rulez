package crud

import (
	"context"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// AddProfile adds a new profile to the config
func (op *OperatorImpl) AddProfile(ctx context.Context, name string, domains []string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}

	if len(domains) == 0 {
		return oops.
			With("name", name).
			Hint("Provide at least one domain for the profile").
			Errorf("profile must have at least one domain")
	}

	// Determine base directory from aiRulezDir
	baseDir := filepath.Dir(op.aiRulezDir)

	// Load current config
	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	// Check if profile already exists
	if cfg.HasProfile(name) {
		return oops.
			With("name", name).
			Hint("Choose a different name for the profile").
			Errorf("profile '%s' already exists", name)
	}

	// Validate all domains exist in content tree
	if cfg.Content != nil {
		for _, domain := range domains {
			if domain != "" && cfg.Content.Domains != nil {
				if _, exists := cfg.Content.Domains[domain]; !exists {
					return oops.
						With("profile", name).
						With("domain", domain).
						Hint("Domain does not exist in .ai-rulez/domains/").
						Errorf("domain '%s' does not exist", domain)
				}
			}
		}
	}

	// Initialize profiles map if needed
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string][]string)
	}

	// Add profile
	cfg.Profiles[name] = domains

	// Save updated config
	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info(
		"Profile added successfully",
		"name", name,
		"domains", len(domains),
	)

	return nil
}

// RemoveProfile removes a profile from the config
func (op *OperatorImpl) RemoveProfile(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid profile name").
			Errorf("profile name is required")
	}

	// Determine base directory from aiRulezDir
	baseDir := filepath.Dir(op.aiRulezDir)

	// Load current config
	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	// Check if profile exists
	if !cfg.HasProfile(name) {
		return oops.
			With("name", name).
			Hint("Use 'profile list' to see available profiles").
			Errorf("profile '%s' does not exist", name)
	}

	// Check if this is the default profile
	if cfg.GetDefaultProfile() == name {
		return oops.
			With("name", name).
			Hint("Set a different default profile first using 'profile set-default'").
			Errorf("cannot remove default profile '%s'", name)
	}

	// Remove profile
	delete(cfg.Profiles, name)

	// Save updated config
	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info("Profile removed successfully", "name", name)

	return nil
}

// SetDefaultProfile sets the default profile in the config
func (op *OperatorImpl) SetDefaultProfile(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid profile name").
			Errorf("profile name is required")
	}

	// Determine base directory from aiRulezDir
	baseDir := filepath.Dir(op.aiRulezDir)

	// Load current config
	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	// Check if profile exists
	if !cfg.HasProfile(name) {
		return oops.
			With("name", name).
			Hint("Create the profile first using 'profile add'").
			Errorf("profile '%s' does not exist", name)
	}

	// Set default
	cfg.Default = name

	// Save updated config
	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info("Default profile set successfully", "name", name)

	return nil
}

// ListProfiles returns all configured profiles
func (op *OperatorImpl) ListProfiles(ctx context.Context) ([]ProfileInfo, error) {
	// Determine base directory from aiRulezDir
	baseDir := filepath.Dir(op.aiRulezDir)

	// Load current config
	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return nil, oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	var infos []ProfileInfo
	defaultProfile := cfg.GetDefaultProfile()

	for name, domains := range cfg.Profiles {
		info := ProfileInfo{
			Name:      name,
			Domains:   domains,
			IsDefault: name == defaultProfile,
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// validateProfileName validates a profile name
func validateProfileName(name string) error {
	if name == "" {
		return oops.
			Hint("Profile name must be alphanumeric with hyphens/underscores").
			Errorf("profile name is required")
	}

	// Check for reserved names
	reservedNames := map[string]bool{
		"default": true,
		"root":    true,
		"all":     true,
	}

	if reservedNames[name] {
		return oops.
			With("name", name).
			Hint("Choose a different name").
			Errorf("'%s' is a reserved profile name", name)
	}

	return nil
}
