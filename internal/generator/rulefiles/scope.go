package rulefiles

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// ScopeOf returns the ScopeInfo of the scope cfg is generating, or the zero
// value for the project root.
func ScopeOf(cfg *config.Config) ScopeInfo {
	if !InScope(cfg) {
		return ScopeInfo{}
	}
	return ScopeInfo{Slug: cfg.Run.Scope.Slug, Prefix: cleanPrefix(cfg.Run.Scope.Path)}
}

// NewScopeRun describes the scope at scopePath for presets: its slash-form
// path relative to the project root and the file-name qualifier its rule files
// carry. The path must pass config.ValidateScopePath.
func NewScopeRun(rootDir, scopePath string) (*config.ScopeRun, error) {
	if err := config.ValidateScopePath(scopePath); err != nil {
		return nil, err
	}
	clean := cleanPrefix(scopePath)
	slug := ID(clean)
	if clean == "" || slug == "" {
		return nil, oops.With("scope", scopePath).Errorf("scope path %q does not name a subdirectory", scopePath)
	}
	return &config.ScopeRun{Path: clean, Slug: slug, RootDir: rootDir}, nil
}

// InScope reports whether cfg generates the outputs of a monorepo scope.
func InScope(cfg *config.Config) bool {
	return cfg != nil && cfg.Run != nil && cfg.Run.Scope != nil
}

// RulesRoot is the directory rule files are written under: tools read rules
// folders at the workspace root only, so a scope writes into the project root
// rather than baseDir (the scope directory).
func RulesRoot(cfg *config.Config, baseDir string) string {
	if InScope(cfg) && cfg.Run.Scope.RootDir != "" {
		return cfg.Run.Scope.RootDir
	}
	return baseDir
}

// RulesDirPath joins the rules root with a target-relative file name.
func RulesDirPath(cfg *config.Config, baseDir string, t Target, name string) string {
	return filepath.Join(RulesRoot(cfg, baseDir), filepath.FromSlash(path.Join(t.Dir, name)))
}

// RegistryFor returns the Registry shared by the root and scope runs of one
// generation for a preset. Without a generation (cfg.Run unset) it returns a
// registry local to the caller.
func RegistryFor(cfg *config.Config, preset string) *Registry {
	if cfg == nil || cfg.Run == nil {
		return NewRegistry()
	}
	return &Registry{claims: cfg.Run.ClaimsFor(preset), root: cfg.ConfigDir}
}

// WarnUnreadScopeFile warns, in a scope run, that rules and context left inline
// in file are never loaded because the tool reads file at the repository root
// only. It is a no-op outside a scope or when nothing is inline.
func WarnUnreadScopeFile(cfg *config.Config, preset, file string, rules, context []config.ContentFile) {
	if !InScope(cfg) || len(rules)+len(context) == 0 {
		return
	}
	names := make([]string, 0, len(rules)+len(context))
	for _, r := range rules {
		names = append(names, "rule "+r.Name)
	}
	for _, c := range context {
		names = append(names, "context "+c.Name)
	}
	warnSink()(preset+" reads "+file+" at the repository root only, so these items in the scope's copy are never loaded; "+
		"use path-scoped rules or [rules] mode = \"split\"",
		"scope", cfg.Run.Scope.Path, "items", strings.Join(names, ", "))
}

// WarnScopeLegacyRules warns that a preset without split rule files keeps a
// scope's rules in the scope directory, where the tool may not load them.
func WarnScopeLegacyRules(cfg *config.Config, preset, dir string) {
	if !InScope(cfg) {
		return
	}
	warnSink()(preset+" does not support split rule files, so a scope's rules are written to "+dir+
		" inside the scope directory",
		"scope", cfg.Run.Scope.Path)
}
