package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/Goldziher/ai-rulez/internal/walkutil"
	"github.com/Goldziher/ai-rulez/schema"
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

	// The editable view is the shared config; the machine-local overlay is
	// reported separately and by key path only (it may hold secrets).
	cfg, err := config.LoadConfig(ctx, dir, config.WithoutLocal())
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
			keyName:   inc.Name,
			keySource: inc.Source,
		}
		if inc.Path != "" {
			entry[keyPath] = inc.Path
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
		keySuccess:     true,
		keyOperation:   "read_config",
		keyName:        cfg.Name,
		keyDescription: cfg.Description,
		"presets":      presets,
		"profiles":     cfg.Profiles,
		keyBuiltins:    builtins,
		"includes":     includes,
		keyGitignore:   cfg.ShouldUpdateGitignore(),
	}

	// Always emit default_effort (possibly "") so MCP clients running read-modify-write
	// loops have a stable contract — absent key vs empty value is otherwise ambiguous.
	result["default_effort"] = configDefaultEffort(cfg)
	result["default_effort_by_preset"] = configDefaultEffortByPreset(cfg)

	// Same stable-contract rule for rules: always emit both keys.
	result["rules_mode"], result["rules_mode_by_preset"] = configRules(cfg)

	if info := localOverlayInfo(cfg); info != nil {
		result["local_overlay"] = info
	}

	return ToolSuccess(result)
}

// localOverlayInfo describes the machine-local overlay beside cfg: its path and
// the key paths it sets, never its values (it may hold secrets). Nil when there
// is no overlay.
const keyError = "error"

func localOverlayInfo(cfg *config.Config) map[string]interface{} {
	overlay, err := config.ReadLocalOverlay(cfg.ConfigDir, cfg.ConfigFile)
	if err != nil {
		return map[string]interface{}{keyError: err.Error()}
	}
	if overlay == nil {
		return nil
	}
	return map[string]interface{}{"path": overlay.Path, "keys": overlay.KeyPaths()}
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

// configRules returns the configured rules.mode ("" when unset) and a non-nil copy
// of rules.mode_by_preset.
func configRules(cfg *config.Config) (mode string, byPreset map[string]string) {
	byPreset = map[string]string{}
	if cfg == nil || cfg.Rules == nil {
		return "", byPreset
	}
	for k, v := range cfg.Rules.ModeByPreset {
		byPreset[k] = v
	}
	return cfg.Rules.Mode, byPreset
}

// applyRulesUpdates applies rules_mode and rules_mode_by_preset when present in args.
// An empty rules_mode clears the mode; an empty map clears the per-preset overrides.
// The Rules block is dropped when nothing is left in it.
func applyRulesUpdates(cfg *config.Config, args map[string]interface{}) ([]string, error) {
	var updated []string
	if raw, ok := args["rules_mode"]; ok {
		mode, isString := raw.(string)
		if !isString {
			return nil, fmt.Errorf("rules_mode must be a string, got %T", raw)
		}
		if cfg.Rules == nil {
			cfg.Rules = &config.RulesConfig{}
		}
		cfg.Rules.Mode = mode
		updated = append(updated, "rules_mode")
	}
	if raw, ok := args["rules_mode_by_preset"]; ok {
		m, err := parseStringMapArg("rules_mode_by_preset", "rules mode", raw)
		if err != nil {
			return nil, err
		}
		if cfg.Rules == nil {
			cfg.Rules = &config.RulesConfig{}
		}
		// Mirror default_effort_by_preset: an empty value removes that preset's
		// override, and the resulting map replaces the previous one.
		cleaned := map[string]string{}
		for k, v := range m {
			if v != "" {
				cleaned[k] = v
			}
		}
		cfg.Rules.ModeByPreset = nil
		if len(cleaned) > 0 {
			cfg.Rules.ModeByPreset = cleaned
		}
		updated = append(updated, "rules_mode_by_preset")
	}
	if cfg.Rules != nil && cfg.Rules.Mode == "" && len(cfg.Rules.ModeByPreset) == 0 {
		cfg.Rules = nil
	}
	return updated, nil
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
	return parseStringMapArg("default_effort_by_preset", "effort value", raw)
}

// parseStringMapArg is the shared coercion behind the per-preset map arguments;
// field and valueDesc only shape the error messages.
func parseStringMapArg(field, valueDesc string, raw interface{}) (map[string]string, error) {
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
				return nil, fmt.Errorf("%s.%s must be a string, got %T", field, k, v)
			}
			out[k] = s
		}
	default:
		return nil, fmt.Errorf("%s must be an object mapping preset name to %s, got %T", field, valueDesc, raw)
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

	if _, ok := args[keyName]; ok {
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
	rulesUpdated, err := applyRulesUpdates(cfg, args)
	if err != nil {
		return nil, err
	}
	updated = append(updated, rulesUpdated...)
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

	if request.GetBool("local", false) {
		return updateLocalConfig(ctx, request, dir)
	}

	cfg, err := config.LoadConfig(ctx, dir, config.WithoutLocal())
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
			keySuccess:   true,
			keyOperation: opUpdateConfig,
			keyMessage:   "No fields to update",
			keyUpdated:   updated,
		})
	}

	// Save into the directory the config was loaded from (.ai-rulez/ or the
	// project-level .config/ai-rulez/), never a fresh .ai-rulez/ beside it.
	if err := config.SaveConfig(cfg, cfg.ConfigDir); err != nil {
		return ToolError(fmt.Errorf("failed to save config: %w", err))
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: opUpdateConfig,
		keyMessage:   "Config updated successfully",
		keyUpdated:   updated,
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

var recursiveConfigFiles = []string{"config.toml", "config.yaml", "config.yml", "config.json"}

func dirHasRecursiveConfig(dir string) bool {
	for _, f := range recursiveConfigFiles {
		if info, statErr := os.Stat(filepath.Join(dir, f)); statErr == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// findRecursiveConfigDirs walks absBase and returns the project directories
// of every config directory that contains a config file: `.ai-rulez/` by
// default, the project-level `.config/ai-rulez/` convention as a fallback, and
// any explicit configDirName. Build outputs, hidden directories, and shared
// rule libraries (`ai-rulez/` with a root config) are pruned. Errors on
// individual entries are skipped, not propagated.
func findRecursiveConfigDirs(absBase, configDirName string) ([]string, error) {
	// An explicit configDirName is honored exactly; only the default triggers
	// the .config/ convention fallback.
	useConventionFallback := configDirName == ""
	if configDirName == "" {
		configDirName = ".ai-rulez"
	}
	target := "/" + filepath.ToSlash(configDirName)
	var dirs []string
	err := filepath.WalkDir(absBase, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(filepath.ToSlash(path), target) {
			if dirHasRecursiveConfig(path) {
				dirs = append(dirs, projectDirAbove(path, configDirName))
			}
			return filepath.SkipDir
		}
		if useConventionFallback && name == ".config" {
			if projectDir, ok := conventionConfigProject(path); ok {
				dirs = append(dirs, projectDir)
			}
			return filepath.SkipDir
		}
		if path == absBase {
			return nil
		}
		return recursiveWalkDecision(path, name, configDirName)
	})
	return dirs, err
}

// conventionConfigProject reports the project directory owning a
// `.config/ai-rulez/` config found at dotConfigPath, unless a sibling
// `.ai-rulez/` config takes precedence.
func conventionConfigProject(dotConfigPath string) (string, bool) {
	projectDir := filepath.Dir(dotConfigPath)
	if dirHasRecursiveConfig(filepath.Join(projectDir, ".ai-rulez")) {
		return "", false
	}
	return projectDir, dirHasRecursiveConfig(filepath.Join(dotConfigPath, "ai-rulez"))
}

// recursiveWalkDecision decides whether the walk descends into a directory
// that is neither a config directory nor the walk root.
func recursiveWalkDecision(path, name, configDirName string) error {
	if name == "ai-rulez" && dirHasRecursiveConfig(path) {
		return filepath.SkipDir
	}
	// Descend through a wrapper directory (e.g. .config/) that is an
	// ancestor of a nested configDirName target such as .config/ai-rulez.
	if config.IsConfigDirAncestor(path, configDirName) {
		return nil
	}
	if walkutil.ShouldSkipDir(name) {
		return filepath.SkipDir
	}
	return nil
}

// projectDirAbove returns the project directory that owns a config directory
// located at path, stripping one path component per component of configDirName
// (e.g. ".config/ai-rulez" → the directory containing .config/).
func projectDirAbove(path, configDirName string) string {
	dir := path
	for range strings.Split(filepath.ToSlash(configDirName), "/") {
		dir = filepath.Dir(dir)
	}
	return dir
}

func generateRecursive(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (*mcp.CallToolResult, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to resolve base directory: %w", err))
	}

	configDirName := request.GetString("config_dir", "")
	dirs, err := findRecursiveConfigDirs(absBase, configDirName)
	if err != nil {
		return ToolError(fmt.Errorf("failed to walk directories: %w", err))
	}

	if len(dirs) == 0 {
		if configDirName == "" {
			configDirName = ".ai-rulez"
		}
		return ToolSuccess(map[string]interface{}{
			keyMessage: fmt.Sprintf("No directories with %s/config.* found", configDirName),
		})
	}

	results := make([]map[string]interface{}, 0, len(dirs))
	for _, dir := range dirs {
		results = append(results, runGenerateForDir(ctx, request, dir, dryRun))
	}

	return ToolSuccess(map[string]interface{}{
		keyMessage: fmt.Sprintf("Recursive generation completed for %d directories", len(dirs)),
		"results":  results,
	})
}

func runGenerateForDir(ctx context.Context, request *ToolRequest, dir string, dryRun bool) map[string]interface{} {
	entry := map[string]interface{}{"directory": dir}
	result, genErr := generateForDirectory(ctx, request, dir, dryRun)
	switch {
	case genErr != nil:
		entry["error"] = genErr.Error()
	case result != nil && result.IsError:
		entry["error"] = result.Content[0]
	default:
		entry[keySuccess] = true
	}
	return entry
}

func generateForDirectory(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (*mcp.CallToolResult, error) {
	cfg, err := loadProjectConfig(ctx, request, baseDir)
	if err != nil {
		return ToolError(err)
	}
	if err := cfg.Validate(); err != nil {
		return ToolError(err)
	}
	gen := generator.NewGenerator(cfg)
	gen.SetContext(ctx)
	if dryRun {
		plan, err := gen.DryRun("") //nolint:contextcheck // the context reaches the baseline load through SetContext
		if err != nil {
			return ToolError(err)
		}
		return ToolSuccess(map[string]interface{}{
			keyMessage: "Dry run complete",
			keyConfig:  cfg.ConfigDir,
			"plan":     plan,
		})
	}
	if err := gen.Generate(""); err != nil { //nolint:contextcheck // the context reaches the baseline load through SetContext
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keyMessage: "Outputs generated successfully",
		keyConfig:  cfg.ConfigDir,
	})
}

// CleanOutputsHandler removes the files produced by generate for the project in
// the working directory — the MCP counterpart of the `clean` CLI command. There
// is no interactive prompt over MCP; callers preview with dry_run.
func CleanOutputsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	dryRun := request.GetBool("dry_run", false)

	cfg, err := loadProjectConfig(ctx, request, baseDir)
	if err != nil {
		return ToolError(err)
	}

	gen := generator.NewGenerator(cfg)
	plan, err := gen.Clean("", generator.CleanOptions{
		DryRun:        dryRun,
		KeepGitignore: request.GetBool("keep_gitignore", false),
		KeepManifest:  request.GetBool("keep_manifest", false),
	})
	if err != nil {
		return ToolError(err)
	}

	message := "Generated files removed"
	if dryRun {
		message = "Dry run complete"
	}
	return ToolSuccess(map[string]interface{}{
		keyMessage:           message,
		keyConfig:            cfg.ConfigDir,
		"profile":            plan.Profile,
		"files":              relPathList(cfg.BaseDir, plan.Files),
		"directories":        relPathList(cfg.BaseDir, plan.Dirs),
		"manifest_removed":   plan.ManifestPath != "",
		"gitignore_stripped": plan.GitignoreEdited,
	})
}

// relPathList renders absolute paths relative to baseDir (slash-separated) for
// tidy, portable MCP responses, falling back to the original path on error.
func relPathList(baseDir string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if rel, err := filepath.Rel(baseDir, p); err == nil {
			out = append(out, filepath.ToSlash(rel))
		} else {
			out = append(out, p)
		}
	}
	return out
}

func ValidateConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)

	// Validate config
	cfg, err := loadProjectConfig(ctx, request, baseDir)
	if err != nil {
		result := map[string]interface{}{
			keyValid: false,
			keyError: err.Error(),
		}
		return ToolSuccess(result)
	}
	if err := cfg.Validate(); err != nil {
		result := map[string]interface{}{
			keyValid: false,
			keyError: err.Error(),
		}
		return ToolSuccess(result)
	}
	if cfg.LocalOverlay != nil {
		if err := schema.ValidateLocalFile(cfg.LocalOverlay.Path); err != nil {
			return ToolSuccess(map[string]interface{}{
				keyValid: false,
				keyError: fmt.Sprintf("local overlay %s: %s", cfg.LocalOverlay.Path, err.Error()),
			})
		}
	}
	result := map[string]interface{}{
		keyValid:   true,
		"warnings": []string{},
	}
	return ToolSuccess(result)
}

func loadProjectConfig(ctx context.Context, request *ToolRequest, baseDir string) (*config.Config, error) {
	// no_local loads the view a teammate without the machine-local overlay sees.
	var opts []config.LoadOption
	if request.GetBool("no_local", false) {
		opts = append(opts, config.WithoutLocal())
	}
	configFile := request.GetString("config_file", "")
	if configFile != "" {
		if !filepath.IsAbs(configFile) {
			configFile = filepath.Join(baseDir, configFile)
		}
		return config.LoadConfigFromFile(ctx, configFile, opts...)
	}
	configDirName := request.GetString("config_dir", "")
	if configDirName != "" {
		return config.LoadConfigFromDir(ctx, baseDir, configDirName, opts...)
	}
	return config.LoadConfig(ctx, baseDir, opts...)
}

// curatedPresets is the provider set emitted for the "all providers" and
// curated "popular" shortcuts. Every entry is a real built-in preset: a config
// listing a non-preset name (the old `popular`) fails validation.
var curatedPresets = []string{
	presetClaude, presetCursor, presetWindsurf, presetCopilot, presetGemini,
	presetAmp, presetCodex, presetCline, presetContinueDev,
}

func getPresetsFromProviders(providers []interface{}, allProviders, popularProviders bool) ([]string, bool) {
	if allProviders {
		return append([]string(nil), curatedPresets...), true
	}
	if popularProviders {
		return append([]string(nil), curatedPresets...), true
	}

	var presets []string
	var hasContinueDev bool

	providerMap := map[string]string{
		presetClaude:      presetClaude,
		presetCursor:      presetCursor,
		presetWindsurf:    presetWindsurf,
		presetCopilot:     presetCopilot,
		presetGemini:      presetGemini,
		presetAmp:         presetAmp,
		presetCodex:       presetCodex,
		presetCline:       presetCline,
		presetContinueDev: presetContinueDev,
	}

	for _, p := range providers {
		if provider, ok := p.(string); ok {
			if preset, exists := providerMap[provider]; exists {
				presets = append(presets, preset)
				if provider == presetContinueDev {
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
		configContent = templates.GenerateConfigWithPresets(projectName, []string{presetClaude})
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
		keyMessage: "Project initialized successfully",
		keyPath:    configPath,
	})
}
