package commands

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// llmsTxtLintFiles lints the llms.txt the llms-txt preset wrote (AR9P0-AR9P6).
// A file that does not exist yet is not a finding here: `generate --check`
// reports it. llms-full.txt is not an index and is not checked.
func llmsTxtLintFiles(cfg *config.Config) []lint.LLMsTxtFile {
	if cfg == nil || !cfg.LLMsTxtEnabled() {
		return nil
	}
	path := filepath.Join(cfg.BaseDir, filepath.FromSlash(cfg.LLMsTxtDir()), presets.LLMsTxtFileName)
	data, err := cfg.ReadExisting(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("Skipped the llms.txt checks", "error", err)
		}
		return nil
	}
	rel, relErr := filepath.Rel(cfg.BaseDir, path)
	if relErr != nil {
		rel = path
	}
	return []lint.LLMsTxtFile{{Path: rel, Findings: llmstxt.Validate(data)}}
}
