package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

const (
	mergeStrategyLocalOverride   = "local-override"
	mergeStrategyIncludeOverride = "include-override"
	mergeStrategyError           = "error"
	domainPrefix                 = "domains"
)

// Resolver resolves include sources and merges content
type Resolver struct {
	baseDir     string
	accessToken string
	visited     map[string]bool // Circular dependency detection
}

// NewResolver creates a new include resolver
func NewResolver(baseDir string, accessToken string) *Resolver {
	return &Resolver{
		baseDir:     baseDir,
		accessToken: accessToken,
		visited:     make(map[string]bool),
	}
}

// ResolveIncludes loads all includes and merges with local content
func (r *Resolver) ResolveIncludes(ctx context.Context, cfg *config.Config) (*config.ContentTree, error) {
	logger.Debug("Resolving includes", "count", len(cfg.Includes))

	// Start with local content
	mergedContent := cfg.Content
	if mergedContent == nil {
		mergedContent = &config.ContentTree{
			Domains: make(map[string]*config.Domain),
		}
	}

	// Process each include
	for i := range cfg.Includes {
		if err := r.processInclude(ctx, &mergedContent, &cfg.Includes[i]); err != nil {
			logger.Warn("Failed to process include", "name", cfg.Includes[i].Name, "error", err)
			// Continue processing other includes despite errors
			continue
		}

		logger.Debug("Successfully resolved include", "name", cfg.Includes[i].Name)
	}

	return mergedContent, nil
}

// processInclude handles a single include configuration
func (r *Resolver) processInclude(ctx context.Context, mergedContent **config.ContentTree, includeConf *config.IncludeConfig) error {
	// Check circular dependency
	if r.visited[includeConf.Name] {
		return oops.Errorf("circular dependency detected: %s", includeConf.Name)
	}
	r.visited[includeConf.Name] = true
	defer delete(r.visited, includeConf.Name) // Allow re-use in different branches

	// Create appropriate source
	source, err := r.createSource(includeConf)
	if err != nil {
		return oops.Wrapf(err, "failed to create source for include '%s'", includeConf.Name)
	}

	// nil source means local_override path not found — skip silently
	if source == nil {
		return nil
	}

	// Fetch content
	includedContent, err := source.Fetch(ctx)
	if err != nil {
		return oops.Wrapf(err, "failed to fetch include '%s'", includeConf.Name)
	}

	// Merge content
	merged, err := r.mergeContent(*mergedContent, includedContent, includeConf.MergeStrategy, includeConf.InstallTo)
	if err != nil {
		return oops.Wrapf(err, "failed to merge include '%s'", includeConf.Name)
	}

	*mergedContent = merged
	return nil
}

// createSource creates the appropriate source type (Git or Local).
// When local_override is set and the path exists, it is used instead of the
// configured source. If the local_override path does not exist, it returns
// (nil, nil) so the caller can skip this include silently.
func (r *Resolver) createSource(includeConf *config.IncludeConfig) (Source, error) {
	// Check for local override: use a local path instead of git
	if includeConf.LocalOverride != "" {
		localPath := r.resolveLocalOverride(includeConf)
		if localPath == "" {
			// Local override path does not exist — skip silently
			logger.Info("Skipping include (local_override path not found)",
				"name", includeConf.Name,
				"local_override", includeConf.LocalOverride)
			return nil, nil
		}
		logger.Info("Using local override for include",
			"name", includeConf.Name,
			"path", localPath)
		return NewLocalSource(
			includeConf.Name,
			localPath,
			r.baseDir,
			includeConf.Include,
		), nil
	}

	sourceType := DetectSourceType(includeConf.Source)

	switch sourceType {
	case SourceTypeLocal:
		return NewLocalSource(
			includeConf.Name,
			includeConf.Source,
			r.baseDir,
			includeConf.Include,
		), nil
	case SourceTypeGit:
		source, err := NewGitSource(
			includeConf.Name,
			includeConf.Source,
			includeConf.Path,
			includeConf.Ref,
			r.baseDir,
			includeConf.Include,
			r.accessToken,
		)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to create git source for include '%s'", includeConf.Name)
		}
		return source, nil
	default:
		return nil, oops.Errorf("unknown source type: %s", sourceType)
	}
}

// resolveLocalOverride resolves the local_override path, combining it with
// the include's Path field (e.g., local_override: "../ai-rulez" + path: "modules/core"
// resolves to "../ai-rulez/modules/core"). Returns the resolved absolute path
// if it exists, or empty string if it does not.
func (r *Resolver) resolveLocalOverride(includeConf *config.IncludeConfig) string {
	overridePath := includeConf.LocalOverride

	// Resolve relative to baseDir
	if !filepath.IsAbs(overridePath) {
		overridePath = filepath.Join(r.baseDir, overridePath)
	}
	overridePath = filepath.Clean(overridePath)

	// Append the Path sub-path (same as git source's path within repo)
	if includeConf.Path != "" {
		overridePath = filepath.Join(overridePath, includeConf.Path)
	}

	// Check existence
	info, err := os.Stat(overridePath)
	if err != nil || !info.IsDir() {
		return ""
	}

	return overridePath
}

// mergeContent merges two content trees based on strategy
func (r *Resolver) mergeContent(base, include *config.ContentTree, strategy, installTo string) (*config.ContentTree, error) {
	// Default strategy
	if strategy == "" {
		strategy = mergeStrategyLocalOverride
	}

	// If installTo is specified, treat as domain import
	if installTo != "" {
		return r.mergeDomainInstall(base, include, installTo, strategy)
	}

	// Otherwise, merge at root level
	return r.mergeRoot(base, include, strategy)
}

// mergeRoot merges content at root level
func (r *Resolver) mergeRoot(base, include *config.ContentTree, strategy string) (*config.ContentTree, error) {
	merged := &config.ContentTree{
		Domains: make(map[string]*config.Domain),
	}

	// Merge rules, context, skills, agents, commands based on strategy
	switch strategy {
	case mergeStrategyLocalOverride:
		// Base wins for root content, add non-conflicting from include
		merged.Rules = mergeContentFiles(base.Rules, include.Rules, true)
		merged.Context = mergeContentFiles(base.Context, include.Context, true)
		merged.Skills = mergeContentFiles(base.Skills, include.Skills, true)
		merged.Agents = mergeContentFiles(base.Agents, include.Agents, true)
		merged.Commands = mergeContentFiles(base.Commands, include.Commands, true)

	case mergeStrategyIncludeOverride:
		// Include wins for root content
		merged.Rules = mergeContentFiles(base.Rules, include.Rules, false)
		merged.Context = mergeContentFiles(base.Context, include.Context, false)
		merged.Skills = mergeContentFiles(base.Skills, include.Skills, false)
		merged.Agents = mergeContentFiles(base.Agents, include.Agents, false)
		merged.Commands = mergeContentFiles(base.Commands, include.Commands, false)

	case mergeStrategyError:
		// Fail on any conflict
		if detectConflicts(base.Rules, include.Rules) {
			return nil, oops.Errorf("conflict detected in rules between base and include")
		}
		if detectConflicts(base.Context, include.Context) {
			return nil, oops.Errorf("conflict detected in context between base and include")
		}
		if detectConflicts(base.Skills, include.Skills) {
			return nil, oops.Errorf("conflict detected in skills between base and include")
		}
		if detectConflicts(base.Agents, include.Agents) {
			return nil, oops.Errorf("conflict detected in agents between base and include")
		}
		if detectConflicts(base.Commands, include.Commands) {
			return nil, oops.Errorf("conflict detected in commands between base and include")
		}
		merged.Rules = make([]config.ContentFile, 0, len(base.Rules)+len(include.Rules))
		merged.Rules = append(merged.Rules, base.Rules...)
		merged.Rules = append(merged.Rules, include.Rules...)
		merged.Context = make([]config.ContentFile, 0, len(base.Context)+len(include.Context))
		merged.Context = append(merged.Context, base.Context...)
		merged.Context = append(merged.Context, include.Context...)
		merged.Skills = make([]config.ContentFile, 0, len(base.Skills)+len(include.Skills))
		merged.Skills = append(merged.Skills, base.Skills...)
		merged.Skills = append(merged.Skills, include.Skills...)
		merged.Agents = make([]config.ContentFile, 0, len(base.Agents)+len(include.Agents))
		merged.Agents = append(merged.Agents, base.Agents...)
		merged.Agents = append(merged.Agents, include.Agents...)
		merged.Commands = make([]config.ContentFile, 0, len(base.Commands)+len(include.Commands))
		merged.Commands = append(merged.Commands, base.Commands...)
		merged.Commands = append(merged.Commands, include.Commands...)

	default:
		return nil, oops.Errorf("unknown merge strategy: %s", strategy)
	}

	// Merge domains
	for name, domain := range base.Domains {
		merged.Domains[name] = domain
	}
	for name, domain := range include.Domains {
		if _, exists := merged.Domains[name]; !exists {
			domain.FromInclude = true
			merged.Domains[name] = domain
		}
		// If domain exists in both, we keep base version for now (can be enhanced)
	}

	return merged, nil
}

// mergeDomainInstall installs included content as a domain
func (r *Resolver) mergeDomainInstall(base, include *config.ContentTree, installTo, strategy string) (*config.ContentTree, error) {
	merged := &config.ContentTree{
		Rules:    base.Rules,
		Context:  base.Context,
		Skills:   base.Skills,
		Agents:   base.Agents,
		Commands: base.Commands,
		Domains:  make(map[string]*config.Domain),
	}

	// Copy existing domains
	for name, domain := range base.Domains {
		merged.Domains[name] = domain
	}

	// Extract domain name from installTo (e.g., "domains/backend" → "backend")
	domainName := extractDomainName(installTo)

	if domainName == "" {
		return nil, oops.Errorf("invalid installTo path: %s", installTo)
	}

	// Create/merge domain
	var targetDomain *config.Domain
	if existing, ok := merged.Domains[domainName]; ok {
		// Domain exists, merge content based on strategy
		targetDomain = r.mergeDomainContent(existing, include, strategy)
	} else {
		// Create new domain from included content
		targetDomain = &config.Domain{
			Name:        domainName,
			Rules:       include.Rules,
			Context:     include.Context,
			Skills:      include.Skills,
			Agents:      include.Agents,
			Commands:    include.Commands,
			FromInclude: true,
		}
	}

	merged.Domains[domainName] = targetDomain

	return merged, nil
}

// mergeDomainContent merges content into an existing domain
func (r *Resolver) mergeDomainContent(target *config.Domain, include *config.ContentTree, strategy string) *config.Domain {
	merged := &config.Domain{
		Name: target.Name,
	}

	switch strategy {
	case mergeStrategyLocalOverride:
		// Domain content wins
		merged.Rules = mergeContentFiles(target.Rules, include.Rules, true)
		merged.Context = mergeContentFiles(target.Context, include.Context, true)
		merged.Skills = mergeContentFiles(target.Skills, include.Skills, true)
		merged.Agents = mergeContentFiles(target.Agents, include.Agents, true)
		merged.Commands = mergeContentFiles(target.Commands, include.Commands, true)

	case mergeStrategyIncludeOverride:
		// Include content wins
		merged.Rules = mergeContentFiles(target.Rules, include.Rules, false)
		merged.Context = mergeContentFiles(target.Context, include.Context, false)
		merged.Skills = mergeContentFiles(target.Skills, include.Skills, false)
		merged.Agents = mergeContentFiles(target.Agents, include.Agents, false)
		merged.Commands = mergeContentFiles(target.Commands, include.Commands, false)

	default:
		// Default to local-override
		merged.Rules = mergeContentFiles(target.Rules, include.Rules, true)
		merged.Context = mergeContentFiles(target.Context, include.Context, true)
		merged.Skills = mergeContentFiles(target.Skills, include.Skills, true)
		merged.Agents = mergeContentFiles(target.Agents, include.Agents, true)
		merged.Commands = mergeContentFiles(target.Commands, include.Commands, true)
	}

	// Copy MCPServers
	if target.MCPServers != nil {
		merged.MCPServers = target.MCPServers
	}

	return merged
}

// mergeContentFiles merges two slices of content files
// If baseWins is true, base files take precedence; otherwise include files win
func mergeContentFiles(base, include []config.ContentFile, baseWins bool) []config.ContentFile {
	// Build a map of base files by name
	baseMap := make(map[string]config.ContentFile)
	for _, file := range base {
		baseMap[file.Name] = file
	}

	// Build result with include files
	result := make([]config.ContentFile, 0, len(base)+len(include))
	seen := make(map[string]bool)

	if baseWins {
		// Add all base files
		for _, file := range base {
			result = append(result, file)
			seen[file.Name] = true
		}
		// Add include files that don't conflict
		for _, file := range include {
			if !seen[file.Name] {
				result = append(result, file)
				seen[file.Name] = true
			}
		}
	} else {
		// Add all include files first (preserve slice order for deterministic output)
		for _, file := range include {
			if !seen[file.Name] {
				result = append(result, file)
				seen[file.Name] = true
			}
		}
		// Add base files that don't conflict
		for _, file := range base {
			if !seen[file.Name] {
				result = append(result, file)
				seen[file.Name] = true
			}
		}
	}

	return result
}

// detectConflicts checks if two content file slices have conflicting names
func detectConflicts(base, include []config.ContentFile) bool {
	baseNames := make(map[string]bool)
	for _, file := range base {
		baseNames[file.Name] = true
	}

	for _, file := range include {
		if baseNames[file.Name] {
			return true
		}
	}

	return false
}

// extractDomainName extracts domain name from installTo path
func extractDomainName(installTo string) string {
	// "domains/backend/" → "backend"
	// "domains/frontend/rules" → "frontend"
	parts := strings.Split(strings.Trim(installTo, "/"), "/")
	if len(parts) >= 2 && parts[0] == domainPrefix {
		return parts[1]
	}
	// If it doesn't follow the standard format, use the installTo as-is
	if installTo != "" && installTo != domainPrefix+"/" {
		return installTo
	}
	return ""
}
