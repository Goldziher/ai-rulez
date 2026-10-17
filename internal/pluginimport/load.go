package pluginimport

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

// SkillScanner reads one skill directory into a content file named name; the
// resolver passes includes.ScanInstalledSkillDir, so a plugin's skills are
// bundled exactly like an installed skill's.
type SkillScanner func(ctx context.Context, dir, name string) (config.ContentFile, error)

// Options tune one Load.
type Options struct {
	// Name is the config entry the plugin is installed as, for messages.
	Name string
	// Format forces the layout (config.PluginFormat*); empty detects it.
	Format string
	// Wants says whether a component kind (config.PluginKind*) is imported; nil imports all.
	Wants func(kind string) bool
	// NamePrefix is prepended to every skill, agent, command, rule and MCP server name.
	NamePrefix string
	// EnableHooks imports the hooks; without it they are counted and reported only.
	EnableHooks bool
	// EnableMCP imports the MCP servers.
	EnableMCP bool
	// Trust is the scan level carried onto the files queued for the security scan.
	Trust string
	// Rewrite is the project-relative path the plugin root variables stand for;
	// empty when the plugin directory is not part of the project.
	Rewrite string
	// Skills reads a skill directory; nil skips skills.
	Skills SkillScanner
	// Inline holds the component fields a marketplace entry declares itself.
	Inline map[string]json.RawMessage
}

// Result is what a plugin delivers.
type Result struct {
	// Format is the layout the plugin was read as.
	Format                     string
	Name, Version, Description string

	Skills, Agents, Commands, Rules []config.ContentFile
	Hooks                           []config.HookGroup
	MCP                             []config.MCPServer
	// Scan lists the plugin files the import-time security scan reads besides the
	// content files: hook documents and scripts, MCP documents and the files
	// their commands run.
	Scan []config.ImportedScanFile
	// HooksDisabled counts the hook groups found but not imported (EnableHooks off).
	HooksDisabled int
	// Findings is the lossiness report: what mapped, what was approximated or dropped.
	Findings []importer.Finding
}

// Empty reports whether the plugin delivered nothing.
func (r *Result) Empty() bool {
	return len(r.Skills)+len(r.Agents)+len(r.Commands)+len(r.Rules)+len(r.Hooks)+len(r.MCP) == 0
}

type loader struct {
	ctx      context.Context
	dir      string
	opt      *Options
	layout   layout
	manifest manifestDoc
	res      *Result
	scanned  map[string]bool
}

func (c *loader) wants(kind string) bool { return c.opt.Wants == nil || c.opt.Wants(kind) }

// Load reads the plugin in dir. The directory must be a real directory (a
// symlink root is refused); nothing outside it is read.
func Load(ctx context.Context, dir string, opt *Options) (*Result, error) {
	info, err := os.Lstat(dir)
	switch {
	case err != nil:
		return nil, oops.With("dir", dir).Wrapf(err, "plugin %q", opt.Name)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return nil, oops.With("dir", dir).Errorf("plugin %q: %s is not a real directory (a symlink is not followed)", opt.Name, dir)
	}
	format := opt.Format
	if format == "" {
		var found bool
		if format, found = detect(dir); !found && len(opt.Inline) > 0 {
			// A marketplace entry that declares its own components needs no manifest.
			format, found = config.PluginFormatClaude, true
		}
		if !found {
			return nil, oops.With("dir", dir).Hint("A plugin has a .claude-plugin/plugin.json, .codex-plugin/plugin.json, .cursor-plugin/plugin.json, gemini-extension.json or a root plugin.json (Agent Plugins), or skills/, agents/, commands/ or hooks/ directories.").
				Errorf("plugin %q: %s holds no recognizable plugin", opt.Name, dir)
		}
	}
	l, ok := layoutFor(format)
	if !ok {
		return nil, oops.Errorf("plugin %q: unknown format %q", opt.Name, format)
	}
	manifest, err := readManifest(dir, l)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "plugin %q", opt.Name)
	}
	c := &loader{ctx: ctx, dir: dir, opt: opt, layout: l, manifest: manifest, scanned: map[string]bool{},
		res: &Result{Format: format, Name: manifest.Name, Version: manifest.Version, Description: manifest.Description}}
	c.collect()
	return c.res, nil
}

func (c *loader) collect() {
	if c.wants(config.PluginKindSkills) {
		c.collectSkills()
	}
	if c.wants(config.PluginKindAgents) {
		c.collectMarkdown(&c.res.Agents, "agents", c.layout.agents, "agents", false, extMD)
	}
	if c.wants(config.PluginKindCommands) {
		if c.layout.tomlCommands {
			c.collectTOMLCommands()
		} else {
			c.collectMarkdown(&c.res.Commands, "commands", c.layout.commands, "commands", true, extMD)
		}
	}
	if c.wants(config.PluginKindRules) {
		c.collectMarkdown(&c.res.Rules, "rules", c.layout.rules, "rules", true, extMD, extMDC)
		c.collectContext()
	}
	if c.wants(config.PluginKindHooks) {
		c.collectHooks()
	}
	if c.wants(config.PluginKindMCP) && c.opt.EnableMCP {
		c.collectMCP()
	}
}

// isAgentPlugin reports whether dir's root plugin.json names an Agent Plugins schema.
func isAgentPlugin(dir string) bool {
	format, ok := importer.Lookup("agent-plugins")
	if !ok {
		return false
	}
	return len(format.Detect(os.DirFS(dir))) > 0
}

// Describe names the plugin for a listing: manifest name, or the directory name.
func (r *Result) Describe(dir string) string {
	if r.Name != "" {
		return r.Name
	}
	return filepath.Base(dir)
}
