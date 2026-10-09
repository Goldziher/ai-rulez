package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

// The authoring server offers more than tools: prompts for the common
// authoring jobs, and read-only resources so a client can open a rule, a skill
// or the configuration by URI without calling a tool.

const (
	resourceScheme  = "ai-rulez://"
	uriConfig       = resourceScheme + "config"
	uriCatalog      = resourceScheme + "catalog"
	uriItemTemplate = resourceScheme + "{kind}/{name}"
	uriDomainItem   = resourceScheme + "domains/{domain}/{kind}/{name}"

	mimeJSON = "application/json"
)

// itemKinds maps the {kind} of an item URI to the tool that reads it.
func (s *Server) itemReaders() map[string]handlerFunc {
	return map[string]handlerFunc{
		"rules": handlers.ReadRuleHandler, "context": handlers.ReadContextHandler, "skills": handlers.ReadSkillHandler,
		"checks": handlers.ReadCheckHandler, "agents": handlers.ReadAgentHandler, "commands": handlers.ReadCommandHandler,
	}
}

func (s *Server) registerAuthoringResources() {
	s.mcpServer.AddResource(&sdkmcp.Resource{
		URI: uriConfig, Name: "config", Title: "Project Configuration", MIMEType: mimeJSON,
		Description: "The parsed settings of the project: name, presets, profiles, builtins, includes (read_config)",
		Annotations: &sdkmcp.Annotations{Audience: []sdkmcp.Role{"assistant"}, Priority: 0.8},
	}, s.readAuthoringResource)
	s.mcpServer.AddResource(&sdkmcp.Resource{
		URI: uriCatalog, Name: "catalog", Title: "Content Catalog", MIMEType: mimeJSON,
		Description: "Every rule, skill, agent, command and context file with owner, tokens and digest (catalog)",
		Annotations: &sdkmcp.Annotations{Audience: []sdkmcp.Role{"assistant"}, Priority: 0.6},
	}, s.readAuthoringResource)
	s.mcpServer.AddResourceTemplate(&sdkmcp.ResourceTemplate{
		URITemplate: uriItemTemplate, Name: "item", Title: "Project Item", MIMEType: mimeMarkdown,
		Description: "One item of the root: ai-rulez://{kind}/{name}, kind being rules, context, skills, checks, agents or commands",
		Annotations: &sdkmcp.Annotations{Audience: []sdkmcp.Role{"assistant"}, Priority: 0.7},
	}, s.readAuthoringResource)
	s.mcpServer.AddResourceTemplate(&sdkmcp.ResourceTemplate{
		URITemplate: uriDomainItem, Name: "domain-item", Title: "Domain Item", MIMEType: mimeMarkdown,
		Description: "One item of a domain: ai-rulez://domains/{domain}/{kind}/{name}",
		Annotations: &sdkmcp.Annotations{Audience: []sdkmcp.Role{"assistant"}, Priority: 0.7},
	}, s.readAuthoringResource)
	s.mcpServer.AddReceivingMiddleware(s.unknownAuthoringResourceMiddleware())
}

// authoringTarget is what an ai-rulez:// URI names.
type authoringTarget struct {
	config, catalog    bool
	kind, domain, name string
}

// parseAuthoringURI reads an ai-rulez:// URI; ok is false when it names nothing this server serves.
func (s *Server) parseAuthoringURI(uri string) (authoringTarget, bool) {
	switch uri {
	case uriConfig:
		return authoringTarget{config: true}, true
	case uriCatalog:
		return authoringTarget{catalog: true}, true
	}
	rest, found := strings.CutPrefix(uri, resourceScheme)
	if !found {
		return authoringTarget{}, false
	}
	parts := strings.Split(rest, "/")
	var t authoringTarget
	switch {
	case len(parts) == 2:
		t.kind, t.name = parts[0], parts[1]
	case len(parts) == 4 && parts[0] == "domains":
		t.domain, t.kind, t.name = parts[1], parts[2], parts[3]
	default:
		return authoringTarget{}, false
	}
	if _, known := s.itemReaders()[t.kind]; !known || t.name == "" || (len(parts) == 4 && t.domain == "") {
		return authoringTarget{}, false
	}
	return t, true
}

// unknownAuthoringResourceMiddleware answers resources/read of a URI that names
// nothing with the well-formed resource-not-found error; the SDK would build it
// from the raw URI with %q, which is invalid JSON for control characters.
func (s *Server) unknownAuthoringResourceMiddleware() sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
			if method == methodResourcesRead {
				if req, ok := request.(*sdkmcp.ReadResourceRequest); ok && req.Params != nil {
					if _, named := s.parseAuthoringURI(req.Params.URI); !named {
						return nil, resourceNotFound(req.Params.URI)
					}
				}
			}
			return next(ctx, method, request)
		}
	}
}

// resourceArgs are the arguments of the tool a resource read runs.
func (s *Server) resourceArgs() (map[string]any, error) {
	args := map[string]any{}
	if s.dirs.anyDir {
		return args, nil
	}
	if s.dirs.root == "" {
		return nil, errors.New("the server has no allowed root; start it with --root or --allow-any-dir")
	}
	args[argWorkingDirectory] = s.dirs.root
	return args, nil
}

// readAuthoringResource serves a config, catalog or item URI by running the
// read-only tool of the same content, so a resource and its tool cannot differ.
func (s *Server) readAuthoringResource(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	target, ok := s.parseAuthoringURI(req.Params.URI)
	if !ok {
		return nil, resourceNotFound(req.Params.URI)
	}
	args, err := s.resourceArgs()
	if err != nil {
		return nil, err
	}
	var (
		handler handlerFunc
		mime    = mimeJSON
	)
	switch {
	case target.config:
		handler = handlers.ReadConfigHandler
	case target.catalog:
		handler = handlers.CatalogHandler(s.version)
	default:
		handler, mime = s.itemReaders()[target.kind], mimeMarkdown
		args["name"] = target.name
		if target.domain != "" {
			args["domain"] = target.domain
		}
	}
	res, err := handler(ctx, handlers.NewToolRequest(nil, args))
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	text := firstText(res)
	if res.IsError {
		return nil, errors.New(strings.TrimSpace(text))
	}
	if mime == mimeMarkdown {
		doc, _ := resultDocument(res)
		content, _ := doc["content"].(string) //nolint:errcheck // an item result always carries its content
		text = content
	}
	return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{{URI: req.Params.URI, MIMEType: mime, Text: text}}}, nil
}

func firstText(res *sdkmcp.CallToolResult) string {
	for _, c := range res.Content {
		if t, ok := c.(*sdkmcp.TextContent); ok {
			return t.Text
		}
	}
	return ""
}

// Prompts.

// promptSpec is one prompt: its arguments and the instructions it expands to.
type promptSpec struct {
	name, title, description string
	args                     []*sdkmcp.PromptArgument
	text                     func(args map[string]string) string
}

func requiredArg(name, description string) *sdkmcp.PromptArgument {
	return &sdkmcp.PromptArgument{Name: name, Description: description, Required: true}
}

func optionalArg(name, description string) *sdkmcp.PromptArgument {
	return &sdkmcp.PromptArgument{Name: name, Description: description}
}

// authoringPrompts are the prompts of the authoring server. Each names only
// tools this server registers (a test keeps them in step).
func authoringPrompts() []promptSpec {
	return []promptSpec{
		{
			name: "author-skill", title: "Author a Skill",
			description: "Write a new skill for the project: draft it, create it, then check it",
			args:        []*sdkmcp.PromptArgument{requiredArg("name", "Skill name: lowercase letters, digits and hyphens"), requiredArg("purpose", "What the skill helps an agent do, and when it should be used")},
			text: func(a map[string]string) string {
				return fmt.Sprintf(`Author the skill %q for this project. Its purpose: %s

1. Call list_skills and list_rules so you do not duplicate content that exists.
2. Write a SKILL.md: frontmatter with name and a description of at least 20 characters that says what the skill does and when to use it (the description is what an agent sees before it loads the skill), then a short body with the steps. Keep the body under 500 lines and put long reference material in separate files.
3. Call create_skill with name %q, the description and the content.
4. Call scan_content to confirm the skill trips no security check, and validate_config to confirm it lints clean. Fix what they report with update_skill.
5. Call cost_report and tell me how many tokens the skill adds to the always-loaded and on-demand surface.
Do not call generate_outputs until I say so.`, a["name"], a["purpose"], a["name"])
			},
		},
		{
			name: "add-rule", title: "Add a Rule",
			description: "Turn a guideline into a rule file with the right priority",
			args:        []*sdkmcp.PromptArgument{requiredArg("name", "Rule name, the filename without .md"), requiredArg("guideline", "The guideline the rule states"), optionalArg("priority", "critical, high, medium, low or minimal (default medium)")},
			text: func(a map[string]string) string {
				priority := a["priority"]
				if priority == "" {
					priority = "medium"
				}
				return fmt.Sprintf(`Add the rule %q: %s

1. Call list_rules and read_rule for any rule that already covers this; if one does, update it with update_rule instead of adding another.
2. Otherwise call create_rule with name %q, priority %q and content that states the guideline as an instruction, in the imperative, with the reason in one sentence.
3. Call validate_config and fix anything it reports about the rule.`, a["name"], a["guideline"], a["name"], priority)
			},
		},
		{
			name: "review-config", title: "Review the Configuration",
			description: "Check the project's AI configuration end to end and summarize what to fix",
			args:        []*sdkmcp.PromptArgument{optionalArg("focus", "Where to look hardest: security, size, drift or governance (default: all)")},
			text: func(a map[string]string) string {
				focus := a["focus"]
				if focus == "" {
					focus = "all four"
				}
				return fmt.Sprintf(`Review this project's AI configuration. Focus: %s.

Run these read-only tools and report what they find, most important first:
- validate_config and scan_content: lint and security findings.
- doctor: drift between sources and generated files, missing gitignore entries, broken hooks.
- token_report and cost_report: how many tokens are always loaded, and which items cost the most.
- lock_status, approvals_status and policy_show: whether the lock is current, what still needs approval, and which organization policy applies.
Do not change anything. End with a short list of the changes you would make, each naming the tool you would use.`, focus)
			},
		},
		{
			name: "trim-context", title: "Trim Always-Loaded Context",
			description: "Find what to cut or move to on-demand to reduce the tokens paid on every request",
			args:        []*sdkmcp.PromptArgument{optionalArg("budget", "Target ceiling for always-loaded tokens")},
			text: func(a map[string]string) string {
				target := "as low as is reasonable"
				if a["budget"] != "" {
					target = "under " + a["budget"] + " tokens"
				}
				return fmt.Sprintf(`Reduce the always-loaded context of this project to %s.

1. Call token_report to see the always-loaded total, then cost_report for the biggest offenders.
2. For each of the top items read it (read_rule, read_context, read_skill) and decide: shorten it, split the detail into a skill so it loads on demand, or delete it if another item covers it.
3. Show me the proposed edits before applying any. Apply them with update_rule, update_context or update_skill only after I agree, then call token_report again to confirm the saving.`, target)
			},
		},
	}
}

func (s *Server) registerPrompts() {
	for _, spec := range authoringPrompts() {
		s.mcpServer.AddPrompt(&sdkmcp.Prompt{
			Name: spec.name, Title: spec.title, Description: spec.description, Arguments: spec.args,
		}, func(_ context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			args := req.Params.Arguments
			for _, a := range spec.args {
				if a.Required && strings.TrimSpace(args[a.Name]) == "" {
					return nil, fmt.Errorf("the %s argument is required", a.Name)
				}
			}
			return &sdkmcp.GetPromptResult{
				Description: spec.description,
				Messages:    []*sdkmcp.PromptMessage{{Role: "user", Content: &sdkmcp.TextContent{Text: spec.text(args)}}},
			}, nil
		})
	}
}
