package settings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want settings.Rule
	}{
		{"bare shell", "Bash", settings.Rule{Raw: "Bash", Tool: "Bash", Kind: settings.KindShell, Bare: true}},
		{"shell exact", "Bash(npm run build)", settings.Rule{
			Raw: "Bash(npm run build)", Tool: "Bash", Kind: settings.KindShell, Specifier: "npm run build", Command: "npm run build"}},
		{"shell legacy prefix", "Bash(npm run test:*)", settings.Rule{
			Raw: "Bash(npm run test:*)", Tool: "Bash", Kind: settings.KindShell, Specifier: "npm run test:*",
			Command: "npm run test", Prefix: true}},
		{"shell wildcard", "Bash(git * --force)", settings.Rule{
			Raw: "Bash(git * --force)", Tool: "Bash", Kind: settings.KindShell, Specifier: "git * --force", Command: "git * --force"}},
		{"shell trailing wildcard", "Bash(git *)", settings.Rule{
			Raw: "Bash(git *)", Tool: "Bash", Kind: settings.KindShell, Specifier: "git *", Command: "git *"}},
		{"shell parens in command", "Bash(echo (hi))", settings.Rule{
			Raw: "Bash(echo (hi))", Tool: "Bash", Kind: settings.KindShell, Specifier: "echo (hi)", Command: "echo (hi)"}},
		{"read cwd path", "Read(./.env)", settings.Rule{
			Raw: "Read(./.env)", Tool: "Read", Kind: settings.KindRead, Specifier: "./.env",
			Path: settings.PathPattern{Anchor: settings.AnchorCwd, Glob: ".env"}}},
		{"read bare relative", "Read(secrets/**)", settings.Rule{
			Raw: "Read(secrets/**)", Tool: "Read", Kind: settings.KindRead, Specifier: "secrets/**",
			Path: settings.PathPattern{Anchor: settings.AnchorCwd, Glob: "secrets/**"}}},
		{"read project path", "Read(/src/**)", settings.Rule{
			Raw: "Read(/src/**)", Tool: "Read", Kind: settings.KindRead, Specifier: "/src/**",
			Path: settings.PathPattern{Anchor: settings.AnchorProject, Glob: "src/**"}}},
		{"read absolute path", "Read(//etc/passwd)", settings.Rule{
			Raw: "Read(//etc/passwd)", Tool: "Read", Kind: settings.KindRead, Specifier: "//etc/passwd",
			Path: settings.PathPattern{Anchor: settings.AnchorAbsolute, Glob: "/etc/passwd"}}},
		{"read home path", "Read(~/.ssh/**)", settings.Rule{
			Raw: "Read(~/.ssh/**)", Tool: "Read", Kind: settings.KindRead, Specifier: "~/.ssh/**",
			Path: settings.PathPattern{Anchor: settings.AnchorHome, Glob: ".ssh/**"}}},
		{"edit glob", "Edit(**/*.go)", settings.Rule{
			Raw: "Edit(**/*.go)", Tool: "Edit", Kind: settings.KindEdit, Specifier: "**/*.go",
			Path: settings.PathPattern{Anchor: settings.AnchorCwd, Glob: "**/*.go"}}},
		{"write path", "Write(./dist/**)", settings.Rule{
			Raw: "Write(./dist/**)", Tool: "Write", Kind: settings.KindEdit, Specifier: "./dist/**",
			Path: settings.PathPattern{Anchor: settings.AnchorCwd, Glob: "dist/**"}}},
		{"bare edit", "Edit", settings.Rule{Raw: "Edit", Tool: "Edit", Kind: settings.KindEdit, Bare: true}},
		{"webfetch domain", "WebFetch(domain:example.com)", settings.Rule{
			Raw: "WebFetch(domain:example.com)", Tool: "WebFetch", Kind: settings.KindFetch,
			Specifier: "domain:example.com", Domain: "example.com"}},
		{"bare webfetch", "WebFetch", settings.Rule{Raw: "WebFetch", Tool: "WebFetch", Kind: settings.KindFetch, Bare: true}},
		{"websearch", "WebSearch", settings.Rule{Raw: "WebSearch", Tool: "WebSearch", Kind: settings.KindSearch, Bare: true}},
		{"mcp tool", "mcp__github__create_issue", settings.Rule{
			Raw: "mcp__github__create_issue", Tool: "mcp", Kind: settings.KindMCP, Bare: true,
			Server: "github", MCPTool: "create_issue"}},
		{"mcp server", "mcp__github", settings.Rule{
			Raw: "mcp__github", Tool: "mcp", Kind: settings.KindMCP, Bare: true, Server: "github"}},
		{"mcp server wildcard", "mcp__github__*", settings.Rule{
			Raw: "mcp__github__*", Tool: "mcp", Kind: settings.KindMCP, Bare: true, Server: "github"}},
		{"mcp hyphenated server", "mcp__my-server__do_thing", settings.Rule{
			Raw: "mcp__my-server__do_thing", Tool: "mcp", Kind: settings.KindMCP, Bare: true,
			Server: "my-server", MCPTool: "do_thing"}},
		{"agent", "Task(Explore)", settings.Rule{
			Raw: "Task(Explore)", Tool: "Task", Kind: settings.KindAgent, Specifier: "Explore"}},
		{"unknown tool", "Skill(deploy)", settings.Rule{
			Raw: "Skill(deploy)", Tool: "Skill", Kind: settings.KindOther, Specifier: "deploy"}},
		{"surrounding space", "  Bash(ls)  ", settings.Rule{
			Raw: "Bash(ls)", Tool: "Bash", Kind: settings.KindShell, Specifier: "ls", Command: "ls"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := settings.ParseRule(tt.raw)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRuleErrors(t *testing.T) {
	for _, raw := range []string{"", "   ", "Bash(", "Bash)", "Bash()", "Bash(ls) extra", "(ls)", "mcp__", "mcp__a__b(x)", "Read(  )", "1Bash"} {
		t.Run(raw, func(t *testing.T) {
			_, err := settings.ParseRule(raw)
			assert.Error(t, err)
		})
	}
}

func TestRuleShell(t *testing.T) {
	tests := []struct {
		raw  string
		want settings.ShellPattern
	}{
		{"Bash(npm run test:*)", settings.ShellPattern{Kind: settings.ShellPrefix, Literal: "npm run test"}},
		{"Bash(git *)", settings.ShellPattern{Kind: settings.ShellPrefix, Literal: "git"}},
		{"Bash(git commit *)", settings.ShellPattern{Kind: settings.ShellPrefix, Literal: "git commit"}},
		{"Bash(npm run build)", settings.ShellPattern{Kind: settings.ShellExact, Literal: "npm run build"}},
		{"Bash(*)", settings.ShellPattern{Kind: settings.ShellAny}},
		{"Bash(:*)", settings.ShellPattern{Kind: settings.ShellAny}},
		{"Bash", settings.ShellPattern{Kind: settings.ShellAny}},
		{"Bash(git * --force)", settings.ShellPattern{Kind: settings.ShellGlob, Literal: "git * --force"}},
		{"Bash(ls*)", settings.ShellPattern{Kind: settings.ShellGlob, Literal: "ls*"}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			// Arrange
			rule, err := settings.ParseRule(tt.raw)
			require.NoError(t, err)

			// Act
			got := rule.Shell()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
