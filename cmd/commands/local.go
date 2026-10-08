package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
)

var (
	localShowJSON   bool
	localShowReveal bool
	localSetString  bool
	localSetStdin   bool
)

// localFlagUsage is the help text of the --local flag on config mutators.
const localFlagUsage = "Write to the machine-local config.local.* overlay instead of the shared config"

// LocalCmd manages the machine-local config.local.* overlay.
var LocalCmd = &cobra.Command{
	Use:   "local",
	Short: "Manage the machine-local config overlay",
	Long: `Manage config.local.{toml,yaml,json}, the machine-local overlay merged onto the
shared config at load time. It is gitignored and never written into the shared
config; use it for personal presets, profiles, MCP servers and secrets.`,
}

// localListHint is the Long-help sentence of list commands, which show the
// shared layer only.
const localListHint = "\n\nThe list shows the shared layer only; machine-local entries from config.local.* " +
	"are not included (see `ai-rulez local show`)."

// logLocalEntriesHint logs the localEntriesHint for key when there is one.
func logLocalEntriesHint(key string) {
	if hint := localEntriesHint(key); hint != "" {
		logger.Info(hint)
	}
}

// localEntriesHint tells the user how many entries of the top-level key the
// machine-local overlay defines, since list commands show the shared layer only.
// It is empty when there is no overlay or it defines none.
func localEntriesHint(key string) string {
	// The list commands read the project in the working directory.
	overlay, _, err := config.DescribeLocalOverlayAt(config.ResolveLocalConfigDir(".", "", ""))
	if err != nil || overlay == nil {
		return ""
	}
	n := 0
	switch v := overlay.Doc[key].(type) {
	case map[string]any:
		n = len(v)
	case []any:
		n = len(v)
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("+ %d local entries; see `ai-rulez local show`", n)
}

// localConfigDir resolves the config directory the local subcommands act on,
// honoring the global --config and the --config-dir flag like other commands.
func localConfigDir() string {
	return config.ResolveLocalConfigDir(".", cfgFile, configDir)
}

var localInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a commented config.local skeleton",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		path, created, err := config.InitLocalOverlayAt(localConfigDir())
		if err != nil {
			return fail(err)
		}
		if !created {
			fmt.Printf("Local overlay already exists: %s\n", path)
			return nil
		}
		fmt.Printf("Created %s (gitignored)\n", path)
		return nil
	},
}

var localShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show what the local overlay overrides",
	Long: `Show every key the local overlay sets and the shared value it replaces.

Values are withheld by default: only keys known to hold no credentials (name, description,
default, presets, profiles, defaults, rules, header, builtins, transport, enabled, ...) are
printed; all others show their key path with <redacted>. Pass --reveal to print everything.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		overlay, changes, err := config.DescribeLocalOverlayAt(localConfigDir())
		if err != nil {
			return fail(err)
		}
		if overlay == nil {
			if localShowJSON {
				return fail(jsondoc.Write(os.Stdout, map[string]any{"overlay": nil, "changes": []any{}}))
			}
			fmt.Println("No local overlay. Run 'ai-rulez local init' to create one.")
			return nil
		}
		if localShowJSON {
			return fail(printOverlayJSON(overlay, changes))
		}
		fmt.Printf("local overlay: %s\n", overlay.Path)
		for _, c := range changes {
			hide := c.Redacted && !localShowReveal
			if c.HasMerged && !hide {
				fmt.Printf("  %s: %s -> %s (local %s is merged, not a replacement)\n", c.Path,
					shownValue(c.Shared, c.HasShared, false), shownValue(c.Merged, true, false), shownValue(c.Local, true, false))
				continue
			}
			fmt.Printf("  %s: %s -> %s\n", c.Path, shownValue(c.Shared, c.HasShared, hide), shownValue(c.Local, true, hide))
		}
		return nil
	},
}

var localSetCmd = &cobra.Command{
	Use:   "set <path> [value]",
	Short: "Set a key in the local overlay",
	Long: `Set a key in the local overlay. The value is parsed as a TOML literal and falls
back to a plain string, except under env/headers and for known text fields (url,
command, source, path, ref, description, name, transport, default, *_version) at
their real positions (top-level scalars and <list>.<name>.<field>), which are
always strings. Use --string to force a string, or --stdin to read the
value from standard input (keeps secrets out of shell history).

A path segment containing a dot is written in brackets with double quotes:
  ai-rulez local set 'mcp_servers["foo.bar"].command' npx

Examples:
  ai-rulez local set default dev
  ai-rulez local set 'presets' '["codex", "!cursor"]'
  ai-rulez local set mcp_servers.github.command npx
  printf %s "$TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.ParseLocalPath(args[0])
		if err != nil {
			return fail(err)
		}
		file, err := editLocal(func(doc *config.LocalDoc) error {
			raw, err := localSetValue(cmd, args)
			if err != nil {
				return err
			}
			return doc.Set(path, parseLocalValue(path, raw, localSetString || localSetStdin))
		})
		if err != nil {
			return fail(err)
		}
		logger.Info("Local overlay updated", "key", args[0], "file", file)
		return nil
	},
}

var localUnsetCmd = &cobra.Command{
	Use:   "unset <path>",
	Short: "Remove a key from the local overlay",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		path, err := config.ParseLocalPath(args[0])
		if err != nil {
			return fail(err)
		}
		file, err := editLocal(func(doc *config.LocalDoc) error {
			if !doc.Exists() {
				return oops.Hint("Run 'ai-rulez local init' first").Errorf("no local overlay to change")
			}
			return doc.Unset(path)
		})
		if err != nil {
			return fail(err)
		}
		logger.Info("Local overlay updated", "removed", args[0], "file", file)
		return nil
	},
}

// editLocal opens the overlay under its lock, applies edit, saves (validating the
// merged config) and releases the lock. It returns the overlay path.
func editLocal(edit func(doc *config.LocalDoc) error) (string, error) {
	doc, err := config.OpenLocalDocAt(localConfigDir())
	if err != nil {
		return "", err //nolint:wrapcheck // already contextual
	}
	doc.WithResolvers(project.Resolvers())
	defer doc.Close()
	if err := edit(doc); err != nil {
		return doc.Path, err
	}
	return doc.Path, doc.Save(cmdContext()) //nolint:wrapcheck // already contextual
}

var localPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the local overlay file path",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		doc, err := config.ViewLocalDocAt(localConfigDir())
		if err != nil {
			return fail(err)
		}
		fmt.Println(doc.Path)
		return nil
	},
}

func init() {
	LocalCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	LocalCmd.AddCommand(localInitCmd, localShowCmd, localSetCmd, localUnsetCmd, localPathCmd)
	addJSONFormat(localShowCmd.Flags(), &localShowJSON, "")
	localShowCmd.Flags().BoolVar(&localShowReveal, "reveal", false, "Print values of keys that are withheld by default (may print secrets)")
	localSetCmd.Flags().BoolVar(&localSetString, "string", false, "Store the value as a string without parsing it")
	localSetCmd.Flags().BoolVar(&localSetStdin, "stdin", false, "Read the value (stored as a string) from standard input")
}

// stringFields are keys whose value is always text, whatever it looks like.
var stringFields = map[string]bool{
	"url": true, "command": true, keySource: true, "path": true, "ref": true,
	keyDesc: true, "name": true, "transport": true, "default": true, "version": true,
}

// isStringPath reports whether the value at path must stay a string: env and
// header values of an MCP server, and the known text fields.
func isStringPath(path []string) bool {
	// mcp_servers.<name>.env.<KEY> and mcp_servers.<name>.headers.<Name> only: a
	// server that happens to be called "env" must not turn its fields into text.
	if len(path) >= 4 && path[0] == "mcp_servers" && (path[2] == "env" || path[2] == "headers") {
		return true
	}
	// The named text fields only count at their real positions: a top-level
	// scalar, or a field of a named-list entry (<list>.<name>.<field>). A key
	// that merely shares the name (profiles.default) keeps its typed value.
	var field string
	switch {
	case len(path) == 1:
		field = path[0]
	case len(path) == 3 && isNamedListPath(path[0]):
		field = path[2]
	default:
		return false
	}
	return stringFields[field] || strings.HasSuffix(field, "_version")
}

// isNamedListPath reports whether key is a top-level list of named entries.
func isNamedListPath(key string) bool {
	switch key {
	case "mcp_servers", "plugins", "includes", "installed_skills", "marketplaces", "scopes", "roles":
		return true
	}
	return false
}

// parseLocalValue reads raw as a TOML literal and falls back to the plain
// string. It is always a string when forceString is set or the path holds text
// (env/headers values, url, command, ...), so `123_456` or `1e5` is not turned
// into a number.
func parseLocalValue(path []string, raw string, forceString bool) any {
	if forceString || isStringPath(path) {
		return raw
	}
	var parsed map[string]any
	if err := toml.Unmarshal([]byte("v = "+raw), &parsed); err == nil {
		if v, ok := parsed["v"]; ok {
			return v
		}
	}
	return raw
}

// localSetValue returns the value argument, or the whole of stdin with --stdin.
func localSetValue(cmd *cobra.Command, args []string) (string, error) {
	if localSetStdin {
		if len(args) != 1 {
			return "", oops.Hint("Pass only the path with --stdin").Errorf("--stdin takes the value from standard input")
		}
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", oops.Wrapf(err, "read value from stdin")
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	if len(args) != 2 {
		return "", oops.Hint("Usage: ai-rulez local set <path> <value>").Errorf("a value is required")
	}
	return args[1], nil
}

// shownValue renders a value for display; secret values are never shown.
func shownValue(v any, present, redacted bool) string {
	switch {
	case !present:
		return "(not set)"
	case redacted:
		return config.RedactedValue
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(string(data))
}

func printOverlayJSON(overlay *config.LocalOverlay, changes []config.OverlayChange) error {
	items := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		item := map[string]any{keyPath: c.Path, "redacted": c.Redacted, "shared_set": c.HasShared}
		switch {
		case c.Redacted && !localShowReveal:
			item["local"] = config.RedactedValue
			if c.HasShared {
				item["shared"] = config.RedactedValue
			}
		default:
			item["local"] = c.Local
			if c.HasShared {
				item["shared"] = c.Shared
			}
			if c.HasMerged {
				item["merged"] = c.Merged
			}
		}
		items = append(items, item)
	}
	return jsondoc.Write(os.Stdout, map[string]any{keyPath: overlay.Path, "changes": items})
}
