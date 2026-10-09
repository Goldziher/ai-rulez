package commands

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// showOptions and editOptions hold the flags of one show or edit command.
type showOptions struct {
	domain string
	local  bool
}

type editOptions struct {
	domain      string
	local       bool
	content     string
	priority    string
	targets     string
	description string
	severity    string
	tools       string
}

const showLong = "Print one content item from your .ai-rulez/ configuration.\n\n" +
	"The file is printed as it is on disk. --format json prints a document with the\n" +
	"type, name, domain, path and content instead."

// NewShowCmd is the read verb of the list/show/add/edit/remove set: one
// subcommand per kind of content.
func NewShowCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "show",
		Short: "Show the content of a rule, context, skill, agent, command or check",
		Long:  showLong,
	}
	for _, k := range contentKinds() {
		var opts showOptions
		cmd := &cobra.Command{
			Use:   k.use + " <name>",
			Short: "Show a " + k.name(),
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return runShow(cmd, args[0], k, &opts) },
		}
		specDomain.String(cmd.Flags(), &opts.domain, "Domain name (optional, uses root if not specified)")
		addResultFormat(cmd.Flags())
		if k.ftype != crud.ContentTypeChecks {
			specLocal.Bool(cmd.Flags(), &opts.local, "Read from the machine-local tree (.ai-rulez/local/)")
		}
		root.AddCommand(cmd)
	}
	return root
}

// NewEditCmd is the update verb of the set.
func NewEditCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "edit",
		Short: "Replace the content of a rule, context, skill, agent, command or check",
		Long: `Rewrite an existing content item atomically.

Pass the new content with --content (use - to read it from stdin). Content that
starts with its own frontmatter replaces the whole file; otherwise a rule, context
or skill gets frontmatter from --priority and --targets. For a check, content
without frontmatter replaces only the body, and --description, --severity, --tools
and --targets are set on the check's own frontmatter. The path of the rewritten
file is printed on stdout; --format json prints a document instead.`,
	}
	for _, k := range contentKinds() {
		var opts editOptions
		cmd := &cobra.Command{
			Use:   k.use + " <name>",
			Short: "Edit a " + k.name(),
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return runEdit(cmd, args[0], k, &opts) },
		}
		flags := cmd.Flags()
		specDomain.String(flags, &opts.domain, "Domain name (optional, uses root if not specified)")
		flags.StringVar(&opts.content, "content", "", "New content, or - to read it from stdin")
		addResultFormat(flags)
		if k.ftype != crud.ContentTypeChecks {
			specLocal.Bool(flags, &opts.local, "Edit in the machine-local tree (.ai-rulez/local/)")
		}
		switch k.ftype {
		case crud.ContentTypeRules, crud.ContentTypeContext, crud.ContentTypeSkills:
			flags.StringVar(&opts.priority, "priority", "", "Priority level: critical|high|medium|low|minimal")
			specTargets.String(flags, &opts.targets, "Comma-separated target providers or path globs")
		case crud.ContentTypeChecks:
			flags.StringVar(&opts.description, "description", "", "Description")
			flags.StringVar(&opts.severity, "severity", "", "Severity: low|medium|high|critical")
			flags.StringVar(&opts.tools, "tools", "", "Comma-separated review tools")
			specTargets.String(flags, &opts.targets, "Comma-separated target presets, paths or globs")
		}
		root.AddCommand(cmd)
	}
	return root
}

type contentKind struct {
	use   string // the subcommand and the type name in documents
	ftype string
	noun  string // how help text names it, when not use
}

func (k contentKind) name() string {
	if k.noun != "" {
		return k.noun
	}
	return k.use
}

func contentKinds() []contentKind {
	return []contentKind{
		{"rule", crud.ContentTypeRules, ""},
		{"context", crud.ContentTypeContext, "context file"},
		{"skill", crud.ContentTypeSkills, ""},
		{"agent", crud.ContentTypeAgents, ""},
		{kindCommand, crud.ContentTypeCommands, ""},
		{"check", crud.ContentTypeChecks, "code-review check"},
	}
}

// contentFile is the file that holds the item: the SKILL.md inside a skill's directory.
func contentFile(op *crud.OperatorImpl, domain, ftype, name string) string {
	path := op.ContentPath(domain, ftype, name)
	if ftype == crud.ContentTypeSkills {
		return filepath.Join(path, "SKILL.md")
	}
	return path
}

type showResult struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Domain  string `json:"domain,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func runShow(cmd *cobra.Command, name string, k contentKind, opts *showOptions) error {
	out := outFor(cmd)
	op, err := newContentOperator(opts.local)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	if err := op.RequireContent(cmdContext(), opts.domain, k.ftype, name); err != nil {
		return failMsg("Failed to show "+k.use, err)
	}
	path := contentFile(op, opts.domain, k.ftype, name)
	content, err := op.ReadFileContent(path)
	if err != nil {
		return failMsg("Failed to read "+k.use, err)
	}
	if out.JSON() {
		return fail(jsondoc.Write(out.Stdout(), showResult{
			Type: k.use, Name: name, Domain: opts.domain, Path: displayPath(path), Content: content,
		}))
	}
	out.Result("%s", content)
	if !strings.HasSuffix(content, "\n") {
		out.Resultln()
	}
	return nil
}

func runEdit(cmd *cobra.Command, name string, k contentKind, opts *editOptions) error {
	out := outFor(cmd)
	content, given := opts.content, cmd.Flags().Changed("content")
	if content == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return failMsg("Failed to read the content from stdin", err)
		}
		content = string(data)
	}
	op, err := newWritingOperator(opts.local)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	ctx := cmdContext()

	var result *crud.FileResult
	if k.ftype == crud.ContentTypeChecks {
		result, err = op.UpdateCheck(ctx, opts.domain, name, content, given, crud.CheckFields{
			Description: opts.description, Severity: opts.severity, Tools: splitList(opts.tools), Targets: splitList(opts.targets),
		})
	} else {
		if !given {
			return failMsg("Failed to edit "+k.use, oops.Hint("Pass the new content with --content (or - for stdin).").Errorf("nothing to change for %s %q", k.use, name))
		}
		result, err = op.UpdateFile(ctx, opts.domain, k.ftype, name, content, opts.priority, splitList(opts.targets))
	}
	if err != nil {
		return failMsg("Failed to edit "+k.use, err)
	}
	out.Info("%s updated successfully\n", strings.ToUpper(k.use[:1])+k.use[1:])
	return reportChange(out, out.JSON(), changeResult{
		Status: statusUpdated, Type: k.use, Name: result.Name, Domain: result.Domain, Path: result.FullPath, Local: opts.local,
	}, true)
}
