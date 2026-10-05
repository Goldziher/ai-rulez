package presets

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/settings"
)

// Base-relative paths of the documents the cursor, copilot and devin presets merge
// [permissions] into, and the file the codex preset owns outright. See the
// registry in merged_documents.go and the per-harness notes in
// internal/generator/settings.
const (
	MergedDocCursorCLI       = ".cursor/cli.json"
	MergedDocVSCodeSettings  = ".vscode/settings.json"
	MergedDocDevinConfig     = ".devin/config.json"
	MergedDocCodexPermission = settings.CodexRulesPath
)

// mergedPermissionsOutput renders [permissions] for a dialect into the shared
// document at relPath below baseDir: rules are owned one by one, so rules the
// consumer wrote in the same file survive generate and clean. It returns no
// output when no rule applies to the harness.
func mergedPermissionsOutput(cfg *config.Config, dialect, baseDir, relPath string, format docmerge.Format) ([]config.OutputFile, error) {
	docPath := filepath.Join(baseDir, filepath.FromSlash(relPath))
	keys, err := settings.PermissionKeys(cfg, dialect, docPath)
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	result, err := applyMergedDocumentAs(docPath, format, keys)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{{
		Path:           docPath,
		Content:        result.Body,
		PartiallyOwned: result.PartiallyOwned,
		MergeClaims:    result.Claims,
		Merge:          mergeSource(docPath, format, result),
	}}, nil
}

func (g *CursorPresetGenerator) permissionsOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedPermissionsOutput(cfg, config.HarnessCursor, baseDir, MergedDocCursorCLI, docmerge.FormatJSON)
}

func (g *CopilotPresetGenerator) permissionsOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedPermissionsOutput(cfg, config.HarnessCopilot, baseDir, MergedDocVSCodeSettings, docmerge.FormatJSONC)
}

func (g *DevinPresetGenerator) permissionsOutputs(cfg *config.Config, baseDir string) ([]config.OutputFile, error) {
	return mergedPermissionsOutput(cfg, "devin", baseDir, MergedDocDevinConfig, docmerge.FormatJSONC)
}

// permissionsOutputs renders .codex/rules/ai-rulez.rules, the Starlark rules file
// Codex loads next to its config. ai-rulez owns the file outright: Codex reads
// every *.rules file of the directory, so the user's own rules live in theirs.
func (g *CodexPresetGenerator) permissionsOutputs(cfg *config.Config, baseDir string) []config.OutputFile {
	body, ok := settings.CodexRules(cfg)
	if !ok {
		return nil
	}
	return []config.OutputFile{{Path: filepath.Join(baseDir, filepath.FromSlash(settings.CodexRulesPath)), Content: body}}
}

// permissionsLegacyClaims is what clean may take out of a merged document when no
// record of a merge exists: the rules the current configuration would render.
func permissionsLegacyClaims(cfg *config.Config, dialect string) []jsonmerge.Claim {
	keys, err := settings.PermissionKeys(cfg, dialect, "")
	if err != nil || len(keys) == 0 {
		return nil
	}
	result, err := docmerge.Apply("", docmerge.FormatJSON, keys)
	if err != nil {
		return nil
	}
	return result.Claims
}
