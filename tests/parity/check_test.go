package parity_test

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/parity"
)

// minReason is the shortest reason that can explain a deliberate asymmetry.
const minReason = 25

// matchesPath reports whether a table entry names the command path.
func matchesPath(entry, path string) bool {
	if base, ok := strings.CutSuffix(entry, " *"); ok {
		return path == base || strings.HasPrefix(path, base+" ")
	}
	return entry == path
}

// coverage reports, for the tree and the surfaces, what the table leaves out or names wrongly.
func coverage(caps []parity.Capability, tree map[string]command, tools surfaces) []string {
	var problems []string
	covered := map[string]bool{}
	for _, c := range caps {
		for _, entry := range c.CLI {
			found := false
			for path := range tree {
				if matchesPath(entry, path) {
					covered[path] = true
					found = true
				}
			}
			if !found {
				problems = append(problems, fmt.Sprintf("capability %q names command %q, which does not exist", c.ID, entry))
			}
		}
		if c.Mode != "" {
			for _, entry := range c.CLI {
				if cmd, ok := tree[entry]; ok && cmd.cmd.Flags().Lookup(c.Mode) == nil {
					problems = append(problems, fmt.Sprintf("capability %q: command %q has no --%s mode flag", c.ID, entry, c.Mode))
				}
			}
		}
	}
	for _, path := range sortedKeys(tree) {
		if tree[path].runnable && !covered[path] {
			problems = append(problems, fmt.Sprintf("command %q is in no capability: pair it with a tool or list it as cli-only with a reason", path))
		}
	}

	owner := map[parity.Server]map[string]string{parity.Authoring: {}, parity.Skills: {}}
	for _, c := range caps {
		if c.Tool == "" {
			continue
		}
		server := c.ServerOrDefault()
		if _, ok := tools[server][c.Tool]; !ok {
			problems = append(problems, fmt.Sprintf("capability %q names tool %q, which the %s server does not list", c.ID, c.Tool, server))
		}
		if prev, dup := owner[server][c.Tool]; dup {
			problems = append(problems, fmt.Sprintf("tool %q is in capabilities %q and %q", c.Tool, prev, c.ID))
		}
		owner[server][c.Tool] = c.ID
	}
	for _, server := range []parity.Server{parity.Authoring, parity.Skills} {
		for _, name := range sortedKeys(tools[server]) {
			if _, ok := owner[server][name]; !ok {
				problems = append(problems, fmt.Sprintf("tool %q of the %s server is in no capability: pair it with a command or list it as mcp-only with a reason", name, server))
			}
		}
	}
	return problems
}

// shape reports rows that are malformed: ids, kinds, reasons.
func shape(caps []parity.Capability) []string {
	var problems []string
	ids := map[string]bool{}
	for _, c := range caps {
		if c.ID == "" || ids[c.ID] {
			problems = append(problems, fmt.Sprintf("capability id %q is empty or repeated", c.ID))
		}
		ids[c.ID] = true
		reasonOK := func(what, reason string) {
			if len(strings.TrimSpace(reason)) < minReason {
				problems = append(problems, fmt.Sprintf("capability %q: %s needs a reason of at least %d characters, has %q", c.ID, what, minReason, reason))
			}
		}
		switch c.Kind {
		case parity.CLIOnly:
			if len(c.CLI) == 0 || c.Tool != "" {
				problems = append(problems, fmt.Sprintf("capability %q is cli-only: it needs a CLI path and no tool", c.ID))
			}
			reasonOK("a cli-only row", c.Reason)
		case parity.MCPOnly:
			if c.Tool == "" || len(c.CLI) != 0 {
				problems = append(problems, fmt.Sprintf("capability %q is mcp-only: it needs a tool and no CLI path", c.ID))
			}
			reasonOK("an mcp-only row", c.Reason)
		case parity.Paired:
			if c.Tool == "" || len(c.CLI) == 0 {
				problems = append(problems, fmt.Sprintf("capability %q is paired: it needs a CLI path and a tool", c.ID))
			}
		default:
			problems = append(problems, fmt.Sprintf("capability %q has unknown kind %q", c.ID, c.Kind))
		}
		for _, group := range append(append([]parity.Exclusion(nil), c.CLIFlags...), c.ToolArgs...) {
			if len(group.Names) == 0 {
				problems = append(problems, fmt.Sprintf("capability %q has an exclusion with no names", c.ID))
			}
			reasonOK("an exclusion "+strings.Join(group.Names, ","), group.Reason)
		}
	}
	return problems
}

var placeholder = regexp.MustCompile(`[<\[]([A-Za-z][A-Za-z-]*)`)

// positionals are the tool arguments a command takes as positional arguments,
// read from the placeholders of its Use line ("add rule <name>").
func positionals(use string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholder.FindAllStringSubmatch(use, -1) {
		out[parity.Normalize(m[1])] = true
	}
	return out
}

// jsonType normalizes a pflag type to the JSON schema type a tool argument has.
func jsonType(flagType string) string {
	switch flagType {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "count":
		return "integer"
	case "float32", "float64":
		return "number"
	case "stringSlice", "stringArray", "intSlice":
		return "array"
	}
	return "string"
}

// schemaType is the JSON type of a tool property, null removed.
func schemaType(s *jsonschema.Schema) string {
	types := s.Types
	if s.Type != "" {
		types = []string{s.Type}
	}
	for _, t := range types {
		if t != "null" {
			return t
		}
	}
	return ""
}

func requiredFlag(f *pflag.Flag) bool {
	return len(f.Annotations[cobra.BashCompOneRequiredFlag]) > 0 && f.Annotations[cobra.BashCompOneRequiredFlag][0] == "true"
}

func names(groups []parity.Exclusion) map[string]bool {
	out := map[string]bool{}
	for _, g := range groups {
		for _, n := range g.Names {
			out[n] = true
		}
	}
	return out
}

// contract compares the flags of a paired command with the arguments of its
// tool: every pair exists with the same type, enum and required-ness, and every
// other flag and argument is named in an exclusion with a reason.
func contract(c parity.Capability, cmd *cobra.Command, tl tool) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("capability %q (%s <-> %s): %s", c.ID, cmd.CommandPath(), tl.name, fmt.Sprintf(format, args...)))
	}
	cmd.InheritedFlags() // merges the persistent flags of the parents into cmd.Flags()
	local := cmd.LocalFlags()
	props := tl.schema.Properties
	required := map[string]bool{}
	for _, r := range tl.schema.Required {
		required[r] = true
	}

	pairedFlags, pairedArgs := map[string]bool{}, map[string]bool{}
	pairs := append([]parity.FlagPair(nil), c.Flags...)
	declared := map[string]bool{}
	for _, p := range pairs {
		declared[p.Flag] = true
	}
	// The root --config-dir is the one configuration-directory flag of every
	// command, and config_dir is the one argument of the tools: they pair without a row.
	if _, hasArg := props["config_dir"]; hasArg && !declared["config-dir"] && cmd.Flags().Lookup("config-dir") != nil && !names(c.CLIFlags)["config-dir"] {
		pairs = append(pairs, parity.FlagPair{Flag: "config-dir"})
	}
	for _, p := range pairs {
		flag := cmd.Flags().Lookup(p.Flag)
		arg := p.ArgOf()
		prop := props[arg]
		switch {
		case flag == nil:
			add("flag --%s does not exist", p.Flag)
			continue
		case prop == nil:
			add("argument %q does not exist (paired with --%s)", arg, p.Flag)
			continue
		}
		pairedFlags[p.Flag], pairedArgs[arg] = true, true
		want, got := jsonType(flag.Value.Type()), schemaType(prop)
		switch {
		case want != got && p.TypeNote == "":
			add("--%s is a %s flag (%s) but argument %q is %s", p.Flag, flag.Value.Type(), want, arg, got)
		case want == got && p.TypeNote != "":
			add("--%s and argument %q have the same type, so the type note is stale", p.Flag, arg)
		}
		if got == "array" && prop.Items != nil && schemaType(prop.Items) != "string" {
			add("argument %q is an array of %s, the flag takes strings", arg, schemaType(prop.Items))
		}
		for _, v := range prop.Enum {
			value := fmt.Sprint(v)
			if !strings.Contains(flag.Usage, value) && p.EnumNote == "" {
				add("argument %q allows %q, which the usage of --%s does not mention", arg, value, p.Flag)
			}
		}
		if required[arg] && !requiredFlag(flag) {
			add("argument %q is required but --%s is optional", arg, p.Flag)
		}
	}

	cliEx, toolEx := names(c.CLIFlags), names(c.ToolArgs)
	global, globalArgs := names(parity.GlobalCLIFlags()), names(parity.GlobalToolArgs())
	for n := range cliEx {
		if cmd.Flags().Lookup(n) == nil {
			add("cli exclusion names --%s, which does not exist", n)
		}
		if pairedFlags[n] {
			add("--%s is both paired and excluded", n)
		}
	}
	for n := range toolEx {
		if props[n] == nil {
			add("tool exclusion names argument %q, which does not exist", n)
		}
		if pairedArgs[n] {
			add("argument %q is both paired and excluded", n)
		}
	}

	if c.Mode == "" {
		var unlisted []string
		local.VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" || pairedFlags[f.Name] || cliEx[f.Name] || global[f.Name] {
				return
			}
			unlisted = append(unlisted, "--"+f.Name)
		})
		if len(unlisted) > 0 {
			sort.Strings(unlisted)
			add("flags with no tool argument and no exclusion: %s", strings.Join(unlisted, " "))
		}
	}
	pos := positionals(cmd.Use)
	var unpaired []string
	for _, n := range sortedKeys(props) {
		if !pairedArgs[n] && !toolEx[n] && !globalArgs[n] && !pos[n] {
			unpaired = append(unpaired, n)
		}
	}
	if len(unpaired) > 0 {
		add("tool arguments with no flag and no exclusion: %s", strings.Join(unpaired, " "))
	}
	return problems
}

// contracts runs the contract of every paired capability.
func contracts(caps []parity.Capability, tree map[string]command, tools surfaces) []string {
	var problems []string
	for _, c := range caps {
		if c.Kind != parity.Paired || len(c.CLI) != 1 {
			continue
		}
		cmd, ok := tree[c.CLI[0]]
		tl, hasTool := tools[c.ServerOrDefault()][c.Tool]
		if !ok || !hasTool {
			continue // reported by coverage
		}
		problems = append(problems, contract(c, cmd.cmd, tl)...)
	}
	return problems
}
