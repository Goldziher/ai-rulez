package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/roles"
	"github.com/Goldziher/ai-rulez/internal/tokens"
)

var rolesFormat string

// RolesCmd groups the role inspection commands.
var RolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "List, inspect and resolve [[roles]]",
	Long: `Roles map a job to the slice of the shared content a person needs: which
domains, which skills, rules, agents and commands, and how Claude Code surfaces
each skill. ai-rulez never decides who holds a role; an identity tool or a UI
picks a role name and runs "ai-rulez generate --role <name>".

  ai-rulez roles list                   every role with its item counts
  ai-rulez roles show <name>            the role as declared and as inherited
  ai-rulez roles resolve <name>         the items the role keeps, with sizes

Each subcommand takes --format json. "roles list --format json" prints the same
document as roles.json (schema/roles-manifest.schema.json).`,
}

var rolesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the roles with their item counts and token estimates",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, _ []string) { exitOn(runRolesList(cmd.OutOrStdout())) },
}

var rolesShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a role as declared and with its parent merged in",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { exitOn(runRolesShow(cmd.OutOrStdout(), args[0])) },
}

var rolesResolveCmd = &cobra.Command{
	Use:   "resolve <name>",
	Short: "List the items a role keeps, with sizes and skill modes",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { exitOn(runRolesResolve(cmd.OutOrStdout(), args[0])) },
}

func init() {
	RolesCmd.PersistentFlags().StringVar(&rolesFormat, "format", "", "Output format: text (default) or json")
	RolesCmd.PersistentFlags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	RolesCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	RolesCmd.AddCommand(rolesListCmd, rolesShowCmd, rolesResolveCmd)
}

func exitOn(err error) {
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

func loadRolesConfig() (*config.Config, error) {
	if rolesFormat != "" && rolesFormat != formatText && rolesFormat != formatJSON {
		return nil, oops.Errorf("unknown --format %q (use text or json)", rolesFormat)
	}
	cfg, err := loadConfigForCommand(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return cfg, nil
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return oops.Wrapf(err, "encode json")
	}
	return nil
}

func runRolesList(out io.Writer) error {
	cfg, err := loadRolesConfig()
	if err != nil {
		return err
	}
	counter, err := tokens.New("")
	if err != nil {
		return oops.Wrap(err)
	}
	manifest := roles.Build(cfg, counter)
	if rolesFormat == formatJSON {
		data, err := manifest.Marshal()
		if err != nil {
			return err //nolint:wrapcheck // already contextual
		}
		_, err = out.Write(data)
		return err //nolint:wrapcheck // writer error
	}
	if len(manifest.Roles) == 0 {
		_, err = fmt.Fprintln(out, "No roles are defined. Add [[roles]] entries to config.toml.")
		return err //nolint:wrapcheck // writer error
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	tp := reportWriter{tw}
	tp.printf("ROLE\tEXTENDS\tDOMAINS\tITEMS\tSKILLS\tTOKENS\tDESCRIPTION\n")
	for i := range manifest.Roles {
		r := &manifest.Roles[i]
		tp.printf("%s\t%s\t%s\t%d\t%d\t%d\t%s\n", r.Name, dash(r.Extends), dash(strings.Join(r.Domains, ",")),
			r.Totals.Items, r.Totals.ByKind[config.RoleKindSkill], r.Totals.Tokens, r.Description)
	}
	return tw.Flush() //nolint:wrapcheck // writer error
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// roleShow is the JSON of `roles show`.
type roleShow struct {
	Declared  *config.RoleConfig   `json:"declared"`
	Effective *config.RoleConfig   `json:"effective"`
	Chain     []string             `json:"chain"`
	Problems  []config.RoleProblem `json:"problems"`
}

func runRolesShow(out io.Writer, name string) error {
	cfg, err := loadRolesConfig()
	if err != nil {
		return err
	}
	declared, ok := cfg.FindRole(name)
	if !ok {
		return oops.With("available", cfg.RoleNames()).Hint("List the roles with `ai-rulez roles list`").Errorf("role %q is not defined", name)
	}
	flat, err := cfg.FlattenRole(name)
	if err != nil {
		return oops.Wrap(err)
	}
	var problems []config.RoleProblem
	for _, p := range cfg.RoleProblems() {
		if p.Role == name {
			problems = append(problems, p)
		}
	}
	view := roleShow{Declared: declared, Effective: &flat.RoleConfig, Chain: flat.Chain, Problems: problems}
	if view.Problems == nil {
		view.Problems = []config.RoleProblem{}
	}
	if rolesFormat == formatJSON {
		return writeJSON(out, view)
	}
	printRole(reportWriter{out}, "declared", declared)
	if declared.Extends != "" {
		printRole(reportWriter{out}, "effective (with "+declared.Extends+" merged in)", &flat.RoleConfig)
	}
	w := reportWriter{out}
	for _, p := range problems {
		w.printf("problem [%s]: %s\n", p.Kind, p.Message)
	}
	return nil
}

func printRole(w reportWriter, label string, r *config.RoleConfig) {
	w.printf("%s: %s\n", r.Name, label)
	if r.Description != "" {
		w.printf("  description: %s\n", r.Description)
	}
	if r.Extends != "" {
		w.printf("  extends: %s\n", r.Extends)
	}
	w.printf("  domains: %s\n", dash(strings.Join(r.Domains, ", ")))
	for _, kind := range config.RoleKinds {
		sel := map[string]*config.RoleSelector{
			config.RoleKindRule: r.Rules, config.RoleKindSkill: r.Skills, config.RoleKindAgent: r.Agents, config.RoleKindCommand: r.Commands,
		}[kind]
		if sel == nil {
			continue
		}
		if len(sel.Include) > 0 {
			w.printf("  %ss include: %s\n", kind, strings.Join(sel.Include, ", "))
		}
		if len(sel.Exclude) > 0 {
			w.printf("  %ss exclude: %s\n", kind, strings.Join(sel.Exclude, ", "))
		}
	}
	patterns := make([]string, 0, len(r.SkillMode))
	for p := range r.SkillMode {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	for _, p := range patterns {
		w.printf("  skill_mode %s = %s\n", p, r.SkillMode[p])
	}
	deliveries := make([]string, 0, len(r.Delivery))
	for p := range r.Delivery {
		deliveries = append(deliveries, p)
	}
	sort.Strings(deliveries)
	for _, p := range deliveries {
		w.printf("  delivery %s = %s\n", p, r.Delivery[p])
	}
	if r.Match != nil && len(r.Match.Groups) > 0 {
		w.printf("  match groups: %s\n", strings.Join(r.Match.Groups, ", "))
	}
}

func runRolesResolve(out io.Writer, name string) error {
	cfg, err := loadRolesConfig()
	if err != nil {
		return err
	}
	counter, err := tokens.New("")
	if err != nil {
		return oops.Wrap(err)
	}
	role, err := roles.BuildRole(cfg, name, counter)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if rolesFormat == formatJSON {
		return writeJSON(out, map[string]any{"schema_version": roles.SchemaVersion, "tokenizer": counter.Name(), "role": role})
	}
	w := reportWriter{out}
	w.printf("role %s: %d item(s), %d bytes, ~%d tokens (%s)\n", role.Name, role.Totals.Items, role.Totals.Bytes, role.Totals.Tokens, counter.Name())
	if role.Totals.Served > 0 {
		w.printf("  %d skill(s) served over MCP (~%d tokens), not listed in the agent's context\n", role.Totals.Served, role.Totals.ServedTokens)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	tp := reportWriter{tw}
	tp.printf("KIND\tDOMAIN\tID\tMODE\tDELIVERY\tBYTES\tTOKENS\n")
	for i := range role.Items {
		it := &role.Items[i]
		tp.printf("%s\t%s\t%s\t%s\t%s\t%d\t%d\n", it.Kind, dash(it.Domain), it.ID, dash(it.Mode), dash(it.Delivery), it.Bytes, it.Tokens)
	}
	return tw.Flush() //nolint:wrapcheck // writer error
}
