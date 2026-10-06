package catalogsite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

// hostile strings that must never reach a page unescaped.
var hostile = []string{
	`<script>alert(1)</script>`,
	`"><img src=x onerror=alert(1)>`,
	`'-alert(1)-'`,
	`</script><script>alert(2)</script>`,
	`<!-- comment`,
	`javascript:alert(3)`,
	`data:text/html,<script>alert(4)</script>`,
	"bidi\u202Eevil\u2066x\u2069 zero\u200Bwidth\uFEFF",
}

func hostileDoc() *govview.CatalogDocV2 {
	item := func(i int, id string) govview.CatalogItemV2 {
		h := hostile[i%len(hostile)]
		return govview.CatalogItemV2{
			Ref: "skill/" + id, Kind: "skill", ID: id, Domain: h, Path: "skills/" + h + "/SKILL.md",
			Description: h, Owner: h, Version: h, Delivery: "static", Source: govview.CatalogSource{Type: "local"},
			Bytes: 10, Tokens: 3, Digest: "sha256:" + strings.Repeat("a", 64),
			LoadCost: govview.LoadCost{ListingTokens: 2, BodyTokens: 3},
			Lint: &govview.ItemLint{Status: govview.LintWarn, Counts: govview.LintCounts{Warning: 1},
				Findings: []govview.LintFinding{{Code: h, Severity: "warning", Message: h, Line: 2}}},
			Excerpt: &govview.Excerpt{Text: h + "\n" + strings.Join(hostile, "\n")},
			Roles:   []string{h}, RoleDelivery: map[string]string{h: h},
		}
	}
	doc := &govview.CatalogDocV2{
		SchemaVersion: govview.CatalogSchemaVersionV2, GeneratedBy: govview.CatalogGenerator{Name: "ai-rulez", Version: "5.0.0"},
		Project: govview.CatalogProject{Name: hostile[0], Description: hostile[1]}, Tokenizer: "cl100k_base",
		Roles: []govview.CatalogRole{{Name: hostile[1], Description: hostile[0], Domains: []string{hostile[2]},
			Delivery: map[string]string{hostile[3]: hostile[4]}, Match: &config.RoleMatch{Groups: []string{hostile[5]}},
			Totals: roles.Totals{Items: 1, ByKind: map[string]int{hostile[0]: 1}}}},
		Lock:  govview.CatalogLock{Present: true, Tree: hostile[0]},
		Notes: []string{hostile[0]},
		Lint: govview.CatalogLint{Available: true, Summary: govview.LintSummary{Warnings: 1}, ByCode: map[string]int{},
			Unattributed: []govview.ProjectFinding{{LintFinding: govview.LintFinding{Code: hostile[1], Severity: "error", Message: hostile[2]}, File: hostile[3]}}},
	}
	yes := true
	for i, h := range hostile {
		doc.MCPServers = append(doc.MCPServers, govview.CatalogMCPServer{Ref: "mcp/" + h, Name: h, Description: h, Transport: "stdio",
			CommandBasename: h, Enabled: true, Profiles: []string{h}, Pinned: &yes,
			Env: []govview.MCPValue{{Name: h, Ref: h}, {Name: "LIT", Literal: true}}, Headers: []govview.MCPValue{{Name: h, Literal: i%2 == 0}},
			Warnings: []string{h}})
	}
	doc.MCPServers = append(doc.MCPServers, doc.MCPServers[0]) // a repeated name must not repeat an anchor
	for i, id := range append([]string{"Deploy", "deploy", "DEPLOY", "con", "a/b", "a b", ".."}, hostile...) {
		doc.Items = append(doc.Items, item(i, id))
	}
	doc.Roles[0].Name = hostile[1]
	for i := range doc.Items {
		doc.Items[i].Roles = []string{hostile[1]}
	}
	return doc
}

var (
	tagRE  = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)((?:\s+[^<>]*)?)>`)
	attrRE = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9:-]*)(?:=("[^"]*"|'[^']*'|[^\s"'>]+))?`)

	allowedTags = map[string]bool{
		"html": true, "head": true, "meta": true, "title": true, "link": true, "body": true, "a": true, "header": true,
		"nav": true, "main": true, "footer": true, "p": true, "h1": true, "h2": true, "table": true, "caption": true,
		"thead": true, "tbody": true, "tr": true, "th": true, "td": true, "code": true, "pre": true, "ul": true, "li": true,
		"span": true, "form": true, "label": true, "input": true, "select": true, "option": true, "section": true, "script": true,
		"div": true, "h3": true, "h4": true, "h5": true, "h6": true, "ol": true, "blockquote": true, "em": true, "strong": true, "br": true, "hr": true, "svg": true, "g": true, "path": true, "polygon": true, "rect": true, "text": true,
	}
	allowedAttrs = map[string]bool{
		"lang": true, "charset": true, "name": true, "content": true, "http-equiv": true, "rel": true, "href": true, "src": true,
		"defer": true, "class": true, "id": true, "scope": true, "dir": true, "aria-label": true, "aria-current": true,
		"aria-live": true, "role": true, "for": true, "type": true, "autocomplete": true, "placeholder": true, "value": true,
		"data-kind": true, "data-status": true, "data-text": true, "data-page": true,
		"viewbox": true, "width": true, "height": true, "d": true, "points": true, "x": true, "y": true, "rx": true,
		"aria-labelledby": true, "tabindex": true, "start": true,
	}
	schemeRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// checkHTML fails on any element or attribute outside the allow-list, any
// event handler, any non-relative URL, and any script that is not a local file.
func checkHTML(t *testing.T, name, page string) {
	t.Helper()
	body := strings.ReplaceAll(page, "<!doctype html>", "")
	for _, m := range tagRE.FindAllStringSubmatch(body, -1) {
		tag := strings.ToLower(m[2])
		require.Truef(t, allowedTags[tag], "%s: element <%s> not allowed in %q", name, tag, m[0])
		for _, a := range attrRE.FindAllStringSubmatch(m[3], -1) {
			attr := strings.ToLower(a[1])
			require.Falsef(t, strings.HasPrefix(attr, "on"), "%s: event handler %s", name, attr)
			require.Truef(t, allowedAttrs[attr], "%s: attribute %s not allowed in %q", name, attr, m[0])
			val := strings.Trim(a[2], `"'`)
			if attr == "href" || attr == "src" {
				require.Falsef(t, schemeRE.MatchString(val) || strings.HasPrefix(val, "//"), "%s: non-relative %s=%q", name, attr, val)
			}
		}
		if tag == "script" && m[1] == "" {
			require.Regexpf(t, `^\s+src="(\.\./)*assets/catalog\.js"\s+defer$`, m[3], "%s: unexpected script", name)
		}
	}
	// A raw "<" outside a tag would be unescaped source text.
	stripped := tagRE.ReplaceAllString(body, "")
	stripped = strings.ReplaceAll(stripped, "<!--", "")
	require.NotContainsf(t, stripped, "<", "%s: raw < outside a tag", name)
}

func TestRender_HostileTextIsEscaped(t *testing.T) {
	// Arrange
	doc := hostileDoc()

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	pages := 0
	for name, data := range site.Files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		pages++
		page := string(data)
		checkHTML(t, name, page)
		for _, r := range []string{"\u202E", "\u2066", "\u2069", "\u200B", "\uFEFF"} {
			assert.NotContainsf(t, page, r, "%s: hidden character %U reached the page", name, []rune(r)[0])
		}
		for _, h := range hostile[:7] {
			if !strings.ContainsAny(h, `<>"'`) {
				continue // plain text such as "javascript:..." is shown as text; checkHTML proves it is never a URL
			}
			assert.NotContainsf(t, page, h, "%s: %q appears unescaped", name, h)
		}
		assert.Contains(t, page, `content="default-src 'none'; img-src 'self' data:; style-src 'self'; script-src 'self'; base-uri 'none'; form-action 'none'"`, name)
		assert.Contains(t, page, `<meta name="referrer" content="no-referrer">`, name)
		assert.NotContains(t, page, "style=", name)
	}
	assert.Greater(t, pages, len(doc.Items))
	assert.Contains(t, string(site.Files["index.html"]), "contains hidden characters")
}

func TestRender_NamesNeverCollideOrEscapeTheDirectory(t *testing.T) {
	// Arrange
	doc := hostileDoc()

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	seen := map[string]string{}
	for _, name := range site.Paths() {
		assert.Falsef(t, strings.Contains(name, ".."), "path %q", name)
		assert.Falsef(t, strings.HasPrefix(name, "/"), "path %q", name)
		lower := strings.ToLower(name)
		other, dup := seen[lower]
		assert.Falsef(t, dup, "%q collides with %q on a case-insensitive file system", name, other)
		seen[lower] = name
	}
}

func TestRender_Deterministic(t *testing.T) {
	// Arrange
	first, err := Render(hostileDoc(), Options{Title: "T"})
	require.NoError(t, err)

	// Act
	t.Setenv("TZ", "Pacific/Kiritimati")
	t.Setenv("LANG", "tr_TR.UTF-8")
	t.Chdir(t.TempDir())
	second, err := Render(hostileDoc(), Options{Title: "T"})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first.Digest, second.Digest)
	require.Equal(t, first.Paths(), second.Paths())
	for _, name := range first.Paths() {
		assert.Equal(t, string(first.Files[name]), string(second.Files[name]), name)
	}
}

func TestRender_NoNetworkReferences(t *testing.T) {
	// Arrange
	site, err := Render(hostileDoc(), Options{})
	require.NoError(t, err)
	external := regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?(https?:)?//|url\(|@import`)

	// Act / Assert
	for name, data := range site.Files {
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".txt") {
			continue
		}
		assert.Falsef(t, external.Match(data), "%s references an external resource", name)
		for _, banned := range []string{"fetch(", "XMLHttpRequest", "WebSocket", "sendBeacon", "import(", "eval(", "innerHTML"} {
			assert.NotContainsf(t, string(data), banned, name)
		}
	}
}

func TestRender_CatalogJSONIsTheContract(t *testing.T) {
	// Arrange
	doc := hostileDoc()

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	var back govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(site.Files["catalog.json"], &back))
	assert.Equal(t, doc.Items[0].ID, back.Items[0].ID)
	assert.Equal(t, hostile[0], back.Notes[0])
	assert.NotContains(t, string(site.Files["catalog.json"]), "<script>", "JSON is HTML-safe by default")
}

func TestRender_RobotsAndIndexable(t *testing.T) {
	tests := []struct {
		name      string
		indexable bool
	}{{"private", false}, {"indexable", true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			site, err := Render(hostileDoc(), Options{Indexable: tt.indexable})

			// Assert
			require.NoError(t, err)
			_, hasRobots := site.Files["robots.txt"]
			assert.Equal(t, !tt.indexable, hasRobots)
			assert.Equal(t, !tt.indexable, strings.Contains(string(site.Files["index.html"]), `name="robots"`))
		})
	}
}

func TestRender_EmptyCatalog(t *testing.T) {
	// Arrange
	doc := &govview.CatalogDocV2{SchemaVersion: 2, GeneratedBy: govview.CatalogGenerator{Name: "ai-rulez", Version: "v"},
		Items: []govview.CatalogItemV2{}, Roles: []govview.CatalogRole{}, Notes: []string{}}

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(site.Files["index.html"]), "no items")
	assert.Contains(t, string(site.Files["lint.html"]), "unavailable")
	for _, name := range []string{"about.html", "lock.html", "assets/catalog.css", "assets/catalog.js"} {
		assert.Contains(t, site.Files, name)
	}
}

func TestRender_StructuralAccessibility(t *testing.T) {
	// Arrange
	site, err := Render(hostileDoc(), Options{})
	require.NoError(t, err)

	// Act / Assert
	for name, data := range site.Files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		page := string(data)
		assert.Equalf(t, 1, strings.Count(page, "<h1"), "%s: exactly one h1", name)
		assert.Contains(t, page, `<html lang="en">`, name)
		assert.Contains(t, page, `class="skip"`, name)
		assert.Equalf(t, strings.Count(page, "<table"), strings.Count(page, "<caption"), "%s: every table has a caption", name)
		assert.NotContainsf(t, page, "<th>", "%s: th needs scope", name)
	}
}

func TestSource_NeverBuildsTrustedTemplateTypes(t *testing.T) {
	// Arrange
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	banned := regexp.MustCompile(`template\.(HTML|JS|JSStr|URL|CSS|HTMLAttr|Srcset)\b`)

	// Act / Assert
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		code := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(data), "")
		assert.Falsef(t, banned.MatchString(code), "%s converts text to a trusted template type", f)
	}
}

func TestEmbeddedScript_IsSmallAndSelfContained(t *testing.T) {
	// Arrange / Act
	site, err := Render(hostileDoc(), Options{})

	// Assert
	require.NoError(t, err)
	assert.LessOrEqual(t, len(site.Files["assets/catalog.js"]), 8<<10)
}

func TestRender_ShowsApprovalStatusEscaped(t *testing.T) {
	// Arrange: one item per approval state, one with hostile reviewer names
	doc := hostileDoc()
	doc.Items = doc.Items[:3]
	doc.Items[0].Approval = &govview.ItemApproval{Required: true, Status: "ok", Reviewers: []string{"alice", hostile[0]}, Assurance: "review-linked", Expires: "2027-01-01"}
	doc.Items[1].Approval = &govview.ItemApproval{Required: true, Status: "missing", Reviewers: []string{}}
	doc.Items[2].Approval = nil

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	index := string(site.Files["index.html"])
	assert.Contains(t, index, "<th scope=\"col\">Approval</th>")
	assert.Contains(t, index, ">ok<")
	assert.Contains(t, index, ">missing<")
	var pages []string
	for name, data := range site.Files {
		if strings.HasPrefix(name, "items/") && strings.HasSuffix(name, ".html") {
			pages = append(pages, string(data))
			checkHTML(t, name, string(data))
		}
	}
	all := strings.Join(pages, "\n")
	assert.NotContains(t, all, "not recorded")
	assert.Contains(t, all, "ok (required) by alice")
	assert.Contains(t, all, "assurance review-linked")
	assert.Contains(t, all, "until 2027-01-01")
	assert.Contains(t, all, "missing (required)")
	assert.Contains(t, all, "not required")
	assert.NotContains(t, all, hostile[0], "a reviewer name is escaped")
}

func TestRender_MCPPageShowsNamesNotValues(t *testing.T) {
	// Arrange
	no := false
	doc := hostileDoc()
	doc.MCPServers = []govview.CatalogMCPServer{
		{Ref: "mcp/github", Name: "github", Transport: "stdio", CommandBasename: "npx", Enabled: true, Profiles: []string{},
			Pinned: &no, Env: []govview.MCPValue{{Name: "GITHUB_TOKEN", Ref: "GITHUB_TOKEN"}, {Name: "REGION", Literal: true}},
			Headers: []govview.MCPValue{}, Warnings: []string{"launch is not pinned to an exact version or digest"}},
		{Ref: "mcp/remote", Name: "remote", Transport: "http", Enabled: false, Profiles: []string{"ci"}, Env: []govview.MCPValue{},
			Headers: []govview.MCPValue{{Name: "Authorization", Literal: true}}, Warnings: []string{}},
	}

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	page := string(site.Files["mcp.html"])
	checkHTML(t, "mcp.html", page)
	assert.Contains(t, page, "GITHUB_TOKEN (from ${GITHUB_TOKEN})")
	assert.Contains(t, page, "REGION (literal value)")
	assert.Contains(t, page, "Authorization (literal value)")
	assert.Contains(t, page, "launch is not pinned")
	assert.Contains(t, string(site.Files["index.html"]), `href="mcp.html"`)
}

func TestRender_MCPPageEmpty(t *testing.T) {
	// Arrange
	doc := hostileDoc()
	doc.MCPServers = []govview.CatalogMCPServer{}

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(site.Files["mcp.html"]), "defines no MCP servers")
}
