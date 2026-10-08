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
		{name: "names that differ only by case", files: mcpJSON(`{"GitHub":{"command":"node"},"github":{"command":"node"}}`), want: []string{"AR602:.mcp.json:1"}},
		{
			name: "a .mcp.json entry that mirrors config.toml is not checked twice", files: mcpJSON(`{"dup":{"command":"node"}}`),
			config: "\n[[mcp_servers]]\nname = \"dup\"\ncommand = \"node\"\n", absent: []string{"AR602"},
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
		{name: "npx caret range is not a pin", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@^1.2.0"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx tilde range is not a pin", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@~1.2"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx bare major is not a pin", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@1"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx comparison range is not a pin", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@>=1"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx github shorthand without a commit", files: mcpJSON(`{"a":{"command":"npx","args":["-y","github:foo/bar"]}}`), want: []string{"AR012:.mcp.json:1"}},
		{name: "npx github shorthand at a commit", files: mcpJSON(`{"a":{"command":"npx","args":["-y","github:foo/bar#0123456789abcdef0123456789abcdef01234567"]}}`), absent: []string{"AR012"}},
		{name: "npx exact and prerelease versions", files: mcpJSON(`{"a":{"command":"npx","args":["-y","pkg@1.2.3"]},"b":{"command":"npx","args":["-y","pkg@2.0.0-beta.1"]},"c":{"command":"npx","args":["-y","@s/pkg@v1.0.0"]}}`), absent: []string{"AR012"}},
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
		{name: "placeholder()", files: mcpJSON(`{"a":{"command":"node","env":{"API_KEY":"your-api-key-here","TOKEN":"xxxxxxxx"}}}`), absent: []string{"AR015"}},
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

func TestMCPJSONEntryWithTheNameOfAConfigServerIsChecked(t *testing.T) {
	cfg := "\n[[mcp_servers]]\nname = \"gh\"\ncommand = \"npx\"\nargs = [\"-y\", \"pkg@1.2.3\"]\n"
	runRuleCases(t, []ruleCase{
		{
			name: "an edited entry with an unpinned package and a literal token", config: cfg,
			files: mcpJSON(`{"gh":{"command":"npx","args":["-y","evil-pkg"],"env":{"GITHUB_TOKEN":"ghp_abcdefghijklmnopqrstuvwxyz0123456789"}}}`),
			want:  []string{"AR012:.mcp.json:1", "AR015:.mcp.json:1"},
		},
		{
			name: "an edited plain http url", config: "\n[[mcp_servers]]\nname = \"gh\"\nurl = \"https://x.test/mcp\"\n",
			files: mcpJSON(`{"gh":{"type":"http","url":"http://x.test/mcp"}}`), want: []string{"AR024:.mcp.json:1"},
		},
		{
			name: "an identical generated copy is checked once", config: cfg,
			files:  mcpJSON(`{"gh":{"command":"npx","args":["-y","pkg@1.2.3"]}}`),
			absent: []string{"AR012", "AR015", "AR602"},
		},
		{
			name: "a copy with a different transport spelling is still a copy", config: "\n[[mcp_servers]]\nname = \"gh\"\nurl = \"https://x.test/mcp\"\ntransport = \"http\"\n",
			files: mcpJSON(`{"gh":{"type":"http","url":"https://x.test/mcp"}}`), absent: []string{"AR602", "AR024"},
		},
	})
}

func TestMCPFindingsAreAnchoredOnTheServerDefinition(t *testing.T) {
	cfg := "\ngitignore = true\n\n[[mcp_servers]]\nname = \"git\"\ncommand = \"npx\"\nargs = [\"-y\", \"pkg\"]\n\n[[mcp_servers]]\nname = \"a\"\ncommand = \"npx\"\nargs = [\"-y\", \"other\"]\n"
	runRuleCases(t, []ruleCase{
		{name: "config.toml blocks", config: cfg, want: []string{"AR012:config.toml:8", "AR012:config.toml:13"}},
		{
			name: "plain http is anchored like the other checks", config: "\n\n[[mcp_servers]]\nname = \"b\"\nurl = \"https://ok.test\"\n\n[[mcp_servers]]\nname = \"a\"\nurl = \"http://x.test\"\n",
			want: []string{"AR024:config.toml:11"},
		},
		{
			name: "json key, not an earlier value", files: map[string]string{".mcp.json": "{\n \"note\": \"a\",\n \"mcpServers\": {\n  \"b\": {\"command\": \"node\"},\n  \"a\": {\"command\": \"npx\", \"args\": [\"-y\", \"p\"]}\n }\n}\n"},
			want: []string{"AR012:.mcp.json:5"},
		},
	})
}

func TestPinExampleUsesTheBarePackageName(t *testing.T) {
	for _, tc := range []struct{ command, pkg, want string }{
		{"uvx", "pkg>=1.0", "pkg==1.2.3"},
		{"uvx", "pkg@latest", "pkg==1.2.3"},
		{"pipx", "mcp-server-git", "mcp-server-git==1.2.3"},
		{"npx", "@scope/pkg@latest", "@scope/pkg@1.2.3"},
		{"npx", "pkg@^1.2.0", "pkg@1.2.3"},
		{"docker", "ghcr.io/o/img", "ghcr.io/o/img@sha256:..."},
	} {
		if got := pinExample(tc.command, tc.pkg); got != tc.want {
			t.Errorf("pinExample(%q, %q) = %q, want %q", tc.command, tc.pkg, got, tc.want)
		}
	}
}

func TestMCPConfigFindingsHonorATomlIgnoreComment(t *testing.T) {
	block := func(comment string) string {
		return "\n[[mcp_servers]]\n" + comment + "name = \"g\"\ncommand = \"npx\"\nargs = [\"-y\", \"pkg\"]\n"
	}
	runRuleCases(t, []ruleCase{
		{name: "comment above the name line", config: block("# ai-rulez-lint-ignore: AR012\n"), absent: []string{"AR012"}},
		{name: "no comment", config: block(""), want: []string{"AR012:config.toml:6"}},
	})
}

func TestExplainShowsTheTomlIgnoreFormForConfigAnchoredRules(t *testing.T) {
	e, ok := Explain("AR012")
	if !ok {
		t.Fatal("AR012 not found")
	}
	joined := strings.Join(e.Suppress, "\n")
	if !strings.Contains(joined, "# ai-rulez-lint-ignore: AR012") {
		t.Errorf("explain should show the TOML comment form for a config-anchored rule:\n%s", joined)
	}
	md, _ := Explain("AR001")
	if strings.Contains(strings.Join(md.Suppress, "\n"), "# ai-rulez-lint-ignore") {
		t.Errorf("a markdown-only rule should not show the TOML form")
	}
}
