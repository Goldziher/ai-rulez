package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Remote Extends Tests ==========

func TestLoadConfigWithRemoteExtends(t *testing.T) {
	t.Run("remote extends success", func(t *testing.T) {
		// Create a mock HTTP server for the base config
		baseConfigContent := `
metadata:
  name: Remote Base Project
  version: 1.0.0
  description: Remote base description
outputs:
  - path: remote-base-output.md
rules:
  - name: Remote Base Rule
    content: Remote base rule content
    priority: medium
agents:
  - name: remote-base-agent
    description: Remote base agent
    system_prompt: Remote base prompt
    priority: low
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(baseConfigContent))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		
		// Create child config that extends remote base
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s
metadata:
  name: Child Project
  version: 2.0.0
outputs:
  - path: child-output.md
rules:
  - name: Child Rule
    content: Child rule content
    priority: high
  - name: Remote Base Rule  # Override remote rule
    content: Overridden remote rule
    priority: critical
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		// Create test loader with test client that doesn't have SSRF protection
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}
		
		cfg, err := loader.loadConfig(context.Background(), childConfigFile)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		
		// Check metadata inheritance and overrides
		assert.Equal(t, "Child Project", cfg.Metadata.Name)
		assert.Equal(t, "2.0.0", cfg.Metadata.Version)
		assert.Equal(t, "Remote base description", cfg.Metadata.Description) // Inherited from remote
		
		// Check outputs are combined
		assert.Len(t, cfg.Outputs, 2)
		outputPaths := []string{cfg.Outputs[0].Path, cfg.Outputs[1].Path}
		assert.Contains(t, outputPaths, "remote-base-output.md")
		assert.Contains(t, outputPaths, "child-output.md")
		
		// Check rules are merged (child overrides remote)
		assert.Len(t, cfg.Rules, 2)
		ruleNames := make(map[string]string)
		for _, rule := range cfg.Rules {
			ruleNames[rule.Name] = rule.Content
		}
		assert.Equal(t, "Overridden remote rule", ruleNames["Remote Base Rule"])
		assert.Equal(t, "Child rule content", ruleNames["Child Rule"])
		
		// Check agents are inherited from remote
		assert.Len(t, cfg.Agents, 1)
		assert.Equal(t, "remote-base-agent", cfg.Agents[0].Name)
		assert.Equal(t, "Remote base agent", cfg.Agents[0].Description)
	})
	
	t.Run("remote extends with relative remote includes", func(t *testing.T) {
		// Create mock server for included config
		includeContent := `
rules:
  - name: Remote Include Rule
    content: Remote include rule content
    priority: minimal
`
		
		// Create mock server for base config that includes the other
		baseConfigContent := `
metadata:
  name: Remote Base with Include
outputs:
  - path: base-output.md
includes:
  - include.yaml
rules:
  - name: Remote Base Rule
    content: Remote base rule content
    priority: medium
`
		
		mux := http.NewServeMux()
		mux.HandleFunc("/base.yaml", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(baseConfigContent))
		})
		mux.HandleFunc("/include.yaml", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(includeContent))
		})
		
		server := httptest.NewServer(mux)
		defer server.Close()

		tempDir := t.TempDir()
		
		// Create child config that extends remote base
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s/base.yaml
metadata:
  name: Child with Remote Base and Include
rules:
  - name: Child Rule
    content: Child rule content
    priority: high
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		// Create test loader with test client that doesn't have SSRF protection
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}
		
		cfg, err := loader.loadConfig(context.Background(), childConfigFile)
		require.NoError(t, err)
		
		// Should have rules from remote base + remote include + child
		assert.Len(t, cfg.Rules, 3)
		ruleNames := make([]string, len(cfg.Rules))
		for i, rule := range cfg.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Remote Base Rule")
		assert.Contains(t, ruleNames, "Remote Include Rule")
		assert.Contains(t, ruleNames, "Child Rule")
	})

	t.Run("remote extends not found", func(t *testing.T) {
		// Create server that returns 404
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s/nonexistent.yaml
metadata:
  name: Child Project
outputs:
  - path: output.md
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		// Create test loader with test client that doesn't have SSRF protection
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}
		
		_, err := loader.loadConfig(context.Background(), childConfigFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "loading extended config")
		assert.Contains(t, err.Error(), "HTTP 404")
	})

	t.Run("remote extends invalid yaml", func(t *testing.T) {
		// Create server that returns invalid YAML
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid: yaml: content: ["))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s
metadata:
  name: Child Project
outputs:
  - path: output.md
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		// Create test loader with test client that doesn't have SSRF protection
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}
		
		_, err := loader.loadConfig(context.Background(), childConfigFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "loading extended config")
	})
	
	t.Run("remote extends with local child includes", func(t *testing.T) {
		// Create remote base config
		baseConfigContent := `
metadata:
  name: Remote Base
outputs:
  - path: remote-output.md
rules:
  - name: Remote Rule
    content: Remote rule content
    priority: medium
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(baseConfigContent))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		
		// Create local include file
		localIncludeFile := filepath.Join(tempDir, "local-include.yaml")
		localIncludeContent := `
rules:
  - name: Local Include Rule
    content: Local include rule content
    priority: low
`
		require.NoError(t, os.WriteFile(localIncludeFile, []byte(localIncludeContent), 0o644))
		
		// Create child config that extends remote and includes local
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s
includes:
  - local-include.yaml
metadata:
  name: Child with Mixed Sources
rules:
  - name: Child Rule
    content: Child rule content
    priority: high
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		// Create test loader with test client that doesn't have SSRF protection
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}
		
		cfg, err := loader.loadConfig(context.Background(), childConfigFile)
		require.NoError(t, err)
		
		// Should have rules from remote base + local include + child
		assert.Len(t, cfg.Rules, 3)
		ruleNames := make([]string, len(cfg.Rules))
		for i, rule := range cfg.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Remote Rule")
		assert.Contains(t, ruleNames, "Local Include Rule")
		assert.Contains(t, ruleNames, "Child Rule")
	})
}