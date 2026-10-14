package config

import (
	"context"
	"path/filepath"
)

// LoadImplicit builds the configuration of a project that has none, so the
// read-only commands (validate, scan, doctor, okf validate) can lint what the
// project holds without a config file: no presets, includes, hooks or MCP
// servers, the default [lint] settings, and the content tree of .ai-rulez/ (or
// .config/ai-rulez/) when it exists even though it has no config.toml. The files
// that follow a convention outside that tree (AGENTS.md, SKILL.md, .claude/,
// .agents/, ...) are found by the lint run, which treats them as hand-written.
//
// Nothing is read from or written to a config file and nothing is generated; the
// result is marked Implicit, and the commands that need a real configuration
// (generate, clean, lock) never accept it.
func LoadImplicit(ctx context.Context, baseDir string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	v, absDir, err := lo.baseView(ctx, baseDir)
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(absDir, aiRulezDirName)
	if !v.Exists(configDir) && v.Exists(filepath.Join(absDir, filepath.FromSlash(altConfigDirName))) {
		configDir = filepath.Join(absDir, filepath.FromSlash(altConfigDirName))
	}
	cfg := &Config{Version: ConfigVersionV5, Name: filepath.Base(absDir), Implicit: true}
	lo.withoutLocal = true // the machine-local overlay belongs to a configuration
	cfg, err = finishLoadConfig(ctx, v, cfg, absDir, configDir, lo)
	if err != nil {
		return nil, err
	}
	cfg.ConfigFilePath = "" // there is no file: ConfigPath names none
	return cfg, nil
}
