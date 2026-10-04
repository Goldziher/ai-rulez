package presets

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/settings"
)

// Base-relative paths of the hooks documents the codex and cursor presets merge
// into. See the registry in merged_documents.go.
const (
	MergedDocCodexHooks  = settings.CodexHooksPath
	MergedDocCursorHooks = settings.CursorHooksPath
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
	}}, nil
}

func (g *CodexPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessCodex, baseDir, settings.CodexHooksPath)
}

func (g *CursorPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedHooksOutput(cfg, config.HarnessCursor, baseDir, settings.CursorHooksPath)
}

// hooksOutputs renders .github/hooks/ai-rulez.json, the file Copilot loads hooks
// from. Copilot reads every *.json in that directory, so ai-rulez owns its own
// file outright instead of merging into a shared one.
func (g *CopilotPresetGenerator) hooksOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	body, ok, err := settings.CopilotHooksDocument(cfg)
	if err != nil || !ok {
		return nil, err
	}
	return []config.OutputFile{{Path: filepath.Join(baseDir, filepath.FromSlash(settings.CopilotHooksPath)), Content: body}}, nil
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
