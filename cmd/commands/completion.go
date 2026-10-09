package commands

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// Shell completion. `ai-rulez completion bash|zsh|fish|powershell` is cobra's
// own command; this file teaches the scripts it prints what to offer:
//
//   - the values of enum flags (--format, --fail-on, --severity, --harness ...),
//     from the same lists the flags' help text names;
//   - the names that live in the project (rules, skills, agents, domains,
//     profiles, includes, roles, installed skills), read when the user presses
//     TAB, so a stale index never offers a deleted item;
//   - nothing at all (no file names) for a command that takes no argument.
//
// A completion never fails and never writes: a project that does not load
// offers no names, and the shell falls back to nothing instead of an error.

// completionSpec says what to complete. One rule per line, "<directive> <key> = <value...>":
//
//	enum  <flag>[@<command>]   fixed values of a flag; "$tokenizers", "$presets" and
//	                           "$feedback-kinds" stand for lists the code owns
//	names <flag>[@<command>]   names read from the project for a flag; "-" for none
//	dirs  <flag>...            flags that name a directory
//	args  <command>            positional arguments: "first <names>" completes the
//	                           first, "all <names>" every one, "after-first <names>"
//	                           all but the first, "list <v>..." fixed values for the first
//
// A <names> source is domains, profiles, includes, installed-skills, roles,
// builtins or content:<type>. A rule for "<flag>@<command>" beats one for "<flag>".
// completion_test.go checks every fixed value against the flag's help text.
const completionSpec = `
enum  fail-on          = error warning info none
enum  fail-on@convert  = approximated dropped needs-action unsupported
enum  severity         = low medium high critical
enum  priority         = critical high medium low minimal
enum  lint-profile     = default strict permissive
enum  gate-level       = info warning error
enum  isolation        = none auto require
enum  harness          = claude codex cursor
enum  tokenizer        = $tokenizers
enum  mode             = cases activation
enum  mode@search      = lexical hybrid vector
enum  scope            = domain all
enum  surface          = retrieval native
enum  runner           = claude-plugin-eval command claude-native codex-native
enum  max-cost-mode    = expected high
enum  rubric-mode      = single items
enum  delivery         = static served both
enum  index-style      = body frontmatter
enum  protocol         = http/json http/protobuf grpc
enum  outcome          = loaded used abandoned
enum  emit             = cursor-team-marketplace agent-plugins ard port aws-agent-registry kiro-steering
enum  format@sbom      = cyclonedx spdx-json
enum  from@convert     = auto native rulesync apm tessl okf skills-lock agent-plugins
enum  from@eval import = tessl
enum  into@import okf  = rules context skills
enum  to@publish       = github-release npm oci
enum  to@telemetry export = file otlp
enum  kind@lock        = include skill source served
enum  kind@update      = include skill source
enum  kind@telemetry feedback = $feedback-kinds
enum  kind@verifiers suggest = rule skill agent command
enum  content@review          = full descriptions
enum  content@review calibrate = full descriptions
enum  content@review fix       = full descriptions
enum  target           = $presets
enum  targets@lock     = $presets
enum  targets@mcp      = $presets
enum  targets@search   = $presets

names profile          = profiles
names role             = roles
names domain           = domains
names domain@convert   = -
names domain@import okf = -

dirs  config-dir root repo-root dist cache-dir

args  remove rule      = first content:rules
args  remove context   = first content:context
args  remove skill     = first content:skills
args  remove agent     = first content:agents
args  remove command   = first content:commands
args  remove check     = first content:checks
args  edit rule        = first content:rules
args  edit context     = first content:context
args  edit skill       = first content:skills
args  edit agent       = first content:agents
args  edit command     = first content:commands
args  edit check       = first content:checks
args  show rule        = first content:rules
args  show context     = first content:context
args  show skill       = first content:skills
args  show agent       = first content:agents
args  show command     = first content:commands
args  show check       = first content:checks
args  domain remove    = first domains
args  profile remove   = first profiles
args  profile set-default = first profiles
args  profile add      = after-first domains
args  include remove   = first includes
args  skill remove     = first installed-skills
args  skill update     = all installed-skills
args  roles show       = first roles
args  roles resolve    = first roles
args  builtins show    = first builtins
args  eval run         = all content:skills
args  improve run      = first content:skills
args  telemetry feedback = first content:skills
args  publish emit     = list cursor-team-marketplace agent-plugins ard port aws-agent-registry kiro-steering
args  review explain   = list AR9G0 AR9G1 AR9G2 AR9G3 AR9G4 AR9G5 AR9G6 AR9G7 AR9G8 AR9G9
`

// Argument modes and name sources of completionSpec.
const (
	modeFirst      = "first"
	modeAll        = "all"
	modeAfterFirst = "after-first"
	modeList       = "list"

	sourceRoles     = "roles"
	sourceBuiltins  = "builtins"
	sourceDomains   = "domains"
	sourceProfiles  = "profiles"
	sourceIncludes  = "includes"
	sourceInstalled = "installed-skills"
)

// completionRules is completionSpec parsed: directive -> key -> values.
type completionRules map[string]map[string][]string

func parseCompletionSpec(spec string) completionRules {
	rules := completionRules{}
	for _, line := range strings.Split(spec, "\n") {
		head, values, hasValues := strings.Cut(line, "=")
		fields := strings.Fields(head)
		switch {
		case len(fields) < 2:
		case hasValues:
			addRule(rules, fields[0], strings.Join(fields[1:], " "), strings.Fields(values))
		default: // "dirs a b c": the flags are the values
			addRule(rules, fields[0], "", fields[1:])
		}
	}
	return rules
}

func addRule(rules completionRules, directive, key string, values []string) {
	if rules[directive] == nil {
		rules[directive] = map[string][]string{}
	}
	rules[directive][key] = values
}

// lookup returns the rule for a flag in a command: "<flag>@<command>" first, then "<flag>".
func (r completionRules) lookup(directive, flag, path string) ([]string, bool) {
	if v, ok := r[directive][flag+"@"+path]; ok {
		return v, true
	}
	v, ok := r[directive][flag]
	return v, ok
}

// registerCompletions wires value and argument completion into every command
// below root. prepareCommandTree calls it once, after the whole tree exists.
func registerCompletions(root *cobra.Command) {
	rules := parseCompletionSpec(completionSpec)
	walkTree(root, func(c *cobra.Command) {
		path := commandPath(root, c)
		completeArgs(rules, path, c)
		c.LocalFlags().VisitAll(func(f *pflag.Flag) { completeFlag(rules, path, c, f) })
	})
}

func walkTree(c *cobra.Command, visit func(*cobra.Command)) {
	visit(c)
	for _, sub := range c.Commands() {
		walkTree(sub, visit)
	}
}

// commandPath is the command's path below the root ("add rule"); "" for the root.
func commandPath(root, c *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), root.Name()))
}

// completeArgs gives a command its positional-argument completion.
func completeArgs(rules completionRules, path string, c *cobra.Command) {
	if c.ValidArgsFunction != nil || c.HasSubCommands() || path == "" {
		return
	}
	if spec, ok := rules["args"][path]; ok && len(spec) > 1 {
		c.ValidArgsFunction = argCompleter(spec[0], spec[1:])
		return
	}
	if isNoArgs(c) {
		c.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

// isNoArgs reports whether the command declares that it takes no positional argument.
func isNoArgs(c *cobra.Command) bool {
	probe := &cobra.Command{}
	return c.Args != nil && c.Args(probe, nil) == nil && c.Args(probe, []string{"x"}) != nil
}

// argCompleter builds the completion of positional arguments: mode is first,
// all, after-first or list; source is a names source (or the fixed values for list).
func argCompleter(mode string, source []string) cobra.CompletionFunc {
	names := func(cmd *cobra.Command) []string {
		if mode == modeList {
			return source
		}
		return namesFrom(source[0], cmd)
	}
	return func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		const noFiles = cobra.ShellCompDirectiveNoFileComp
		switch {
		case (mode == modeFirst || mode == modeList) && len(args) > 0:
			return nil, noFiles
		case mode == modeAfterFirst && len(args) == 0:
			return nil, noFiles
		}
		given := map[string]bool{}
		for _, a := range args {
			given[a] = true
		}
		var out []string
		for _, n := range filterPrefix(names(cmd), prefix) {
			if mode != modeAll || !given[n] {
				out = append(out, n)
			}
		}
		return out, noFiles
	}
}

// completeFlag registers the value completion of one flag of c.
func completeFlag(rules completionRules, path string, c *cobra.Command, f *pflag.Flag) {
	var fn cobra.CompletionFunc
	source, named := rules.lookup("names", f.Name, path)
	switch values := flagValues(rules, path, f); {
	case values != nil:
		fn = listValues(values, isListFlag(f))
	case named && len(source) == 1 && source[0] != "-":
		fn = listDynamic(func(cmd *cobra.Command) []string { return namesFrom(source[0], cmd) })
	case !named && containsString(rules["dirs"][""], f.Name):
		fn = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
	}
	if fn != nil {
		// RegisterFlagCompletionFunc only fails for an unknown flag or a second
		// registration; neither can happen when walking the flags of c once.
		_ = c.RegisterFlagCompletionFunc(f.Name, fn) //nolint:errcheck // see above
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isListFlag(f *pflag.Flag) bool {
	_, ok := f.Value.(pflag.SliceValue)
	return ok
}

// flagValues returns the fixed values a flag accepts, or nil: the values its
// --format annotation declares, or the enum rule of the spec.
func flagValues(rules completionRules, path string, f *pflag.Flag) []string {
	if allowed := f.Annotations[formatValuesAnnotation]; len(allowed) > 0 {
		return allowed
	}
	values, ok := rules.lookup("enum", f.Name, path)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range values {
		out = append(out, expandValue(v)...)
	}
	return out
}

// expandValue resolves the "$name" placeholders of the spec to the lists the code owns.
func expandValue(v string) []string {
	switch v {
	case "$tokenizers":
		return tokens.Names()
	case "$presets":
		return config.IndividualPresetNames()
	case "$feedback-kinds":
		return usage.FeedbackKinds
	}
	return []string{v}
}

// listValues offers the values starting with the typed prefix. A list flag takes
// a comma-separated list, so the part after the last comma is completed.
func listValues(values []string, list bool) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
		return completeList(values, prefix, list), cobra.ShellCompDirectiveNoFileComp
	}
}

// listDynamic is listValues for names read from the project.
func listDynamic(names func(*cobra.Command) []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
		return completeList(names(cmd), prefix, true), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeList(values []string, prefix string, list bool) []string {
	head := ""
	if list {
		if i := strings.LastIndex(prefix, ","); i >= 0 {
			head, prefix = prefix[:i+1], prefix[i+1:]
		}
	}
	out := filterPrefix(values, prefix)
	for i := range out {
		out[i] = head + out[i]
	}
	return out
}

// filterPrefix returns the sorted, de-duplicated values that start with prefix.
func filterPrefix(values []string, prefix string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v != "" && strings.HasPrefix(v, prefix) && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// namesFrom reads one kind of name from the project. A failure (no project, a
// configuration that does not load) offers no names.
func namesFrom(source string, cmd *cobra.Command) []string {
	if ftype, ok := strings.CutPrefix(source, "content:"); ok {
		return contentNames(ftype, cmd)
	}
	switch source {
	case sourceBuiltins:
		return builtinNames()
	case sourceRoles:
		return roleNames()
	}
	return operatorNames(source)
}

func contentNames(ftype string, cmd *cobra.Command) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	domain, _ := cmd.Flags().GetString("domain") //nolint:errcheck // absent on commands without --domain
	files, err := op.ListFiles(cmdContext(), domain, ftype)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}

func roleNames() []string {
	ctx := config.WithOfflineIncludes(config.WithUnresolvedIncludesTolerated(cmdContext()))
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutRemote())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Roles))
	for i := range cfg.Roles {
		names = append(names, cfg.Roles[i].Name)
	}
	return names
}

func builtinNames() []string {
	list := builtins.List()
	names := make([]string, 0, len(list))
	for _, d := range list {
		names = append(names, d.Name)
	}
	return names
}
