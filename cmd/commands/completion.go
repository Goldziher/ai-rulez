package commands

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
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

// completionContentTypes maps the `<verb> <type>` subcommand to its content type.
func completionContentTypes() map[string]string {
	return map[string]string{
		"rule":    crud.ContentTypeRules,
		"context": crud.ContentTypeContext,
		"skill":   crud.ContentTypeSkills,
		"agent":   crud.ContentTypeAgents,
		"command": crud.ContentTypeCommands,
		"check":   crud.ContentTypeChecks,
	}
}

func severityValues() []string { return []string{"error", "warning", "info", "none"} }
func harnessValues() []string  { return []string{"claude", "codex", "cursor"} }
func convertStatuses() []string {
	return []string{"approximated", "dropped", "needs-action", "unsupported"}
}

func emitterValues() []string {
	return []string{"cursor-team-marketplace", "agent-plugins", "ard", "port", "aws-agent-registry", "kiro-steering"}
}

func convertImporters() []string {
	return []string{"auto", "native", "rulesync", "apm", "tessl", "okf", "skills-lock", "agent-plugins"}
}

// registerCompletions wires value and argument completion into every command
// below root. root.go calls it once, after the whole tree exists.
func registerCompletions(root *cobra.Command) {
	walkTree(root, func(c *cobra.Command) {
		completeArgs(root, c)
		c.LocalFlags().VisitAll(func(f *pflag.Flag) { completeFlag(root, c, f) })
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
func completeArgs(root, c *cobra.Command) {
	if c.ValidArgsFunction != nil || c.HasSubCommands() {
		return
	}
	path := commandPath(root, c)
	verb, kind, _ := strings.Cut(path, " ")
	switch {
	case (verb == "remove" || verb == "edit" || verb == "show") && completionContentTypes()[kind] != "":
		c.ValidArgsFunction = firstArg(contentNames(completionContentTypes()[kind]))
	case path == "domain remove":
		c.ValidArgsFunction = firstArg(domainNames)
	case path == "profile remove" || path == "profile set-default":
		c.ValidArgsFunction = firstArg(profileNames)
	case path == "profile add":
		c.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterPrefix(domainNames(cmd), prefix), cobra.ShellCompDirectiveNoFileComp
		}
	case path == "include remove":
		c.ValidArgsFunction = firstArg(includeNames)
	case path == "skill remove":
		c.ValidArgsFunction = firstArg(installedSkillNames)
	case path == "skill update":
		c.ValidArgsFunction = allArgs(installedSkillNames)
	case path == "roles show" || path == "roles resolve":
		c.ValidArgsFunction = firstArg(roleNames)
	case path == "builtins show":
		c.ValidArgsFunction = firstArg(builtinNames)
	case path == "eval run":
		c.ValidArgsFunction = allArgs(contentNames(crud.ContentTypeSkills))
	case path == "improve run" || path == "telemetry feedback":
		c.ValidArgsFunction = firstArg(contentNames(crud.ContentTypeSkills))
	case path == "publish emit":
		c.ValidArgsFunction = firstArg(func(*cobra.Command) []string { return emitterValues() })
	case path == "review explain":
		c.ValidArgsFunction = firstArg(func(*cobra.Command) []string {
			return []string{"AR9G0", "AR9G1", "AR9G2", "AR9G3", "AR9G4", "AR9G5", "AR9G6", "AR9G7", "AR9G8", "AR9G9"}
		})
	case path != "" && c.Name() != "help" && isNoArgs(c):
		c.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

// isNoArgs reports whether the command declares that it takes no positional argument.
func isNoArgs(c *cobra.Command) bool {
	return c.Args != nil && c.Args(&cobra.Command{}, []string{"x"}) != nil && c.Args(&cobra.Command{}, nil) == nil
}

// firstArg completes the first positional argument from names and offers nothing after it.
func firstArg(names func(*cobra.Command) []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterPrefix(names(cmd), prefix), cobra.ShellCompDirectiveNoFileComp
	}
}

// allArgs completes every positional argument from names, skipping those already given.
func allArgs(names func(*cobra.Command) []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		given := map[string]bool{}
		for _, a := range args {
			given[a] = true
		}
		var out []string
		for _, n := range filterPrefix(names(cmd), prefix) {
			if !given[n] {
				out = append(out, n)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeFlag registers the value completion of one flag of c.
func completeFlag(root, c *cobra.Command, f *pflag.Flag) {
	path := commandPath(root, c)
	var fn cobra.CompletionFunc
	switch values := staticFlagValues(path, f); {
	case values != nil:
		fn = listValues(values, isListFlag(f))
	case f.Name == "profile":
		fn = listDynamic(profileNames)
	case f.Name == "role":
		fn = listDynamic(roleNames)
	case f.Name == "domain" && path != "convert" && path != "import okf":
		fn = listDynamic(domainNames)
	case f.Name == "config-dir" || f.Name == "root" || f.Name == "repo-root" || f.Name == "dist" || f.Name == "cache-dir":
		fn = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
	case f.Name == "target" || (f.Name == "targets" && (path == "lock" || path == "mcp" || path == "search")):
		fn = listValues(config.IndividualPresetNames(), false)
	}
	if fn != nil {
		// RegisterFlagCompletionFunc only fails for an unknown flag or a second
		// registration; neither can happen when walking the flags of c once.
		_ = c.RegisterFlagCompletionFunc(f.Name, fn)
	}
}

func isListFlag(f *pflag.Flag) bool {
	_, ok := f.Value.(pflag.SliceValue)
	return ok
}

// staticFlagValues returns the fixed values a flag accepts, or nil. The lists
// repeat what the flag's help text names; completion_test.go fails when a value
// is missing from that text.
func staticFlagValues(path string, f *pflag.Flag) []string {
	if allowed := f.Annotations[formatValuesAnnotation]; len(allowed) > 0 {
		return allowed
	}
	top, _, _ := strings.Cut(path, " ")
	switch f.Name {
	case "fail-on":
		if path == "convert" {
			return convertStatuses()
		}
		return severityValues()
	case "severity":
		return []string{"low", "medium", "high", "critical"}
	case "priority":
		return []string{"critical", "high", "medium", "low", "minimal"}
	case "lint-profile":
		return []string{"default", "strict", "permissive"}
	case "gate-level":
		return []string{"info", "warning", "error"}
	case "isolation":
		return []string{"none", "auto", "require"}
	case "harness":
		return harnessValues()
	case "tokenizer":
		return tokens.Names()
	case "mode":
		if top == "search" {
			return []string{"lexical", "hybrid", "vector"}
		}
		return []string{evals.ModeCases, evals.ModeActivation}
	case "scope":
		return []string{evals.ScopeDomain, evals.ScopeAll}
	case "surface":
		return []string{evals.SurfaceRetrieval, evals.SurfaceNative}
	case "runner":
		return []string{evals.RunnerClaudePluginEval, evals.RunnerCommand, evals.RunnerClaudeNative, evals.RunnerCodexNative}
	case "max-cost-mode":
		return []string{evals.CostModeExpected, evals.CostModeHigh}
	case "rubric-mode":
		return []string{"single", "items"}
	case "delivery":
		return []string{"static", "served", "both"}
	case "index-style":
		return []string{"body", "frontmatter"}
	case "protocol":
		return []string{"http/json", "http/protobuf", "grpc"}
	case "outcome":
		return []string{"loaded", "used", "abandoned"}
	case "emit":
		return emitterValues()
	}
	return pathFlagValues(path, top, f.Name)
}

// pathFlagValues covers the flags whose values depend on the command.
func pathFlagValues(path, top, flag string) []string {
	switch flag + "@" + path {
	case "format@sbom":
		return []string{"cyclonedx", "spdx-json"}
	case "from@convert":
		return convertImporters()
	case "from@eval import":
		return []string{"tessl"}
	case "into@import okf":
		return []string{"rules", "context", "skills"}
	case "to@publish":
		return []string{"github-release", "npm", "oci"}
	case "to@telemetry export":
		return []string{"file", "otlp"}
	case "kind@lock":
		return []string{"include", "skill", "source", "served"}
	case "kind@update":
		return []string{"include", "skill", "source"}
	case "kind@telemetry feedback":
		return usage.FeedbackKinds
	case "kind@verifiers suggest":
		return []string{"rule", "skill", "agent", "command"}
	}
	if flag == "content" && top == "review" {
		return []string{"full", "descriptions"}
	}
	return nil
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

// contentNames reads the names of one content type, in the domain given by
// --domain when the command has that flag.
func contentNames(ftype string) func(*cobra.Command) []string {
	return func(cmd *cobra.Command) []string {
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
}

func domainNames(*cobra.Command) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	domains, err := op.ListDomains(cmdContext())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		names = append(names, d.Name)
	}
	return names
}

func profileNames(*cobra.Command) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	profiles, err := op.ListProfiles(cmdContext())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

func includeNames(*cobra.Command) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	includes, err := op.ListIncludes(cmdContext())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(includes))
	for _, i := range includes {
		names = append(names, i.Name)
	}
	return names
}

func installedSkillNames(*cobra.Command) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	skills, err := op.ListInstalledSkills(cmdContext())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return names
}

func roleNames(*cobra.Command) []string {
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

func builtinNames(*cobra.Command) []string {
	list := builtins.List()
	names := make([]string, 0, len(list))
	for _, d := range list {
		names = append(names, d.Name)
	}
	return names
}
