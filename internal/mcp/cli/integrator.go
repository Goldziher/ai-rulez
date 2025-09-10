package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/samber/oops"
)

// CLIIntegrator interface for tool-specific MCP CLI integrations
type CLIIntegrator interface {
	// ToolName returns the name of the tool (e.g., "claude", "gemini")
	ToolName() string

	// IsAvailable checks if the CLI tool is installed and accessible
	IsAvailable() bool

	// ConfigureServer executes the MCP server configuration command
	ConfigureServer(server *config.MCPServer) error

	// RemoveServer removes an MCP server configuration (optional)
	RemoveServer(serverName string) error
}

// ConfigureCLITools configures MCP servers for available CLI tools
func ConfigureCLITools(servers []config.MCPServer) error {
	if len(servers) == 0 {
		return nil
	}

	// Initialize all available integrators
	integrators := []CLIIntegrator{
		&ClaudeIntegrator{},
		&GeminiIntegrator{},
	}

	// Filter to only available tools
	availableIntegrators := make([]CLIIntegrator, 0, len(integrators))
	for _, integrator := range integrators {
		if integrator.IsAvailable() {
			availableIntegrators = append(availableIntegrators, integrator)
		}
	}

	if len(availableIntegrators) == 0 {
		progress.PrintIfNotQuiet("ℹ️  No CLI MCP tools available (claude, gemini, etc.)\n")
		return nil
	}

	// Configure servers for each available tool
	var errors []error
	for i := range servers {
		server := &servers[i]
		if !server.IsEnabled() {
			continue
		}

		for _, integrator := range availableIntegrators {
			// Check if this server targets this tool
			if shouldConfigureForTool(server, integrator.ToolName()) {
				if err := integrator.ConfigureServer(server); err != nil {
					errorMsg := fmt.Sprintf("failed to configure %s MCP server '%s': %v",
						integrator.ToolName(), server.Name, err)
					errors = append(errors, oops.Errorf("MCP configuration error: %s", errorMsg))
					progress.PrintIfNotQuiet("❌ %s\n", errorMsg)
				} else {
					progress.PrintIfNotQuiet("✅ Configured %s MCP server: %s\n",
						integrator.ToolName(), server.Name)
				}
			}
		}
	}

	if len(errors) > 0 {
		return oops.
			With("configured_tools", len(availableIntegrators)).
			With("failed_configurations", len(errors)).
			Errorf("some MCP CLI configurations failed")
	}

	return nil
}

// shouldConfigureForTool checks if a server should be configured for a specific tool
func shouldConfigureForTool(server *config.MCPServer, toolName string) bool {
	// If no targets specified, configure for all available tools
	if len(server.Targets) == 0 {
		return true
	}

	// Check for explicit tool targeting: @claude-cli, @gemini-cli
	targetPattern := fmt.Sprintf("@%s-cli", toolName)
	for _, target := range server.Targets {
		if target == targetPattern {
			return true
		}
	}

	return false
}

// isToolAvailable checks if a CLI tool is available in the system PATH
func isToolAvailable(toolName string) bool {
	cmd := exec.Command(toolName, "--help")
	err := cmd.Run()
	return err == nil
}

// buildEnvFlags constructs environment variable flags for CLI commands
func buildEnvFlags(env map[string]string, flagPattern string) []string {
	if len(env) == 0 {
		return nil
	}

	var flags []string
	for key, value := range env {
		flags = append(flags, flagPattern, fmt.Sprintf("%s=%s", key, value))
	}
	return flags
}

// executeCommand runs a command and returns detailed error information
func executeCommand(cmdArgs []string) error {
	if len(cmdArgs) == 0 {
		return oops.Errorf("empty command arguments")
	}

	// Validate that the first argument is a known safe command
	allowedCommands := map[string]bool{
		"claude": true,
		"gemini": true,
	}

	if !allowedCommands[cmdArgs[0]] {
		return oops.
			With("command", cmdArgs[0]).
			Errorf("command not allowed: %s", cmdArgs[0])
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec G204 - command is validated above
	output, err := cmd.CombinedOutput()

	if err != nil {
		return oops.
			With("command", strings.Join(cmdArgs, " ")).
			With("output", string(output)).
			Wrapf(err, "command execution failed")
	}

	return nil
}
