package lint

import (
	"strings"
	"testing"
)

func mcpJSON(servers string) map[string]string {
	return map[string]string{".mcp.json": `{"mcpServers":` + servers + `}`}
}

func TestMCPConfigAR602(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "stdio without command", files: mcpJSON(`{"a":{"type":"stdio","args":["x"]}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "empty server", files: mcpJSON(`{"a":{}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "http without url", files: mcpJSON(`{"a":{"type":"http"}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "relative url", files: mcpJSON(`{"a":{"type":"http","url":"/mcp"}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "unknown transport", files: mcpJSON(`{"a":{"type":"grpc","url":"https://x.test"}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "deprecated sse is a warning", files: mcpJSON(`{"a":{"type":"sse","url":"https://x.test/sse"}}`), want: []string{"AR602:.mcp.json:1"}, sev: map[string]Severity{"AR602": SeverityWarning}},
		{name: "args must be a list", files: mcpJSON(`{"a":{"command":"node","args":"server.js"}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "env must map to strings", files: mcpJSON(`{"a":{"command":"node","env":{"A":1}}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "name with a dot", files: mcpJSON(`{"my.server":{"command":"node"}}`), want: []string{"AR602:.mcp.json:1"}},
		{name: "valid stdio and http", files: mcpJSON(`{"a":{"command":"node","args":["s.js"]},"b":{"type":"http","url":"https://x.test/mcp"},"c":{"url":"https://x.test/mcp"}}`), absent: []string{"AR602"}},
		{
			name: "duplicate across config and .mcp.json", files: mcpJSON(`{"dup":{"command":"node"}}`),
			config: "\n[[mcp_servers]]\nname = \"dup\"\ncommand = \"node\"\n", want: []string{"AR602:config.toml:0"},
		},
		{name: "disabled servers are not checked", files: mcpJSON(`{"a":{"disabled":true}}`), absent: []string{"AR602"}},
		{
			name: "config.toml sse", config: "\n[[mcp_servers]]\nname = \"s\"\ntransport = \"sse\"\nurl = \"https://x.test/sse\"\n",
			want: []string{"AR602:config.toml:0"},
		},
		{name: "vscode servers key", files: map[string]string{".vscode/mcp.json": `{"servers":{"a":{"type":"stdio"}}}`}, want: []string{"AR602:mcp.json:1"}},
		{name: "severity override", files: mcpJSON(`{"a":{}}`), config: "\n[lint.severity]\nAR602 = \"info\"\n", want: []string{"AR602:.mcp.json:1"}, sev: map[string]Severity{"AR602": SeverityInfo}},
	})
}

func TestMCPUnpinnedAR012(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "npx unpinned", files: mcpJSON(`{"a":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx latest", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@latest"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx pinned", files: mcpJSON(`{"a":{"command":"npx","args":["-y","@scope/pkg@1.2.3"]}}`), absent: []string{"AR012"}},
		{name: "uvx unpinned", files: mcpJSON(`{"a":{"command":"uvx","args":["mcp-server-git"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "uvx pinned", files: mcpJSON(`{"a":{"command":"uvx","args":["mcp-server-git==0.6.2"]}}`), absent: []string{"AR012"}},
		{name: "uvx --from pinned", files: mcpJSON(`{"a":{"command":"uvx","args":["--from","pkg==1.0","cmd"]}}`), absent: []string{"AR012"}},
		{name: "pipx run", files: mcpJSON(`{"a":{"command":"pipx","args":["run","pkg"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "docker latest", files: mcpJSON(`{"a":{"command":"docker","args":["run","-i","--rm","-e","X","ghcr.io/o/img:latest"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "docker untagged", files: mcpJSON(`{"a":{"command":"docker","args":["run","-i","--rm","ghcr.io/o/img"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "docker digest", files: mcpJSON(`{"a":{"command":"docker","args":["run","--rm","ghcr.io/o/img@sha256:abc"]}}`), absent: []string{"AR012"}},
		{name: "docker tag", files: mcpJSON(`{"a":{"command":"docker","args":["run","--rm","ghcr.io/o/img:1.4.0"]}}`), absent: []string{"AR012"}},
		{name: "local binaries", files: mcpJSON(`{"a":{"command":"node","args":["server.js"]},"b":{"command":"./bin/server"}}`), absent: []string{"AR012"}},
		{name: "config.toml", config: "\n[[mcp_servers]]\nname = \"g\"\ncommand = \"npx\"\nargs = [\"-y\", \"ai-rulez@latest\", \"mcp\"]\n", want: []string{"AR012:config.toml:0"}},
		{name: "off", files: mcpJSON(`{"a":{"command":"uvx","args":["x"]}}`), config: "\n[lint.severity]\nmcp-unpinned-package = \"off\"\n", absent: []string{"AR012"}},
	})
}

func TestMCPSecretsAR015(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "literal env token", files: mcpJSON(`{"a":{"command":"node","env":{"GITHUB_TOKEN":"ghp_abcdefghijklmnopqrstuvwxyz0123456789"}}}`), want: []string{"AR015:.mcp.json:1"}},
		{name: "literal env by key name", files: mcpJSON(`{"a":{"command":"node","env":{"API_KEY":"s3cr3t-value-123"}}}`), want: []string{"AR015:.mcp.json:1"}},
		{name: "env reference is fine", files: mcpJSON(`{"a":{"command":"node","env":{"GITHUB_TOKEN":"${GITHUB_TOKEN}","API_KEY":"$API_KEY"}}}`), absent: []string{"AR015"}},
		{name: "non secret env", files: mcpJSON(`{"a":{"command":"node","env":{"LOG_LEVEL":"debug","AUTH_URL":"https://auth.example.com/login","TOKEN_FILE":"/run/secrets/token"}}}`), absent: []string{"AR015"}},
		{name: "placeholder", files: mcpJSON(`{"a":{"command":"node","env":{"API_KEY":"your-api-key-here","TOKEN":"xxxxxxxx"}}}`), absent: []string{"AR015"}},
		{name: "literal authorization header", files: mcpJSON(`{"a":{"type":"http","url":"https://x.test/mcp","headers":{"Authorization":"Bearer abcdef1234567890abcdef"}}}`), want: []string{"AR015:.mcp.json:1"}},
		{name: "header reference", files: mcpJSON(`{"a":{"type":"http","url":"https://x.test/mcp","headers":{"Authorization":"Bearer ${TOKEN}","X-Api-Key":"${env:KEY}"}}}`), absent: []string{"AR015"}},
		{name: "token flag", files: mcpJSON(`{"a":{"command":"node","args":["s.js","--api-key","abcdef1234567890"]}}`), want: []string{"AR015:.mcp.json:1"}},
		{name: "token flag with equals", files: mcpJSON(`{"a":{"command":"node","args":["--token=abcdef1234567890"]}}`), want: []string{"AR015:.mcp.json:1"}},
		{name: "credentials in the url", files: mcpJSON(`{"a":{"type":"http","url":"https://x.test/mcp?api_key=abcdef123456"}}`), want: []string{"AR015:.mcp.json:1"}},
		{
			name: "config.toml literal", config: "\n[[mcp_servers]]\nname = \"g\"\ncommand = \"node\"\nenv = { SLACK_BOT_TOKEN = \"xoxb-1234567890-abcdefghij\" }\n",
			want: []string{"AR015:config.toml:0"},
		},
		{name: "settings env", files: map[string]string{".claude/settings.json": `{"env":{"ANTHROPIC_API_KEY":"abc123def456ghi789jkl012","DEBUG":"1"}}`}, want: []string{"AR015:settings.json:1"}},
		{name: "settings http hook header", files: map[string]string{".claude/settings.json": `{"hooks":{"Stop":[{"hooks":[{"type":"http","url":"https://h.test","headers":{"Authorization":"Bearer literal-token-value-123"}}]}]}}`}, want: []string{"AR015:settings.json:1"}},
		{name: "settings env reference", files: map[string]string{".claude/settings.json": `{"env":{"ANTHROPIC_API_KEY":"$ANTHROPIC_API_KEY"}}`}, absent: []string{"AR015"}},
	})
}

func TestMCPSecretFindingDoesNotEchoTheValue(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".mcp.json":             `{"mcpServers":{"a":{"command":"node","env":{"API_KEY":"s3cr3t-value-123"}}}}`,
	})
	gitAdd(t, root)
	found := false
	for _, f := range lintDir(t, root) {
		if f.Code == CodeSecretInConfig {
			found = true
			if strings.Contains(f.Message, "s3cr3t") {
				t.Errorf("finding echoes the credential: %s", f.Message)
			}
		}
	}
	if !found {
		t.Fatal("no AR015 finding")
	}
}
