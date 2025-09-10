package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/samber/oops"
)

type CLIIntegrator interface {
	ToolName() string

	IsAvailable() bool

	ConfigureServer(server *config.MCPServer) error

	RemoveServer(serverName string) error
}

func ConfigureCLITools(servers []config.MCPServer) error {
	if len(servers) == 0 {
		return nil
	}

	integrators := []CLIIntegrator{
		&ClaudeIntegrator{},
		&GeminiIntegrator{},
	}

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

	var errors []error
	for i := range servers {
		server := &servers[i]
		if !server.IsEnabled() {
			continue
		}

		for _, integrator := range availableIntegrators {
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

func shouldConfigureForTool(server *config.MCPServer, toolName string) bool {
	if len(server.Targets) == 0 {
		return true
	}

	targetPattern := fmt.Sprintf("@%s-cli", toolName)
	for _, target := range server.Targets {
		if target == targetPattern {
			return true
		}
	}

	return false
}

func isToolAvailable(toolName string) bool {
	cmd := exec.Command(toolName, "--help")
	err := cmd.Run()
	return err == nil
}

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

func executeCommand(cmdArgs []string) error {
	if len(cmdArgs) == 0 {
		return oops.Errorf("empty command arguments")
	}

	allowedCommands := map[string]bool{
		"claude": true,
		"gemini": true,
	}

	if !allowedCommands[cmdArgs[0]] {
		return oops.
			With("command", cmdArgs[0]).
			Errorf("command not allowed: %s", cmdArgs[0])
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec G204 - cmdArgs[0] is validated against allowedCommands
	output, err := cmd.CombinedOutput()

	if err != nil {
		return oops.
			With("command", strings.Join(cmdArgs, " ")).
			With("output", string(output)).
			Wrapf(err, "command execution failed")
	}

	return nil
}
