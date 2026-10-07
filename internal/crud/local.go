package crud

import (
	"context"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
)

const (
	listIncludes        = "includes"
	listInstalledSkills = "installed_skills"
)

// Local returns an operator for the machine-local layer. Config mutations
// (profiles, includes, installed skills) are written to the config.local.*
// overlay instead of the shared config, and content-file operations (rules,
// context, skills, and their domains) work on <configdir>/local/, which mirrors
// the shared layout. Everything under it is gitignored.
func (op *OperatorImpl) Local() *OperatorImpl {
	local := *op
	local.local = true
	local.filesMgr = NewFileManager(filepath.Join(op.aiRulezDir, localContentDir))
	local.filesMgr.private = true
	// Content written here may hold secrets: fail closed unless the tree is
	// ignored first.
	local.filesMgr.guard = func() error {
		return gitignore.EnsureEntries(op.log, op.baseDir, config.LocalGitignorePatterns(op.aiRulezDir)) //nolint:wrapcheck // already contextual
	}
	return &local
}

// localContentDir is the machine-local content tree inside the config directory.
const localContentDir = "local"

// IsLocal reports whether config mutations go to the local overlay.
func (op *OperatorImpl) IsLocal() bool { return op.local }

func (op *OperatorImpl) openLocalDoc() (*config.LocalDoc, error) {
	doc, err := config.OpenLocalDocInDir(op.baseDir)
	if err != nil {
		return nil, oops.With("base_dir", op.baseDir).Wrapf(err, "open local config")
	}
	return doc.WithResolvers(project.Resolvers()), nil
}

// loadMerged loads the shared config plus the local overlay, for existence
// checks that must see both layers.
func (op *OperatorImpl) loadMerged(ctx context.Context) (*config.Config, error) {
	cfg, err := project.Load(config.WithOfflineIncludes(ctx), op.baseDir)
	if err != nil {
		return nil, oops.With("base_dir", op.baseDir).Wrapf(err, "load config")
	}
	return cfg, nil
}

func (op *OperatorImpl) loadShared(ctx context.Context) (*config.Config, error) {
	cfg, err := project.Load(config.WithOfflineIncludes(ctx), op.baseDir, config.WithoutLocal())
	if err != nil {
		return nil, oops.With("base_dir", op.baseDir).Wrapf(err, "load config")
	}
	return cfg, nil
}

func (op *OperatorImpl) addProfileLocal(ctx context.Context, name string, domains []string) error {
	cfg, err := op.loadMerged(ctx)
	if err != nil {
		return err
	}
	if cfg.HasProfile(name) {
		return oops.With("name", name).Hint("Choose a different name for the profile").
			Errorf("profile '%s' already exists", name)
	}
	if err := validateProfileDomains(cfg, name, domains); err != nil {
		return err
	}
	doc, err := op.openLocalDoc()
	if err != nil {
		return err
	}
	defer doc.Close()
	if err := doc.SetProfile(name, domains); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if err := doc.Save(ctx); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	op.logger().Info("Local profile added successfully", "name", name, "domains", len(domains))
	return nil
}

func (op *OperatorImpl) removeProfileLocal(ctx context.Context, name string) error {
	cfg, err := op.loadMerged(ctx)
	if err != nil {
		return err
	}
	doc, err := op.openLocalDoc()
	if err != nil {
		return err
	}
	defer doc.Close()
	if !doc.HasLocalProfile(name) {
		if cfg.HasProfile(name) {
			return oops.With("name", name).
				Hint("Shared profiles cannot be removed locally; remove it from the shared config").
				Errorf("profile '%s' is defined in the shared config", name)
		}
		return oops.With("name", name).Hint("Use 'profile list' to see available profiles").
			Errorf("profile '%s' does not exist", name)
	}
	if cfg.GetDefaultProfile() == name {
		return oops.With("name", name).
			Hint("Set a different default profile first using 'profile set-default'").
			Errorf("cannot remove default profile '%s'", name)
	}
	if err := doc.RemoveProfile(name); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if err := doc.Save(ctx); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	op.logger().Info("Local profile removed successfully", "name", name)
	return nil
}

func (op *OperatorImpl) setDefaultProfileLocal(ctx context.Context, name string) error {
	cfg, err := op.loadMerged(ctx)
	if err != nil {
		return err
	}
	if !cfg.HasProfile(name) {
		return oops.With("name", name).Hint("Create the profile first using 'profile add'").
			Errorf("profile '%s' does not exist", name)
	}
	doc, err := op.openLocalDoc()
	if err != nil {
		return err
	}
	defer doc.Close()
	if err := doc.SetDefault(name); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if err := doc.Save(ctx); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	op.logger().Info("Local default profile set successfully", "name", name)
	return nil
}

func (op *OperatorImpl) addIncludeLocal(ctx context.Context, req *AddIncludeRequest) error {
	cfg, err := op.loadMerged(ctx)
	if err != nil {
		return err
	}
	for i := range cfg.Includes {
		if cfg.Includes[i].Name == req.Name {
			return oops.With("name", req.Name).Hint("Choose a different name for the include").
				Errorf("include with name '%s' already exists", req.Name)
		}
	}
	sourceType, err := validateIncludeSource(op.env, req.Source)
	if err != nil {
		return err
	}
	fields := map[string]any{"source": req.Source}
	setIfNotEmpty(fields, "path", req.Path)
	setIfNotEmpty(fields, "ref", req.Ref)
	setIfNotEmpty(fields, "merge_strategy", req.MergeStrategy)
	setIfNotEmpty(fields, "install_to", req.InstallTo)
	if len(req.Include) > 0 {
		fields["include"] = req.Include
	}
	if err := op.upsertLocalNamed(ctx, listIncludes, req.Name, fields); err != nil {
		return err
	}
	op.logger().Info("Local include added successfully", "name", req.Name, "type", sourceType)
	return nil
}

func (op *OperatorImpl) removeIncludeLocal(ctx context.Context, name string) error {
	shared, err := op.loadShared(ctx)
	if err != nil {
		return err
	}
	sharedHas := false
	for i := range shared.Includes {
		sharedHas = sharedHas || shared.Includes[i].Name == name
	}
	if err := op.removeLocalNamed(ctx, listIncludes, name, sharedHas, "include"); err != nil {
		return err
	}
	op.logger().Info("Local include removed successfully", "name", name)
	return nil
}

func (op *OperatorImpl) installSkillLocal(ctx context.Context, req *InstallSkillRequest) error {
	cfg, err := op.loadMerged(ctx)
	if err != nil {
		return err
	}
	for i := range cfg.InstalledSkills {
		if cfg.InstalledSkills[i].Name == req.Name {
			return oops.With("name", req.Name).
				Hint("Choose a different name or remove the existing skill first").
				Errorf("skill '%s' is already installed", req.Name)
		}
	}
	sourceType, err := validateSkillSource(req.Source)
	if err != nil {
		return err
	}
	fields := map[string]any{"source": req.Source}
	setIfNotEmpty(fields, "path", req.Path)
	setIfNotEmpty(fields, "ref", req.Ref)
	if err := op.upsertLocalNamed(ctx, listInstalledSkills, req.Name, fields); err != nil {
		return err
	}
	op.logger().Info("Local skill installed successfully", "name", req.Name, "type", sourceType)
	return nil
}

func (op *OperatorImpl) uninstallSkillLocal(ctx context.Context, name string) error {
	shared, err := op.loadShared(ctx)
	if err != nil {
		return err
	}
	sharedHas := false
	for i := range shared.InstalledSkills {
		sharedHas = sharedHas || shared.InstalledSkills[i].Name == name
	}
	if err := op.removeLocalNamed(ctx, listInstalledSkills, name, sharedHas, "installed skill"); err != nil {
		return err
	}
	op.logger().Info("Local skill uninstalled successfully", "name", name)
	return nil
}

func (op *OperatorImpl) upsertLocalNamed(ctx context.Context, list, name string, fields map[string]any) error {
	doc, err := op.openLocalDoc()
	if err != nil {
		return err
	}
	defer doc.Close()
	if err := doc.UpsertNamed(list, name, fields); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	return doc.Save(ctx) //nolint:wrapcheck // already contextual
}

func (op *OperatorImpl) removeLocalNamed(ctx context.Context, list, name string, sharedHas bool, what string) error {
	doc, err := op.openLocalDoc()
	if err != nil {
		return err
	}
	defer doc.Close()
	if !sharedHas && !doc.HasLocalNamed(list, name) {
		return oops.With("name", name).Errorf("%s '%s' not found", what, name)
	}
	if err := doc.RemoveNamed(list, name, sharedHas); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	return doc.Save(ctx) //nolint:wrapcheck // already contextual
}

func setIfNotEmpty(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
