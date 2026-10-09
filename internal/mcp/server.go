package mcp

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverInstructions is surfaced to MCP clients (and their models) during
// initialization to explain what this server does and how to drive it.
const serverInstructions = "ai-rulez manages AI assistant governance from a single source of truth in the " +
	".ai-rulez/ directory and generates tool-specific outputs (CLAUDE.md, .cursor/rules, etc.). " +
	"Edit source rules, context, skills, agents, domains, includes, and profiles through the CRUD tools, " +
	"then call generate_outputs to render assistant files. Use read/list tools to inspect state and " +
	"validate_config to check the configuration. Never edit generated files directly."

type Server struct {
	mcpServer *sdkmcp.Server
	version   string
	catalog   *Catalog
	// catMu guards catalog, which a live reload replaces (see serve_live.go).
	catMu     sync.RWMutex
	serve     *serveState
	telemetry atomic.Pointer[itemTelemetry]
	// closers run from Close, once the transport has ended.
	closers []func()
	// validator runs the lint behind validate_config; the CLI supplies the
	// engine of `ai-rulez validate` so both report the same findings.
	validator handlers.Validator
	// policyViewer builds the report of policy_show; the CLI supplies the policy
	// discovery of `validate --show-policy`.
	policyViewer handlers.PolicyViewer
	// dirs confines the project directories the tools may touch.
	dirs dirPolicy
}

// Option configures the authoring server.
type Option func(*Server)

// WithValidator sets the lint engine validate_config runs after the structural
// checks. Without it validate_config reports that it cannot lint.
func WithValidator(v handlers.Validator) Option {
	return func(s *Server) { s.validator = v }
}

// WithPolicyViewer sets the policy discovery policy_show reports from. Without it
// policy_show reports that it cannot discover a policy.
func WithPolicyViewer(v handlers.PolicyViewer) Option {
	return func(s *Server) { s.policyViewer = v }
}

// WithRoot confines working_directory, config_file and config_dir to dir and
// its subdirectories; a call without working_directory uses dir. Without it (or
// WithAnyDirectory) no directory is allowed, so a caller embedding the server
// must choose one; the command passes the directory it was started in.
func WithRoot(dir string) Option {
	return func(s *Server) {
		if dir == "" {
			return
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		s.dirs.root = dir
	}
}

// WithAnyDirectory lifts the directory confinement (mcp --allow-any-dir).
func WithAnyDirectory() Option {
	return func(s *Server) { s.dirs.anyDir = true }
}

// pageSize is how many tools, resources or prompts a list answer holds before
// it paginates; it is explicit so a change of the SDK default cannot silently
// reshape what clients see.
const pageSize = 100

// sdkLogger routes the SDK's own diagnostics (protocol misuse, duplicate
// initialize) to stderr through the CLI logger; stdout is the protocol. Only
// warnings and errors: the per-session connect and disconnect lines the SDK
// logs at info would drown the server's own output.
func sdkLogger() *slog.Logger { return logger.New(os.Stderr, slog.LevelWarn) }

// Close releases what the server started in the background, such as the
// delivery of queued usage-sink records. Call it after the transport ends.
func (s *Server) Close() {
	for _, fn := range s.closers {
		fn()
	}
	s.closers = nil
}

func NewServer(version string, opts ...Option) *Server {
	serverImpl := &sdkmcp.Implementation{
		Name:    "ai-rulez",
		Title:   "AI-Rulez",
		Version: version,
	}
	mcpServer := sdkmcp.NewServer(serverImpl, &sdkmcp.ServerOptions{
		// Advertise tools explicitly rather than relying on inference. Setting
		// Capabilities at all drops the SDK's default "logging" capability, which
		// is deprecated as of protocol version 2026-07-28 and which this server
		// never uses. The tool, prompt and resource sets are fixed at construction,
		// so listChanged is not promised.
		Capabilities: &sdkmcp.ServerCapabilities{
			Tools:     &sdkmcp.ToolCapabilities{},
			Prompts:   &sdkmcp.PromptCapabilities{},
			Resources: &sdkmcp.ResourceCapabilities{},
		},
		SetCacheable: authoringCacheable,
		Instructions: serverInstructions,
		Logger:       sdkLogger(),
		PageSize:     pageSize,
		// KeepAlive stays off: the server speaks stdio to the process that
		// launched it and ends with it, so there is no idle connection to probe.
	})

	mcpServer.AddReceivingMiddleware(tolerantInitializeMiddleware(mcpServer))

	srv := &Server{
		mcpServer: mcpServer,
		version:   version,
	}
	for _, opt := range opts {
		opt(srv)
	}

	srv.registerTools()
	srv.registerPrompts()
	srv.registerAuthoringResources()
	return srv
}

func (s *Server) GetMCPServer() *sdkmcp.Server {
	return s.mcpServer
}
