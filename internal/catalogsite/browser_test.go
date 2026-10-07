package catalogsite

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// These tests drive a real headless Chrome over the DevTools protocol (see
// browser_cdp_test.go) against sites opened from file://. They skip when no
// Chrome or Chromium is installed (set AI_RULEZ_CHROME to choose one) and with
// -short. They check what no HTML parser can: that the Content-Security-Policy
// is enforced and never violated, that no page logs a console error or throws,
// that nothing is requested outside the file system, that no script reaches a
// dialog, and that filtering, paging and navigation work in a real engine.

// richDoc is a catalog with every kind of page: seven items (three overview pages
// at a page size of three), MCP servers, dependencies, roles, eval and usage.
func richDoc() *govview.CatalogDocV2 {
	doc := manyItemsDoc(0)
	mk := func(kind, id, desc string, roleNames ...string) govview.CatalogItemV2 {
		it := graphItem(kind, id, roleNames...)
		it.Description = desc
		it.Owner = "team-" + id[:1]
		it.Version = "1.0.0"
		it.LoadCost = govview.LoadCost{ListingTokens: 12, BodyTokens: 340, ResourceTokens: 20, Resources: 1}
		it.Lint = &govview.ItemLint{Status: govview.LintOK, Findings: []govview.LintFinding{}}
		it.Excerpt = &govview.Excerpt{Text: "# " + id + "\n\n- first\n- second\n\n`code` and **bold** [link](https://example.com)\n\n<script>alert(1)</script>\n"}
		return it
	}
	doc.Items = []govview.CatalogItemV2{
		mk("agent", "reviewer", "Reviews changes", "dev"),
		mk("skill", "deploy", "Deploy the service", "dev", "ops"),
		mk("skill", "lint", "Lint the code", "dev"),
		mk("skill", "rollback", "Roll a deploy back", "ops"),
		mk("skill", "test", "Run the tests", "dev"),
		mk("rule", "style", "House style"),
		mk("command", "ship", "Ship it", "ops"),
	}
	doc.Items[1].Eval = &govview.ItemEval{Cases: 4, PassRate: 0.75, Passing: true, Verified: true, Date: "2026-10-01"}
	doc.Items[1].Usage = &govview.ItemUsage{Invocations: 14, LastSeen: "2026-10-04"}
	doc.Items[2].Lint = &govview.ItemLint{Status: govview.LintWarn, Counts: govview.LintCounts{Warning: 1},
		Findings: []govview.LintFinding{{Code: "AR303", Severity: "warning", Message: "unknown key", Line: 2}}}
	doc.Edges = []govview.CatalogEdge{use("agent/-/reviewer", "skill/-/deploy"), use("agent/-/reviewer", "skill/-/lint"),
		use("skill/-/deploy", "skill/-/rollback")}
	no := false
	doc.MCPServers = []govview.CatalogMCPServer{{Ref: "mcp/github", Name: "github", Transport: "stdio", CommandBasename: "npx", Enabled: true,
		Profiles: []string{}, Pinned: &no, Env: []govview.MCPValue{{Name: "GITHUB_TOKEN", Ref: "GITHUB_TOKEN"}}, Headers: []govview.MCPValue{},
		Warnings: []string{"launch is not pinned to an exact version or digest"}}}
	doc.Lint = govview.CatalogLint{Available: true, Summary: govview.LintSummary{Warnings: 1}, ByCode: map[string]int{"AR303": 1},
		Unattributed: []govview.ProjectFinding{}}
	doc.Roles = []govview.CatalogRole{{Name: "dev", Domains: []string{}}, {Name: "ops", Domains: []string{}}}
	return doc
}

func buildSite(t *testing.T, doc *govview.CatalogDocV2, opts Options) string {
	t.Helper()
	site, err := Render(doc, opts)
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "site")
	_, err = Write(nil, dir, site, false)
	require.NoError(t, err)
	return dir
}

func htmlPages(t *testing.T, dir string) []string {
	t.Helper()
	var pages []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".html") {
			rel, relErr := filepath.Rel(dir, p)
			require.NoError(t, relErr)
			pages = append(pages, filepath.ToSlash(rel))
		}
		return err
	}))
	sort.Strings(pages)
	return pages
}

func TestBrowser_EveryPageLoadsWithoutViolationsOrErrors(t *testing.T) {
	bin := requireBrowser(t)
	for name, doc := range map[string]*govview.CatalogDocV2{"rich": richDoc(), "hostile": hostileDoc()} {
		t.Run(name, func(t *testing.T) {
			// Arrange
			dir := buildSite(t, doc, Options{PageSize: 3, Markdown: true})
			b := startBrowser(t, bin)
			pages := htmlPages(t, dir)
			require.Greater(t, len(pages), 8)

			for _, page := range pages {
				// Act
				b.navigate(fileURL(dir, page))

				// Assert
				assert.Equalf(t, 0, b.evalInt(`window.__csp.length`), "%s: CSP violations: %s", page, b.eval(`window.__csp.join("; ")`))
				assert.Equalf(t, 1, b.evalInt(`document.querySelectorAll("h1").length`), "%s: one h1", page)
				assert.NotEmptyf(t, b.evalString(`document.title`), "%s: title", page)
				assert.Equalf(t, 0, b.evalInt(`document.querySelectorAll("script:not([src])").length`), "%s: no inline script", page)
				assert.Equalf(t, 0, b.evalInt(`document.querySelectorAll("[style]").length`), "%s: no inline style", page)
			}
			assert.Empty(t, b.problems(), "console errors, exceptions, dialogs and non-file requests across every page")
		})
	}
}

func TestBrowser_FilterPagingAndNavigationWork(t *testing.T) {
	bin := requireBrowser(t)
	dir := buildSite(t, richDoc(), Options{PageSize: 3})
	b := startBrowser(t, bin)
	b.navigate(fileURL(dir, "index.html"))

	const visibleRows = `document.querySelectorAll("#items tbody:not([hidden]) tr:not([hidden])").length`
	const firstVisible = `document.querySelector("#items tbody:not([hidden]) tr:not([hidden]) a").textContent`
	setField := func(id, value string) {
		b.eval(`(function(){var e=document.getElementById("` + id + `");e.value=` + jsString(value) + `;e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}))})()`)
	}

	// Paging: 7 items at 3 per page.
	require.Equal(t, 3, b.evalInt(visibleRows))
	assert.Equal(t, "Page 1 of 3", b.evalString(`document.querySelector(".pager span").textContent`))
	first := b.evalString(firstVisible)
	b.click(".pager button:last-of-type", false)
	assert.Equal(t, "Page 2 of 3", b.evalString(`document.querySelector(".pager span").textContent`))
	assert.NotEqual(t, first, b.evalString(firstVisible))
	b.click(".pager button:last-of-type", false)
	assert.Equal(t, 1, b.evalInt(visibleRows), "the last page holds the remainder")
	assert.True(t, b.evalBool(`document.querySelector(".pager button:last-of-type").disabled`))

	// Filtering looks across every page.
	setField("q", "deploy")
	assert.Equal(t, 2, b.evalInt(visibleRows), "deploy and rollback (whose description mentions deploy)")
	assert.Equal(t, "2 of 7 shown", b.evalString(`document.getElementById("count").textContent`))
	assert.True(t, b.evalBool(`document.querySelector(".pager").hidden`), "the pager steps aside while a filter is active")
	setField("q", "")
	setField("kind", "skill")
	assert.Equal(t, 4, b.evalInt(`document.querySelectorAll("#items tr:not([hidden])[data-kind]").length`))
	setField("kind", "")
	setField("status", "warn")
	assert.Equal(t, 1, b.evalInt(`document.querySelectorAll("#items tr:not([hidden])[data-kind]").length`))
	setField("status", "")
	assert.Equal(t, 3, b.evalInt(visibleRows), "clearing the filters pages again")

	// The "/" shortcut focuses the search box.
	b.eval(`document.body.focus();document.dispatchEvent(new KeyboardEvent("keydown",{key:"/",bubbles:true}))`)
	assert.Equal(t, "q", b.evalString(`document.activeElement.id`))

	// Navigation: overview -> item -> overview -> graph -> item -> MCP.
	title := b.evalString(`document.querySelector("h1").textContent`)
	name := b.evalString(firstVisible)
	b.click("#items tbody:not([hidden]) tr:not([hidden]) a", true)
	assert.Contains(t, b.evalString(`location.pathname`), "/items/")
	assert.Equal(t, name, b.evalString(`document.querySelector("h1").textContent`))
	assert.Contains(t, b.evalString(`document.querySelector("main").textContent`), "Load cost")
	b.click(`nav a[href$="index.html"]`, true)
	assert.Equal(t, title, b.evalString(`document.querySelector("h1").textContent`))

	b.click(`nav a[href$="graph.html"]`, true)
	assert.Equal(t, "Dependency graph", b.evalString(`document.querySelector("h1").textContent`))
	assert.Equal(t, 6, b.evalInt(`document.querySelectorAll("svg.graph-svg a").length`), "reviewer, deploy, lint, rollback and the two roles")
	assert.Equal(t, 3, b.evalInt(`document.querySelectorAll("svg.graph-svg path.edge-uses").length`))
	assert.Equal(t, 5, b.evalInt(`document.querySelectorAll("svg.graph-svg path.edge-role").length`))
	b.click(`svg.graph-svg a[href^="items/"]`, true)
	assert.Contains(t, b.evalString(`location.pathname`), "/items/")

	b.click(`nav a[href$="mcp.html"]`, true)
	assert.Equal(t, 1, b.evalInt(`document.querySelectorAll("#mcp tbody tr").length`))
	assert.Contains(t, b.evalString(`document.querySelector("#mcp").textContent`), "GITHUB_TOKEN (from ${GITHUB_TOKEN})")

	assert.Empty(t, b.problems())
}

func TestBrowser_ContentSecurityPolicyIsEnforced(t *testing.T) {
	// Arrange: a control that proves the violation listener works and the policy
	// is not vacuous, so "zero violations" in the other tests means something.
	bin := requireBrowser(t)
	dir := buildSite(t, richDoc(), Options{})
	b := startBrowser(t, bin)
	b.navigate(fileURL(dir, "index.html"))
	require.Equal(t, 0, b.evalInt(`window.__csp.length`))

	// Act: inject an inline script the way an XSS would
	b.eval(`(function(){var s=document.createElement("script");s.textContent="window.__ran=1";document.head.appendChild(s)})()`)

	// Assert
	assert.Equal(t, 1, b.evalInt(`typeof window.__ran === "undefined" ? 1 : 0`), "the inline script did not run")
	assert.GreaterOrEqual(t, b.evalInt(`window.__csp.length`), 1)
	assert.Contains(t, b.evalString(`window.__csp.join(";")`), "script-src")
}

func jsString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case '\n':
			sb.WriteString(`\n`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
