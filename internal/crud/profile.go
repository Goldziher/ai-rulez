package crud

import (
	"context"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
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

	if op.local {
		return op.addProfileLocal(ctx, name, domains)
	}

	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
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

	if err := validateProfileDomains(cfg, name, domains); err != nil {
		return err
	}

	// Initialize profiles map if needed
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string][]string)
	}

	// Add profile
	cfg.Profiles[name] = domains

	// Save updated config
	if err := config.SaveConfig(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	op.logger().Info(
		"Profile added successfully",
		"name", name,
		"domains", len(domains),
	)

	return nil
}

// validateProfileDomains checks every domain a profile names. A "builtin:<name>"
// element must name a real builtin pack; a bare name must exist in the content
// tree.
func validateProfileDomains(cfg *config.Config, profile string, domains []string) error {
	for _, domain := range domains {
		if domain == "" {
			continue
		}
		if builtins.HasRefPrefix(domain) {
			if !builtins.IsValid(domain) {
				return oops.
					With("profile", profile).
					With("builtin", domain).
					Hint("Use a name from `ai-rulez builtins list`").
					Errorf("unknown builtin '%s'", builtins.TrimRefPrefix(domain))
			}
			continue
		}
		if cfg.Content == nil || cfg.Content.Domains == nil {
			continue
		}
		if _, exists := cfg.Content.Domains[domain]; !exists && !hasLocalDomain(cfg, domain) {
			return oops.
				With("profile", profile).
				With("domain", domain).
				Hint("Domain does not exist in .ai-rulez/domains/").
				Errorf("domain '%s' does not exist", domain)
		}
	}
	return nil
}

// hasLocalDomain reports whether the machine-local content tree defines the
// domain. LocalContent is only populated for merged loads, so the shared path
// (config.WithoutLocal) never accepts a local-only domain.
func hasLocalDomain(cfg *config.Config, domain string) bool {
	if cfg.LocalContent == nil {
		return false
	}
	_, ok := cfg.LocalContent.Domains[domain]
	return ok
}

// RemoveProfile removes a profile from the config
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) RemoveProfile(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid profile name").
			Errorf("profile name is required")
	}

	if op.local {
		return op.removeProfileLocal(ctx, name)
	}

	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
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
	if err := config.SaveConfig(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	op.logger().Info("Profile removed successfully", "name", name)

	return nil
}

// SetDefaultProfile sets the default profile in the config
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) SetDefaultProfile(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid profile name").
			Errorf("profile name is required")
	}

	if op.local {
		return op.setDefaultProfileLocal(ctx, name)
	}

	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
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
	if err := config.SaveConfig(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	op.logger().Info("Default profile set successfully", "name", name)

	return nil
}

// ListProfiles returns all configured profiles
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) ListProfiles(ctx context.Context) ([]ProfileInfo, error) {
	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
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
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })

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

	// A comma composes several profiles into one value, so a name holding one
	// could be written but never selected.
	if strings.Contains(name, config.ProfileSeparator) {
		return oops.
			With("name", name).
			Hint("A comma composes profiles when selecting one (--profile base,backend), so it cannot appear in a name").
			Errorf("profile name %q contains %q", name, config.ProfileSeparator)
	}

	return nil
}
