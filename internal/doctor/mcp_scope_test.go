package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	projectNaraURL = "https://nara.armis.com/api/mcp/allMcpServer/mcp"
	localNaraURL   = "https://nara.armis.com/api/mcp/haverikoMcpServer/mcp"
	mcpScopeConfig = baseConfig + "\n[[mcp_servers]]\nname = \"nara\"\ntransport = \"http\"\nurl = \"" + projectNaraURL + "\"\n"
)

// setHome points the user-scope lookup at a fresh directory holding the given
// ~/.claude.json (empty means no file), so a test never reads the real home.
func setHome(t *testing.T, claudeJSON string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if claudeJSON == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(claudeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runMCPScope(t *testing.T, dir string, readUser bool) *Report {
	t.Helper()
	return Run(context.Background(), Options{
		Load:        loader(dir),
		LookPath:    func(name string) (string, error) { return "/bin/" + name, nil },
		ReadUserMCP: readUser,
	})
}

func TestCheckMCPScope_ProjectVsUserScope(t *testing.T) {
	// Arrange: the project server and a user-scope server share a name but not a url.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"nara":{"type":"http","url":%q}}}`, localNaraURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"nara", projectNaraURL, localNaraURL, "user", "claude mcp remove nara -s user"} {
		if !strings.Contains(got[0].Message+got[0].Hint, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_ProjectVsLocalScope(t *testing.T) {
	// Arrange: the shadowing server is in the per-project (local) section.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"projects":{%q:{"mcpServers":{"nara":{"type":"http","url":%q}}}}}`, dir, localNaraURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"local", "claude mcp remove nara -s local"} {
		if !strings.Contains(got[0].Message+got[0].Hint, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_IdenticalEndpointDoesNotWarn(t *testing.T) {
	// Arrange: the same name and the same url in both scopes is one server, not a conflict.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"nara":{"type":"http","url":%q}}}`, projectNaraURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none for identical endpoints", got)
	}
}

func TestCheckMCPScope_OffUnlessOptedIn(t *testing.T) {
	// Arrange: a real conflict that must stay invisible without the opt-in.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"nara":{"type":"http","url":%q}}}`, localNaraURL))

	// Act
	got := byCheck(runMCPScope(t, dir, false), CheckMCPScope)

	// Assert
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none without --mcp-user-scope", got)
	}
}

func TestCheckMCPScope_DuplicateURLDifferentNames(t *testing.T) {
	// Arrange: two generated servers that differ only by name but point at one url.
	cfg := baseConfig + "\n[[mcp_servers]]\nname = \"nara\"\ntransport = \"http\"\nurl = \"" + projectNaraURL + "\"\n\n[[mcp_servers]]\nname = \"nara-alias\"\ntransport = \"http\"\nurl = \"" + projectNaraURL + "\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	setHome(t, "")

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"nara", "nara-alias", projectNaraURL, "likely duplicates"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_HandAuthoredProjectFile(t *testing.T) {
	// Arrange: the project server is hand-authored in .mcp.json, not in config.toml.
	dir := project(t, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".mcp.json":             fmt.Sprintf(`{"mcpServers":{"nara":{"type":"http","url":%q}}}`, projectNaraURL),
	})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"nara":{"type":"http","url":%q}}}`, localNaraURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning for the hand-authored entry", got)
	}
}

func TestCheckMCPScope_MissingOrBrokenUserFileIsNotAFinding(t *testing.T) {
	tests := []struct {
		name       string
		claudeJSON string
	}{
		{name: "missing file"},
		{name: "broken json", claudeJSON: "{"},
		{name: "no servers", claudeJSON: "{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
			setHome(t, tt.claudeJSON)

			// Act
			got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

			// Assert
			if len(got) != 0 {
				t.Fatalf("findings = %+v, want none", got)
			}
		})
	}
}

func TestCheckMCPScope_SkipsHarnessesItCannotRead(t *testing.T) {
	// Arrange: cursor has no known user-scope MCP file, so it is named as skipped.
	cfg := strings.Replace(baseConfig, `presets = ["claude"]`, `presets = ["claude", "cursor"]`, 1)
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	setHome(t, "")

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityInfo || !strings.Contains(got[0].Message, "cursor") {
		t.Fatalf("findings = %+v, want one info naming cursor", got)
	}
}
