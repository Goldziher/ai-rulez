package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoader_RemoteIncludes(t *testing.T) {
	// Create a test server with remote YAML content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/remote-rules.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Remote Rule"
    content: "This is a remote rule"
    priority: 5
`))
		case "/remote-sections.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
sections:
  - title: "Remote Section"
    content: "This is a remote section"
    priority: 3
`))
		case "/relative-include.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Relative Remote Rule"
    content: "This is from a relative include"
    priority: 2
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("load_remote_config_directly", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewTestClient(nil),
		}

		config, err := loader.loadRemoteConfig(server.URL + "/remote-rules.yaml")
		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "Remote Rule", config.Rules[0].Name)
		assert.Equal(t, 5, config.Rules[0].Priority)
	})

	t.Run("resolve_remote_url_path", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewTestClient(nil),
		}

		// Test absolute URL resolution
		resolved := loader.resolvePath(server.URL+"/test.yaml", "/some/local/path")
		assert.Equal(t, server.URL+"/test.yaml", resolved)

		// Test relative URL resolution with URL base
		resolved = loader.resolvePath("relative-include.yaml", server.URL+"/base/")
		assert.Equal(t, server.URL+"/base/relative-include.yaml", resolved)

		// Test relative path with local base
		resolved = loader.resolvePath("local-file.yaml", "/local/base")
		assert.Equal(t, "/local/base/local-file.yaml", resolved)
	})

	t.Run("is_url_detection", func(t *testing.T) {
		loader := &configLoader{}

		assert.True(t, loader.isURL("https://example.com/config.yaml"))
		assert.True(t, loader.isURL("http://example.com/config.yaml"))
		assert.False(t, loader.isURL("/local/path/config.yaml"))
		assert.False(t, loader.isURL("./relative/config.yaml"))
		assert.False(t, loader.isURL("config.yaml"))
		assert.False(t, loader.isURL("ftp://example.com/config.yaml"))
	})

	t.Run("remote_include_not_found", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewTestClient(nil),
		}

		_, err := loader.loadRemoteConfig(server.URL + "/nonexistent.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})
}

func TestValidateIncludes_Remote(t *testing.T) {
	// Create a test server with remote YAML content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid-remote.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Valid Remote Rule"
    content: "This is valid"
    priority: 1
`))
		case "/invalid-yaml.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
invalid: yaml: content: [unclosed
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("validate_remote_includes_success", func(t *testing.T) {
		// Create a test loader with bypassed SSRF validation
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: remote.NewTestClient(nil),
		}

		config := &Config{
			Includes: []string{
				server.URL + "/valid-remote.yaml",
			},
		}

		// Test the loader directly since ValidateIncludes creates its own loader
		for _, includePath := range config.Includes {
			resolvedPath := loader.resolvePath(includePath, "/some/base/dir")
			_, err := loader.loadRemoteConfig(resolvedPath)
			assert.NoError(t, err)
		}
	})

	t.Run("validate_remote_includes_not_found", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: remote.NewTestClient(nil),
		}

		_, err := loader.loadRemoteConfig(server.URL + "/nonexistent.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("validate_remote_includes_invalid_yaml", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: remote.NewTestClient(nil),
		}

		_, err := loader.loadRemoteConfig(server.URL + "/invalid-yaml.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}

func TestLoadConfigWithIncludes_RemoteIntegration(t *testing.T) {
	// Create a test server with remote YAML content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/remote-rules.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Remote Rule 1"
    content: "This is remote rule 1"
    priority: 5
  - name: "Remote Rule 2"
    content: "This is remote rule 2"
    priority: 3
`))
		case "/nested/remote-sections.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
sections:
  - title: "Remote Section"
    content: "This is a remote section"
    priority: 2
rules:
  - name: "Nested Remote Rule"
    content: "From nested include"
    priority: 1
`))
		case "/agents.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
agents:
  - name: "Remote Agent"
    system_prompt: "You are a remote agent"
    tools: ["tool1", "tool2"]
    priority: 4
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("integration_test_with_mixed_includes", func(t *testing.T) {
		// Create a loader with test client to bypass SSRF
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewTestClient(nil),
		}

		// Create a main config with remote includes
		mainConfig := &Config{
			Metadata: Metadata{
				Name:    "Test Config with Remote Includes",
				Version: "1.0.0",
			},
			Includes: []string{
				server.URL + "/remote-rules.yaml",
				server.URL + "/nested/remote-sections.yaml",
				server.URL + "/agents.yaml",
			},
			Rules: []Rule{
				{
					Name:     "Local Rule",
					Content:  "This is a local rule",
					Priority: 6,
				},
			},
		}

		// Resolve includes
		err := loader.resolveIncludes(mainConfig, "/tmp")
		require.NoError(t, err)

		// Verify merged content
		assert.Len(t, mainConfig.Rules, 4)    // 1 local + 3 remote
		assert.Len(t, mainConfig.Sections, 1) // 1 remote section
		assert.Len(t, mainConfig.Agents, 1)   // 1 remote agent
		assert.Nil(t, mainConfig.Includes)    // Should be cleared after resolution

		// Check rule priorities are preserved
		ruleNames := make([]string, len(mainConfig.Rules))
		for i, rule := range mainConfig.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Local Rule")
		assert.Contains(t, ruleNames, "Remote Rule 1")
		assert.Contains(t, ruleNames, "Remote Rule 2")
		assert.Contains(t, ruleNames, "Nested Remote Rule")

		// Check agent details
		assert.Equal(t, "Remote Agent", mainConfig.Agents[0].Name)
		assert.Equal(t, "You are a remote agent", mainConfig.Agents[0].SystemPrompt)
		assert.Equal(t, []string{"tool1", "tool2"}, mainConfig.Agents[0].Tools)
		assert.Equal(t, 4, mainConfig.Agents[0].Priority)

		// Check section details
		assert.Equal(t, "Remote Section", mainConfig.Sections[0].Title)
		assert.Equal(t, "This is a remote section", mainConfig.Sections[0].Content)
		assert.Equal(t, 2, mainConfig.Sections[0].Priority)
	})

	t.Run("relative_url_resolution", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      server.URL + "/base/",
			remoteClient: remote.NewTestClient(nil),
		}

		mainConfig := &Config{
			Includes: []string{
				"../remote-rules.yaml", // Relative to server.URL/base/
			},
		}

		err := loader.resolveIncludes(mainConfig, server.URL+"/base/")
		require.NoError(t, err)

		assert.Len(t, mainConfig.Rules, 2)
		assert.Equal(t, "Remote Rule 1", mainConfig.Rules[0].Name)
	})

	t.Run("circular_include_detection_with_remote", func(t *testing.T) {
		// This would require more complex server setup to test circular includes
		// For now, just test that the visited map works with URLs
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewTestClient(nil),
		}

		// Mark a URL as visited
		testURL := server.URL + "/test.yaml"
		loader.visited[testURL] = true

		// Attempt to load it again should detect circular reference
		_, err := loader.loadConfig(testURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "circular include")
	})
}
