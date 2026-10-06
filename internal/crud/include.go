package crud

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
)

// Include merge strategies accepted by the include resolver. Kept in sync with
// internal/includes; a config value outside this set makes the resolver error
// and skip the include, so the CRUD layer rejects it up front.
const (
	MergeStrategyLocalOverride   = "local-override"
	MergeStrategyIncludeOverride = "include-override"
	MergeStrategyError           = "error"
)

// AddInclude adds a new include source to the config and validates it
func (op *OperatorImpl) AddInclude(ctx context.Context, req *AddIncludeRequest) error {
	if err := validateAddIncludeRequest(req); err != nil {
		return err
	}

	if op.local {
		return op.addIncludeLocal(ctx, req)
	}

	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	// Check if include already exists
	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
		if inc.Name == req.Name {
			return oops.
				With("name", req.Name).
				Hint("Choose a different name for the include").
				Errorf("include with name '%s' already exists", req.Name)
		}
	}

	// Validate the source is accessible
	sourceType, err := validateIncludeSource(op.env, req.Source)
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
	if err := config.SaveConfig(cfg, op.aiRulezDir); err != nil {
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
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) RemoveInclude(ctx context.Context, name string) error {
	if name == "" {
		return oops.
			Hint("Provide a valid include name").
			Errorf("include name is required")
	}

	if op.local {
		return op.removeIncludeLocal(ctx, name)
	}

	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	// Find the include
	foundIdx := -1
	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
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
	if err := config.SaveConfig(cfg, op.aiRulezDir); err != nil {
		return oops.
			With("config_dir", op.aiRulezDir).
			Wrapf(err, "save config")
	}

	logger.Info("Include removed successfully", "name", name)

	return nil
}

// ListIncludes returns all configured includes from the config
// NOTE: this reads and writes the shared config layer (config.WithoutLocal); the
// Local() operator variant writes the machine-local overlay instead. Listing
// always shows the shared layer.
func (op *OperatorImpl) ListIncludes(ctx context.Context) ([]IncludeInfo, error) {
	baseDir := op.baseDir

	// Load current config
	cfg, err := project.Load(ctx, baseDir, config.WithoutLocal())
	if err != nil {
		return nil, oops.
			With("base_dir", baseDir).
			Wrapf(err, "load config")
	}

	var infos []IncludeInfo

	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
		sourceType := sourceTypeLocal
		if isGitURL(inc.Source) {
			sourceType = sourceTypeGit
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

	// Validate include types if provided. These are the content kinds the
	// include resolver merges; MCP servers are not importable from an include.
	validTypes := map[string]bool{
		ContentTypeRules:   true,
		ContentTypeContext: true,
		ContentTypeSkills:  true,
		"agents":           true,
		"commands":         true,
	}

	for _, t := range req.Include {
		if !validTypes[t] {
			return oops.
				With("type", t).
				Hint("Valid types: rules, context, skills, agents, commands").
				Errorf("invalid include type '%s'", t)
		}
	}

	// Validate merge strategy if provided. These are the values the include
	// resolver accepts; a stored value outside this set silently drops the
	// include at generation.
	if req.MergeStrategy != "" {
		validStrategies := map[string]bool{
			MergeStrategyLocalOverride:   true,
			MergeStrategyIncludeOverride: true,
			MergeStrategyError:           true,
		}
		if !validStrategies[req.MergeStrategy] {
			return oops.
				With("strategy", req.MergeStrategy).
				Hint("Valid strategies: local-override, include-override, error").
				Errorf("invalid merge strategy '%s'", req.MergeStrategy)
		}
	}

	return nil
}

// validateIncludeSource validates that the include source is accessible
// Returns the source type ("git" or "local") and any errors
func validateIncludeSource(env ambient.Env, source string) (string, error) {
	if isGitURL(source) {
		return sourceTypeGit, validateGitURL(source)
	}

	// It's a local path
	return sourceTypeLocal, validateLocalPath(env, source)
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
	if strings.HasPrefix(strings.ToLower(gitURL), "http://") {
		return oops.
			With("url", gitURL).
			Hint("Plain http:// is not accepted since ai-rulez 5; use https:// or git@host:path").
			Errorf("insecure git URL %q", gitURL)
	}
	if !strings.HasPrefix(gitURL, "https://") && !strings.HasPrefix(gitURL, "git@") {
		return oops.
			With("url", gitURL).
			Hint("Git URLs must start with 'https://' or 'git@'").
			Errorf("invalid git URL format")
	}

	// Basic validation - could be expanded with git ls-remote check
	if strings.HasPrefix(gitURL, "https://") {
		if _, err := url.Parse(gitURL); err != nil {
			return oops.
				With("url", gitURL).
				Wrapf(err, "parse git URL")
		}
	}

	return nil
}

// validateLocalPath validates a local filesystem path
func validateLocalPath(env ambient.Env, path string) error {
	// Expand ~ and environment variables
	expandedPath := ambient.Expand(env, path)
	if strings.HasPrefix(expandedPath, "~") {
		home, err := ambient.OrOS(env).UserHomeDir()
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
