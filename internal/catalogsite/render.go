// Package catalogsite renders a version 2 catalog (govview.CatalogDocV2) as a
// static, self-contained, deterministic website that opens from file:// and
// makes no network request.
//
// Every string that comes from the repository (ids, descriptions, owners, lint
// messages, excerpts) reaches the page through html/template's contextual
// escaping; the package never builds template.HTML, template.JS or
// template.URL values (a test enforces that). Links are relative and built from
// slugged keys, never from source text.
package catalogsite

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed assets/catalog.css assets/catalog.js
var assetFS embed.FS

// MarkerFile marks a directory as one this command may write into and clean.
const MarkerFile = ".ai-rulez-catalog"

// DefaultTitle is the site title when none is given.
const DefaultTitle = "AI-Rulez catalog"

// Options tunes Render.
type Options struct {
	// Title is the site title; DefaultTitle when empty.
	Title string
	// Indexable lets search engines crawl the site (robots.txt and the noindex
	// meta are left out).
	Indexable bool
	// PageSize is how many overview rows one page shows before the rest moves to
	// the next page; DefaultPageSize when not positive.
	PageSize int
}

// DefaultPageSize is the overview page size when Options.PageSize is not set.
const DefaultPageSize = 200

// Site is a rendered site: every file by slash path, and the digest of its
// catalog.json.
type Site struct {
	Files  map[string][]byte
	Digest string
}

// Paths returns the file paths in sorted order.
func (s *Site) Paths() []string {
	paths := make([]string, 0, len(s.Files))
	for p := range s.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

type linkRef struct{ Name, Href string }

type row struct {
	Kind, ID, Domain, Owner, Version string
	Listing                          int
	Status, StatusLabel              string
	Roles                            int
	DigestShort                      string
	Approval, ApprovalFull           string
	EvalCell, UsageCell              string
	Href                             string
	Search                           string
	Hidden                           bool
}

type findingRow struct {
	Code, Anchor, Severity, Message string
	Line                            int
}

type kv struct{ Key, Value string }

type itemPage struct {
	row
	Ref, Description, Path, Source, Delivery, Mode, Digest string
	Bytes                                                  int
	Cost                                                   govview.LoadCost
	HasLint                                                bool
	Counts                                                 govview.LintCounts
	Findings                                               []findingRow
	RoleLinks                                              []linkRef
	RoleDelivery                                           []kv
	HasExcerpt, ExcerptTruncated                           bool
	Excerpt                                                string
	Eval                                                   []kv
	Usage                                                  []kv
}

type rolePage struct {
	Name, Description, Extends string
	Domains                    []string
	Delivery                   []kv
	Groups                     []string
	Totals                     govview.CatalogRole
	Kinds                      []kv
	Items                      []linkRef
}

type lintEntry struct {
	Subject, Href, Severity, Message, File string
	Line                                   int
}

type lintGroup struct {
	Code, Anchor string
	Entries      []lintEntry
}

type mcpRow struct {
	Name, Anchor, Transport, Command, Profiles, Pinned, Env, Headers, Description string
	Enabled, Hidden                                                               bool
	Warnings                                                                      []string
}

type page struct {
	Title, Root, SiteTitle, Version, Digest, DigestShort, Current string
	Indexable                                                     bool
	Doc                                                           *govview.CatalogDocV2
	Rows                                                          []row
	RowGroups                                                     [][]row
	Kinds                                                         []string
	Item                                                          *itemPage
	Role                                                          *rolePage
	RoleLinks                                                     []linkRef
	Groups                                                        []lintGroup
	MCP                                                           []mcpRow
	LockInSync                                                    string
	SchemaVersion                                                 int
	HasEval, HasUsage                                             bool
}

var funcs = template.FuncMap{"s": display}

// Render renders doc as a static site. The result depends on doc and opts only.
func Render(doc *govview.CatalogDocV2, opts Options) (*Site, error) {
	if doc == nil {
		return nil, oops.Errorf("no catalog to render")
	}
	if opts.Title == "" {
		opts.Title = DefaultTitle
	}
	tmpl, err := template.New("site").Funcs(funcs).ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, oops.Wrapf(err, "parse catalog templates")
	}
	catalogJSON, err := encodeCatalog(doc)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(catalogJSON)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	site := &Site{Files: map[string][]byte{"catalog.json": catalogJSON}, Digest: digest}
	if err := addAssets(site); err != nil {
		return nil, err
	}
	if !opts.Indexable {
		site.Files["robots.txt"] = []byte("User-agent: *\nDisallow: /\n")
	}

	b := &builder{doc: doc, opts: opts, digest: digest, tmpl: tmpl, site: site, slugs: newSlugger()}
	if err := b.render(); err != nil {
		return nil, err
	}
	return site, nil
}

func encodeCatalog(doc *govview.CatalogDocV2) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode catalog")
	}
	return append(data, '\n'), nil
}

func addAssets(site *Site) error {
	for _, name := range []string{"catalog.css", "catalog.js"} {
		data, err := fs.ReadFile(assetFS, "assets/"+name)
		if err != nil {
			return oops.Wrapf(err, "read embedded asset %s", name)
		}
		site.Files["assets/"+name] = data
	}
	return nil
}

type builder struct {
	doc    *govview.CatalogDocV2
	opts   Options
	digest string
	tmpl   *template.Template
	site   *Site
	slugs  *slugger

	itemHref map[int]string
	roleHref map[string]string
}

func (b *builder) base(title, root, current string) page {
	return page{Title: title, Root: root, SiteTitle: b.opts.Title, Version: b.doc.GeneratedBy.Version, Digest: b.digest,
		DigestShort: shortDigest(b.digest), Current: current, Indexable: b.opts.Indexable, Doc: b.doc,
		SchemaVersion: b.doc.SchemaVersion}
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}

// depth returns the relative prefix back to the site root for a page path.
func depth(path string) string {
	return strings.Repeat("../", strings.Count(path, "/"))
}

func (b *builder) emit(path, name string, data page) error {
	var buf bytes.Buffer
	if err := b.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return oops.Wrapf(err, "render %s", path)
	}
	b.site.Files[path] = buf.Bytes()
	return nil
}

func (b *builder) render() error {
	b.assignPaths()
	steps := []func() error{b.renderIndex, b.renderItems, b.renderRoles, b.renderLock, b.renderMCP, b.renderLint, b.renderAbout}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// assignPaths names every page up front, in document order, so links resolve
// and collisions are settled the same way on every run.
func (b *builder) assignPaths() {
	b.itemHref, b.roleHref = map[int]string{}, map[string]string{}
	for i := range b.doc.Items {
		it := &b.doc.Items[i]
		domain := it.Domain
		if domain == "" {
			domain = "-"
		}
		b.itemHref[i] = b.slugs.path("items/"+segment(it.Kind)+"/", []string{domain, it.ID}, it.Ref)
	}
	for i := range b.doc.Roles {
		name := b.doc.Roles[i].Name
		b.roleHref[name] = b.slugs.path("roles/", []string{name}, "role:"+name)
	}
}

func statusOf(l *govview.ItemLint) (class, label string) {
	if l == nil {
		return "na", "n/a"
	}
	switch l.Status {
	case govview.LintError:
		return "error", "error"
	case govview.LintWarn:
		return "warn", "warning"
	}
	return "ok", "ok"
}

// approvalLabels renders the catalog approval of an item as plain text: the
// status alone for the overview, and the status with the reviewers and expiry
// for the item page. The caller escapes it.
func approvalLabels(a *govview.ItemApproval) (short, full string) {
	if a == nil {
		return "not required", "not required"
	}
	full = a.Status
	if a.Required {
		full += " (required)"
	}
	if len(a.Reviewers) > 0 {
		full += " by " + strings.Join(a.Reviewers, ", ")
	}
	if a.Expires != "" {
		full += "; until " + a.Expires
	}
	return a.Status, full
}

func (b *builder) rowOf(i int, prefix string) row {
	it := &b.doc.Items[i]
	class, label := statusOf(it.Lint)
	short, full := approvalLabels(it.Approval)
	r := row{Approval: short, ApprovalFull: full, Kind: it.Kind, ID: it.ID, Domain: it.Domain, Owner: it.Owner, Version: it.Version,
		Listing: it.LoadCost.ListingTokens, Status: class, StatusLabel: label, Roles: len(it.Roles),
		DigestShort: shortDigest(it.Digest), Href: prefix + b.itemHref[i],
		Search: strings.ToLower(display(strings.Join([]string{it.Kind, it.Domain, it.ID, it.Owner, it.Description}, " ")))}
	r.EvalCell, r.UsageCell = "-", "-"
	if e := it.Eval; e != nil {
		r.EvalCell = percent(e.PassRate)
		if e.Stale {
			r.EvalCell += " (stale)"
		}
	}
	if u := it.Usage; u != nil {
		r.UsageCell = strconv.Itoa(u.Invocations)
	}
	r.Hidden = hasHidden(it.ID) || hasHidden(it.Domain) || hasHidden(it.Owner) || hasHidden(it.Version) ||
		hasHidden(it.Description) || (it.Excerpt != nil && hasHidden(it.Excerpt.Text))
	return r
}

func (b *builder) renderIndex() error {
	p := b.base("Overview", "", "index")
	kinds := map[string]bool{}
	for i := range b.doc.Items {
		p.Rows = append(p.Rows, b.rowOf(i, ""))
		kinds[b.doc.Items[i].Kind] = true
	}
	size := b.opts.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	for start := 0; start < len(p.Rows); start += size {
		p.RowGroups = append(p.RowGroups, p.Rows[start:min(start+size, len(p.Rows))])
	}
	for k := range kinds {
		p.Kinds = append(p.Kinds, k)
	}
	for i := range b.doc.Items {
		p.HasEval = p.HasEval || b.doc.Items[i].Eval != nil
		p.HasUsage = p.HasUsage || b.doc.Items[i].Usage != nil
	}
	sort.Strings(p.Kinds)
	for i := range b.doc.Roles {
		p.RoleLinks = append(p.RoleLinks, linkRef{Name: b.doc.Roles[i].Name, Href: b.roleHref[b.doc.Roles[i].Name]})
	}
	return b.emit("index.html", "index", p)
}

func (b *builder) renderItems() error {
	for i := range b.doc.Items {
		it := &b.doc.Items[i]
		path := b.itemHref[i]
		root := depth(path)
		ip := &itemPage{row: b.rowOf(i, root), Ref: it.Ref, Description: it.Description, Path: it.Path,
			Source: it.Source.Type, Bytes: it.Bytes, Delivery: it.Delivery, Mode: it.Mode, Digest: it.Digest, Cost: it.LoadCost}
		if it.Lint != nil {
			ip.HasLint, ip.Counts = true, it.Lint.Counts
			for _, f := range it.Lint.Findings {
				ip.Findings = append(ip.Findings, findingRow{Code: f.Code, Anchor: root + "lint.html#" + lintAnchor(f.Code),
					Severity: f.Severity, Message: f.Message, Line: f.Line})
				ip.Hidden = ip.Hidden || hasHidden(f.Message)
			}
		}
		for _, r := range it.Roles {
			if href, ok := b.roleHref[r]; ok {
				ip.RoleLinks = append(ip.RoleLinks, linkRef{Name: r, Href: root + href})
			} else {
				ip.RoleLinks = append(ip.RoleLinks, linkRef{Name: r})
			}
		}
		ip.RoleDelivery = sortedKV(it.RoleDelivery)
		if it.Excerpt != nil {
			ip.HasExcerpt, ip.ExcerptTruncated, ip.Excerpt = true, it.Excerpt.Truncated, it.Excerpt.Text
		}
		ip.Eval, ip.Usage = evalRows(it.Eval), usageRows(it.Usage)
		p := b.base(it.ID, root, "items")
		p.Item = ip
		if err := b.emit(path, "item", p); err != nil {
			return err
		}
	}
	return nil
}

func sortedKV(m map[string]string) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (b *builder) renderRoles() error {
	for i := range b.doc.Roles {
		role := &b.doc.Roles[i]
		path := b.roleHref[role.Name]
		root := depth(path)
		rp := &rolePage{Name: role.Name, Description: role.Description, Extends: role.Extends, Domains: role.Domains,
			Delivery: sortedKV(role.Delivery), Totals: *role}
		if role.Match != nil {
			rp.Groups = role.Match.Groups
		}
		byKind := map[string]string{}
		for k, n := range role.Totals.ByKind {
			byKind[k] = strconv.Itoa(n)
		}
		rp.Kinds = sortedKV(byKind)
		for j := range b.doc.Items {
			for _, r := range b.doc.Items[j].Roles {
				if r == role.Name {
					rp.Items = append(rp.Items, linkRef{Name: b.doc.Items[j].Kind + "/" + b.doc.Items[j].ID, Href: root + b.itemHref[j]})
				}
			}
		}
		p := b.base("Role "+role.Name, root, "roles")
		p.Role = rp
		if err := b.emit(path, "role", p); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) renderLock() error {
	p := b.base("Lock", "", "lock")
	switch {
	case !b.doc.Lock.Present:
		p.LockInSync = "no lock file"
	case b.doc.Lock.SourcesInSync == nil:
		p.LockInSync = "the lock has no content pins to compare with"
	case *b.doc.Lock.SourcesInSync:
		p.LockInSync = "authored sources match the lock"
	default:
		p.LockInSync = "authored sources differ from the lock"
	}
	return b.emit("lock.html", "lock", p)
}

func lintAnchor(code string) string { return "code-" + segment(strings.ToLower(code)) }

func (b *builder) renderLint() error {
	p := b.base("Lint", "", "lint")
	groups := map[string]*lintGroup{}
	group := func(code string) *lintGroup {
		g := groups[code]
		if g == nil {
			g = &lintGroup{Code: code, Anchor: lintAnchor(code)}
			groups[code] = g
		}
		return g
	}
	for i := range b.doc.Items {
		it := &b.doc.Items[i]
		if it.Lint == nil {
			continue
		}
		for _, f := range it.Lint.Findings {
			g := group(f.Code)
			g.Entries = append(g.Entries, lintEntry{Subject: it.Kind + "/" + it.ID, Href: b.itemHref[i], Severity: f.Severity,
				Message: f.Message, Line: f.Line})
		}
	}
	for _, f := range b.doc.Lint.Unattributed {
		g := group(f.Code)
		g.Entries = append(g.Entries, lintEntry{Subject: "project", Severity: f.Severity, Message: f.Message, File: f.File, Line: f.Line})
	}
	for _, g := range groups {
		p.Groups = append(p.Groups, *g)
	}
	sort.Slice(p.Groups, func(i, j int) bool { return p.Groups[i].Code < p.Groups[j].Code })
	return b.emit("lint.html", "lint", p)
}

func (b *builder) renderAbout() error {
	p := b.base("About", "", "about")
	for i := range b.doc.Roles {
		p.RoleLinks = append(p.RoleLinks, linkRef{Name: b.doc.Roles[i].Name, Href: b.roleHref[b.doc.Roles[i].Name]})
	}
	return b.emit("about.html", "about", p)
}

// mcpNames lists the names of env or header entries: a variable that references
// another one reads "NAME (from ${REF})", a literal value reads "NAME (literal
// value)". The values themselves are not in the catalog.
func mcpNames(values []govview.MCPValue) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		switch {
		case v.Ref != "":
			parts = append(parts, v.Name+" (from ${"+v.Ref+"})")
		case v.Literal:
			parts = append(parts, v.Name+" (literal value)")
		default:
			parts = append(parts, v.Name)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func (b *builder) renderMCP() error {
	p := b.base("MCP servers", "", "mcp")
	anchors := newSlugger()
	for i := range b.doc.MCPServers {
		m := &b.doc.MCPServers[i]
		pinned := "n/a"
		if m.Pinned != nil {
			pinned = "no"
			if *m.Pinned {
				pinned = "yes"
			}
		}
		profiles := "all"
		if len(m.Profiles) > 0 {
			profiles = strings.Join(m.Profiles, ", ")
		}
		command := m.CommandBasename
		if command == "" {
			command = "-"
		}
		anchor := strings.TrimSuffix(anchors.path("", []string{m.Name}, m.Ref), ".html")
		p.MCP = append(p.MCP, mcpRow{Name: m.Name, Anchor: "mcp-" + anchor, Transport: m.Transport, Command: command,
			Profiles: profiles, Pinned: pinned, Env: mcpNames(m.Env), Headers: mcpNames(m.Headers), Description: m.Description,
			Enabled: m.Enabled, Warnings: m.Warnings,
			Hidden: hasHidden(m.Name) || hasHidden(m.Description) || hasHidden(m.CommandBasename)})
	}
	return b.emit("mcp.html", "mcp", p)
}

// percent formats a 0..1 rate with fixed precision, so output never depends on
// the Go version's float formatting.
func percent(rate float64) string { return strconv.FormatFloat(rate*100, 'f', 0, 64) + "%" }

func optPercent(rate *float64) string {
	if rate == nil {
		return "n/a"
	}
	return percent(*rate)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func evalRows(e *govview.ItemEval) []kv {
	if e == nil {
		return nil
	}
	rows := []kv{{"Pass rate", percent(e.PassRate) + " over " + strconv.Itoa(e.Cases) + " scored case(s)"},
		{"Met the pass threshold", yesNo(e.Passing)},
		{"Ablation delta", optPercent(e.AblationDelta)},
		{"Trigger precision", optPercent(e.TriggerPrecision)},
		{"Trigger recall", optPercent(e.TriggerRecall)},
		{"Stale", yesNo(e.Stale)},
		{"Verified on this machine", yesNo(e.Verified)}}
	if e.Date != "" {
		rows = append(rows, kv{"Date", e.Date})
	}
	return rows
}

func usageRows(u *govview.ItemUsage) []kv {
	if u == nil {
		return nil
	}
	rows := []kv{{"Invocations", strconv.Itoa(u.Invocations)}}
	if u.LastSeen != "" {
		rows = append(rows, kv{"Last seen", u.LastSeen})
	}
	return rows
}
