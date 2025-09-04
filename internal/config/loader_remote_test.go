package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/remote"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestClient() *remote.Client {
	return remote.NewTestClient(nil)
}

func TestConfigLoader_RemoteIncludes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/remote-rules.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Remote Rule"
    content: "This is a remote rule"
    priority: medium
`))
		case "/remote-sections.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
sections:
  - name: "Remote Section"
    content: "This is a remote section"
    priority: low
`))
		case "/relative-include.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Relative Remote Rule"
    content: "This is from a relative include"
    priority: low
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
			remoteClient: createTestClient(),
		}

		config, err := loader.loadRemoteConfig(context.Background(), server.URL+"/remote-rules.yaml")
		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "Remote Rule", config.Rules[0].Name)
		assert.Equal(t, "medium", string(config.Rules[0].Priority))
	})

	t.Run("resolve_remote_url_path", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

		resolved := loader.resolvePath(server.URL+"/test.yaml", "/some/local/path")
		assert.Equal(t, server.URL+"/test.yaml", resolved)

		resolved = loader.resolvePath("relative-include.yaml", server.URL+"/base/")
		assert.Equal(t, server.URL+"/base/relative-include.yaml", resolved)

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
			remoteClient: createTestClient(),
		}

		_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/nonexistent.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})
}

func TestValidateIncludes_Remote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid-remote.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Valid Remote Rule"
    content: "This is valid"
    priority: minimal
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
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: createTestClient(),
		}

		config := &Config{
			Includes: []string{
				server.URL + "/valid-remote.yaml",
			},
		}

		for _, includePath := range config.Includes {
			resolvedPath := loader.resolvePath(includePath, "/some/base/dir")
			_, err := loader.loadRemoteConfig(context.Background(), resolvedPath)
			assert.NoError(t, err)
		}
	})

	t.Run("validate_remote_includes_not_found", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: createTestClient(),
		}

		_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/nonexistent.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("validate_remote_includes_invalid_yaml", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/some/base/dir",
			remoteClient: createTestClient(),
		}

		_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/invalid-yaml.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse remote config")
	})
}

func TestLoadConfigWithIncludes_RemoteIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/remote-rules.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
rules:
  - name: "Remote Rule 1"
    content: "This is remote rule 1"
    priority: medium
  - name: "Remote Rule 2"
    content: "This is remote rule 2"
    priority: low
`))
		case "/nested/remote-sections.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
sections:
  - name: "Remote Section"
    content: "This is a remote section"
    priority: low
rules:
  - name: "Nested Remote Rule"
    content: "From nested include"
    priority: minimal
`))
		case "/agents.yaml":
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
agents:
  - name: "Remote Agent"
    system_prompt: "You are a remote agent"
    tools: ["tool1", "tool2"]
    priority: medium
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("integration_test_with_mixed_includes", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

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
					Priority: PriorityMedium,
				},
			},
		}

		err := loader.resolveIncludes(context.Background(), mainConfig, "/tmp")
		require.NoError(t, err)

		assert.Len(t, mainConfig.Rules, 4)
		assert.Len(t, mainConfig.Sections, 1)
		assert.Len(t, mainConfig.Agents, 1)
		assert.Nil(t, mainConfig.Includes)

		ruleNames := make([]string, len(mainConfig.Rules))
		for i, rule := range mainConfig.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Local Rule")
		assert.Contains(t, ruleNames, "Remote Rule 1")
		assert.Contains(t, ruleNames, "Remote Rule 2")
		assert.Contains(t, ruleNames, "Nested Remote Rule")

		assert.Equal(t, "Remote Agent", mainConfig.Agents[0].Name)
		assert.Equal(t, "You are a remote agent", mainConfig.Agents[0].SystemPrompt)
		assert.Equal(t, []string{"tool1", "tool2"}, mainConfig.Agents[0].Tools)
		assert.Equal(t, "medium", string(mainConfig.Agents[0].Priority))

		assert.Equal(t, "Remote Section", mainConfig.Sections[0].Name)
		assert.Equal(t, "This is a remote section", mainConfig.Sections[0].Content)
		assert.Equal(t, "low", string(mainConfig.Sections[0].Priority))
	})

	t.Run("relative_url_resolution", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      server.URL + "/base/",
			remoteClient: createTestClient(),
		}

		mainConfig := &Config{
			Includes: []string{
				"../remote-rules.yaml",
			},
		}

		err := loader.resolveIncludes(context.Background(), mainConfig, server.URL+"/base/")
		require.NoError(t, err)

		assert.Len(t, mainConfig.Rules, 2)

		ruleNames := make([]string, len(mainConfig.Rules))
		for i, rule := range mainConfig.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Remote Rule 1")
		assert.Contains(t, ruleNames, "Remote Rule 2")
	})

	t.Run("circular_include_detection_with_remote", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

		testURL := server.URL + "/test.yaml"
		loader.visited[testURL] = true

		_, err := loader.loadConfig(context.Background(), testURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "circular include")
	})
}

func TestConfigLoader_ErrorHandling(t *testing.T) {
	t.Run("remote_fetch_errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/404-config.yaml":
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Config not found"))
			case "/500-config.yaml":
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Server error"))
			case "/timeout-config.yaml":
				time.Sleep(200 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("rules: []"))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

		t.Run("404_error", func(t *testing.T) {
			_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/404-config.yaml")
			require.Error(t, err)

			oopsErr, ok := oops.AsOops(err)
			require.True(t, ok, "expected oops error")
			ctx := oopsErr.Context()
			assert.Equal(t, server.URL+"/404-config.yaml", ctx["url"])
			assert.Contains(t, err.Error(), "404")
		})

		t.Run("500_error", func(t *testing.T) {
			_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/500-config.yaml")
			require.Error(t, err)

			oopsErr, ok := oops.AsOops(err)
			require.True(t, ok, "expected oops error")
			ctx := oopsErr.Context()
			assert.Equal(t, server.URL+"/500-config.yaml", ctx["url"])
			assert.Contains(t, err.Error(), "500")
		})
	})

	t.Run("remote_parse_errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`invalid: yaml: content: [unclosed`))
		}))
		defer server.Close()

		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

		_, err := loader.loadRemoteConfig(context.Background(), server.URL+"/invalid.yaml")
		require.Error(t, err)

		oopsErr, ok := oops.AsOops(err)
		require.True(t, ok, "expected oops error")
		ctx := oopsErr.Context()
		assert.Equal(t, server.URL+"/invalid.yaml", ctx["url"])
		assert.Contains(t, err.Error(), "parse remote config")
		assert.Contains(t, err.Error(), "yaml:")
	})

	t.Run("ssrf_protection_in_config_loading", func(t *testing.T) {
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: remote.NewClient(nil),
		}

		testCases := []string{
			"http://localhost:8080/config.yaml",
			"http://127.0.0.1/config.yaml",
			"http://192.168.1.1/config.yaml",
			"http://169.254.169.254/config.yaml",
		}

		for _, url := range testCases {
			t.Run(url, func(t *testing.T) {
				_, err := loader.loadRemoteConfig(context.Background(), url)
				require.Error(t, err)

				oopsErr, ok := oops.AsOops(err)
				require.True(t, ok, "expected oops error")
				assert.Contains(t, err.Error(), "URL blocked for security reasons")
				ctx := oopsErr.Context()
				assert.Equal(t, url, ctx["url"])
				assert.Contains(t, err.Error(), "fetch remote config")
			})
		}
	})

	t.Run("mixed_local_and_remote_error_propagation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found"))
		}))
		defer server.Close()

		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      "/tmp",
			remoteClient: createTestClient(),
		}

		mainConfig := &Config{
			Metadata: Metadata{Name: "Test", Version: "1.0.0"},
			Includes: []string{server.URL + "/missing.yaml"},
		}

		err := loader.resolveIncludes(context.Background(), mainConfig, "/tmp")
		require.Error(t, err)

		_, ok := oops.AsOops(err)
		require.True(t, ok, "expected oops error")
		assert.Contains(t, err.Error(), "fetch remote config")
		assert.Contains(t, err.Error(), "404")
	})
}
