package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	projectTeamURL = "https://mcp.example.com/api/mcp/project/mcp"
	localTeamURL   = "https://mcp.example.com/api/mcp/personal/mcp"
	mcpScopeConfig = baseConfig + "\n[[mcp_servers]]\nname = \"team\"\ntransport = \"http\"\nurl = \"" + projectTeamURL + "\"\n"
)

// setHome points the user-scope lookup at a fresh directory holding the given
// ~/.claude.json (empty means no file), so a test never reads the real home.
// os.UserHomeDir reads USERPROFILE on Windows and HOME elsewhere.
func setHome(t *testing.T, claudeJSON string) {
	t.Helper()
	files := map[string]string{}
	if claudeJSON != "" {
		files[".claude.json"] = claudeJSON
	}
	setHomeFiles(t, files)
}

// setHomeFiles points the user-scope lookup at a fresh directory holding the
// given files (slash-separated, relative to home), so a test never reads the real
// home. It clears XDG_CONFIG_HOME, which relocates OpenCode's user config.
func setHomeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	for rel, body := range files {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runMCPScope(t *testing.T, dir string, readUser bool) *Report {
	t.Helper()
	return runMCPScopeLookPath(t, dir, readUser, func(name string) (string, error) { return "/bin/" + name, nil })
}

func runMCPScopeLookPath(t *testing.T, dir string, readUser bool, lookPath func(string) (string, error)) *Report {
	t.Helper()
	return Run(context.Background(), Options{
		Load:        loader(dir),
		LookPath:    lookPath,
		ReadUserMCP: readUser,
	})
}

func TestCheckMCPScope_ProjectVsUserScope(t *testing.T) {
	// Arrange: the project server and a user-scope server share a name but not a url.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"team":{"type":"http","url":%q}}}`, localTeamURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"team", projectTeamURL, localTeamURL, "user", "claude mcp remove team -s user"} {
		if !strings.Contains(got[0].Message+got[0].Hint, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_ProjectVsLocalScope(t *testing.T) {
	// Arrange: the shadowing server is in the per-project (local) section.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"projects":{%q:{"mcpServers":{"team":{"type":"http","url":%q}}}}}`, dir, localTeamURL))

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"local", "claude mcp remove team -s local"} {
		if !strings.Contains(got[0].Message+got[0].Hint, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_IdenticalEndpointDoesNotWarn(t *testing.T) {
	// Arrange: the same name and the same url in both scopes is one server, not a conflict.
	dir := project(t, map[string]string{".ai-rulez/config.toml": mcpScopeConfig})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"team":{"type":"http","url":%q}}}`, projectTeamURL))

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
	setHome(t, fmt.Sprintf(`{"mcpServers":{"team":{"type":"http","url":%q}}}`, localTeamURL))

	// Act
	got := byCheck(runMCPScope(t, dir, false), CheckMCPScope)

	// Assert
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none without --mcp-user-scope", got)
	}
}

func TestCheckMCPScope_DuplicateURLDifferentNames(t *testing.T) {
	// Arrange: two generated servers that differ only by name but point at one url.
	cfg := baseConfig + "\n[[mcp_servers]]\nname = \"team\"\ntransport = \"http\"\nurl = \"" + projectTeamURL + "\"\n\n[[mcp_servers]]\nname = \"team-alias\"\ntransport = \"http\"\nurl = \"" + projectTeamURL + "\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	setHome(t, "")

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"team", "team-alias", projectTeamURL, "likely duplicates"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_HandAuthoredProjectFile(t *testing.T) {
	// Arrange: the project server is hand-authored in .mcp.json, not in config.toml.
	dir := project(t, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".mcp.json":             fmt.Sprintf(`{"mcpServers":{"team":{"type":"http","url":%q}}}`, projectTeamURL),
	})
	setHome(t, fmt.Sprintf(`{"mcpServers":{"team":{"type":"http","url":%q}}}`, localTeamURL))

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

// opencodeScopeConfig is the base config with the given presets, so a test can
// opt OpenCode into the user-scope comparison.
func opencodeScopeConfig(presets string) string {
	return strings.Replace(baseConfig, `presets = ["claude"]`, presets, 1)
}

func TestCheckMCPScope_OpenCodeUserScopeConflict(t *testing.T) {
	// Arrange: OpenCode's canonical mcp.servers shape shadows the project server.
	dir := project(t, map[string]string{".ai-rulez/config.toml": opencodeScopeConfig(`presets = ["opencode"]`) + "[[mcp_servers]]\nname = \"team\"\ntransport = \"http\"\nurl = \"" + projectTeamURL + "\"\n"})
	setHomeFiles(t, map[string]string{
		".config/opencode/opencode.json": fmt.Sprintf(`{"mcp":{"servers":{"team":{"type":"remote","url":%q}}}}`, localTeamURL),
	})

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("findings = %+v, want one warning", got)
	}
	for _, want := range []string{"team", projectTeamURL, localTeamURL, "opencode", `remove "team" from ~/.config/opencode/opencode.json`} {
		if !strings.Contains(got[0].Message+got[0].Hint, want) {
			t.Errorf("finding %+v does not name %q", got[0], want)
		}
	}
}

func TestCheckMCPScope_OpenCodeV1ShapeIsAlsoRead(t *testing.T) {
	// Arrange: OpenCode still accepts the flat mcp.<name> entries next to mcp.servers.
	dir := project(t, map[string]string{".ai-rulez/config.toml": opencodeScopeConfig(`presets = ["opencode"]`) + "[[mcp_servers]]\nname = \"team\"\ntransport = \"http\"\nurl = \"" + projectTeamURL + "\"\n"})
	setHomeFiles(t, map[string]string{
		".config/opencode/opencode.json": fmt.Sprintf(`{"mcp":{"team":{"type":"remote","url":%q}}}`, localTeamURL),
	})

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 1 || got[0].Severity != SeverityWarning || !strings.Contains(got[0].Message, localTeamURL) {
		t.Fatalf("findings = %+v, want one warning naming the flat-shape url", got)
	}
}

func TestCheckMCPScope_UnresolvedUserCommandWarns(t *testing.T) {
	// Arrange: one entry names a missing file, the other a binary that is not on PATH.
	dir := project(t, map[string]string{".ai-rulez/config.toml": opencodeScopeConfig(`presets = ["opencode"]`)})
	// Built from a temp dir so the path is absolute on Windows too, where "/nonexistent/..." has no drive.
	missing := filepath.Join(t.TempDir(), "nonexistent", "ai-rulez")
	setHomeFiles(t, map[string]string{
		".config/opencode/opencode.json": fmt.Sprintf(`{"mcp":{"servers":{
			"gone":{"type":"local","command":[%q,"mcp"]},
			"ghost":{"type":"local","command":["ghost-binary","mcp"]}}}}`, missing),
	})
	lookPath := func(name string) (string, error) {
		if name == "ghost-binary" {
			return "", os.ErrNotExist
		}
		return "/bin/" + name, nil
	}

	// Act
	got := byCheck(runMCPScopeLookPath(t, dir, true, lookPath), CheckMCPScope)

	// Assert
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want two", got)
	}
	messages := got[0].Message + got[0].Hint + got[1].Message + got[1].Hint
	for _, want := range []string{strconv.Quote(missing), "does not exist", "ghost-binary", "is not on PATH"} {
		if !strings.Contains(messages, want) {
			t.Errorf("findings %+v do not name %q", got, want)
		}
	}
}

func TestCheckMCPScope_DisabledUserServerIsNotChecked(t *testing.T) {
	// Arrange: an entry OpenCode does not start cannot fail to start.
	dir := project(t, map[string]string{".ai-rulez/config.toml": opencodeScopeConfig(`presets = ["opencode"]`)})
	setHomeFiles(t, map[string]string{
		".config/opencode/opencode.json": `{"mcp":{"servers":{"gone":{"type":"local","command":["/nonexistent/ai-rulez","mcp"],"disabled":true}}}}`,
	})

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none for a disabled server", got)
	}
}

func TestCheckMCPScope_OpenCodeIsNoLongerSkipped(t *testing.T) {
	// Arrange: OpenCode has a known user-scope MCP file now.
	dir := project(t, map[string]string{".ai-rulez/config.toml": opencodeScopeConfig(`presets = ["opencode"]`)})
	setHomeFiles(t, nil)

	// Act
	got := byCheck(runMCPScope(t, dir, true), CheckMCPScope)

	// Assert
	for _, finding := range got {
		if finding.Check == CheckMCPScope && strings.Contains(finding.Message, "not checked for") && strings.Contains(finding.Message, "opencode") {
			t.Fatalf("opencode is still skipped: %+v", finding)
		}
	}
}

// TestXdgConfigHomeRelocatesOpenCode checks the user config path honors XDG.
func TestXdgConfigHomeRelocatesOpenCode(t *testing.T) {
	// Arrange
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Act
	got := opencodeUserConfigPath(home)

	// Assert
	if want := filepath.Join(xdg, "opencode", "opencode.json"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
