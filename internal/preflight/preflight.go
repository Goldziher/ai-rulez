// Package preflight lists the commands a generate run is about to write into
// harness settings, so a person who cloned a repository sees what their tools
// will run before they start one. It is shared by the generate command, its
// watch mode and the MCP generate_outputs tool.
package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

const (
	// EnvAck silences the summary, like --yes.
	EnvAck = "AI_RULEZ_ACK_COMMANDS"
	// maxCommandText caps one printed command so a long argument list stays one line.
	maxCommandText = 160
	// maxScriptBytes bounds the hook script read to digest its content.
	maxScriptBytes = 4 << 20
	digestChars    = 12
)

// AckedByEnv reports whether AI_RULEZ_ACK_COMMANDS silences the summary.
func AckedByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvAck))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Item is one thing generate writes that makes a harness run a command or
// changes what it trusts.
type Item struct {
	Kind    string // hook, mcp, allow, env, plugin
	Command string
	Where   []string
	// Content is a digest of a file the item runs (a hook script), so a changed
	// script behind an unchanged path is announced again.
	Content string
}

func (c Item) digest() string {
	sum := sha256.Sum256([]byte(c.Kind + "\x00" + c.Command + "\x00" + strings.Join(c.Where, ",") + "\x00" + c.Content))
	return hex.EncodeToString(sum[:])
}

// Line renders the item for a terminal: escaped, redacted, one line.
func (c Item) Line() string {
	text := Display(c.Command, maxCommandText)
	if c.Content != "" {
		text += " [sha256 " + c.Content + "]"
	}
	where := make([]string, len(c.Where))
	for i, w := range c.Where {
		where[i] = Sanitize(w)
	}
	return fmt.Sprintf("  %-10s %s  -> %s", c.Kind, text, strings.Join(where, ", "))
}

// Collect lists what the configuration makes generate write, by where it lands.
func Collect(cfg *config.Config) []Item {
	presets := presetNames(cfg)
	items := hookItems(cfg, presets)
	items = append(items, mcpItems(cfg, presets)...)
	items = append(items, permissionItems(cfg, presets)...)
	items = append(items, claudeSettingsItems(cfg, presets)...)
	items = append(items, automationItems(cfg)...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Command < items[j].Command
	})
	return items
}

func permissionItems(cfg *config.Config, presets []string) []Item {
	if cfg.Permissions == nil || len(cfg.Permissions.Allow) == 0 {
		return nil
	}
	var items []Item
	for _, rule := range cfg.Permissions.Allow {
		if where := allowLands(cfg, presets, rule); len(where) > 0 {
			items = append(items, Item{Kind: "allow", Command: rule, Where: where})
		}
	}
	return items
}

// claudeSettingsItems covers what [claude.settings] writes into
// .claude/settings.json beyond hooks and permissions: managed env (NODE_OPTIONS,
// ANTHROPIC_BASE_URL and the like change what every session runs and talks to),
// plugin enablement and marketplace registration.
func claudeSettingsItems(cfg *config.Config, presets []string) []Item {
	if !contains(presets, config.HarnessClaude) {
		return nil
	}
	where := []string{config.HarnessClaude}
	var items []Item
	if managed := cfg.ManagedClaudeSettings(); managed != nil {
		for name, value := range managed.Env {
			items = append(items, Item{Kind: "env", Command: EnvValue(name, value), Where: where})
		}
	}
	if !cfg.ManagesClaudeSettings() || cfg.UserScope {
		return items
	}
	s := cfg.Claude.Settings
	if s.RegistersMarketplace() {
		items = append(items, Item{Kind: "plugin", Command: "register marketplace " + marketplaceSource(s), Where: where})
	}
	for _, name := range s.EnablePlugins {
		items = append(items, Item{Kind: "plugin", Command: "enable " + name, Where: where})
	}
	return items
}

func marketplaceSource(s *config.ClaudeSettings) string {
	src := s.MarketplaceSource
	if src == nil {
		return "(project directory)"
	}
	parts := []string{src.Source}
	for _, v := range []string{src.Path, src.Repo, src.URL, src.Ref} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func hookItems(cfg *config.Config, presets []string) []Item {
	var items []Item
	for i := range cfg.Hooks {
		group := &cfg.Hooks[i]
		label := group.Event
		if group.Matcher != "" {
			label += "(" + group.Matcher + ")"
		}
		for j := range group.Hooks {
			where := hookLands(cfg, presets, i, j)
			if len(where) == 0 {
				continue
			}
			if item, ok := hookActionItem(cfg, label, &group.Hooks[j], where); ok {
				items = append(items, item)
			}
		}
	}
	return items
}

func hookActionItem(cfg *config.Config, label string, action *config.HookAction, where []string) (Item, bool) {
	command := action.Command
	item := Item{Kind: "hook", Where: where}
	if command == "" && action.Script != "" {
		command = action.Script
		item.Content = scriptDigest(cfg.BaseDir, action.Script)
	}
	switch {
	case command != "":
		item.Command = label + ": " + joinCommand(command, action.Args)
	case action.Type != "" && action.Type != config.HookTypeCommand:
		// An http or prompt hook has no command, but still makes the harness act.
		item.Command = label + ": " + action.Type + " hook"
	default:
		return Item{}, false
	}
	return item, true
}

// scriptDigest is a short hash of a project-relative hook script, or empty when
// the path is unsafe or unreadable.
func scriptDigest(baseDir, script string) string {
	if !config.IsSafeHookScript(script) {
		return ""
	}
	f, err := os.Open(filepath.Join(baseDir, filepath.FromSlash(script))) //nolint:gosec // validated project-relative path
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read only
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxScriptBytes)); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:digestChars]
}

func mcpItems(cfg *config.Config, presets []string) []Item {
	var where []string
	for _, p := range presets {
		if config.HarnessSupportsMCP(p) {
			where = append(where, p)
		}
	}
	if len(where) == 0 {
		where = []string{"mcp"}
	}
	var items []Item
	for _, server := range cfg.MCPServers {
		if server != nil && server.IsEnabled() && server.Command != "" {
			items = append(items, Item{Kind: "mcp", Command: server.Name + ": " + joinCommand(server.Command, server.Args), Where: where})
		}
	}
	return items
}

func presetNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Presets))
	for i := range cfg.Presets {
		names = append(names, cfg.Presets[i].GetName())
	}
	sort.Strings(names)
	return names
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func joinCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return command + " " + strings.Join(args, " ")
}

// record is the machine-local record of the commands earlier generate runs
// announced, kept in the user cache (never in the repository, or a clone would
// inherit the acknowledgement it exists to deny).
type record struct {
	Digests []string `json:"digests"`
}

func recordPath(cfg *config.Config) (string, error) {
	abs, err := filepath.Abs(cfg.ConfigDir)
	if err != nil {
		return "", oops.Wrapf(err, "resolve the config directory")
	}
	sum := sha256.Sum256([]byte(abs))
	return config.CacheDir("commands", hex.EncodeToString(sum[:8])+".json") //nolint:wrapcheck // already contextual
}

func readRecord(path string) map[string]bool {
	known := map[string]bool{}
	data, err := os.ReadFile(path) //nolint:gosec // our own cache file
	if err != nil {
		return known
	}
	var rec record
	if json.Unmarshal(data, &rec) == nil {
		for _, d := range rec.Digests {
			known[d] = true
		}
	}
	return known
}

func writeRecord(path string, items []Item) {
	rec := record{Digests: make([]string, 0, len(items))}
	for _, it := range items {
		rec.Digests = append(rec.Digests, it.digest())
	}
	sort.Strings(rec.Digests)
	data, err := json.Marshal(rec)
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			err = os.WriteFile(path, data, 0o600)
		}
	}
	if err != nil {
		logger.Debug("Could not record the announced commands", "path", path, "error", err)
	}
}

// previousManifestExists reports whether an earlier generate wrote a manifest in
// the config directory; without one this is a first run, so every command is new.
func previousManifestExists(cfg *config.Config) bool {
	for _, name := range generator.GeneratedManifestNames() {
		if _, err := os.Stat(filepath.Join(cfg.ConfigDir, name)); err == nil {
			return true
		}
	}
	return false
}

// NewCommands returns the printable lines of the commands this run writes that
// were not part of the previous run. With remember it also stores the full list,
// so the next run reports only what changed (leave it off for a dry run).
func NewCommands(cfg *config.Config, remember bool) []string {
	items := Collect(cfg)
	if len(items) == 0 {
		return nil
	}
	path, err := recordPath(cfg)
	if err != nil {
		logger.Debug("No place to record the announced commands", "error", err)
	}
	known := map[string]bool{}
	if err == nil && previousManifestExists(cfg) {
		known = readRecord(path)
	}
	var lines []string
	for _, it := range items {
		if !known[it.digest()] {
			lines = append(lines, it.Line())
		}
	}
	if remember && err == nil {
		writeRecord(path, items)
	}
	return lines
}

// Remember stores the current commands as announced. A caller that must not
// record before it knows the run succeeded calls NewCommands with remember off
// and this afterwards.
func Remember(cfg *config.Config) {
	items := Collect(cfg)
	if len(items) == 0 {
		return
	}
	if path, err := recordPath(cfg); err == nil {
		writeRecord(path, items)
	}
}

// Header is the first line of the printed summary.
func Header(cfg *config.Config, count int) string {
	return fmt.Sprintf("Warning: generate will write %d new or changed command(s) that your tools will run (from %s):",
		count, Sanitize(filepath.Join(cfg.ConfigDir, "config.toml")))
}

// Footer is the last line of the printed summary.
func Footer() string {
	return "Review them before you start a tool in this project. Silence with --yes or " + EnvAck + "=1."
}

// Summary renders the lines as the full text of the warning.
func Summary(cfg *config.Config, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return Header(cfg, len(lines)) + "\n" + strings.Join(lines, "\n") + "\n" + Footer() + "\n"
}
