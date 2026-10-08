package includes

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
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
	memo        *fetchMemo      // shared fetch cache for this run (see Config.IncludeMemo)
	cfg         *config.Config
	lock        *lockfile.File // ai-rulez.lock of the project being resolved (nil: none)
	// okfScan runs the security scan over the text of an OKF include before it
	// is converted. The caller supplies it (see Resolvers); nil skips the scan.
	okfScan okfbridge.Scanner
}

// WithOKFScan sets the security scan an OKF include runs over its text before
// conversion. It returns the resolver for chaining.
func (r *Resolver) WithOKFScan(scan okfbridge.Scanner) *Resolver {
	r.okfScan = scan
	return r
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
	cfg.Log().Debug("Resolving includes", "count", len(cfg.Includes))
	ctx = policyContext(ctx, cfg)
	r.memo = memoFor(cfg)
	r.cfg = cfg
	lock, err := loadLockFor(cfg)
	if err != nil {
		return nil, err
	}
	r.lock = lock

	// Start with local content
	mergedContent := cfg.Content
	if mergedContent == nil {
		mergedContent = &config.ContentTree{
			Domains: make(map[string]*config.Domain),
		}
	}

	// Process each include
	var violations, failures []error
	var skipped []includeFailure
	for i := range cfg.Includes {
		if err := r.processInclude(ctx, &mergedContent, &cfg.Includes[i]); err != nil {
			switch {
			case errors.Is(err, config.ErrLockViolation):
				violations = append(violations, err)
			case errors.Is(err, config.ErrIncludeOutsideProject):
				// A path the committed config must not name is an error, not a skipped include.
				return nil, oops.Wrapf(err, "include %q", cfg.Includes[i].Name)
			default:
				// Both sentinels make the config loader propagate the error
				// instead of continuing with local content only; a lock
				// violation also exits as drift.
				sentinel := config.ErrIncludeUnresolved
				if strictLock(cfg) {
					sentinel = config.ErrLockViolation
				}
				failures = append(failures, oops.Wrapf(errors.Join(sentinel, err), "include %q", cfg.Includes[i].Name))
			}
			skipped = append(skipped, includeFailure{name: cfg.Includes[i].Name, err: err})
			// Continue processing other includes despite errors
			continue
		}

		cfg.Log().Debug("Successfully resolved include", "name", cfg.Includes[i].Name)
	}

	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}
	// A missing include is not a warning: generating without it silently produces
	// outputs the committed configuration never described, and a CI gate on
	// `generate --check` would pass on a broken checkout. Only an explicit offline
	// run (--offline) keeps the warning, since it asked for cached content only,
	// and `lock` (refresh), which collects the failures and reports them itself.
	if len(failures) > 0 && ((!tolerated(ctx) && cfg.LockPolicy.Mode != LockRefresh) || strictLock(cfg)) {
		return nil, errors.Join(failures...)
	}
	// Only a run that goes on without the include warns: otherwise the returned
	// error already carries the same text, and printing both says it twice.
	for _, f := range skipped {
		cfg.Warn("Failed to process include", "name", f.name, "error", f.err)
	}
	return mergedContent, nil
}

// includeFailure is an include that could not be processed.
type includeFailure struct {
	name string
	err  error
}

// tolerated reports whether the run goes on without an include it cannot
// resolve: an explicit offline run, or a command that only inventories sources.
func tolerated(ctx context.Context) bool {
	return config.OfflineIncludes(ctx) || config.UnresolvedIncludesTolerated(ctx)
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
	source, err := r.createSource(ctx, includeConf)
	if err != nil {
		return oops.Wrapf(err, "failed to create source for include '%s'", includeConf.Name)
	}

	// nil source means local_override path not found — skip silently
	if source == nil {
		return nil
	}

	// Fetch content (once per run per configured source)
	key := memoKey(r.baseDir, includeConf, r.accessToken != "")
	var includedContent *config.ContentTree
	if r.memo != nil && key != "" {
		includedContent, err = r.memo.fetchTree(key,
			func() (*config.ContentTree, error) { return source.Fetch(ctx) },
			func() (*config.ContentTree, error) { return source.Fetch(config.WithOfflineIncludes(ctx)) })
	} else {
		includedContent, err = source.Fetch(ctx)
	}
	if err != nil {
		return oops.Wrapf(err, "failed to fetch include '%s'", includeConf.Name)
	}

	// Merge content
	merged, err := r.mergeContent(*mergedContent, includedContent, includeConf.MergeStrategy, includeConf.InstallTo)
	if err != nil {
		return oops.Wrapf(err, "failed to merge include '%s'", includeConf.Name)
	}

	// Verifier declaration files travel with the tree, tagged with their include.
	merged.ImportedVerifiers = append([]config.ImportedVerifierFile(nil), (*mergedContent).ImportedVerifiers...)
	for _, f := range includedContent.ImportedVerifiers {
		f.Include = includeConf.Name
		merged.ImportedVerifiers = append(merged.ImportedVerifiers, f)
	}
	*mergedContent = merged
	return nil
}

// createSource creates the appropriate source type (Git or Local).
// When local_override is set and the path exists, it is used instead of the
// configured source. If the local_override path does not exist, it returns
// (nil, nil) so the caller can skip this include silently.
func (r *Resolver) createSource(ctx context.Context, includeConf *config.IncludeConfig) (Source, error) { //nolint:gocyclo // a flat sequence of independent checks; splitting it scatters the rules
	if includeConf.Format == config.IncludeFormatOKF {
		return r.createOKFSource(ctx, includeConf)
	}
	// Check for local override: use a local path instead of git
	if includeConf.LocalOverride != "" && !refreshing(r.cfg, lockfile.KindInclude, includeConf.Name) {
		if err := checkLocalOverride(r.cfg, "includes", includeConf.Name); err != nil {
			return nil, err
		}
		localPath := r.resolveLocalOverride(includeConf)
		if localPath == "" {
			// Local override path does not exist — skip silently
			logger.FromContext(ctx).Info("Skipping include (local_override path not found)",
				"name", includeConf.Name,
				"local_override", includeConf.LocalOverride)
			return nil, nil
		}
		if err := checkInsideProject(r.cfg, r.baseDir, "local_override", includeConf.Name, localPath); err != nil {
			return nil, err
		}
		logger.FromContext(ctx).Info("Using local override for include",
			"name", includeConf.Name,
			"path", localPath)
		return NewLocalSource(
			includeConf.Name,
			localPath,
			r.baseDir,
			includeConf.Include,
		).In(viewFor(r.cfg, r.baseDir)), nil
	}

	sourceType := DetectSourceType(includeConf.Source)

	switch sourceType {
	case SourceTypeLocal:
		if err := checkInsideProject(r.cfg, r.baseDir, "source", includeConf.Name, includeConf.Source); err != nil {
			return nil, err
		}
		return NewLocalSource(
			includeConf.Name,
			includeConf.Source,
			r.baseDir,
			includeConf.Include,
		).In(viewFor(r.cfg, r.baseDir)), nil
	case SourceTypeGit:
		// A file:// URL is a local path in git clothing: it stays inside the project too.
		if path, ok := lockfile.FileURLPath(includeConf.Source); ok && !lockfile.AllowFileURLsOutside(r.host().Env) {
			if err := checkInsideProject(r.cfg, r.baseDir, "source", includeConf.Name, path); err != nil {
				return nil, err
			}
		}
		w := withVersion(lockfile.Want{
			Kind: lockfile.KindInclude, Name: includeConf.Name, Source: lockSource(r.baseDir, includeConf.Source),
			Path: includeConf.Path, Ref: includeConf.Ref,
		}, includeConf.VersionSpec())
		p, err := pinFor(r.cfg, r.lock, w)
		if err != nil {
			return nil, err
		}
		ref, err := versionRef(ctx, r.cfg, r.lock, w, p, stripGitPlus(includeConf.Source), r.accessToken, r.baseDir)
		if err != nil {
			return nil, err
		}
		source, err := NewGitSourceIn(
			r.host(),
			includeConf.Name,
			includeConf.Source,
			includeConf.Path,
			ref,
			r.baseDir,
			includeConf.Include,
			r.accessToken,
		)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to create git source for include '%s'", includeConf.Name)
		}
		source.pin, source.state = p, stateFor(r.cfg)
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
	info, err := viewFor(r.cfg, r.baseDir).For(overridePath).Stat(overridePath)
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
		merged.Checks = mergeContentFiles(base.Checks, include.Checks, true)

	case mergeStrategyIncludeOverride:
		// Include wins for root content
		merged.Rules = mergeContentFiles(base.Rules, include.Rules, false)
		merged.Context = mergeContentFiles(base.Context, include.Context, false)
		merged.Skills = mergeContentFiles(base.Skills, include.Skills, false)
		merged.Agents = mergeContentFiles(base.Agents, include.Agents, false)
		merged.Commands = mergeContentFiles(base.Commands, include.Commands, false)
		merged.Checks = mergeContentFiles(base.Checks, include.Checks, false)

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
		if detectConflicts(base.Checks, include.Checks) {
			return nil, oops.Errorf("conflict detected in checks between base and include")
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
		merged.Checks = make([]config.ContentFile, 0, len(base.Checks)+len(include.Checks))
		merged.Checks = append(merged.Checks, base.Checks...)
		merged.Checks = append(merged.Checks, include.Checks...)

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
		Checks:   base.Checks,
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
			Checks:      include.Checks,
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
		merged.Checks = mergeContentFiles(target.Checks, include.Checks, true)

	case mergeStrategyIncludeOverride:
		// Include content wins
		merged.Rules = mergeContentFiles(target.Rules, include.Rules, false)
		merged.Context = mergeContentFiles(target.Context, include.Context, false)
		merged.Skills = mergeContentFiles(target.Skills, include.Skills, false)
		merged.Agents = mergeContentFiles(target.Agents, include.Agents, false)
		merged.Commands = mergeContentFiles(target.Commands, include.Commands, false)
		merged.Checks = mergeContentFiles(target.Checks, include.Checks, false)

	default:
		// Default to local-override
		merged.Rules = mergeContentFiles(target.Rules, include.Rules, true)
		merged.Context = mergeContentFiles(target.Context, include.Context, true)
		merged.Skills = mergeContentFiles(target.Skills, include.Skills, true)
		merged.Agents = mergeContentFiles(target.Agents, include.Agents, true)
		merged.Commands = mergeContentFiles(target.Commands, include.Commands, true)
		merged.Checks = mergeContentFiles(target.Checks, include.Checks, true)
	}

	return merged
}

// mergeContentFiles merges two slices of content files
// If baseWins is true, base files take precedence; otherwise include files win
func mergeContentFiles(base, include []config.ContentFile, baseWins bool) []config.ContentFile {
	// Build a map of base files by name
	baseMap := make(map[string]config.ContentFile)
	for i := range base {
		baseMap[base[i].Name] = base[i]
	}

	// Build result with include files
	result := make([]config.ContentFile, 0, len(base)+len(include))
	seen := make(map[string]bool)

	if baseWins {
		// Add all base files
		for i := range base {
			result = append(result, base[i])
			seen[base[i].Name] = true
		}
		// Add include files that don't conflict
		for i := range include {
			if !seen[include[i].Name] {
				result = append(result, include[i])
				seen[include[i].Name] = true
			}
		}
	} else {
		// Add all include files first (preserve slice order for deterministic output)
		for i := range include {
			if !seen[include[i].Name] {
				result = append(result, include[i])
				seen[include[i].Name] = true
			}
		}
		// Add base files that don't conflict
		for i := range base {
			if !seen[base[i].Name] {
				result = append(result, base[i])
				seen[base[i].Name] = true
			}
		}
	}

	return result
}

// detectConflicts checks if two content file slices have conflicting names
func detectConflicts(base, include []config.ContentFile) bool {
	baseNames := make(map[string]bool)
	for i := range base {
		baseNames[base[i].Name] = true
	}

	for i := range include {
		if baseNames[include[i].Name] {
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

// host is the ambient host of the config being resolved (the real process when
// there is none).
func (r *Resolver) host() ambient.Host {
	if r.cfg == nil {
		return ambient.Host{}
	}
	return r.cfg.Host
}
