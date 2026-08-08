package mcp

import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

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
}

func NewServer(version string) *Server {
	serverImpl := &sdkmcp.Implementation{
		Name:    "ai-rulez",
		Title:   "AI-Rulez",
		Version: version,
	}
	mcpServer := sdkmcp.NewServer(serverImpl, &sdkmcp.ServerOptions{
		HasTools:     true,
		Instructions: serverInstructions,
	})

	mcpServer.AddReceivingMiddleware(tolerantInitializeMiddleware())

	srv := &Server{
		mcpServer: mcpServer,
		version:   version,
	}

	srv.registerTools()
	return srv
}

func (s *Server) GetMCPServer() *sdkmcp.Server {
	return s.mcpServer
}
