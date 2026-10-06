package presets

import (
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// Base-relative paths of the hooks documents the codex and cursor presets merge
// into. See the registry in merged_documents.go.
const (
	MergedDocCodexHooks       = settings.CodexHooksPath
	MergedDocCursorHooks      = settings.CursorHooksPath
	MergedDocAntigravityHooks = settings.AntigravityHooksPath
	MergedDocDevinHooks       = settings.DevinHooksPath
	// MergedDocCopilotHooks is owned outright by ai-rulez, not merged.
	MergedDocCopilotHooks = settings.CopilotHooksPath
)

// mergedHooksOutput renders [[hooks]] for a harness into the shared hooks
// document at relPath below baseDir: hook groups are owned one by one, so hooks
// the consumer wrote in the same file survive generate and clean. It returns no
// output when no hook applies to the harness.
func mergedHooksOutput(cfg *config.Config, harness, baseDir, relPath string) ([]config.OutputFile, error) {
	docPath := filepath.Join(baseDir, filepath.FromSlash(relPath))
	keys, err := settings.HookKeys(cfg, harness, docPath)
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	result, err := applyMergedDocument(docPath, keys)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{{
		Path:           docPath,
		Content:        result.Body,
		PartiallyOwned: result.PartiallyOwned,
		MergeClaims:    result.Claims,
		Merge:          mergeSource(docPath, docmerge.FormatJSON, result),
	}}, nil
}

func (g *CodexPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessCodex, baseDir, settings.CodexHooksPath)
}

func (g *CursorPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessCursor, baseDir, settings.CursorHooksPath)
}

func (g *AntigravityPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessAntigravity, baseDir, settings.AntigravityHooksPath)
}

// hooksOutputs renders .devin/hooks.v1.json. Devin has no user-level hooks file:
// with --user the hooks are the `hooks` key of config.json, the document
// [permissions] merge into, so settingsOutputs renders both there together.
func (g *DevinPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessDevin, baseDir, settings.DevinHooksPath)
}

// settingsOutputs renders what Devin reads from its settings files: [permissions]
// and [[hooks]]. In user scope both are keys of one document, so they are merged
// in one pass; two outputs for one path would replace each other.
func (g *DevinPresetGenerator) settingsOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	if !cfg.UserScope {
		outputs, err := g.permissionsOutputs(cfg, baseDir)
		if err != nil {
			return nil, err
		}
		hooks, err := g.hooksOutputs(cfg, baseDir)
		return append(outputs, hooks...), err
	}
	docPath := filepath.Join(baseDir, filepath.FromSlash(MergedDocDevinConfig))
	keys, err := settings.PermissionKeys(cfg, config.HarnessDevin, docPath)
	if err != nil {
		return nil, err
	}
	hookKeys, err := settings.HookKeys(cfg, config.HarnessDevin, docPath)
	if err != nil {
		return nil, err
	}
	if keys = append(keys, hookKeys...); len(keys) == 0 {
		return nil, nil
	}
	result, err := applyMergedDocumentAs(docPath, docmerge.FormatJSONC, keys)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{{
		Path: docPath, Content: result.Body, PartiallyOwned: result.PartiallyOwned, MergeClaims: result.Claims,
		Merge: mergeSource(docPath, docmerge.FormatJSONC, result),
	}}, nil
}

// hooksOutputs renders .github/hooks/ai-rulez.json, the file Copilot loads hooks
// from. Copilot reads every *.json in that directory, so ai-rulez owns its own
// file outright instead of merging into a shared one.
func (g *CopilotPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	keys, ok, err := settings.OwnedHooksKeys(cfg, config.HarnessCopilot)
	if err != nil || !ok {
		return nil, err
	}
	body, err := settings.RenderOwnedHooks(keys)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(baseDir, filepath.FromSlash(settings.CopilotHooksPath))
	return []config.OutputFile{{
		Path: path, Content: body,
		Merge: &config.MergeSource{Path: path, Format: config.MergeFormatOwnedHooks, Owned: keys},
	}}, nil
}

// hooksLegacyClaims is what clean may take out of a hooks document when no
// record of a merge exists: the groups the current configuration would render.
func hooksLegacyClaims(cfg *config.Config, harness string) []jsonmerge.Claim {
	keys, err := settings.HookKeys(cfg, harness, "")
	if err != nil || len(keys) == 0 {
		return nil
	}
	result, err := jsonmerge.Apply("", keys)
	if err != nil {
		return nil
	}
	return result.Claims
}

// hooksOutputs renders the Cline hook scripts: .clinerules/hooks/<Event>, one
// executable per event that runs the configured commands. A script the consumer
// wrote under the same name is theirs and is left alone, with a warning.
func (g *ClinePresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) []config.OutputFile {
	var outputs []config.OutputFile
	for _, script := range settings.ClineHookScripts(cfg) {
		path := filepath.Join(baseDir, filepath.FromSlash(settings.ClineHooksDir), script.Name)
		if existing, err := os.ReadFile(path); err == nil && !settings.IsClineHookGenerated(existing) { //nolint:gosec // path is derived from the layout
			cfg.Diag.Warn("[[hooks]] not generated for cline: "+filepath.ToSlash(settings.ClineHooksDir)+"/"+script.Name+
				" already exists and was not written by ai-rulez", "hint", "rename or remove it to let ai-rulez write the hook")
			continue
		}
		outputs = append(outputs, config.OutputFile{Path: path, RawContent: []byte(script.Body), Mode: 0o755})
	}
	return outputs
}
