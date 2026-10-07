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

		contentFile, err := memoFor(cfg).fetchSkill(memoKey(cfg.BaseDir, skillConf, accessToken != ""),
			func() (config.ContentFile, error) {
				return resolveInstalledSkill(ctx, cfg, lock, skillConf, accessToken)
			},
			func() (config.ContentFile, error) {
				return resolveInstalledSkill(config.WithOfflineIncludes(ctx), cfg, lock, skillConf, accessToken)
			})
		if errors.Is(err, errSkillSkipped) {
			continue
		}
		if err != nil {
			switch {
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

		skills = append(skills, contentFile)
		cfg.Log().Debug("Successfully resolved installed skill", "name", skillConf.Name)
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

// resolveInstalledSkill resolves a single installed skill
func resolveInstalledSkill(ctx context.Context, cfg *config.Config, lock *lockfile.File, skillConf *config.InstalledSkillConfig, accessToken string) (config.ContentFile, error) {
	baseDir := cfg.BaseDir
	// Check for local override first
	if skillConf.LocalOverride != "" && !refreshing(cfg, lockfile.KindSkill, skillConf.Name) {
		if err := checkLocalOverride(cfg, "installed_skills", skillConf.Name); err != nil {
			return config.ContentFile{}, err
		}
		localDir := resolveSkillLocalOverride(baseDir, skillConf)
		if localDir != "" {
			cfg.Log().Info("Using local override for installed skill", "name", skillConf.Name, "path", localDir)
			return ScanInstalledSkillDir(logger.WithContext(ctx, cfg.Host.Log), localDir, skillConf.Name)
		}
		cfg.Log().Info("Skipping installed skill (local_override path not found)", "name", skillConf.Name, "local_override", skillConf.LocalOverride)
		return config.ContentFile{}, errSkillSkipped
	}

	sourceType := DetectSourceType(skillConf.Source)
	skillPath := skillConf.GetPath()

	switch sourceType {
	case SourceTypeGit:
		w := withVersion(lockfile.Want{
			Kind: lockfile.KindSkill, Name: skillConf.Name, Source: lockSource(baseDir, skillConf.Source),
			Path: skillPath, Ref: skillConf.Ref,
		}, skillConf.VersionSpec())
		p, err := pinFor(cfg, lock, w)
		if err != nil {
			return config.ContentFile{}, err
		}
		ref, err := versionRef(ctx, cfg, lock, w, p, stripGitPlus(skillConf.Source), accessToken, baseDir)
		if err != nil {
			return config.ContentFile{}, err
		}
		source, err := NewSkillGitSourceIn(cfg.Host, skillConf.Name, skillConf.Source, skillPath, ref, accessToken)
		if err != nil {
			return config.ContentFile{}, oops.Wrapf(err, "failed to create git source for skill '%s'", skillConf.Name)
		}
		source.pin, source.baseDir = p, baseDir
		return source.Fetch(ctx)

	case SourceTypeLocal:
		localPath := skillConf.Source
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(baseDir, localPath)
		}
		localPath = filepath.Clean(localPath)

		// Append the skill path within the local source
		skillDir := filepath.Join(localPath, skillPath)
		if !hasSkillMarker(skillDir) {
			if link := symlinkedMarker(skillDir); link != "" {
				return config.ContentFile{}, oops.
					With("path", link).
					Errorf("skill '%s': %s is a symlink, and symlinks are not followed", skillConf.Name, link)
			}
			return config.ContentFile{}, oops.
				With("path", skillDir).
				Errorf("no SKILL.md found at local path for skill '%s'", skillConf.Name)
		}
		return ScanInstalledSkillDir(logger.WithContext(ctx, cfg.Host.Log), skillDir, skillConf.Name)

	default:
		return config.ContentFile{}, oops.Errorf("unknown source type for skill '%s': %s", skillConf.Name, sourceType)
	}
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
