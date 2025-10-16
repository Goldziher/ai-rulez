package mcp

import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

type Server struct {
	mcpServer *sdkmcp.Server
	version   string
}

func NewServer(version string) *Server {
	serverImpl := &sdkmcp.Implementation{
		Name:    "ai-rulez",
		Version: version,
	}
	mcpServer := sdkmcp.NewServer(serverImpl, &sdkmcp.ServerOptions{
		HasTools: true,
	})

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
