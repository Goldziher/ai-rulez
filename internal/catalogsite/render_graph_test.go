package catalogsite

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

func graphItem(kind, id string, roleNames ...string) govview.CatalogItemV2 {
	return govview.CatalogItemV2{Ref: kind + "/-/" + id, Kind: kind, ID: id, Source: govview.CatalogSource{Type: "local"},
		Digest: "sha256:" + strings.Repeat("b", 64), Roles: append([]string{}, roleNames...)}
}

func graphDoc(items []govview.CatalogItemV2, edges ...govview.CatalogEdge) *govview.CatalogDocV2 {
	doc := &govview.CatalogDocV2{SchemaVersion: 2, GeneratedBy: govview.CatalogGenerator{Name: "ai-rulez", Version: "v"},
		Items: items, MCPServers: []govview.CatalogMCPServer{}, Edges: edges, Roles: []govview.CatalogRole{}, Notes: []string{}}
	seen := map[string]bool{}
	for _, it := range items {
		for _, r := range it.Roles {
			if !seen[r] {
				seen[r] = true
				doc.Roles = append(doc.Roles, govview.CatalogRole{Name: r, Domains: []string{}, Totals: roles.Totals{}})
			}
		}
	}
	if doc.Edges == nil {
		doc.Edges = []govview.CatalogEdge{}
	}
	return doc
}

func use(from, to string) govview.CatalogEdge {
	return govview.CatalogEdge{From: from, To: to, Kind: "uses"}
}

func renderGraphPage(t *testing.T, doc *govview.CatalogDocV2) string {
	t.Helper()
	site, err := Render(doc, Options{})
	require.NoError(t, err)
	page := string(site.Files["graph.html"])
	checkHTML(t, "graph.html", page)
	return page
}

func TestGraph_LayersRolesAndLinks(t *testing.T) {
	// Arrange: reviewer -> deploy -> lint, reviewer -> lint; role dev keeps reviewer
	doc := graphDoc([]govview.CatalogItemV2{
		graphItem("agent", "reviewer", "dev"), graphItem("skill", "deploy"), graphItem("skill", "lint"), graphItem("rule", "unrelated"),
	}, use("agent/-/reviewer", "skill/-/deploy"), use("agent/-/reviewer", "skill/-/lint"), use("skill/-/deploy", "skill/-/lint"))

	// Act
	page := renderGraphPage(t, doc)

	// Assert
	assert.Contains(t, page, "<svg")
	assert.Contains(t, page, `href="items/agent/-/reviewer.html"`)
	assert.Contains(t, page, `href="roles/dev.html"`)
	assert.NotContains(t, page, "unrelated", "an item with no dependency is not part of the graph")
	// x positions: role at column 0, reviewer 1, deploy 2, lint 3
	xs := map[string]int{}
	for _, m := range regexp.MustCompile(`<title>([^<]*)</title><rect x="(\d+)"`).FindAllStringSubmatch(page, -1) {
		x, err := strconv.Atoi(m[2])
		require.NoError(t, err)
		xs[m[1]] = x
	}
	assert.Less(t, xs["role dev"], xs["agent/-/reviewer"])
	assert.Less(t, xs["agent/-/reviewer"], xs["skill/-/deploy"])
	assert.Less(t, xs["skill/-/deploy"], xs["skill/-/lint"], "a dependency sits right of the skill that uses it, through the longest path")
	assert.Contains(t, page, "3 items and")
	assert.Contains(t, page, "Dependencies as a table")
	assert.NotContains(t, page, "Dependency cycles")
}

func TestGraph_NoDependencies(t *testing.T) {
	// Arrange
	doc := graphDoc([]govview.CatalogItemV2{graphItem("skill", "a")})

	// Act
	page := renderGraphPage(t, doc)

	// Assert
	assert.Contains(t, page, "no dependencies to draw")
	assert.NotContains(t, page, "<svg")
}

func TestGraph_CyclesAreListedAndDrawnDashed(t *testing.T) {
	// Arrange
	doc := graphDoc([]govview.CatalogItemV2{graphItem("skill", "a"), graphItem("skill", "b")},
		use("skill/-/a", "skill/-/b"), use("skill/-/b", "skill/-/a"))

	// Act
	page := renderGraphPage(t, doc)

	// Assert
	assert.Contains(t, page, "Dependency cycles")
	assert.Equal(t, 1, strings.Count(page, `class="edge edge-cycle"`), "exactly the edge that closes the loop")
	assert.Equal(t, 1, strings.Count(page, `class="edge edge-uses"`))
}

func TestGraph_TooLargeGraphIsATableOnly(t *testing.T) {
	// Arrange
	items := []govview.CatalogItemV2{graphItem("agent", "root")}
	var edges []govview.CatalogEdge
	for i := range maxGraphNodes {
		id := "s" + strconv.Itoa(i)
		items = append(items, graphItem("skill", id))
		edges = append(edges, use("agent/-/root", "skill/-/"+id))
	}
	doc := graphDoc(items, edges...)

	// Act
	page := renderGraphPage(t, doc)

	// Assert
	assert.NotContains(t, page, "<svg")
	assert.Contains(t, page, "too many to draw")
	assert.Equal(t, maxGraphNodes, strings.Count(page, "<tr><th scope=\"row\""))
}

func TestGraph_ManyRoleEdgesAreLeftOut(t *testing.T) {
	// Arrange: 20 roles keep 20 dependent items: 400 role edges
	var roleNames []string
	for i := range 20 {
		roleNames = append(roleNames, "r"+strconv.Itoa(i))
	}
	items := []govview.CatalogItemV2{graphItem("agent", "root", roleNames...)}
	var edges []govview.CatalogEdge
	for i := range 20 {
		id := "s" + strconv.Itoa(i)
		items = append(items, graphItem("skill", id, roleNames...))
		edges = append(edges, use("agent/-/root", "skill/-/"+id))
	}
	doc := graphDoc(items, edges...)

	// Act
	page := renderGraphPage(t, doc)

	// Assert
	assert.Contains(t, page, "Role edges are not drawn")
	assert.NotContains(t, page, "edge-role")
	assert.Contains(t, page, "<svg")
}

func TestGraph_HostileTextIsEscaped(t *testing.T) {
	// Arrange
	doc := hostileDoc()
	doc.Edges = []govview.CatalogEdge{use(doc.Items[0].Ref, doc.Items[1].Ref), use(doc.Items[1].Ref, doc.Items[2].Ref)}

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	page := string(site.Files["graph.html"])
	checkHTML(t, "graph.html", page)
	assert.Contains(t, page, "<svg")
	for _, h := range hostile[:7] {
		if strings.ContainsAny(h, `<>"'`) {
			assert.NotContains(t, page, h)
		}
	}
	assert.NotContains(t, page, "style=")
}

func TestGraph_IsDeterministic(t *testing.T) {
	// Arrange
	mk := func() *govview.CatalogDocV2 {
		return graphDoc([]govview.CatalogItemV2{graphItem("agent", "b", "dev"), graphItem("agent", "a", "dev"), graphItem("skill", "s")},
			use("agent/-/b", "skill/-/s"), use("agent/-/a", "skill/-/s"))
	}

	// Act
	first, second := renderGraphPage(t, mk()), renderGraphPage(t, mk())

	// Assert
	assert.Equal(t, first, second)
}
