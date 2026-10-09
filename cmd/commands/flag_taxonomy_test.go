package commands

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// allowedShorthands is every single-letter flag the CLI has, with the one flag
// name each letter stands for. Anything else is spelled out.
var allowedShorthands = map[string]string{
	"C": "config",
	"D": "debug",
	"T": "token",
	"n": "dry-run",
	"o": "output",
	"q": "quiet",
	"y": "yes",
}

// universalShorthands are the flags that carry their shorthand on every command
// that has them.
var universalShorthands = map[string]string{
	"dry-run": "n",
	"output":  "o",
	"yes":     "y",
}

// policyCommands are the commands that evaluate the organization policy and so
// carry the --policy-* flags (the command path, without the binary name).
var policyCommands = map[string]bool{
	"generate": true, "validate": true, "scan": true, "lock": true, "doctor": true,
	"verify": true, "catalog": true, "mcp": true, "sbom": true, "sign": true,
	"tokens": true, "cost": true, "publish": true, "update": true,
}

// strictCommands are the commands where --strict is the shortcut of --fail-on
// warning; elsewhere the flag has a specific name (--strict-config,
// --refuse-findings).
var strictCommands = map[string]bool{
	"validate": true, "scan": true, "doctor": true, "verifiers run": true,
}

var outSpelling = regexp.MustCompile(`--out($|[^a-z-])`)

type ownedFlag struct {
	cmd  *cobra.Command
	flag *pflag.Flag
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), RootCmd.Name()))
}

func isPolicyFlag(name string) bool {
	return name == "policy" || name == "discover-org" || strings.HasPrefix(name, "policy-")
}

// walkOwnFlags visits every flag a command defines itself, not the ones it
// inherits from a parent's persistent set.
func walkOwnFlags(t *testing.T, visit func(cmd *cobra.Command, f *pflag.Flag)) {
	t.Helper()
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		seen := map[string]bool{}
		for _, set := range []*pflag.FlagSet{cmd.LocalNonPersistentFlags(), cmd.PersistentFlags()} {
			set.VisitAll(func(f *pflag.Flag) {
				if seen[f.Name] {
					return
				}
				seen[f.Name] = true
				visit(cmd, f)
			})
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
}

func inheritedFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	out := map[string]*pflag.Flag{}
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		p.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if _, ok := out[f.Name]; !ok {
				out[f.Name] = f
			}
		})
	}
	return out
}

func TestFlagTaxonomyShorthands(t *testing.T) {
	// Arrange
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		where := commandName(cmd) + " --" + f.Name
		if f.Shorthand != "" {
			if want, ok := allowedShorthands[f.Shorthand]; !ok {
				problems = append(problems, where+": -"+f.Shorthand+" is not one of the shorthands -C -D -T -n -o -q -y")
			} else if want != f.Name {
				problems = append(problems, where+": -"+f.Shorthand+" stands for --"+want)
			}
		}
		if want, ok := universalShorthands[f.Name]; ok && f.Shorthand != want {
			problems = append(problems, where+": must carry -"+want)
		}
		if inherited, ok := inheritedFlags(cmd)[f.Name]; ok {
			problems = append(problems, where+": redeclares the global --"+inherited.Name)
		}
		for _, in := range inheritedFlags(cmd) {
			if f.Shorthand != "" && in.Shorthand == f.Shorthand && in.Name != f.Name {
				problems = append(problems, where+": -"+f.Shorthand+" collides with the global --"+in.Name)
			}
		}
	})

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("flag shorthand problems:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyOneMeaningPerName(t *testing.T) {
	// Arrange
	type meaning struct{ typ, short, first string }
	byName := map[string]meaning{}
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		here := meaning{f.Value.Type(), f.Shorthand, commandName(cmd)}
		prev, ok := byName[f.Name]
		if !ok {
			byName[f.Name] = here
			return
		}
		sliceish := func(s string) bool { return strings.HasSuffix(s, "Slice") || strings.HasSuffix(s, "Array") }
		if prev.typ != here.typ && !(sliceish(prev.typ) && sliceish(here.typ)) {
			problems = append(problems, "--"+f.Name+" is "+prev.typ+" on "+prev.first+" but "+here.typ+" on "+here.first)
		}
		if prev.short != here.short {
			problems = append(problems, "--"+f.Name+" has shorthand \""+prev.short+"\" on "+prev.first+" but \""+here.short+"\" on "+here.first)
		}
	})
	// A command never offers two spellings of one flag.
	synonyms := [][2]string{{"out", "output"}, {"config", "config-dir"}, {"out", "output-dir"}}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, pair := range synonyms {
			if cmd.Flags().Lookup(pair[0]) != nil && cmd.Flags().Lookup(pair[1]) != nil && cmd != RootCmd {
				problems = append(problems, commandName(cmd)+": both --"+pair[0]+" and --"+pair[1])
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("a flag name must mean one thing everywhere:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyUsageText(t *testing.T) {
	// Arrange
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		if strings.TrimSpace(f.Usage) == "" {
			problems = append(problems, commandName(cmd)+" --"+f.Name+": no usage text")
		}
	})

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("every flag, hidden ones included, needs usage text:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyNoOutSpelling(t *testing.T) {
	// Arrange
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		if f.Name == "out" {
			problems = append(problems, commandName(cmd)+": --out is spelled --output (a file) or --output-dir (a directory)")
		}
		if outSpelling.MatchString(f.Usage) {
			problems = append(problems, commandName(cmd)+" --"+f.Name+": usage mentions --out")
		}
	})
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for label, text := range map[string]string{"short": cmd.Short, "long": cmd.Long, "example": cmd.Example} {
			if outSpelling.MatchString(text) {
				problems = append(problems, commandName(cmd)+": "+label+" text mentions --out")
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("--out is gone:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyNoPositionalConfigPath(t *testing.T) {
	// Arrange
	var problems []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		// Act
		if strings.Contains(cmd.Use, "[config-file]") || strings.Contains(cmd.Use, "[config") {
			problems = append(problems, commandName(cmd)+": Use still takes a positional config path: "+cmd.Use)
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)

	// Assert
	if len(problems) > 0 {
		t.Fatalf("the config location is -C/--config or --config-dir, never an argument:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyPolicyFlagsOnlyWherePolicyIsEvaluated(t *testing.T) {
	// Arrange
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		if isPolicyFlag(f.Name) && !policyCommands[commandName(cmd)] {
			problems = append(problems, commandName(cmd)+" --"+f.Name)
		}
	})
	covered := map[string]bool{}
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		if f.Name == "policy" {
			covered[commandName(cmd)] = true
		}
	})
	for name := range policyCommands {
		if !covered[name] {
			problems = append(problems, name+": evaluates policy but has no --policy")
		}
	}

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("policy flags belong on the commands that evaluate policy only:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlagTaxonomyStrictMeaning(t *testing.T) {
	// Arrange
	var problems []string
	walkOwnFlags(t, func(cmd *cobra.Command, f *pflag.Flag) {
		// Act
		if f.Name == "strict" && !strictCommands[commandName(cmd)] {
			problems = append(problems, commandName(cmd)+": --strict is only the --fail-on warning shortcut; use --strict-config or --refuse-findings")
		}
	})
	for _, name := range []string{"generate", "lock"} {
		want := map[string]string{"generate": "strict-config", "lock": "refuse-findings"}[name]
		cmd, _, err := RootCmd.Find([]string{name})
		if err != nil || cmd.Flags().Lookup(want) == nil {
			problems = append(problems, name+": missing --"+want)
		}
	}

	// Assert
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("--strict has one meaning:\n%s", strings.Join(problems, "\n"))
	}
}
