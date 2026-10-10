package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagSpec names a flag that more than one command has. Its name, shorthand and
// default usage text are defined once (def), and commands register the flag
// through the spec so the three never drift apart; a command whose behavior needs
// its own wording passes it as the usage argument.
//
// The only shorthands in the CLI are -C -D -T -q (global) and -n -o -y (below).
// Everything else is spelled out; flag_taxonomy_test.go walks the command tree
// and fails on any other shorthand, on a flag redeclared from the global set and
// on a flag without usage text.
type flagSpec uint8

const (
	// specDryRun is -n, as in git, rsync and make: compute and report, write nothing.
	specDryRun flagSpec = iota
	// specYes skips a confirmation prompt (required without a terminal).
	specYes
	// specOutput is a file the result is written to; specOutputDir is a directory.
	specOutput
	specOutputDir

	specProfile
	specRole
	specDomain
	specTargets
	specRecursive
	specForce
	specLocal
	specNoLocal

	// specConfigDir is global (root.go); no command declares its own copy.
	specConfigDir
)

type flagDef struct{ name, short, usage string }

func (s flagSpec) def() flagDef {
	switch s {
	case specDryRun:
		return flagDef{"dry-run", "n", "Show what would change and write nothing"}
	case specYes:
		return flagDef{"yes", "y", "Do not ask for confirmation (required without a terminal)"}
	case specOutput:
		return flagDef{"output", "o", "Write the result to this file instead of stdout"}
	case specOutputDir:
		return flagDef{"output-dir", "", "Write the result into this directory"}
	case specProfile:
		return flagDef{flagServeProfile, "", "Profile to use, or a comma-separated list to compose several (default: from config or 'default')"}
	case specRole:
		return flagDef{flagRole, "", "Only the content slice of this role (see 'ai-rulez roles list')"}
	case specDomain:
		return flagDef{flagServeDomain, "", "Domain name (optional, uses root if not specified)"}
	case specTargets:
		return flagDef{flagServeTargets, "", "Comma-separated target presets, providers or path globs"}
	case specRecursive:
		return flagDef{"recursive", "", "Process every configuration found below the working directory"}
	case specForce:
		return flagDef{"force", "", "Overwrite existing files"}
	case specLocal:
		return flagDef{flagLocal, "", "Use the machine-local tree (.ai-rulez/local/)"}
	case specNoLocal:
		return flagDef{"no-local", "", usageNoLocal}
	case specConfigDir:
		return flagDef{"config-dir", "", "Configuration directory name below the working directory (default: .ai-rulez, then .config/ai-rulez)"}
	}
	panic(fmt.Sprintf("unknown flag spec %d", s))
}

// usageNoLocal is the --no-local text the commands that load a project share.
const usageNoLocal = "Ignore the machine-local config.local.* overlay and local/ content"

func (s flagSpec) text(usage []string) string {
	if len(usage) > 0 && usage[0] != "" {
		return usage[0]
	}
	return s.def().usage
}

// Bool registers the spec as a bool flag, false by default.
func (s flagSpec) Bool(fs *pflag.FlagSet, dst *bool, usage ...string) {
	d := s.def()
	fs.BoolVarP(dst, d.name, d.short, false, s.text(usage))
}

// String registers the spec as a string flag, empty by default.
func (s flagSpec) String(fs *pflag.FlagSet, dst *string, usage ...string) {
	d := s.def()
	fs.StringVarP(dst, d.name, d.short, "", s.text(usage))
}

// addPolicyFlags gives each command the --policy-* group: the flags of the
// commands that evaluate the organization policy (generate, validate, lock, ...).
// Every other command still honors AI_RULEZ_POLICY and the managed policy path, it
// just has no flags for them.
func addPolicyFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		registerPolicyFlags(c.Flags())
	}
}

// addPersistentPolicyFlags is addPolicyFlags for a command whose subcommands
// evaluate policy as well.
func addPersistentPolicyFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		registerPolicyFlags(c.PersistentFlags())
	}
}

// isPolicyFlag reports whether name belongs to the --policy-* group.
func isPolicyFlag(name string) bool {
	return name == "policy" || name == "discover-org" || strings.HasPrefix(name, "policy-")
}
