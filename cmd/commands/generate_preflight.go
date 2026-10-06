package commands

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
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

const (
	envStrict      = "AI_RULEZ_STRICT"
	envAckCommands = "AI_RULEZ_ACK_COMMANDS"
	// maxCommandText caps one printed command so a long argument list stays one line.
	maxCommandText = 160
)

var generateStrict bool

func init() {
	GenerateCmd.Flags().BoolVar(&generateStrict, "strict", false,
		"Fail on unknown or invalid configuration keys instead of warning (env AI_RULEZ_STRICT=1)")
}

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// generatePreflight runs the checks that belong before generate writes anything:
// the schema (unknown keys), the role selection findings, the commands the run is
// about to write, and the hand-written skillOverrides a role replaces. A real run
// (not --dry-run) also records the commands it announced and restores skill
// overrides a role released.
func generatePreflight(cfg *config.Config, gen *generator.Generator) error {
	if err := checkConfigSchema(cfg, generateStrict || envTrue(envStrict)); err != nil {
		return err
	}
	if generateRole != "" {
		warnRoleSelection(cfg, generateRole)
	}
	if pluginMode {
		return nil // bundles carry the [plugin] hooks, not the project's
	}
	warnNewCommands(cfg, os.Stderr, !dryRun)
	if dryRun {
		return nil
	}
	return gen.ReconcileRoleSkillOverrides() //nolint:wrapcheck // already contextual
}

// checkConfigSchema runs the schema validation `validate` runs (shared code in
// config.SchemaFindings, so both commands agree on the unknown keys). Findings
// are warnings, or an error when strict.
func checkConfigSchema(cfg *config.Config, strict bool) error {
	findings, err := config.SchemaFindings(cfg)
	if err != nil {
		return oops.Wrapf(err, "check the configuration against the schema")
	}
	if len(findings) == 0 {
		return nil
	}
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = f.String()
	}
	if strict {
		return oops.With("errors", lines).
			Hint("Fix the keys above, or run generate without --strict to only warn about them").
			Errorf("configuration has %d unknown or invalid key(s)", len(lines))
	}
	for _, line := range lines {
		logger.Warn("Configuration problem: " + line)
	}
	logger.Warn("These keys have no effect; `ai-rulez validate` fails on them, `generate --strict` (or AI_RULEZ_STRICT=1) does too")
	return nil
}

// warnRoleSelection prints the AR971 findings of the role about to be rendered: a
// domain that does not exist or a selector that matches nothing makes the role
// render less than its author meant, and only validate --strict reported it.
// It uses the same computation as the AR971 lint rule (Config.RoleProblems).
func warnRoleSelection(cfg *config.Config, role string) {
	for _, p := range cfg.RoleProblems() {
		if p.Role == role && p.Kind == config.RoleProblemReference {
			logger.Warn("AR971 " + p.Message)
		}
	}
}

// commandItem is one thing generate writes that makes a harness run a command.
type commandItem struct {
	Kind    string // hook, mcp, permission
	Command string
	Where   []string
}

func (c commandItem) digest() string {
	sum := sha256.Sum256([]byte(c.Kind + "\x00" + c.Command + "\x00" + strings.Join(c.Where, ",")))
	return hex.EncodeToString(sum[:])
}

func (c commandItem) line() string {
	text := c.Command
	if len(text) > maxCommandText {
		text = text[:maxCommandText] + "..."
	}
	return fmt.Sprintf("  %-10s %s  -> %s", c.Kind, text, strings.Join(c.Where, ", "))
}

// collectCommandItems lists the hook commands, MCP server commands and permission
// allow rules the configuration makes generate write, by where they land.
func collectCommandItems(cfg *config.Config) []commandItem {
	presets := presetNames(cfg)
	var items []commandItem
	for i := range cfg.Hooks {
		group := &cfg.Hooks[i]
		var where []string
		for _, p := range presets {
			if settings.HasHookDialect(p) && group.HookTargetsHarness(p) {
				where = append(where, p)
			}
		}
		if len(where) == 0 {
			continue
		}
		label := group.Event
		if group.Matcher != "" {
			label += "(" + group.Matcher + ")"
		}
		for j := range group.Hooks {
			action := &group.Hooks[j]
			command := action.Command
			if command == "" {
				command = action.Script
			}
			if command == "" {
				continue
			}
			items = append(items, commandItem{Kind: "hook", Command: label + ": " + joinCommand(command, action.Args), Where: where})
		}
	}
	var mcpWhere []string
	for _, p := range presets {
		if config.HarnessSupportsMCP(p) {
			mcpWhere = append(mcpWhere, p)
		}
	}
	if len(mcpWhere) == 0 {
		mcpWhere = []string{"mcp"}
	}
	for _, server := range cfg.MCPServers {
		if server == nil || !server.IsEnabled() || server.Command == "" {
			continue
		}
		items = append(items, commandItem{Kind: "mcp", Command: server.Name + ": " + joinCommand(server.Command, server.Args), Where: mcpWhere})
	}
	if cfg.Permissions != nil && len(cfg.Permissions.Allow) > 0 && contains(presets, config.HarnessClaude) {
		for _, rule := range cfg.Permissions.Allow {
			items = append(items, commandItem{Kind: "allow", Command: rule, Where: []string{"permissions"}})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Command < items[j].Command
	})
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

// commandRecord is the machine-local record of the commands earlier generate runs
// announced, kept in the user cache (never in the repository, or a clone would
// inherit the acknowledgement it exists to deny).
type commandRecord struct {
	Digests []string `json:"digests"`
}

func commandRecordPath(cfg *config.Config) (string, error) {
	abs, err := filepath.Abs(cfg.ConfigDir)
	if err != nil {
		return "", oops.Wrapf(err, "resolve the config directory")
	}
	sum := sha256.Sum256([]byte(abs))
	return config.CacheDir("commands", hex.EncodeToString(sum[:8])+".json") //nolint:wrapcheck // already contextual
}

func readCommandRecord(path string) map[string]bool {
	known := map[string]bool{}
	data, err := os.ReadFile(path) //nolint:gosec // our own cache file
	if err != nil {
		return known
	}
	var rec commandRecord
	if json.Unmarshal(data, &rec) == nil {
		for _, d := range rec.Digests {
			known[d] = true
		}
	}
	return known
}

func writeCommandRecord(path string, items []commandItem) {
	rec := commandRecord{Digests: make([]string, 0, len(items))}
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

// warnNewCommands prints, to w, the hook, MCP server and allow-rule commands this
// run will write that were not part of the previous run: a developer who clones a
// repository sees what the harness will run. It only warns. It is printed even
// with --quiet because it is security relevant, and silenced by --yes or
// AI_RULEZ_ACK_COMMANDS=1. record stores the commands so the next run announces
// only what changed.
func warnNewCommands(cfg *config.Config, w io.Writer, record bool) {
	items := collectCommandItems(cfg)
	if len(items) == 0 {
		return
	}
	path, err := commandRecordPath(cfg)
	if err != nil {
		logger.Debug("No place to record the announced commands", "error", err)
	}
	known := map[string]bool{}
	if err == nil && previousManifestExists(cfg) {
		known = readCommandRecord(path)
	}
	var fresh []commandItem
	for _, it := range items {
		if !known[it.digest()] {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) > 0 && !assumeYes && !envTrue(envAckCommands) {
		fmt.Fprintf(w, "Warning: generate will write %d new or changed command(s) that your tools will run (from %s):\n",
			len(fresh), filepath.Join(cfg.ConfigDir, "config.toml"))
		for _, it := range fresh {
			fmt.Fprintln(w, it.line())
		}
		fmt.Fprintf(w, "Review them before you start a tool in this project. Silence with --yes or %s=1.\n", envAckCommands)
	}
	if record && err == nil {
		writeCommandRecord(path, items)
	}
}
