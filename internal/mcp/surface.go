package mcp

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
)

// The surface of a server is what it registered: the tools that passed the
// read-only and allow-list filters, the prompts whose tools are among them, and
// the instructions the model reads at initialization. The instructions and the
// prompts are derived from the registered tool set, never written for the
// widest server, so a read-only or restricted server does not send the model to
// tools it does not have.

// toolSurface records the tools a server saw and registered. Construction
// fills it; it is read-only afterwards.
type toolSurface struct {
	// allow, when non-nil, is the only set of tools registered (WithTools).
	allow map[string]bool
	// known holds every tool name a registration was attempted for, filtered or
	// not: the names ValidateToolNames accepts.
	known map[string]bool
	// registered holds the tools the server registered.
	registered map[string]bool
	// writes is true when a registered tool changes the project.
	writes bool
}

// WithTools restricts the server to the named tools (mcp --tools, [mcp] tools).
// A name the server would not register anyway (a writing tool on a read-only
// server, a context tool without an indexed repository) is simply absent.
// Unknown names are rejected by ValidateToolNames before a server is built.
func WithTools(names ...string) Option {
	return func(s *Server) {
		s.surface.allow = make(map[string]bool, len(names))
		for _, n := range names {
			s.surface.allow[n] = true
		}
	}
}

// WithoutProject marks a server that serves a directory with no ai-rulez
// configuration (`mcp --serve-skills --discover-skills` in a plain repository).
// It registers only the tools that work without one, offers no config or item
// resources, and builds the catalog resource from the discovered content.
func WithoutProject() Option {
	return func(s *Server) { s.noProject = true }
}

// projectlessTool reports whether a tool works without a configuration: the
// serving tools, the builtin catalog, the version, the policy view, the OKF
// validator and init_project, which creates the configuration.
func projectlessTool(name string) bool {
	switch name {
	case toolFindSkill, toolLoadSkill, toolListSkillResources, toolFindContext, toolLoadContext,
		toolSearchSkills, toolGetSkill, toolReadSkillFile,
		"get_version", "list_builtins", "show_builtin", "policy_show", "okf_validate", "init_project":
		return true
	}
	return false
}

// noteKnown records tool names that exist even when this server cannot
// register them (the context tools of a server without a repository index).
func (s *Server) noteKnown(names ...string) {
	if s.surface.known == nil {
		s.surface.known = map[string]bool{}
	}
	for _, n := range names {
		s.surface.known[n] = true
	}
}

// admitTool records spec as known and reports whether this server registers it.
func (s *Server) admitTool(spec *toolSpec) bool {
	s.noteKnown(spec.name)
	if s.readOnly && (!spec.annotations.ReadOnlyHint || (opensWorld(spec.annotations) && !spec.remote)) {
		return false
	}
	if s.noProject && !projectlessTool(spec.name) {
		return false
	}
	if s.surface.allow != nil && !s.surface.allow[spec.name] {
		return false
	}
	if s.surface.registered == nil {
		s.surface.registered = map[string]bool{}
	}
	s.surface.registered[spec.name] = true
	if !spec.annotations.ReadOnlyHint && !spec.initializes {
		s.surface.writes = true
	}
	return true
}

// has reports whether the server registered the tool.
func (s *Server) has(name string) bool { return s.surface.registered[name] }

// hasAll reports whether the server registered every named tool.
func (s *Server) hasAll(names ...string) bool {
	for _, n := range names {
		if !s.has(n) {
			return false
		}
	}
	return true
}

// promptNeeds are the tools a prompt tells the model to call: the tool names its
// text mentions. The prompt is registered only on a server that registered all
// of them, so it cannot send the model to a tool the server lacks.
func (s *Server) promptNeeds(spec promptSpec) []string {
	args := make(map[string]string, len(spec.args))
	for _, a := range spec.args {
		args[a.Name] = "x"
	}
	text := spec.text(args)
	var needs []string
	for name := range s.surface.known {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(text) {
			needs = append(needs, name)
		}
	}
	sort.Strings(needs)
	return needs
}

// KnownToolNames lists every tool name the merged server can register, sorted.
func KnownToolNames() []string {
	catalog, err := BuildCatalogIn(logger.Discard(), "", "", nil, SkillFilter{})
	if err != nil {
		return nil
	}
	srv := NewMergedServer("", catalog, ServeOptions{}, WithAnyDirectory())
	names := make([]string, 0, len(srv.surface.known))
	for n := range srv.surface.known {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ValidateToolNames reports the names that are not tools of the server, with
// the valid ones.
func ValidateToolNames(names []string) error {
	known := KnownToolNames()
	var unknown []string
	for _, n := range names {
		if !slices.Contains(known, n) {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return oops.Errorf("unknown tool %s; valid tools are: %s", strings.Join(unknown, ", "), strings.Join(known, ", "))
}

// instructionsMiddleware answers initialize with instructions composed from the
// registered tools, for both handshakes (initialize and server/discover). The
// SDK fixes ServerOptions.Instructions at construction, before any tool is
// registered, so the result is rewritten here, on a copy: the initialize
// middleware caches the result it returns.
func (s *Server) instructionsMiddleware() sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
			res, err := next(ctx, method, request)
			if err != nil {
				return res, err
			}
			switch result := res.(type) {
			case *sdkmcp.InitializeResult:
				if method == methodInitialize && result != nil {
					out := *result
					out.Instructions = s.composeInstructions()
					return &out, nil
				}
			case *sdkmcp.DiscoverResult:
				if result != nil {
					out := *result
					out.Instructions = s.composeInstructions()
					return &out, nil
				}
			}
			return res, err
		}
	}
}

// composeInstructions is the text the model reads at initialization.
func (s *Server) composeInstructions() string {
	var parts []string
	if s.authoring {
		parts = append(parts, s.authoringInstructions())
	}
	if s.serve != nil {
		parts = append(parts, s.serveInstructions())
	}
	if s.has(toolFindContext) {
		parts = append(parts, strings.TrimSpace(scopeSummary(s.cat().repository())))
	}
	return strings.TrimSpace(strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), " "))
}

// authoringInstructions describes the authoring tools that are registered.
func (s *Server) authoringInstructions() string {
	if s.noProject {
		return s.projectlessInstructions()
	}
	parts := []string{"ai-rulez manages AI assistant governance from a single source of truth in the " +
		".ai-rulez/ directory and generates tool-specific outputs (CLAUDE.md, .cursor/rules, etc.)."}
	switch {
	case s.surface.writes && s.has("generate_outputs"):
		parts = append(parts, "Edit source rules, context, skills, agents, domains, includes, and profiles through the CRUD tools, "+
			"then call generate_outputs to render assistant files.")
	case s.surface.writes:
		parts = append(parts, "Edit source rules, context, skills, agents, domains, includes, and profiles through the CRUD tools.")
	default:
		parts = append(parts, "This server is read-only: it cannot change the configuration or generate files.")
	}
	if s.has("validate_config") {
		parts = append(parts, "Use read/list tools to inspect state and validate_config to check the configuration.")
	} else if s.hasAnyReader() {
		parts = append(parts, "Use read/list tools to inspect state.")
	}
	parts = append(parts, "Never edit generated files directly.")
	return strings.Join(parts, " ")
}

// hasAnyReader reports whether an authoring read tool is registered.
func (s *Server) hasAnyReader() bool {
	for name := range s.surface.registered {
		if strings.HasPrefix(name, "list_") && name != toolListSkillResources || strings.HasPrefix(name, "read_") && name != toolReadSkillFile {
			return true
		}
	}
	return false
}

// serveInstructions describes the skills surface and the serving tools that are
// registered. Without the authoring surface it is the whole text.
func (s *Server) serveInstructions() string {
	var parts []string
	if s.authoring {
		parts = append(parts, "This server also serves the project's skills read-only: skills are MCP resources under skill://<name>/SKILL.md.")
	} else {
		parts = append(parts, "ai-rulez serves the skills of one profile read-only. "+
			"Skills are MCP resources under skill://<name>/SKILL.md (supporting files sit beside it); "+
			"skills/list and skills/get return each skill's frontmatter plus the SHA-256 digest and size of every file.")
	}
	if use := s.skillToolSentence(); use != "" {
		parts = append(parts, use)
	}
	if lexical := s.lexicalToolSentence(); lexical != "" {
		parts = append(parts, lexical)
	}
	if !s.authoring {
		parts = append(parts, "This server cannot modify any content.")
	} else if !s.readOnly && s.surface.writes {
		parts = append(parts, "Start it with --read-only to register the read-only tools only.")
	}
	return strings.Join(parts, " ")
}

// skillToolSentence says how to find and load a skill with the tools that exist.
func (s *Server) skillToolSentence() string {
	find, load := s.has(toolFindSkill), s.has(toolLoadSkill)
	switch {
	case find && load:
		sentence := "Call find_skill with a task to get ranked matches and load_skill to read one"
		if s.has(toolListSkillResources) {
			sentence += " (list_skill_resources shows its files)"
		}
		return sentence + "."
	case find:
		return "Call find_skill with a task to get ranked matches."
	case load:
		return "Call load_skill to read a skill by name."
	}
	return ""
}

// lexicalToolSentence names the lexical skill tools that are registered.
func (s *Server) lexicalToolSentence() string {
	var names []string
	for _, n := range []string{toolSearchSkills, toolGetSkill, toolReadSkillFile} {
		if s.has(n) {
			names = append(names, n)
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " is the lexical equivalent."
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " are the lexical equivalents."
}

// projectlessInstructions is the authoring part of the instructions of a server
// that has no configuration to manage.
func (s *Server) projectlessInstructions() string {
	text := "This directory has no ai-rulez configuration, so the server serves the skills and instruction files found in the repository and none of the configuration tools."
	if s.has("init_project") {
		text += " init_project creates a configuration."
	}
	return text
}
