package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagSpec is the one definition of a flag that more than one command has: its
// name, its shorthand and its default usage text. Commands register the flag
// through the spec so the three never drift apart; a command whose behavior
// needs its own wording passes it as the usage argument.
//
// The only shorthands in the CLI are -C -D -T -q (global) and -n -o -y (below).
// Everything else is spelled out; flag_taxonomy_test.go walks the command tree
// and fails on any other shorthand, on a flag redeclared from the global set and
// on a flag without usage text.
type flagSpec struct {
	name  string
	short string
	usage string
}

var (
	// specDryRun is -n, as in git, rsync and make: compute and report, write nothing.
	specDryRun = flagSpec{"dry-run", "n", "Show what would change and write nothing"}
	// specYes skips a confirmation prompt (required without a terminal).
	specYes = flagSpec{"yes", "y", "Do not ask for confirmation (required without a terminal)"}
	// specOutput is a file the result is written to; specOutputDir is a directory.
	specOutput    = flagSpec{"output", "o", "Write the result to this file instead of stdout"}
	specOutputDir = flagSpec{"output-dir", "", "Write the result into this directory"}

	specProfile   = flagSpec{flagServeProfile, "", "Profile to use, or a comma-separated list to compose several (default: from config or 'default')"}
	specRole      = flagSpec{"role", "", "Only the content slice of this role (see 'ai-rulez roles list')"}
	specDomain    = flagSpec{flagServeDomain, "", "Domain name (optional, uses root if not specified)"}
	specTargets   = flagSpec{flagServeTargets, "", "Comma-separated target presets, providers or path globs"}
	specRecursive = flagSpec{"recursive", "", "Process every configuration found below the working directory"}
	specForce     = flagSpec{"force", "", "Overwrite existing files"}
	specLocal     = flagSpec{"local", "", "Use the machine-local tree (.ai-rulez/local/)"}
	specNoLocal   = flagSpec{"no-local", "", usageNoLocal}

	// specConfigDir is global (root.go); no command declares its own copy.
	specConfigDir = flagSpec{"config-dir", "", "Configuration directory name (default: .ai-rulez, then .config/ai-rulez)"}
)

// usageNoLocal is the --no-local text the commands that load a project share.
const usageNoLocal = "Ignore the machine-local config.local.* overlay and local/ content"

func (s flagSpec) text(usage []string) string {
	switch {
	case len(usage) > 0 && usage[0] != "":
		return usage[0]
	case s.usage != "":
		return s.usage
	}
	panic(fmt.Sprintf("flag --%s has no usage text", s.name))
}

// Bool registers the spec as a bool flag, false by default.
func (s flagSpec) Bool(fs *pflag.FlagSet, dst *bool, usage ...string) {
	fs.BoolVarP(dst, s.name, s.short, false, s.text(usage))
}

// String registers the spec as a string flag, empty by default.
func (s flagSpec) String(fs *pflag.FlagSet, dst *string, usage ...string) {
	fs.StringVarP(dst, s.name, s.short, "", s.text(usage))
}

// policyFlagSet is the --policy-* group: the flags of the commands that evaluate
// the organization policy (generate, validate, lock, ...). Every other command
// still honors AI_RULEZ_POLICY and the managed policy path, it just has no flags
// for them.
var policyFlagSet = pflag.NewFlagSet("policy", pflag.ContinueOnError)

// addPolicyFlags gives each command the --policy-* flags. A command with
// subcommands that evaluate policy too passes persistent.
func addPolicyFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.Flags().AddFlagSet(policyFlagSet)
	}
}

// addPersistentPolicyFlags is addPolicyFlags for a command whose subcommands
// evaluate policy as well.
func addPersistentPolicyFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.PersistentFlags().AddFlagSet(policyFlagSet)
	}
}

// isPolicyFlag reports whether name belongs to the --policy-* group.
func isPolicyFlag(name string) bool {
	return name == "policy" || name == "discover-org" || strings.HasPrefix(name, "policy-")
}
