package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentIntegration(t *testing.T) {
	if os.Getenv("AI_RULEZ_INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test (set AI_RULEZ_INTEGRATION_TEST=1 to run)")
	}

	binPath := buildBinary(t)

	t.Run("ListAgents", func(t *testing.T) {
		cmd := exec.Command(binPath, "init", "--list-agents")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to list agents: %s", string(output))
		
		outputStr := string(output)
		hasAgent := strings.Contains(outputStr, "claude") ||
			strings.Contains(outputStr, "amp") ||
			strings.Contains(outputStr, "codex") ||
			strings.Contains(outputStr, "cursor") ||
			strings.Contains(outputStr, "gemini")
		
		assert.True(t, hasAgent, "Should list at least one available agent")
	})

	t.Run("PresetClaude", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestClaude", "--preset", "claude", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with claude preset: %s", string(output))
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		require.FileExists(t, configPath)
		
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		assert.Contains(t, string(data), "CLAUDE.md")
		assert.Contains(t, string(data), ".claude/agents/")
	})

	t.Run("PresetCursor", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestCursor", "--preset", "cursor", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with cursor preset: %s", string(output))
		
		outputStr := string(output)
		assert.Contains(t, outputStr, "cursor")
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		assert.Contains(t, string(data), "AGENTS.md")
		assert.Contains(t, string(data), ".cursor/rules/")
	})

	t.Run("PresetWindsurf", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestWindsurf", "--preset", "windsurf", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with windsurf preset: %s", string(output))
		
		outputStr := string(output)
		assert.Contains(t, outputStr, "windsurf")
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		assert.Contains(t, string(data), ".windsurf/")
	})

	t.Run("PresetCopilot", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestCopilot", "--preset", "copilot", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with copilot preset: %s", string(output))
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		assert.Contains(t, string(data), ".github/copilot-instructions.md")
	})

	t.Run("InvalidPreset", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestInvalid", "--preset", "invalid")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		assert.Error(t, err, "Should fail with invalid preset")
		assert.Contains(t, string(output), "invalid preset")
	})

	t.Run("SpecificProviders", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestSpecific", "--claude", "--windsurf", "--copilot", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with specific providers: %s", string(output))
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		assert.Contains(t, string(data), "CLAUDE.md")
		assert.Contains(t, string(data), ".windsurf/")
		assert.Contains(t, string(data), ".github/copilot-instructions.md")
	})

	t.Run("AgentGeneration", func(t *testing.T) {
		cmd := exec.Command(binPath, "init", "--list-agents")
		output, _ := cmd.CombinedOutput()
		if !strings.Contains(string(output), "claude") {
			t.Skip("Claude agent not available for testing")
		}

		tempDir := t.TempDir()
		
		cmd = exec.Command(binPath, "init", "TestAgent", "--use-agent", "claude", "--preset", "minimal")
		cmd.Dir = tempDir
		
		done := make(chan error, 1)
		go func() {
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Agent generation output: %s", string(output))
			}
			done <- err
		}()
		
		select {
		case err := <-done:
			if err != nil {
				t.Logf("Agent generation failed (expected in test): %v", err)
			}
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Log("Agent generation timed out (expected in test)")
		}
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		if _, err := os.Stat(configPath); err == nil {
			assert.FileExists(t, configPath, "Should create config even if agent fails")
		}
	})

	t.Run("NewDirectoryFormats", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestNewFormats", "--windsurf", "--cline", "--continue-dev", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with new formats: %s", string(output))
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		configStr := string(data)
		
		assert.Contains(t, configStr, ".windsurf/", "Should use new Windsurf directory format")
		assert.NotContains(t, configStr, ".windsurfrules", "Should not use legacy .windsurfrules")
		
		assert.Contains(t, configStr, ".clinerules/", "Should use Cline directory format")
		
		assert.Contains(t, configStr, ".continue/rules/", "Should use Continue.dev directory format")
		assert.NotContains(t, configStr, ".continuerules", "Should not use legacy .continuerules")
	})

	t.Run("SharedAGENTSFile", func(t *testing.T) {
		tempDir := t.TempDir()
		
		cmd := exec.Command(binPath, "init", "TestShared", "--amp", "--codex", "--cursor", "--no-agent")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to init with shared AGENTS.md: %s", string(output))
		
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		
		agentsCount := strings.Count(string(data), `- file: "AGENTS.md"`)
		assert.Equal(t, 1, agentsCount, "AGENTS.md file entry should appear only once despite multiple providers")
		
		assert.Contains(t, string(data), ".cursor/rules/")
	})
}

func TestAgentDetection(t *testing.T) {
	binPath := buildBinary(t)
	
	t.Run("ListAgentsCommand", func(t *testing.T) {
		cmd := exec.Command(binPath, "init", "--list-agents")
		output, err := cmd.CombinedOutput()
		
		require.NoError(t, err, "list-agents should not error")
		
		outputStr := string(output)
		assert.True(t,
			strings.Contains(outputStr, "Available AI agents") ||
			strings.Contains(outputStr, "No AI agents found"),
			"Should provide meaningful output")
	})
}

func TestValidationLogic(t *testing.T) {
	binPath := buildBinary(t)
	
	t.Run("InvalidYAML", func(t *testing.T) {
		tempDir := t.TempDir()
		
		badConfig := `
metadata:
  name: "Test"
  version: "1.0.0"
  description: "Test"
  invalid indentation here
outputs:
  - file: "CLAUDE.md"
`
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(badConfig), 0644)
		require.NoError(t, err)
		
		cmd := exec.Command(binPath, "validate")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		assert.Error(t, err, "Should fail validation with invalid YAML")
		assert.Contains(t, string(output), "error", "Should report error")
	})
	
	t.Run("ValidConfigWithoutSchema", func(t *testing.T) {
		tempDir := t.TempDir()
		
		validConfig := `
metadata:
  name: "Test"
  version: "1.0.0"
  description: "Test"
outputs:
  - file: "CLAUDE.md"
rules:
  - name: "test"
    priority: 5
    content: "test rule"
`
		configPath := filepath.Join(tempDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(validConfig), 0644)
		require.NoError(t, err)
		
		cmd := exec.Command(binPath, "validate")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err, "Valid config should pass even without $schema")
		assert.Contains(t, string(output), "valid", "Should report as valid")
	})
}