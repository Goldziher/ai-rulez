package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/Goldziher/ai-rulez/v5/internal/preflight"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

func ReadConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)

	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return ToolError(fmt.Errorf("failed to get directory: %w", err))
	}

	// The editable view is the shared config; the machine-local overlay is
	// reported separately and by key path only (it may hold secrets).
	cfg, err := project.Load(ctx, dir, config.WithoutLocal())
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
			keySource: incl.RedactURL(inc.Source),
		}
		if inc.Path != "" {
			entry[keyPath] = inc.Path
		}
		if ref := inc.RequestedRef(); ref != "" {
			entry["ref"] = ref
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
		"agents_md":    cfg.AgentsMD,
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

const keyError = "error"

// localOverlayInfo describes the machine-local overlay beside cfg: its path and
// the key paths it sets, never its values (it may hold secrets). Nil when there
// is no overlay.
func localOverlayInfo(cfg *config.Config) map[string]interface{} {
	overlay, err := config.ReadLocalOverlay(cfg.ConfigDir)
	if err != nil {
		return map[string]interface{}{keyError: redactError(err)}
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
	if _, ok := args["agents_md"]; ok {
		cfg.AgentsMD = request.GetBool("agents_md", false)
		updated = append(updated, "agents_md")
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

	if request.GetBool("local", false) {
		return updateLocalConfig(ctx, request, dir)
	}

	cfg, err := project.Load(ctx, dir, config.WithoutLocal())
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

var recursiveConfigFiles = []string{"config.toml"}

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
	payload, err := generateDirectory(ctx, request, dir, dryRun)
	if err != nil {
		entry["error"] = err.Error()
		return entry
	}
	entry[keySuccess] = true
	if commands, ok := payload["new_commands"]; ok {
		entry["new_commands"] = commands
	}
	return entry
}

func generateForDirectory(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (*mcp.CallToolResult, error) {
	payload, err := generateDirectory(ctx, request, baseDir, dryRun)
	if err != nil {
		return ToolError(err)
	}
	return ToolSuccess(payload)
}

func generateDirectory(ctx context.Context, request *ToolRequest, baseDir string, dryRun bool) (map[string]interface{}, error) {
	// generate_outputs writes outputs: an enforced lock is required, as by `generate`.
	cfg, err := loadProjectConfigWith(ctx, request, baseDir, config.WithLockPolicy(config.LockPolicy{RequireWhenEnforced: true}))
	if err != nil {
		return nil, err
	}
	if err := config.CheckPolicy(cfg); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if err := cfg.Validate(); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	gen := generator.NewGenerator(cfg)
	gen.SetContext(ctx)
	if dryRun {
		plan, err := gen.DryRun("")
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		return map[string]interface{}{
			keyMessage: "Dry run complete",
			keyConfig:  cfg.ConfigDir,
			"plan":     plan,
		}, nil
	}
	// There is no terminal to warn over MCP: the commands this run makes the
	// harnesses run go into the tool result and to stderr (stdout is the protocol).
	newCommands := preflight.NewCommands(cfg, false)
	if err := gen.Generate(""); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	preflight.Remember(cfg)
	result := map[string]interface{}{
		keyMessage: "Outputs generated successfully",
		keyConfig:  cfg.ConfigDir,
	}
	if len(newCommands) > 0 && !preflight.AckedByEnv() {
		summary := preflight.Summary(cfg, newCommands)
		fmt.Fprint(os.Stderr, summary)
		result["new_commands"] = newCommands
	}
	return result, nil
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

// redactError renders err for a tool response with URL credentials removed and
// any quoted source values dropped from decode errors: validation of a merged
// config can quote machine-local values (secrets).
func redactError(err error) string {
	return incl.RedactURL(config.SanitizeDecodeError(err).Error())
}

// ValidateConfigHandler runs the structural checks of validate_config: the
// load, structural validation, the organization policy and the machine-local
// overlay schema. Anything it finds is an error result carrying the document
// {"valid": false, "error": ...}; a clean configuration is a success result
// with {"valid": true}. ValidateConfigWith adds the lint on top.
func ValidateConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := validateStructure(ctx, request)
	if err != nil {
		return invalidConfig(err)
	}
	return ToolSuccess(map[string]interface{}{
		keyValid:   true,
		"warnings": []string{},
		keyConfig:  cfg.ConfigDir,
	})
}

// validateStructure loads the configuration and applies the structural checks.
func validateStructure(ctx context.Context, request *ToolRequest) (*config.Config, error) {
	cfg, err := loadProjectConfigWith(ctx, request, workingDir(request), config.WithFrontmatterErrors())
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if err := config.CheckPolicy(cfg); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if cfg.LocalOverlay != nil {
		if err := schema.ValidateLocalFile(cfg.LocalOverlay.Path); err != nil {
			return nil, fmt.Errorf("local overlay %s: %s", cfg.LocalOverlay.Path, redactError(err))
		}
	}
	return cfg, nil
}

// invalidConfig is the error result of a configuration that does not validate:
// IsError is set so a client never reads it as success, and the text is the
// {"valid": false, "error": ..., "hint": ...} document.
func invalidConfig(err error) (*mcp.CallToolResult, error) {
	doc := map[string]interface{}{keyValid: false, keyError: redactError(err)}
	if hint := hintOf(err); hint != "" {
		doc["hint"] = hint
	}
	return toolErrorDocument(doc)
}

func loadProjectConfig(ctx context.Context, request *ToolRequest, baseDir string) (*config.Config, error) {
	return loadProjectConfigWith(ctx, request, baseDir)
}

// loadProjectConfigWith is loadProjectConfig plus extra load options.
func loadProjectConfigWith(ctx context.Context, request *ToolRequest, baseDir string, extra ...config.LoadOption) (*config.Config, error) {
	// no_local loads the view a teammate without the machine-local overlay sees.
	opts := append([]config.LoadOption(nil), extra...)
	if request.GetBool("no_local", false) {
		opts = append(opts, config.WithoutLocal())
	}
	configFile := request.GetString("config_file", "")
	if configFile != "" {
		if !filepath.IsAbs(configFile) {
			configFile = filepath.Join(baseDir, configFile)
		}
		return project.LoadFile(ctx, configFile, opts...)
	}
	configDirName := request.GetString("config_dir", "")
	if configDirName != "" {
		return project.LoadDir(ctx, baseDir, configDirName, opts...)
	}
	return project.Load(ctx, baseDir, opts...)
}

// curatedPresets is the provider set emitted for the "all providers" and
// curated "popular" shortcuts. Every entry is a real built-in preset: a config
// listing a non-preset name (the old `popular`) fails validation.
var curatedPresets = []string{
	presetClaude, presetCursor, presetDevin, presetCopilot, presetGemini,
	presetAmp, presetCodex, presetCline,
}

func getPresetsFromProviders(providers []interface{}, allProviders, popularProviders bool) ([]string, error) {
	if allProviders || popularProviders {
		return append([]string(nil), curatedPresets...), nil
	}

	known := map[string]bool{}
	for _, preset := range curatedPresets {
		known[preset] = true
	}

	var presets []string
	var unknown []string
	for _, p := range providers {
		provider, ok := p.(string)
		switch {
		case ok && known[provider]:
			presets = append(presets, provider)
		case ok:
			unknown = append(unknown, provider)
		default:
			unknown = append(unknown, fmt.Sprintf("%v", p))
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown provider(s): %s (supported: %s)", strings.Join(unknown, ", "), strings.Join(curatedPresets, ", "))
	}
	return presets, nil
}

func InitProjectHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	projectName := request.GetString("project_name", "")
	if strings.TrimSpace(projectName) == "" {
		// name is required and non-empty; Base of "." would be "." so resolve it first.
		abs, err := filepath.Abs(baseDir)
		if err != nil {
			abs = baseDir
		}
		projectName = filepath.Base(abs)
	}
	providersInterface := request.GetArguments()["providers"]
	allProviders := request.GetBool("all_providers", false)
	popularProviders := request.GetBool("popular_providers", false)

	var providers []interface{}
	if providersSlice, ok := providersInterface.([]interface{}); ok {
		providers = providersSlice
	}

	presets, err := getPresetsFromProviders(providers, allProviders, popularProviders)
	if err != nil {
		return ToolError(err)
	}

	// The same V4 TOML layout `ai-rulez init` creates.
	aiRulesDir := filepath.Join(baseDir, ".ai-rulez")
	configPath := filepath.Join(aiRulesDir, "config.toml")
	if existing := existingConfigFile(aiRulesDir); existing != "" {
		return ToolError(fmt.Errorf("%s already exists; init_project does not overwrite a configuration", existing))
	}
	for _, sub := range templates.InitSubdirs {
		if err := os.MkdirAll(filepath.Join(aiRulesDir, sub), 0o755); err != nil {
			return ToolError(fmt.Errorf("failed to create %s directory: %w", sub, err))
		}
	}
	if err := os.WriteFile(configPath, []byte(templates.InitConfigTOML(projectName, presets)), 0o644); err != nil {
		return ToolError(fmt.Errorf("failed to write config file: %w", err))
	}
	// The configuration directory is an OKF bundle: the root index.md marks it.
	if err := okfbridge.RefreshIndexes(ctx, aiRulesDir); err != nil {
		return ToolError(fmt.Errorf("failed to write the index.md files: %w", err))
	}

	return ToolSuccess(map[string]interface{}{
		keyMessage: "Project initialized successfully",
		keyPath:    configPath,
	})
}

// existingConfigFile returns the config file already in dir, or "".
func existingConfigFile(dir string) string {
	for _, name := range []string{"config.toml"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
