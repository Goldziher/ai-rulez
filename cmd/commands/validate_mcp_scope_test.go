package commands

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const (
	validateProjectNaraURL = "https://nara.armis.com/api/mcp/allMcpServer/mcp"
	validateLocalNaraURL   = "https://nara.armis.com/api/mcp/haverikoMcpServer/mcp"
)

// TestValidateMCPUserScopeFlagIsOptIn pins the flag: it exists on validate, not
// on scan, and it needs the content checks.
func TestValidateMCPUserScopeFlagIsOptIn(t *testing.T) {
	if ValidateCmd.Flags().Lookup("mcp-user-scope") == nil {
		t.Error("validate is missing --mcp-user-scope")
	}
	if ScanCmd.Flags().Lookup("mcp-user-scope") != nil {
		t.Error("scan must not expose --mcp-user-scope")
	}

	oldStrict, oldScope := validateStrict, validateMCPUserScope
	t.Cleanup(func() { validateStrict, validateMCPUserScope = oldStrict, oldScope })

	validateStrict, validateMCPUserScope = false, true
	if err := checkStrictFlags(); err == nil {
		t.Error("--mcp-user-scope without --strict must be rejected")
	}
	validateStrict = true
	if err := checkStrictFlags(); err != nil {
		t.Errorf("--mcp-user-scope with --strict must be accepted: %v", err)
	}
}

// TestStrictLintMCPUserScope checks that --mcp-user-scope adds the scope-conflict
// finding to a strict report, and that the same run without the flag stays silent.
func TestStrictLintMCPUserScope(t *testing.T) {
	// Arrange: a project server also defined, at a different url, in the local
	// scope of the configured claude harness.
	extra := fmt.Sprintf("\n[[mcp_servers]]\nname = \"nara\"\ntransport = \"http\"\nurl = %q\n", validateProjectNaraURL)
	_, cfg := strictProject(t, extra, nil)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".claude.json"),
		fmt.Sprintf(`{"projects":{%q:{"mcpServers":{"nara":{"type":"http","url":%q}}}}}`, cfg.BaseDir, validateLocalNaraURL))

	oldScope := validateMCPUserScope
	t.Cleanup(func() { validateMCPUserScope = oldScope })

	// Act: off by default.
	validateMCPUserScope = false
	off := mcpScopeFindings(lintProject(t, cfg))

	// Assert: no finding names the shadowing server.
	if len(off) != 0 {
		t.Fatalf("without the flag: findings = %+v, want none", off)
	}

	// Act: on.
	validateMCPUserScope = true
	on := mcpScopeFindings(lintProject(t, cfg))

	// Assert
	if len(on) != 1 {
		t.Fatalf("with the flag: findings = %+v, want one", on)
	}
	if on[0].Code != lint.CodeMCPConfigInvalid || on[0].Severity != lint.SeverityWarning {
		t.Errorf("finding = %+v, want a warning under %s", on[0], lint.CodeMCPConfigInvalid)
	}
	for _, want := range []string{"nara", validateProjectNaraURL, validateLocalNaraURL, "claude mcp remove nara -s local"} {
		if !strings.Contains(on[0].Message+on[0].File, want) {
			t.Errorf("finding %+v does not name %q", on[0], want)
		}
	}
}

// mcpScopeFindings filters a report to the MCP scope findings this change adds.
func mcpScopeFindings(report *lint.Report) []lint.Finding {
	var out []lint.Finding
	for _, f := range report.Findings {
		if f.Code == lint.CodeMCPConfigInvalid && strings.Contains(f.Message, "multiple scopes") {
			out = append(out, f)
		}
	}
	return out
}
