package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ReadConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)

	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to get directory: %w", err))
	}

	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}
	if version != config.VersionDir {
		return ToolError(fmt.Errorf("read_config requires config (.ai-rulez/); found %s config — migrate with 'ai-rulez migrate'", version))
	}

	cfg, err := config.LoadConfig(ctx, dir)
	if err != nil {
		return ToolError(err)
	}

	// Build presets list
	presets := make([]string, 0, len(cfg.Presets))
	for _, p := range cfg.Presets {
		presets = append(presets, p.GetName())
	}

	// Build includes list
	includes := make([]map[string]interface{}, 0, len(cfg.Includes))
	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
		entry := map[string]interface{}{
			"name":   inc.Name,
			"source": inc.Source,
		}
		if inc.Path != "" {
			entry["path"] = inc.Path
		}
		if inc.Ref != "" {
			entry["ref"] = inc.Ref
		}
		if inc.MergeStrategy != "" {
			entry["merge_strategy"] = inc.MergeStrategy
		}
		if inc.InstallTo != "" {
			entry["install_to"] = inc.InstallTo
		}
		if len(inc.Include) > 0 {
			entry["include"] = inc.Include
		}
		includes = append(includes, entry)
	}

	// Build builtins representation
	var builtins interface{}
	if cfg.Builtins != nil {
		if cfg.Builtins.All != nil {
			builtins = *cfg.Builtins.All
		} else {
			builtins = cfg.Builtins.Names
		}
	}

	result := map[string]interface{}{
		"success":     true,
		"operation":   "read_config",
		"name":        cfg.Name,
		"description": cfg.Description,
		"presets":     presets,
		"profiles":    cfg.Profiles,
		"builtins":    builtins,
		"includes":    includes,
		"gitignore":   cfg.ShouldUpdateGitignore(),
	}

	// Always emit default_effort (possibly "") so MCP clients running read-modify-write
	// loops have a stable contract — absent key vs empty value is otherwise ambiguous.
	result["default_effort"] = configDefaultEffort(cfg)
	result["default_effort_by_preset"] = configDefaultEffortByPreset(cfg)

	return ToolSuccess(result)
}

// configDefaultEffort returns the resolved defaults.effort value, or "" when unset.
func configDefaultEffort(cfg *config.Config) string {
	if cfg == nil || cfg.Defaults == nil {
		return ""
	}
	return cfg.Defaults.Effort
}

// configDefaultEffortByPreset returns a map of per-preset overrides. Always returns a
// non-nil map so MCP clients can distinguish "no overrides" from "key absent".
func configDefaultEffortByPreset(cfg *config.Config) map[string]string {
	out := map[string]string{}
	if cfg == nil || cfg.Defaults == nil {
		return out
	}
	for k, v := range cfg.Defaults.EffortByPreset {
		out[k] = v
	}
	return out
}

// defaultsIsEmpty reports whether a DefaultsConfig has nothing worth persisting.
// Equivalence to the zero value can't be done with == because the struct contains
// a map; we check each field explicitly.
func defaultsIsEmpty(d *config.DefaultsConfig) bool {
	return d.Effort == "" && len(d.EffortByPreset) == 0
}

// parseEffortByPresetArg coerces an MCP argument into map[string]string. Accepts the
// JSON-decoded shape (map[string]interface{}) and the already-typed shape. Returns an
// empty (non-nil) map for nil input so callers can clear the field by passing {}.
func parseEffortByPresetArg(raw interface{}) (map[string]string, error) {
	out := map[string]string{}
	if raw == nil {
		return out, nil
	}
	switch m := raw.(type) {
	case map[string]string:
		for k, v := range m {
			out[k] = v
		}
	case map[string]interface{}:
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("default_effort_by_preset.%s must be a string, got %T", k, v)
			}
			out[k] = s
		}
	default:
		return nil, fmt.Errorf("default_effort_by_preset must be an object mapping preset name to effort value, got %T", raw)
	}
	return out, nil
}

// applyDefaultEffortByPresetUpdate writes the per-preset overrides onto cfg.Defaults.
// Empty values inside the map clear that preset's entry. An empty map clears the whole
// field. The Defaults block is dropped only when every field on it is zero so future
// fields aren't silently wiped.
func applyDefaultEffortByPresetUpdate(cfg *config.Config, m map[string]string) {
	cleaned := map[string]string{}
	for k, v := range m {
		if v != "" {
			cleaned[k] = v
		}
	}
	if len(cleaned) == 0 {
		if cfg.Defaults == nil {
			return
		}
		cfg.Defaults.EffortByPreset = nil
		if defaultsIsEmpty(cfg.Defaults) {
			cfg.Defaults = nil
		}
		return
	}
	if cfg.Defaults == nil {
		cfg.Defaults = &config.DefaultsConfig{}
	}
	cfg.Defaults.EffortByPreset = cleaned
}

// applyDefaultEffortUpdate writes the requested effort onto cfg.Defaults. Passing
// an empty string clears Effort only; the surrounding Defaults block is dropped
// only when every field on it is zero, so future fields aren't silently wiped.
func applyDefaultEffortUpdate(cfg *config.Config, effort string) {
	if effort == "" {
		if cfg.Defaults == nil {
			return
		}
		cfg.Defaults.Effort = ""
		if defaultsIsEmpty(cfg.Defaults) {
			cfg.Defaults = nil
		}
		return
	}
	if cfg.Defaults == nil {
		cfg.Defaults = &config.DefaultsConfig{}
	}
	cfg.Defaults.Effort = effort
}

// applyConfigUpdates dispatches each top-level field on the request into cfg.
// Returns the list of fields that were actually present in args, plus any parse
// error from a structured argument. Validation runs in the caller.
func applyConfigUpdates(cfg *config.Config, request *ToolRequest) ([]string, error) {
	args := request.GetArguments()
	updated := []string{}

	if _, ok := args["name"]; ok {
		cfg.Name = request.GetString("name", cfg.Name)
		updated = append(updated, "name")
	}
	if _, ok := args["description"]; ok {
		cfg.Description = request.GetString("description", cfg.Description)
		updated = append(updated, "description")
	}
	if _, ok := args["builtins"]; ok {
		builtinNames := request.GetStringSlice("builtins", nil)
		if builtinNames != nil {
			cfg.Builtins = &config.BuiltinsConfig{Names: builtinNames}
		} else {
			cfg.Builtins = nil
		}
		updated = append(updated, "builtins")
	}
	if _, ok := args["gitignore"]; ok {
		val := request.GetBool("gitignore", true)
		cfg.Gitignore = &val
		updated = append(updated, "gitignore")
	}
	if _, ok := args["default_effort"]; ok {
		applyDefaultEffortUpdate(cfg, request.GetString("default_effort", ""))
		updated = append(updated, "default_effort")
	}
	if raw, ok := args["default_effort_by_preset"]; ok {
		m, parseErr := parseEffortByPresetArg(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		applyDefaultEffortByPresetUpdate(cfg, m)
		updated = append(updated, "default_effort_by_preset")
	}
	return updated, nil
}

func UpdateConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)

	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to get directory: %w", err))
	}

	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}
	if version != config.VersionDir {
		return ToolError(fmt.Errorf("update_config requires config (.ai-rulez/); found %s config — migrate with 'ai-rulez migrate'", version))
	}

	cfg, err := config.LoadConfig(ctx, dir)
	if err != nil {
		return ToolError(err)
	}

	updated, applyErr := applyConfigUpdates(cfg, request)
	if applyErr != nil {
		return ToolError(applyErr)
	}

	// Validate after applying changes so invalid input is rejected before write.
	if validateErr := cfg.Validate(); validateErr != nil {
		return ToolError(fmt.Errorf("validation failed after update: %w", validateErr))
	}

	if len(updated) == 0 {
		return ToolSuccess(map[string]interface{}{
			"success":   true,
			"operation": "update_config",
			"message":   "No fields to update",
			"updated":   updated,
		})
	}

	configDir := filepath.Join(dir, ".ai-rulez")
	if err := config.SaveConfig(cfg, configDir); err != nil {
		return ToolError(fmt.Errorf("failed to save config: %w", err))
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "update_config",
		"message":   "Config updated successfully",
		"updated":   updated,
	})
}

func GenerateOutputsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	dryRun := request.GetBool("dry_run", false)
	recursive := request.GetBool("recursive", false)

	if recursive {
		return generateRecursive(ctx, request, baseDir, dryRun)
	}

	return generateForDirectory(ctx, request, baseDir, dryRun)
}

func generateRecursive(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (*mcp.CallToolResult, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to resolve base directory: %w", err))
	}

	var dirs []string
	skipDirs := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".cache": true}
	err = filepath.WalkDir(absBase, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		configPath := filepath.Join(path, ".ai-rulez", "config.yaml")
		if _, statErr := os.Stat(configPath); statErr == nil {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return ToolError(fmt.Errorf("failed to walk directories: %w", err))
	}

	if len(dirs) == 0 {
		return ToolSuccess(map[string]interface{}{
			"message": "No directories with .ai-rulez/config.yaml found",
		})
	}

	var results []map[string]interface{}
	for _, dir := range dirs {
		result, genErr := generateForDirectory(ctx, request, dir, dryRun)
		entry := map[string]interface{}{
			"directory": dir,
		}
		switch {
		case genErr != nil:
			entry["error"] = genErr.Error()
		case result != nil && result.IsError:
			entry["error"] = result.Content[0]
		default:
			entry["success"] = true
		}
		results = append(results, entry)
	}

	return ToolSuccess(map[string]interface{}{
		"message": fmt.Sprintf("Recursive generation completed for %d directories", len(dirs)),
		"results": results,
	})
}

func generateForDirectory(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		var err error
		configFile, err = config.FindConfigFile(baseDir)
		if err != nil {
			return ToolError(err)
		}
	}

	// Detect config version and load appropriately
	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to get directory: %w", err))
	}
	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}

	if version != config.VersionDir {
		return ToolError(fmt.Errorf("generate_outputs requires config (.ai-rulez/); found %s config — migrate with 'ai-rulez migrate'", version))
	}

	if dryRun {
		return ToolSuccess(map[string]interface{}{
			"message": "dry_run: not yet implemented for configs",
			"config":  configFile,
		})
	}
	// Load and generate config
	cfg, err := config.LoadConfig(ctx, dir)
	if err != nil {
		return ToolError(err)
	}
	if err := cfg.Validate(); err != nil {
		return ToolError(err)
	}
	gen := generator.NewGenerator(cfg)
	if err := gen.Generate(""); err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		"message": "Outputs generated successfully",
		"config":  configFile,
	})
}

func ValidateConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)

	// Detect config version
	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to get directory: %w", err))
	}
	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}

	if version != config.VersionDir {
		return ToolError(fmt.Errorf("validate_config requires config (.ai-rulez/); found %s config — migrate with 'ai-rulez migrate'", version))
	}

	// Validate config
	cfg, err := config.LoadConfig(ctx, dir)
	if err != nil {
		result := map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		}
		return ToolSuccess(result)
	}
	if err := cfg.Validate(); err != nil {
		result := map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		}
		return ToolSuccess(result)
	}
	result := map[string]interface{}{
		"valid":    true,
		"warnings": []string{},
	}
	return ToolSuccess(result)
}

func getPresetsFromProviders(providers []interface{}, allProviders, popularProviders bool) ([]string, bool) {
	if allProviders {
		return []string{"claude", "cursor", "windsurf", "copilot", "gemini", "amp", "codex", "cline", "continue-dev"}, true
	}
	if popularProviders {
		return []string{"popular"}, false
	}

	var presets []string
	var hasContinueDev bool

	providerMap := map[string]string{
		"claude":       "claude",
		"cursor":       "cursor",
		"windsurf":     "windsurf",
		"copilot":      "copilot",
		"gemini":       "gemini",
		"amp":          "amp",
		"codex":        "codex",
		"cline":        "cline",
		"continue-dev": "continue-dev",
	}

	for _, p := range providers {
		if provider, ok := p.(string); ok {
			if preset, exists := providerMap[provider]; exists {
				presets = append(presets, preset)
				if provider == "continue-dev" {
					hasContinueDev = true
				}
			}
		}
	}

	return presets, hasContinueDev
}

func InitProjectHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	projectName := request.GetString("project_name", "")
	providersInterface := request.GetArguments()["providers"]
	withAgents := request.GetBool("with_agents", false)
	allProviders := request.GetBool("all_providers", false)
	popularProviders := request.GetBool("popular_providers", false)

	var providers []interface{}
	if providersSlice, ok := providersInterface.([]interface{}); ok {
		providers = providersSlice
	}

	presets, hasContinueDev := getPresetsFromProviders(providers, allProviders, popularProviders)

	var configContent string
	if len(presets) > 0 {
		configContent = templates.GenerateConfigWithPresets(projectName, presets)
	} else {
		configContent = templates.GenerateConfigWithPresets(projectName, []string{"claude"})
	}

	// Create .ai-rulez/ directory structure
	aiRulesDir := filepath.Join(baseDir, ".ai-rulez")
	if err := os.MkdirAll(aiRulesDir, 0o755); err != nil {
		return ToolError(fmt.Errorf("failed to create .ai-rulez directory: %w", err))
	}

	configPath := filepath.Join(aiRulesDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return ToolError(fmt.Errorf("failed to write config file: %w", err))
	}

	// Create agents directory if requested
	if withAgents {
		agentsDir := filepath.Join(aiRulesDir, "agents")
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			return ToolError(fmt.Errorf("failed to create agents directory: %w", err))
		}
	}

	if hasContinueDev {
		if err := CreateContinueDevConfig(); err != nil {
			return ToolError(fmt.Errorf("failed to create continue.dev config: %w", err))
		}
	}

	return ToolSuccess(map[string]interface{}{
		"message": "Project initialized successfully",
		"path":    configPath,
	})
}
