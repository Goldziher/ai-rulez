package crud

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// AddInclude adds a new include source to the config and validates it
func (op *OperatorImpl) AddInclude(ctx context.Context, req *AddIncludeRequest) error {
	if err := validateAddIncludeRequest(req); err != nil {
		return err
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

	// Check if include already exists
	for _, inc := range cfg.Includes {
		if inc.Name == req.Name {
			return oops.
				With("name", req.Name).
				Hint("Choose a different name for the include").
				Errorf("include with name '%s' already exists", req.Name)
		}
	}

	// Validate the source is accessible
	sourceType, err := validateIncludeSource(req.Source)
	if err != nil {
		return err
	}

	// Create the include config entry
	includeConfig := config.IncludeConfig{
		Name:          req.Name,
		Source:        req.Source,
		Path:          req.Path,
		Ref:           req.Ref,
		Include:       req.Include,
		MergeStrategy: req.MergeStrategy,
		InstallTo:     req.InstallTo,
	}

	// Append to includes array
	cfg.Includes = append(cfg.Includes, includeConfig)

	// Save updated config
	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info(
		"Include added successfully",
		"name", req.Name,
		"source", req.Source,
		"type", sourceType,
	)

	return nil
}

// RemoveInclude removes an include source from the config
func (op *OperatorImpl) RemoveInclude(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid include name").
			Errorf("include name is required")
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

	// Find the include
	foundIdx := -1
	for i, inc := range cfg.Includes {
		if inc.Name == name {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return oops.
			With("name", name).
			Hint("Use 'list includes' to see available includes").
			Errorf("include '%s' not found", name)
	}

	// Remove from array
	cfg.Includes = append(cfg.Includes[:foundIdx], cfg.Includes[foundIdx+1:]...)

	// Save updated config
	if err := config.SaveConfigV3(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info("Include removed successfully", "name", name)

	return nil
}

// ListIncludes returns all configured includes from the config
func (op *OperatorImpl) ListIncludes(ctx context.Context) ([]IncludeInfo, error) {
	// Determine base directory from aiRulezDir
	baseDir := filepath.Dir(op.aiRulezDir)

	// Load current config
	cfg, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return nil, oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	var infos []IncludeInfo

	for _, inc := range cfg.Includes {
		sourceType := "local"
		if isGitURL(inc.Source) {
			sourceType = "git"
		}

		info := IncludeInfo{
			Name:   inc.Name,
			Source: inc.Source,
			Type:   sourceType,
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// validateAddIncludeRequest validates the add include request
func validateAddIncludeRequest(req *AddIncludeRequest) error {
	if req == nil {
		return oops.
			Hint("Provide a valid AddIncludeRequest").
			Errorf("request is nil")
	}

	if req.Name == "" {
		return oops.
			Hint("Include name must be alphanumeric with hyphens/underscores").
			Errorf("include name is required")
	}

	if req.Source == "" {
		return oops.
			Hint("Provide a git URL (https:// or git@) or local path").
			Errorf("include source is required")
	}

	// Validate include types if provided
	validTypes := map[string]bool{
		"rules":   true,
		"context": true,
		"skills":  true,
		"mcp":     true,
	}

	for _, t := range req.Include {
		if !validTypes[t] {
			return oops.
				With("type", t).
				Hint("Valid types: rules, context, skills, mcp").
				Errorf("invalid include type '%s'", t)
		}
	}

	// Validate merge strategy if provided
	if req.MergeStrategy != "" {
		validStrategies := map[string]bool{
			"default":  true,
			"override": true,
			"append":   true,
		}
		if !validStrategies[req.MergeStrategy] {
			return oops.
				With("strategy", req.MergeStrategy).
				Hint("Valid strategies: default, override, append").
				Errorf("invalid merge strategy '%s'", req.MergeStrategy)
		}
	}

	return nil
}

// validateIncludeSource validates that the include source is accessible
// Returns the source type ("git" or "local") and any errors
func validateIncludeSource(source string) (string, error) {
	if isGitURL(source) {
		return "git", validateGitURL(source)
	}

	// It's a local path
	return "local", validateLocalPath(source)
}

// isGitURL checks if a source is a git URL
func isGitURL(source string) bool {
	// Check for common git URL patterns
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return strings.Contains(source, "git") || strings.HasSuffix(source, ".git")
	}
	if strings.HasPrefix(source, "git@") {
		return true
	}
	return false
}

// validateGitURL validates a git URL syntax
func validateGitURL(gitURL string) error {
	if !strings.HasPrefix(gitURL, "https://") && !strings.HasPrefix(gitURL, "http://") && !strings.HasPrefix(gitURL, "git@") {
		return oops.
			With("url", gitURL).
			Hint("Git URLs must start with 'https://', 'http://', or 'git@'").
			Errorf("invalid git URL format")
	}

	// Basic validation - could be expanded with git ls-remote check
	if strings.HasPrefix(gitURL, "https://") || strings.HasPrefix(gitURL, "http://") {
		if _, err := url.Parse(gitURL); err != nil {
			return oops.
				With("url", gitURL).
				Wrapf(err, "parse git URL")
		}
	}

	return nil
}

// validateLocalPath validates a local filesystem path
func validateLocalPath(path string) error {
	// Expand ~ and environment variables
	expandedPath := os.ExpandEnv(path)
	if strings.HasPrefix(expandedPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return oops.Wrapf(err, "get home directory")
		}
		expandedPath = filepath.Join(home, expandedPath[1:])
	}

	// Check if path exists
	info, err := os.Stat(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return oops.
				With("path", path).
				Hint("Ensure the path exists and is accessible").
				Errorf("local path does not exist")
		}
		return oops.
			With("path", path).
			Wrapf(err, "stat path")
	}

	// Check if it's a directory
	if !info.IsDir() {
		return oops.
			With("path", path).
			Hint("Include path must be a directory").
			Errorf("local path is not a directory")
	}

	// Check if it contains .ai-rulez directory or config files
	aiRulezPath := filepath.Join(expandedPath, ".ai-rulez")
	if _, err := os.Stat(aiRulezPath); err != nil {
		if os.IsNotExist(err) {
			return oops.
				With("path", path).
				Hint("Directory must contain .ai-rulez/ subdirectory with config").
				Errorf("directory does not contain .ai-rulez configuration")
		}
	}

	return nil
}
