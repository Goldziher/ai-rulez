package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server and its handlers
type Server struct {
	mcpServer *server.MCPServer
	version   string
}

// NewServer creates a new MCP server instance
func NewServer(version string) *Server {
	s := server.NewMCPServer(
		"ai-rulez",
		version,
		server.WithToolCapabilities(true),
	)

	srv := &Server{
		mcpServer: s,
		version:   version,
	}

	srv.registerTools()
	return srv
}

// GetMCPServer returns the underlying MCP server
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}
