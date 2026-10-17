package pluginimport_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func FuzzParseIndex(f *testing.F) {
	for _, seed := range []string{
		``, `{}`, `{"plugins":[]}`, `{"name":"m","plugins":[{"name":"p","source":"./p"}]}`,
		`{"metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"p","source":"p"}]}`,
		`{"plugins":[{"name":"p","source":{"source":"github","repo":"a/b","ref":"v1","sha":"0123456789abcdef0123456789abcdef01234567"}}]}`,
		`{"plugins":[{"name":"p","source":{"source":"git-subdir","url":"https://e.com/x.git","path":"a/b"}}]}`,
		`{"plugins":[{"name":"p","source":"../x"}]}`, `{"plugins":[{"name":"p","source":{"source":"url","url":"https://u:p@e.com/x"}}]}`,
		`{"plugins":[{"name":"p","source":"./p","strict":false,"commands":["./c.md"],"mcpServers":{"s":{}}}]}`, `[`, `{"plugins":5}`,
	} {
		f.Add([]byte(seed), pluginimport.IndexClaude)
	}
	f.Fuzz(func(t *testing.T, data []byte, format string) {
		// Act
		ix, err := pluginimport.ParseIndex(data, format)
		if err != nil {
			return
		}

		// Assert: whatever parsed is safe to use. A path source never leaves the
		// marketplace and a repository source never carries a credential.
		_ = ix.Names()
		_ = ix.SortedNames()
		for _, e := range ix.Plugins {
			if e.Problem != "" {
				continue
			}
			switch e.Source.Kind {
			case pluginimport.SourcePath:
				if strings.HasPrefix(e.Source.Path, "/") || strings.HasPrefix(e.Source.Path, "..") || strings.Contains(e.Source.Path, "/../") || strings.Contains(e.Source.Path, "\\") {
					t.Fatalf("source path %q escapes the marketplace", e.Source.Path)
				}
			case pluginimport.SourceRepo:
				if u, perr := url.Parse(e.Source.URL); perr == nil && u.User != nil && u.Scheme != "ssh" {
					t.Fatalf("repository source %q carries a credential", e.Source.URL)
				}
			default:
				t.Fatalf("entry %q has no source kind", e.Name)
			}
		}
	})
}

func FuzzCleanRel(f *testing.F) {
	for _, seed := range []string{"", ".", "..", "./a", "a/../b", "../a", "/a", "a\\b", "~/a", "C:/a", "a\x00b", "a//b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := pluginimport.CleanRel(in)
		if err != nil {
			return
		}
		if strings.HasPrefix(got, "/") || got == ".." || strings.HasPrefix(got, "../") || strings.Contains(got, "/../") {
			t.Fatalf("CleanRel(%q) = %q escapes", in, got)
		}
		again, err := pluginimport.CleanRel(got)
		if err != nil || again != got {
			t.Fatalf("CleanRel is not idempotent: %q -> %q -> %q (%v)", in, got, again, err)
		}
	})
}

func FuzzLoadManifestAndDocuments(f *testing.F) {
	f.Add(`{"name":"p","skills":"./s","agents":["./a"],"hooks":{"Stop":[]},"mcpServers":{"s":{"command":"x"}}}`,
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/x"}]}]}}`,
		`{"mcpServers":{"s":{"command":"node","args":["${CLAUDE_PLUGIN_ROOT}/s.js"],"env":{"K":"v"}}}}`)
	f.Add(`[]`, `{"hooks":5}`, `{"mcpServers":[]}`)
	f.Add(`{"commands":"../../etc"}`, ``, `{`)
	f.Fuzz(func(t *testing.T, manifest, hooks, mcp string) {
		// Arrange
		dir := t.TempDir()
		testutil.WriteTree(t, dir, map[string]string{
			".claude-plugin/plugin.json": manifest, "hooks/hooks.json": hooks, ".mcp.json": mcp, "commands/c.md": "c\n",
		})

		// Act: hostile documents fail or are reported, never panic or escape the directory
		res, err := pluginimport.Load(context.Background(), dir, &pluginimport.Options{EnableHooks: true, EnableMCP: true, Rewrite: "./p"})
		if err != nil {
			return
		}

		// Assert
		for _, c := range res.Commands {
			if !strings.HasPrefix(c.Path, dir) {
				t.Fatalf("command %q read from outside the plugin: %s", c.Name, c.Path)
			}
		}
		for _, s := range res.Scan {
			if !strings.HasPrefix(s.Path, dir) {
				t.Fatalf("scan file outside the plugin: %s", s.Path)
			}
		}
	})
}

func FuzzPluginHooksAndMCP(f *testing.F) {
	f.Add([]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x","timeout":5}]}]}}`))
	f.Add([]byte(`{"mcpServers":{"a":{"command":"c","args":[1,2],"env":{"T":"sk-abcdefghijklmnopqrstuvwxyz0123456789"}}}}`))
	f.Add([]byte(`{"a":{"url":"https://u:p@e.com","headers":{"Authorization":"Bearer x"}}}`))
	f.Add([]byte(`[`))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, harness := range []string{"claude", "cursor", "gemini", "codex"} {
			_, _ = importer.PluginHooks("hooks.json", data, harness)
		}
		servers, _ := importer.PluginMCP(".mcp.json", data)
		for _, s := range servers {
			if s.Name == "" {
				t.Fatal("a server without a name was imported")
			}
		}
	})
}
