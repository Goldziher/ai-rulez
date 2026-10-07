package catalogsite

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// Geometry of the dependency graph, in SVG user units. Everything is an integer,
// so the drawing is the same bytes on every platform.
const (
	graphNodeW  = 220
	graphNodeH  = 28
	graphColGap = 90
	graphRowGap = 16
	graphMargin = 16
	graphArrow  = 8
	graphLabel  = 30
	graphTextX  = 8
	graphTextY  = 19
	graphCurve  = 60
	// maxGraphNodes bounds the drawing; a larger graph is listed as a table only.
	maxGraphNodes = 150
	// maxGraphRoleEdges bounds the role-to-item edges drawn; past it roles are left out.
	maxGraphRoleEdges = 300
)

type graphNode struct {
	Ref, Label, Full, Href, Class string
	X, Y, TX, TY, W, H            int
	col                           int
}

type graphEdge struct {
	Path, Head, Class string
}

type graphRow struct{ From, To, Kind string }

// graphView is what graph.tmpl draws: an SVG and the same edges as a table.
type graphView struct {
	Width, Height int
	// Drawn says there is a drawing; false for an empty graph or one too large.
	Drawn  bool
	Nodes  []graphNode
	Edges  []graphEdge
	Rows   []graphRow
	Cycles []string
	Notes  []string
}

// buildGraph lays out the dependencies of doc: the items that use or are used by
// another item, and the roles that keep them. Layers follow the longest path of
// `uses` edges, so a dependency always sits to the right of what depends on it;
// an edge that would close a cycle is drawn dashed and listed.
func (b *builder) buildGraph() graphView {
	var gv graphView
	itemIndex := map[string]int{}
	for i := range b.doc.Items {
		itemIndex[b.doc.Items[i].Ref] = i
	}
	edges, inGraph := b.itemEdges(itemIndex)
	for _, e := range edges {
		gv.Rows = append(gv.Rows, graphRow{From: e.From, To: e.To, Kind: e.Kind})
	}
	if len(inGraph) == 0 {
		return gv
	}
	refs := make([]string, 0, len(inGraph))
	for ref := range inGraph {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) > maxGraphNodes {
		gv.Notes = append(gv.Notes, "The graph has "+strconv.Itoa(len(refs))+" items, too many to draw (the limit is "+
			strconv.Itoa(maxGraphNodes)+"): the dependencies are listed in the table.")
		return gv
	}

	layer, cycles := graphLayers(refs, edges)
	gv.Cycles = cycles
	roleEdges := b.roleEdges(refs)
	showRoles := len(roleEdges) > 0 && len(roleEdges) <= maxGraphRoleEdges
	if len(roleEdges) > maxGraphRoleEdges {
		gv.Notes = append(gv.Notes, "Role edges are not drawn: "+strconv.Itoa(len(roleEdges))+" would be too many (the limit is "+
			strconv.Itoa(maxGraphRoleEdges)+"). Each role page lists what the role keeps.")
	}

	nodes := map[string]*graphNode{}
	shift := 0
	if showRoles {
		shift = 1
		b.addRoleNodes(nodes, roleEdges)
	}
	for _, ref := range refs {
		it := &b.doc.Items[itemIndex[ref]]
		nodes[ref] = &graphNode{Ref: ref, Label: shortLabel(it.Kind + "/" + it.ID), Full: ref, Href: b.itemHref[itemIndex[ref]],
			Class: "node-" + segment(it.Kind), col: layer[ref] + shift}
	}
	placeNodes(nodes, &gv)

	byRef := nodes
	for _, e := range edges {
		cls := "edge edge-uses"
		if isCycleEdge(gv.Cycles, e) {
			cls = "edge edge-cycle"
		}
		gv.Edges = append(gv.Edges, edgeShape(byRef[e.From], byRef[e.To], cls))
	}
	if showRoles {
		for _, re := range roleEdges {
			gv.Edges = append(gv.Edges, edgeShape(byRef["role:"+re.role], byRef[re.ref], "edge edge-role"))
			gv.Rows = append(gv.Rows, graphRow{From: "role:" + re.role, To: re.ref, Kind: "keeps"})
		}
	}
	gv.Drawn = true
	return gv
}

// itemEdges returns the catalog edges whose ends are both items, and the set of
// items they touch.
func (b *builder) itemEdges(itemIndex map[string]int) (edges []govview.CatalogEdge, inGraph map[string]bool) {
	inGraph = map[string]bool{}
	for _, e := range b.doc.Edges {
		_, okFrom := itemIndex[e.From]
		_, okTo := itemIndex[e.To]
		if okFrom && okTo {
			edges = append(edges, e)
			inGraph[e.From], inGraph[e.To] = true, true
		}
	}
	return edges, inGraph
}

// addRoleNodes adds one node in the first column for each role in roleEdges.
func (b *builder) addRoleNodes(nodes map[string]*graphNode, roleEdges []roleEdge) {
	for _, re := range roleEdges {
		if _, seen := nodes["role:"+re.role]; seen {
			continue
		}
		nodes["role:"+re.role] = &graphNode{Ref: "role:" + re.role, Label: shortLabel(re.role), Full: "role " + re.role,
			Href: b.roleHref[re.role], Class: "node-role", col: 0}
	}
}

type roleEdge struct{ role, ref string }

// roleEdges lists, in order, which roles keep which graph items.
func (b *builder) roleEdges(refs []string) []roleEdge {
	in := map[string]bool{}
	for _, r := range refs {
		in[r] = true
	}
	var out []roleEdge
	for i := range b.doc.Items {
		it := &b.doc.Items[i]
		if !in[it.Ref] {
			continue
		}
		for _, role := range it.Roles {
			if _, ok := b.roleHref[role]; ok {
				out = append(out, roleEdge{role: role, ref: it.Ref})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].role != out[j].role {
			return out[i].role < out[j].role
		}
		return out[i].ref < out[j].ref
	})
	return out
}

// graphLayers assigns each ref its longest-path layer over the `uses` edges. An
// edge into a node that is still being resolved closes a cycle: it is left out of
// the layering and reported as "from -> to".
func graphLayers(refs []string, edges []govview.CatalogEdge) (layer map[string]int, cycles []string) {
	preds := map[string][]string{}
	for _, e := range edges {
		preds[e.To] = append(preds[e.To], e.From)
	}
	for _, p := range preds {
		sort.Strings(p)
	}
	layer = map[string]int{}
	state := map[string]int{} // 0 new, 1 resolving, 2 done
	var resolve func(ref string) int
	resolve = func(ref string) int {
		switch state[ref] {
		case 2:
			return layer[ref]
		case 1:
			return -1
		}
		state[ref] = 1
		depth := 0
		for _, p := range preds[ref] {
			pl := resolve(p)
			if pl < 0 {
				cycles = append(cycles, p+" -> "+ref)
				continue
			}
			depth = max(depth, pl+1)
		}
		state[ref] = 2
		layer[ref] = depth
		return depth
	}
	for _, ref := range refs {
		resolve(ref)
	}
	sort.Strings(cycles)
	return layer, cycles
}

func isCycleEdge(cycles []string, e govview.CatalogEdge) bool {
	label := e.From + " -> " + e.To
	i := sort.SearchStrings(cycles, label)
	return i < len(cycles) && cycles[i] == label
}

// placeNodes gives every node its column and row position and sizes the canvas.
func placeNodes(nodes map[string]*graphNode, gv *graphView) {
	cols := map[int][]*graphNode{}
	maxCol, maxRows := 0, 0
	for _, n := range nodes {
		cols[n.col] = append(cols[n.col], n)
		maxCol = max(maxCol, n.col)
	}
	for c := 0; c <= maxCol; c++ {
		list := cols[c]
		sort.Slice(list, func(i, j int) bool { return list[i].Ref < list[j].Ref })
		maxRows = max(maxRows, len(list))
		for r, n := range list {
			n.W, n.H = graphNodeW, graphNodeH
			n.X = graphMargin + c*(graphNodeW+graphColGap)
			n.Y = graphMargin + r*(graphNodeH+graphRowGap)
			n.TX, n.TY = n.X+graphTextX, n.Y+graphTextY
		}
		for _, n := range list {
			gv.Nodes = append(gv.Nodes, *n)
		}
	}
	gv.Width = graphMargin*2 + (maxCol+1)*graphNodeW + maxCol*graphColGap
	gv.Height = graphMargin*2 + maxRows*graphNodeH + max(maxRows-1, 0)*graphRowGap
}

// edgeShape draws a curve from the right side of from to the left side of to,
// with an arrow head at the end.
func edgeShape(from, to *graphNode, class string) graphEdge {
	x1, y1 := from.X+from.W, from.Y+from.H/2
	x2, y2 := to.X, to.Y+to.H/2
	var path string
	if x2 > x1 {
		mid := (x1 + x2) / 2
		path = "M " + itoa(x1) + " " + itoa(y1) + " C " + itoa(mid) + " " + itoa(y1) + ", " + itoa(mid) + " " + itoa(y2) + ", " + itoa(x2) + " " + itoa(y2)
	} else {
		path = "M " + itoa(x1) + " " + itoa(y1) + " C " + itoa(x1+graphCurve) + " " + itoa(y1) + ", " + itoa(x2-graphCurve) + " " + itoa(y2) + ", " + itoa(x2) + " " + itoa(y2)
	}
	head := itoa(x2) + "," + itoa(y2) + " " + itoa(x2-graphArrow) + "," + itoa(y2-graphArrow/2) + " " + itoa(x2-graphArrow) + "," + itoa(y2+graphArrow/2)
	return graphEdge{Path: path, Head: head, Class: class}
}

func itoa(n int) string { return strconv.Itoa(n) }

// shortLabel cuts a long label for the drawing; the full text is in the node's title.
func shortLabel(s string) string {
	s = display(s)
	if utf8.RuneCountInString(s) <= graphLabel {
		return s
	}
	return string([]rune(s)[:graphLabel-1]) + "…"
}

// Summary is a one-line description of the graph for assistive technology.
func (gv *graphView) Summary() string {
	var n int
	for i := range gv.Nodes {
		if !strings.HasPrefix(gv.Nodes[i].Ref, "role:") {
			n++
		}
	}
	return strconv.Itoa(n) + " items and " + strconv.Itoa(len(gv.Edges)) + " dependencies, drawn left to right: an item sits to the left of the skills it uses."
}
