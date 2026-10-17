package includes

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// ResolveInstalledSkills resolves all configured installed skills and returns their content
func ResolveInstalledSkills(ctx context.Context, cfg *config.Config, accessToken string) ([]config.ContentFile, error) {
	cfg.Log().Debug("Resolving installed skills", "count", len(cfg.InstalledSkills))
	ctx = policyContext(ctx, cfg)

	var skills []config.ContentFile
	var violations, failures []error

	lock, err := loadLockFor(cfg)
	if err != nil {
		return nil, err
	}

	for i := range cfg.InstalledSkills {
		skillConf := &cfg.InstalledSkills[i]

		resolved, err := memoFor(cfg).fetchSkills(memoKey(cfg.BaseDir, skillConf, accessToken != ""),
			func() ([]config.ContentFile, error) {
				return resolveInstalledSkill(ctx, cfg, lock, skillConf, accessToken)
			},
			func() ([]config.ContentFile, error) {
				return resolveInstalledSkill(config.WithOfflineIncludes(ctx), cfg, lock, skillConf, accessToken)
			})
		if errors.Is(err, errSkillSkipped) {
			continue
		}
		if err != nil {
			switch {
			case errors.Is(err, config.ErrIncludeOutsideProject):
				// A path the committed config must not name is an error, not a skipped skill.
				failures = append(failures, oops.Wrapf(errors.Join(config.ErrSkillUnresolved, err), "installed skill %q", skillConf.Name))
			case errors.Is(err, config.ErrLockViolation) || strictLock(cfg):
				violations = append(violations, oops.Wrapf(errors.Join(config.ErrLockViolation, err), "installed skill %q", skillConf.Name))
			case !tolerated(ctx) && cfg.LockPolicy.Mode != LockRefresh:
				// Like an include: going on without the skill would drop its
				// outputs as stale and let `generate --check` pass.
				failures = append(failures, oops.Wrapf(errors.Join(config.ErrSkillUnresolved, err), "installed skill %q", skillConf.Name))
			default:
				cfg.Warn("Failed to resolve installed skill", "name", skillConf.Name, "error", err)
			}
			continue
		}

		// Stamp the entry's scope on every file it resolved, so a multi-skill
		// entry keeps it even though the resolved names no longer match the entry
		// name.
		for j := range resolved {
			resolved[j].Profiles = skillConf.Profiles
			resolved[j].Domain = skillConf.Domain
			resolved[j].Trust = skillConf.Trust
		}
		skills = append(skills, resolved...)
		cfg.Log().Debug("Successfully resolved installed skill", "name", skillConf.Name, "resolved", len(resolved))
	}

	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}
	if len(failures) > 0 {
		return nil, errors.Join(failures...)
	}
	return skills, nil
}

// errSkillSkipped is a skill whose machine-local override points at nothing:
// skipped with a notice, as an include in the same state is.
var errSkillSkipped = errors.New("installed skill skipped")

// resolveInstalledSkill resolves one installed_skills entry into the skills it
// names. An entry whose root holds a SKILL.md is one skill named after the entry
// (with its name_prefix); otherwise the skill directories below the root are the
// skills, selected by include/exclude and named with name_prefix.
func resolveInstalledSkill(ctx context.Context, cfg *config.Config, lock *lockfile.File, skillConf *config.InstalledSkillConfig, accessToken string) ([]config.ContentFile, error) {
	ctx = config.WithSkillBundleBudget(config.WithSkillResources(ctx, skillConf.Resources))
	// Check for local override first
	if skillConf.LocalOverride != "" && !refreshing(cfg, lockfile.KindSkill, skillConf.Name) {
		return resolveSkillOverride(ctx, cfg, skillConf)
	}

	sourceType, localSource, err := SkillSourceRoot(cfg, skillConf.Source)
	if err != nil {
		return nil, oops.Wrapf(err, "installed skill %q", skillConf.Name)
	}
	switch sourceType {
	case SourceTypeGit:
		return resolveSkillGit(ctx, cfg, lock, skillConf, accessToken)
	case SourceTypeLocal:
		return resolveSkillLocal(ctx, cfg, skillConf, localSource)
	default:
		return nil, oops.Errorf("unknown source type for skill '%s': %s", skillConf.Name, sourceType)
	}
}

// resolveSkillOverride resolves an entry through its local_override.
func resolveSkillOverride(ctx context.Context, cfg *config.Config, skillConf *config.InstalledSkillConfig) ([]config.ContentFile, error) {
	if err := checkLocalOverride(cfg, "installed_skills", skillConf.Name); err != nil {
		return nil, err
	}
	localDir := resolveSkillLocalOverride(cfg.BaseDir, skillConf)
	if localDir == "" {
		cfg.Log().Info("Skipping installed skill (local_override path not found)", "name", skillConf.Name, "local_override", skillConf.LocalOverride)
		return nil, errSkillSkipped
	}
	cfg.Log().Info("Using local override for installed skill", "name", skillConf.Name, "path", localDir)
	cf, err := scanPrefixedSkillDir(logger.WithContext(ctx, cfg.Host.Log), localDir, skillConf.NamePrefix, skillConf.Name)
	if err != nil {
		return nil, err
	}
	return []config.ContentFile{cf}, nil
}

// resolveSkillGit resolves an entry whose source is a git repository.
func resolveSkillGit(ctx context.Context, cfg *config.Config, lock *lockfile.File, skillConf *config.InstalledSkillConfig, accessToken string) ([]config.ContentFile, error) {
	baseDir := cfg.BaseDir
	skillPath := skillConf.GetPath()
	// A file:// URL is a local path in git clothing: it stays inside the project too.
	if path, ok := lockfile.FileURLPath(skillConf.Source); ok && !lockfile.AllowFileURLsOutside(cfg.Host.Env) {
		if err := checkSkillInsideProject(cfg, baseDir, "source", skillConf.Name, path); err != nil {
			return nil, err
		}
	}
	w := withVersion(lockfile.Want{
		Kind: lockfile.KindSkill, Name: skillConf.Name, Source: lockSource(baseDir, skillConf.Source),
		Path: skillPath, Ref: skillConf.Ref,
	}, skillConf.VersionSpec())
	p, err := pinFor(cfg, lock, w)
	if err != nil {
		return nil, err
	}
	ref, err := versionRef(ctx, cfg, lock, w, p, stripGitPlus(skillConf.Source), accessToken, baseDir)
	if err != nil {
		return nil, err
	}
	source, err := NewSkillGitSourceIn(cfg.Host, skillConf.Name, skillConf.Source, skillPath, w.Ref, accessToken)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to create git source for skill '%s'", skillConf.Name)
	}
	source.ref, source.probe = ref, skillConf.Path == ""
	source.pin, source.baseDir, source.state = p, baseDir, stateFor(cfg)
	return source.DiscoverSkills(ctx, skillConf.Include, skillConf.Exclude, skillConf.NamePrefix)
}

// resolveSkillLocal resolves an entry whose source is a local directory.
func resolveSkillLocal(ctx context.Context, cfg *config.Config, skillConf *config.InstalledSkillConfig, localSource string) ([]config.ContentFile, error) {
	localPath, err := checkedLocalSkillPath(cfg, skillConf, localSource)
	if err != nil {
		return nil, err
	}
	// The entry root is the skill path within the local source.
	root := filepath.Join(localPath, skillConf.GetPath())
	if err := checkedSkillRoot(cfg, skillConf, root); err != nil {
		return nil, err
	}
	if link := symlinkedMarker(root); link != "" {
		return nil, oops.
			With("path", link).
			Errorf("skill '%s': %s is a symlink, and symlinks are not followed", skillConf.Name, link)
	}
	// Without a path, a source that has nothing at the default location is
	// searched for skills where harnesses keep them.
	probed := false
	if skillConf.Path == "" && !holdsSkills(root) && len(probeRoots(localPath)) > 0 {
		root, probed = localPath, true
	}
	if !probed && !holdsSkills(root) {
		return nil, oops.
			With("path", root).
			Errorf("no SKILL.md found at local path for skill '%s'", skillConf.Name)
	}
	return scanEntrySkills(logger.WithContext(ctx, cfg.Host.Log), root, probed, skillConf.NamePrefix, skillConf.Name, skillConf.Include, skillConf.Exclude)
}

// checkedLocalSkillPath returns the absolute local source directory after the
// inside-project check.
func checkedLocalSkillPath(cfg *config.Config, skillConf *config.InstalledSkillConfig, localSource string) (string, error) {
	localPath := localSource
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(cfg.BaseDir, localPath)
	}
	localPath = filepath.Clean(localPath)
	// A file:// directory follows the file:// rule (AI_RULEZ_ALLOW_FILE_URLS), the rest the include rule.
	_, viaFileURL := lockfile.FileURLPath(skillConf.Source)
	if viaFileURL && lockfile.AllowFileURLsOutside(cfg.Host.Env) {
		return localPath, nil
	}
	if err := checkSkillInsideProject(cfg, cfg.BaseDir, "source", skillConf.Name, localPath); err != nil {
		if config.IsHomeSource(skillConf.Source) {
			err = oops.Hint("A '~' path is honored only from config.local.toml (machine-local) or the user config; a committed config must stay inside the project").Wrap(err)
		}
		return "", err
	}
	return localPath, nil
}

// checkedSkillRoot applies the inside-project rule to the entry root itself: a
// path with ".." or a symlinked component must not lead out of a source that
// passed the check.
func checkedSkillRoot(cfg *config.Config, skillConf *config.InstalledSkillConfig, root string) error {
	if _, viaFileURL := lockfile.FileURLPath(skillConf.Source); viaFileURL && lockfile.AllowFileURLsOutside(cfg.Host.Env) {
		return nil
	}
	return checkSkillInsideProject(cfg, cfg.BaseDir, "source", skillConf.Name, root)
}

// checkSkillInsideProject refuses a local installed-skill path that resolves
// outside the project, with the include rules: the machine-local overlay and the
// user scope may leave the project, a committed config may not.
func checkSkillInsideProject(cfg *config.Config, baseDir, field, name, path string) error {
	return checkEntryInsideProject(cfg, "installed_skills", "installed skill", baseDir, field, name, path)
}

// resolveSkillLocalOverride resolves a local_override path for an installed skill.
// Returns the resolved path if it exists and contains SKILL.md, or empty string.
func resolveSkillLocalOverride(baseDir string, skillConf *config.InstalledSkillConfig) string {
	overridePath := skillConf.LocalOverride

	if !filepath.IsAbs(overridePath) {
		overridePath = filepath.Join(baseDir, overridePath)
	}
	overridePath = filepath.Clean(overridePath)

	// Append the skill path within the override
	skillPath := skillConf.GetPath()
	skillDir := filepath.Join(overridePath, skillPath)

	if hasSkillMarker(skillDir) {
		return skillDir
	}

	return ""
}
