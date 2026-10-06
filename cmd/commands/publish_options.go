package commands

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	pemit "github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
)

// publishOptions are the [publish] table and the flags, merged: a flag wins
// over the table, and a flag never lowers a policy the table sets.
type publishOptions struct {
	runtimes         []string
	allowDirty       bool
	requireSignature bool
	requireApproved  bool
	ghRepo           string
	ociRepo          string
	npm              publish.NPMOptions
	channelRefs      map[string]string
	emitters         []string
	experimental     bool
	emitOptions      map[string]map[string]string
	templates        []publish.Template
}

// publishConfigError wraps a [publish] validation failure as AR9N6, and leaves
// every other config error as it is.
func publishConfigError(err error) error {
	if err == nil {
		return nil
	}
	if oe, ok := oops.AsOops(err); ok {
		if field, _ := oe.Context()["field"].(string); strings.HasPrefix(field, "publish") {
			return publish.Errorf(publish.CodeConfig, publish.ExitGate, "fix the [publish] table in the configuration", "%v", err)
		}
	}
	return err
}

// resolvePublishOptions merges the [publish] table with the flags and reads the
// template files. Template paths stay inside the project: a symlink or an
// escaping path is refused.
func resolvePublishOptions(cfg *config.Config) (*publishOptions, error) {
	p := cfg.Publish
	if p == nil {
		p = &config.PublishConfig{}
	}
	o := &publishOptions{
		runtimes:         append([]string(nil), p.Runtimes...),
		allowDirty:       p.AllowDirty || publishAllowDirty,
		requireSignature: p.RequireSignature,
		requireApproved:  p.RequireApproved,
		npm:              publish.NPMOptions{},
		channelRefs:      map[string]string{},
		experimental:     publishExperimental,
		emitOptions:      map[string]map[string]string{},
	}
	if len(publishRuntimes) > 0 {
		o.runtimes = append([]string(nil), publishRuntimes...)
	}
	for _, r := range o.runtimes {
		if !slices.Contains(config.KnownPluginRuntimes, r) {
			return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "known runtimes: "+strings.Join(config.KnownPluginRuntimes, ", "), "unknown runtime %q", r)
		}
	}
	o.applyTargetTables(p)
	if err := o.collectEmitters(cfg.BaseDir, p); err != nil {
		return nil, err
	}
	return o, nil
}

// applyTargetTables merges the target tables of [publish] with the flags.
func (o *publishOptions) applyTargetTables(p *config.PublishConfig) {
	if p.GitHubRelease != nil {
		o.ghRepo = p.GitHubRelease.Repo
	}
	if p.OCI != nil {
		o.ociRepo = p.OCI.Ref
	}
	if publishOCIRef != "" {
		o.ociRepo = publishOCIRef
	}
	if p.NPM != nil {
		o.npm = publish.NPMOptions{Scope: p.NPM.Scope, Access: p.NPM.Access, Registry: p.NPM.Registry}
	}
	if publishNPMScope != "" {
		o.npm.Scope = publishNPMScope
	}
	if publishPublic {
		o.npm.Access = config.PublishAccessPublic
	}
	if p.Marketplace != nil {
		for name, ref := range p.Marketplace.Channels {
			o.channelRefs[name] = ref
		}
	}
}

// collectEmitters gathers the emitters of the flags and the table, and reads
// the template files.
func (o *publishOptions) collectEmitters(base string, p *config.PublishConfig) error {
	names := map[string]bool{}
	for _, name := range publishEmit {
		names[name] = true
	}
	for _, e := range p.Emitters {
		if e.Name != config.PublishEmitterTemplate {
			names[e.Name] = true
			if len(e.Options) > 0 {
				o.emitOptions[e.Name] = e.Options
			}
			continue
		}
		t, err := readProjectTemplate(base, e.Template, e.Output)
		if err != nil {
			return err
		}
		o.templates = append(o.templates, t)
	}
	for _, path := range publishTemplates {
		body, err := os.ReadFile(path) //nolint:gosec // an explicit --template chosen by the user
		if err != nil {
			return oops.With("path", path).Wrapf(err, "read template")
		}
		o.templates = append(o.templates, publish.Template{Name: filepath.Base(path), Body: string(body)})
	}
	for name := range names {
		if _, ok := pemit.Lookup(name); !ok {
			return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "known emitters: "+strings.Join(pemit.Names(), ", "), "unknown emitter %q", name)
		}
		o.emitters = append(o.emitters, name)
	}
	sort.Strings(o.emitters)
	return nil
}

// readProjectTemplate reads a [[publish.emitters]] template that must sit
// inside the project and cannot be reached through a symlink.
func readProjectTemplate(base, rel, output string) (publish.Template, error) {
	if err := publish.CheckTree(base, []string{filepath.ToSlash(filepath.Clean(rel))}); err != nil {
		return publish.Template{}, publish.Errorf(publish.CodeBundleUnsafe, publish.ExitFailed, "", "template %s: %v", rel, err)
	}
	body, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel))) //nolint:gosec // validated project-relative path
	if err != nil {
		return publish.Template{}, oops.With("path", rel).Wrapf(err, "read template")
	}
	name := output
	if name == "" {
		name = filepath.Base(rel)
	}
	return publish.Template{Name: name, Body: string(body)}, nil
}
