package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	mcpServer *server.MCPServer
	version   string
}

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

func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}
