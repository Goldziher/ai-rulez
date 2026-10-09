package commands

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/spf13/cobra"
)

var (
	addDomain   string
	addPriority string
	addTargets  string
	addContent  string
	addDesc     string
	addLocal    bool
)

const addLocalUsage = "Write to .ai-rulez/local/ as a machine-local item (gitignored); combine with --domain for a local domain"

var AddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add content to your rules",
	Long: `Add rules, context, skills, agents, commands or checks to your .ai-rulez/ configuration.

The path of the created file is printed on stdout; --format json prints a document
with the type, name, domain and path instead. Names are kebab-case file names
without the .md extension and without spaces.`,
}

var addRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Add a new rule",
	Long: `Add a new rule file with optional metadata.

Rules contain instructions and guidelines for AI assistants.
You can specify domain, priority level, and target providers.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddRule,
}

var addContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Add a new context file",
	Long: `Add a new context file with optional metadata.

Context provides background information, architecture details, or project-specific information.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddContext,
}

var addSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Add a new skill",
	Long: `Add a new skill directory with SKILL.md.

Skills define specialized capabilities or personas for AI assistants.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddSkill,
}

var addAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Add a new agent",
	Long: `Add a new agent file.

Agents define specialized sub-agents for tools that support them.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddAgent,
}

var addCommandCmd = &cobra.Command{
	Use:   "command <name>",
	Short: "Add a new command",
	Long: `Add a new command file.

Commands define reusable slash commands for tools that support them.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddCommand,
}

var addCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Add a new code-review check",
	Long: `Add a new check file.

Checks are code-review guidelines (.ai-rulez/checks/<name>.md) rendered for the
review tools that support them. Frontmatter: description, severity
(low|medium|high|critical), tools, targets.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddCheck,
}

func init() {
	AddCmd.AddCommand(addRuleCmd, addContextCmd, addSkillCmd, addAgentCmd, addCommandCmd, addCheckCmd)
	for _, c := range []*cobra.Command{addRuleCmd, addContextCmd, addSkillCmd, addAgentCmd, addCommandCmd, addCheckCmd} {
		specDomain.String(c.Flags(), &addDomain, "Domain name (optional, uses root if not specified)")
		c.Flags().StringVar(&addContent, "content", "", "File content (uses template if not specified)")
		addResultFormat(c.Flags())
	}
	// Checks are shared review guidance: there is no machine-local check tree.
	for _, c := range []*cobra.Command{addRuleCmd, addContextCmd, addSkillCmd, addAgentCmd, addCommandCmd} {
		specLocal.Bool(c.Flags(), &addLocal, addLocalUsage)
	}
	for _, c := range []*cobra.Command{addSkillCmd, addAgentCmd, addCommandCmd, addCheckCmd} {
		c.Flags().StringVar(&addDesc, "description", "", "Description")
	}
	for _, c := range []*cobra.Command{addRuleCmd, addContextCmd, addSkillCmd} {
		c.Flags().StringVar(&addPriority, "priority", "medium", "Priority level: critical|high|medium|low|minimal")
		specTargets.String(c.Flags(), &addTargets, "Comma-separated target providers or path globs (e.g. claude,cursor)")
	}
	addCheckCmd.Flags().String("severity", "", "Severity: low|medium|high|critical")
	addCheckCmd.Flags().String("tools", "", "Comma-separated review tools the check is for")
	specTargets.String(addCheckCmd.Flags(), &addTargets, "Comma-separated target presets, paths or globs the check applies to")
}

func runAddRule(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeRules, "rule", (*crud.OperatorImpl).AddRule)
}

func runAddContext(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeContext, "context", (*crud.OperatorImpl).AddContext)
}

func runAddSkill(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeSkills, "skill", (*crud.OperatorImpl).AddSkill)
}

func runAddAgent(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeAgents, "agent", (*crud.OperatorImpl).AddAgent)
}

func runAddCommand(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeCommands, "command", (*crud.OperatorImpl).AddCommand)
}

func runAddCheck(cmd *cobra.Command, args []string) error {
	return runAddItem(cmd, args[0], crud.ContentTypeChecks, "check", (*crud.OperatorImpl).AddCheck)
}

// splitList splits a comma-separated flag value, dropping blanks.
func splitList(value string) []string {
	var out []string
	for _, v := range strings.Split(value, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// runAddItem creates one content item and reports its path: on stdout, or as the
// change document under --format json. The confirmation sentence goes to stderr.
func runAddItem(cmd *cobra.Command, name, ftype, label string,
	add func(*crud.OperatorImpl, context.Context, *crud.AddFileRequest) (*crud.FileResult, error),
) error {
	out := outFor(cmd)
	op, err := newWritingOperator(addLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	req := &crud.AddFileRequest{
		Domain: addDomain, Type: ftype, Name: name, Description: addDesc, Content: addContent,
		Priority: addPriority, Targets: splitList(addTargets),
	}
	if ftype == crud.ContentTypeChecks {
		req.Priority, req.Targets = "", nil
		if err := buildCheckRequest(cmd, req); err != nil {
			return failMsg("Failed to add check", err)
		}
	}
	result, err := add(op, cmdContext(), req)
	if err != nil {
		return failMsg("Failed to add "+label, err)
	}
	out.Info("%s added successfully\n", strings.ToUpper(label[:1])+label[1:])
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: label, Name: result.Name, Domain: result.Domain,
		Path: result.FullPath, Local: addLocal,
	}, true)
}

// buildCheckRequest folds --severity, --tools and --targets into the check's
// frontmatter, validated by the same code the MCP create_check tool uses.
func buildCheckRequest(cmd *cobra.Command, req *crud.AddFileRequest) error {
	severity, _ := cmd.Flags().GetString("severity") //nolint:errcheck // the flag is registered in init
	tools, _ := cmd.Flags().GetString("tools")       //nolint:errcheck // the flag is registered in init
	return foldCheckFields(req, severity, splitList(tools), splitList(addTargets))
}

func foldCheckFields(req *crud.AddFileRequest, severity string, tools, targets []string) error {
	if severity == "" && len(tools) == 0 && len(targets) == 0 {
		return nil
	}
	body := req.Content
	if body == "" {
		body = crud.GenerateCheckTemplate(req.Name, req.Description)
	}
	content, err := crud.BuildCheckContent(body, req.Description, severity, tools, targets)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	req.Content = content
	return nil
}
