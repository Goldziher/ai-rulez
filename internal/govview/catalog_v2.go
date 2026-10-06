package govview

import (
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// CatalogSchemaVersionV2 versions the JSON of `ai-rulez catalog --format json
// --schema-version 2`, the document the static site renders.
const CatalogSchemaVersionV2 = 2

// ExcerptLimit is how much of an item body the catalog carries (bytes, cut at a
// rune boundary).
const ExcerptLimit = 2048

// Lint status values of an item.
const (
	LintOK    = "ok"
	LintWarn  = "warn"
	LintError = "error"
)

// CatalogSource says where an item's content comes from.
type CatalogSource struct {
	// Type is local or include.
	Type string `json:"type"`
}

// LoadCost splits what an item costs the agent's context: what the harness
// lists always (name and description), what loads when it is used, and the
// bundled resources a skill may pull in.
type LoadCost struct {
	ListingTokens  int `json:"listing_tokens"`
	BodyTokens     int `json:"body_tokens"`
	ResourceTokens int `json:"resource_tokens"`
	Resources      int `json:"resources"`
}

// LintCounts counts findings by severity.
type LintCounts struct {
	Error   int `json:"error"`
	Warning int `json:"warning"`
	Info    int `json:"info"`
}

// LintFinding is one finding attributed to an item.
type LintFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
}

// ItemLint is the lint result of one item.
type ItemLint struct {
	Status   string        `json:"status"`
	Counts   LintCounts    `json:"counts"`
	Findings []LintFinding `json:"findings"`
}

// Excerpt is the start of an item's body, plain text.
type Excerpt struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// ItemApproval is the reviewer-approval state of one item (docs/approvals.md).
type ItemApproval struct {
	// Required says [governance] require_approval selects the item.
	Required bool `json:"required"`
	// Status is approval.Status*: ok, missing, stale, expired, unauthorized,
	// insufficient, or not_required for an item that has records but needs none.
	Status string `json:"status"`
	// Reviewers are the distinct reviewers whose approval of the current digest applies.
	Reviewers []string `json:"reviewers"`
	Assurance string   `json:"assurance,omitempty"`
	Expires   string   `json:"expires,omitempty"`
}

// CatalogItemV2 is one item of the version 2 catalog.
type CatalogItemV2 struct {
	Ref         string        `json:"ref"`
	Kind        string        `json:"kind"`
	ID          string        `json:"id"`
	Domain      string        `json:"domain,omitempty"`
	Path        string        `json:"path,omitempty"`
	Description string        `json:"description,omitempty"`
	Owner       string        `json:"owner,omitempty"`
	Version     string        `json:"version,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	Delivery    string        `json:"delivery,omitempty"`
	Source      CatalogSource `json:"source"`
	Bytes       int           `json:"bytes"`
	Tokens      int           `json:"tokens"`
	Digest      string        `json:"digest,omitempty"`
	LoadCost    LoadCost      `json:"load_cost"`
	// Lint is absent when the lint engine could not run (see CatalogLint).
	Lint *ItemLint `json:"lint,omitempty"`
	// Approval is null unless [governance] requires approval of the item or the
	// lock records an approval of it (see ItemApproval).
	Approval *ItemApproval `json:"approval"`
	// Eval and Usage are set for skills when the build was given eval results or
	// a usage log (see CatalogOptions); absent otherwise, never invented.
	Eval  *ItemEval  `json:"eval,omitempty"`
	Usage *ItemUsage `json:"usage,omitempty"`
	// Excerpt is absent when excerpts were switched off.
	Excerpt      *Excerpt          `json:"excerpt,omitempty"`
	Roles        []string          `json:"roles"`
	RoleDelivery map[string]string `json:"role_delivery,omitempty"`
}

// CatalogGenerator names the tool that built the document.
type CatalogGenerator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CatalogProject describes the project the catalog is of.
type CatalogProject struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	LockTree    string `json:"lock_tree,omitempty"`
}

// LintSummary counts findings over the whole project.
type LintSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// ProjectFinding is a finding that belongs to no item (configuration, hooks, ...).
type ProjectFinding struct {
	LintFinding
	// File is relative to the project root; empty when the file lies elsewhere.
	File string `json:"file,omitempty"`
}

// CatalogLint is the project-wide lint overview.
type CatalogLint struct {
	// Available is false when the lint engine could not run; Reason says so.
	Available bool           `json:"available"`
	Reason    string         `json:"reason,omitempty"`
	Summary   LintSummary    `json:"summary"`
	ByCode    map[string]int `json:"by_code"`
	// Unattributed are the findings that belong to no catalog item.
	Unattributed []ProjectFinding `json:"unattributed"`
}

// CatalogDocV2 is the version 2 catalog: a strict superset of v1 with the
// description, source, load cost, lint result and excerpt of every item.
type CatalogDocV2 struct {
	SchemaVersion int              `json:"schema_version"`
	GeneratedBy   CatalogGenerator `json:"generated_by"`
	Project       CatalogProject   `json:"project"`
	Tokenizer     string           `json:"tokenizer"`
	Items         []CatalogItemV2  `json:"items"`
	// MCPServers are the project's MCP servers: names, transports and the names
	// of their env and header entries, never their values (see CatalogMCPServer).
	MCPServers []CatalogMCPServer `json:"mcp_servers"`
	// Edges are the dependencies between items: an item that names a skill in its
	// `skills:` frontmatter uses that skill (see CatalogEdge).
	Edges []CatalogEdge `json:"edges"`
	Roles []CatalogRole `json:"roles"`
	Lock  CatalogLock   `json:"lock"`
	Lint  CatalogLint   `json:"lint"`
	Notes []string      `json:"notes"`
}

// CatalogOptions tunes BuildCatalogV2.
type CatalogOptions struct {
	// Lint is the lint report of the project; nil when lint was not run.
	Lint *lint.Report
	// LintReason explains a nil Lint (shown in the catalog).
	LintReason string
	// NoExcerpt leaves the body excerpt out.
	NoExcerpt bool
	// WithEval adds the recorded eval result of each skill from Eval. A nil Eval
	// says the results were not found (EvalNote says why).
	WithEval bool
	Eval     *evals.Store
	EvalNote string
	// WithUsage adds the use count of each skill from Usage; UsageLoaded false
	// says the log was not found (UsageNote says why).
	WithUsage   bool
	Usage       []usage.Entry
	UsageLoaded bool
	UsageNote   string
}

// BuildCatalogV2 builds the version 2 catalog. It shares every v1 field with
// BuildCatalog, so the two documents cannot disagree.
func BuildCatalogV2(cfg *config.Config, counter tokens.Counter, toolVersion string, opts CatalogOptions) (*CatalogDocV2, error) {
	v1, files, err := buildCatalogCore(cfg, counter, toolVersion)
	if err != nil {
		return nil, err
	}
	doc := &CatalogDocV2{
		SchemaVersion: CatalogSchemaVersionV2,
		GeneratedBy:   CatalogGenerator{Name: "ai-rulez", Version: toolVersion},
		Project:       CatalogProject{Name: cfg.Name, Description: cfg.Description, LockTree: v1.Lock.Tree},
		Tokenizer:     v1.Tokenizer,
		Items:         make([]CatalogItemV2, 0, len(v1.Items)),
		MCPServers:    catalogMCPServers(cfg),
		Roles:         v1.Roles,
		Lock:          v1.Lock,
		Notes:         append([]string{}, v1.notes...),
	}
	attrib := newLintAttribution(cfg, opts.Lint)
	approvals := newApprovalIndex(cfg)
	seen := map[string]int{}
	for i := range v1.Items {
		it := &v1.Items[i]
		out := CatalogItemV2{
			Ref: uniqueRef(seen, it.Kind, it.Domain, it.ID), Kind: it.Kind, ID: it.ID, Domain: it.Domain, Path: it.Path,
			Owner: it.Owner, Version: it.Version, Mode: it.Mode, Delivery: it.Delivery,
			Source: CatalogSource{Type: sourceType(it.Path, files[i])},
			Bytes:  it.Bytes, Tokens: it.Tokens, Digest: it.Digest, Roles: it.Roles, RoleDelivery: it.RoleDelivery,
			LoadCost: LoadCost{BodyTokens: it.Tokens},
			Approval: approvals.forItem(it.Kind, it.Domain, it.ID, it.Digest),
		}
		if cf := files[i]; cf != nil {
			out.Description = config.SkillDescription(cf.Metadata)
			out.LoadCost = loadCost(it, cf, out.Description, counter)
			if !opts.NoExcerpt {
				out.Excerpt = excerptOf(cf.Content)
			}
			if opts.Lint != nil {
				out.Lint = attrib.forItem(cf)
			}
		} else if opts.Lint != nil {
			out.Lint = &ItemLint{Status: LintOK, Findings: []LintFinding{}}
		}
		doc.Items = append(doc.Items, out)
	}
	var edgeNotes []string
	doc.Edges, edgeNotes = buildEdges(doc.Items, files)
	doc.Notes = append(doc.Notes, edgeNotes...)
	attachSignals(doc, &opts)
	doc.Lint = attrib.overview(opts.LintReason)
	if opts.NoExcerpt {
		doc.Notes = append(doc.Notes, "excerpts were switched off: item excerpts are omitted")
	}
	if !doc.Lint.Available {
		doc.Notes = append(doc.Notes, "lint did not run: lint fields are omitted")
	}
	return doc, nil
}

// ViewCatalogV2 narrows doc to the items kept by role (every role when empty).
func ViewCatalogV2(doc *CatalogDocV2, role string) (*CatalogDocV2, error) {
	if role == "" {
		return doc, nil
	}
	idx := slices.IndexFunc(doc.Roles, func(r CatalogRole) bool { return r.Name == role })
	if idx < 0 {
		return nil, oops.Errorf("role %q is not defined", role)
	}
	out := *doc
	out.Roles = []CatalogRole{doc.Roles[idx]}
	out.Items = []CatalogItemV2{}
	for i := range doc.Items {
		if slices.Contains(doc.Items[i].Roles, role) {
			out.Items = append(out.Items, doc.Items[i])
		}
	}
	kept := map[string]bool{}
	for i := range out.Items {
		kept[out.Items[i].Ref] = true
	}
	out.Edges = []CatalogEdge{}
	for _, e := range doc.Edges {
		if kept[e.From] && kept[e.To] {
			out.Edges = append(out.Edges, e)
		}
	}
	out.Notes = append(append([]string{}, doc.Notes...), "role-scoped catalog: only the items role "+role+" keeps")
	return &out, nil
}

// uniqueRef is the stable key kind/domain/id (domain "-" when none); a second
// item with the same key gets the lock's "#2" suffix.
func uniqueRef(seen map[string]int, kind, domain, id string) string {
	d := domain
	if d == "" {
		d = "-"
	}
	ref := kind + "/" + d + "/" + id
	seen[ref]++
	if n := seen[ref]; n > 1 {
		return ref + "#" + strconv.Itoa(n)
	}
	return ref
}

func sourceType(path string, cf *config.ContentFile) string {
	if strings.HasPrefix(path, "included/") || (cf != nil && strings.HasPrefix(filepath.ToSlash(cf.Path), "included/")) {
		return "include"
	}
	return "local"
}

// loadCost measures what an item puts into the agent's context. Only skills are
// listed by the harness (name and description); the other kinds load whole.
func loadCost(it *CatalogItem, cf *config.ContentFile, description string, counter tokens.Counter) LoadCost {
	cost := LoadCost{BodyTokens: it.Tokens, Resources: len(cf.Resources)}
	if it.Kind == config.RoleKindSkill {
		cost.ListingTokens = counter.Count(it.ID + ": " + description)
	}
	for _, res := range cf.Resources {
		if utf8.Valid(res.Content) {
			cost.ResourceTokens += counter.Count(string(res.Content))
		}
	}
	return cost
}

// excerptOf returns the start of body: frontmatter dropped, CR normalised,
// invalid UTF-8 replaced, cut at a rune boundary.
func excerptOf(body string) *Excerpt {
	body = strings.ToValidUTF8(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"), "�")
	body = stripFrontmatter(body)
	if len(body) <= ExcerptLimit {
		return &Excerpt{Text: body}
	}
	cut := ExcerptLimit
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return &Excerpt{Text: body[:cut], Truncated: true}
}

// stripFrontmatter drops a leading "---" block when a line of exactly "---"
// closes it (at a line end or the end of the text); text that only starts with
// dashes, or whose fence is never closed, is not frontmatter and is kept whole.
func stripFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	rest := body[4:]
	offset := 0
	for offset <= len(rest) {
		end := strings.IndexByte(rest[offset:], '\n')
		line := rest[offset:]
		next := len(rest)
		if end >= 0 {
			line, next = rest[offset:offset+end], offset+end+1
		}
		if strings.TrimRight(line, " \t") == "---" {
			return strings.TrimLeft(rest[min(next, len(rest)):], "\n")
		}
		if end < 0 {
			break
		}
		offset = next
	}
	return body
}

// lintAttribution maps the findings of a lint report onto catalog items by file.
type lintAttribution struct {
	cfg     *config.Config
	report  *lint.Report
	baseAbs string
	cwd     string
	// byFile holds the findings per absolute (and symlink-resolved) path.
	byFile map[string][]*lint.Finding
	used   map[*lint.Finding]bool
}

func newLintAttribution(cfg *config.Config, report *lint.Report) *lintAttribution {
	a := &lintAttribution{cfg: cfg, report: report, byFile: map[string][]*lint.Finding{}, used: map[*lint.Finding]bool{}}
	a.baseAbs, _ = filepath.Abs(cfg.BaseDir) //nolint:errcheck // display only
	a.cwd, _ = filepath.Abs(".")             //nolint:errcheck // display only
	if report == nil {
		return a
	}
	for i := range report.Findings {
		f := &report.Findings[i]
		abs := f.File
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(a.cwd, filepath.FromSlash(abs))
		}
		a.byFile[abs] = append(a.byFile[abs], f)
		if resolved := gitutil.Resolve(abs); resolved != abs {
			a.byFile[resolved] = append(a.byFile[resolved], f)
		}
	}
	return a
}

// forItem returns the findings in the item's file, or in its directory for a
// skill (scripts and references).
func (a *lintAttribution) forItem(cf *config.ContentFile) *ItemLint {
	out := &ItemLint{Status: LintOK, Findings: []LintFinding{}}
	var found []*lint.Finding
	collect := func(path string) {
		for _, key := range []string{path, gitutil.Resolve(path)} {
			for _, f := range a.byFile[key] {
				if !a.used[f] {
					a.used[f] = true
					found = append(found, f)
				}
			}
		}
	}
	collect(cf.Path)
	if filepath.Base(cf.Path) == "SKILL.md" {
		dir := filepath.Dir(cf.Path)
		for _, res := range cf.Resources {
			collect(filepath.Join(dir, filepath.FromSlash(res.RelPath)))
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Line != found[j].Line {
			return found[i].Line < found[j].Line
		}
		if found[i].Code != found[j].Code {
			return found[i].Code < found[j].Code
		}
		return found[i].Message < found[j].Message
	})
	for _, f := range found {
		lf := a.finding(f)
		out.Findings = append(out.Findings, lf)
		switch lint.Severity(lf.Severity) {
		case lint.SeverityError:
			out.Counts.Error++
		case lint.SeverityWarning:
			out.Counts.Warning++
		default:
			out.Counts.Info++
		}
	}
	switch {
	case out.Counts.Error > 0:
		out.Status = LintError
	case out.Counts.Warning > 0:
		out.Status = LintWarn
	}
	return out
}

// finding converts a finding, keeping this machine's absolute paths out of the message.
func (a *lintAttribution) finding(f *lint.Finding) LintFinding {
	msg := f.Message
	for _, root := range []string{a.baseAbs, gitutil.Resolve(a.baseAbs)} {
		if root != "" {
			// Both separator forms: a message may quote a path written with either.
			msg = strings.ReplaceAll(strings.ReplaceAll(msg, root+"/", ""), root+`\`, "")
		}
	}
	return LintFinding{Code: f.Code, Severity: string(f.Severity), Message: msg, Line: f.Line}
}

// overview counts every finding of the report and lists those no item owns. It
// must run after every item was attributed.
func (a *lintAttribution) overview(reason string) CatalogLint {
	out := CatalogLint{ByCode: map[string]int{}, Unattributed: []ProjectFinding{}}
	if a.report == nil {
		out.Reason = reason
		return out
	}
	out.Available = true
	for i := range a.report.Findings {
		f := &a.report.Findings[i]
		switch f.Severity {
		case lint.SeverityError:
			out.Summary.Errors++
		case lint.SeverityWarning:
			out.Summary.Warnings++
		default:
			out.Summary.Infos++
		}
		out.ByCode[f.Code]++
		if a.used[f] {
			continue
		}
		pf := ProjectFinding{LintFinding: a.finding(f)}
		abs := f.File
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(a.cwd, filepath.FromSlash(abs))
		}
		if rel, err := filepath.Rel(a.baseAbs, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			pf.File = filepath.ToSlash(rel)
		}
		out.Unattributed = append(out.Unattributed, pf)
	}
	sort.SliceStable(out.Unattributed, func(i, j int) bool {
		a, b := out.Unattributed[i], out.Unattributed[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return out
}

// OmitOwners returns doc without the owner of any item (catalog [catalog]
// exclude_owners, --no-owners). doc itself is not changed.
func OmitOwners(doc *CatalogDocV2) *CatalogDocV2 {
	out := *doc
	out.Items = make([]CatalogItemV2, len(doc.Items))
	copy(out.Items, doc.Items)
	for i := range out.Items {
		out.Items[i].Owner = ""
	}
	out.Notes = append(append([]string{}, doc.Notes...), "owners were switched off: item owners are omitted")
	return &out
}
