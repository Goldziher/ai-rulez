package sbom

import (
	"crypto/sha1" //nolint:gosec // UUIDv5 is defined over SHA-1; it is an identifier, not a security hash
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const projectRef = "ai-rulez:project"

// serialNamespace is the UUIDv5 namespace of ai-rulez SBOM serial numbers:
// UUIDv5(DNS namespace, "sbom.ai-rulez").
var serialNamespace = uuidV5Bytes([16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}, "sbom.ai-rulez")

// Build assembles the document of cfg. toolVersion is the running ai-rulez
// release. Nothing is rendered or written.
func Build(cfg *config.Config, toolVersion string, opts Options) (*BOM, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	sc, err := newScope(cfg, opts)
	if err != nil {
		return nil, err
	}
	snap, err := govview.Snapshot(cfg, "", true, toolVersion)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	now := opts.Now // zero judges no approval expired; the caller supplies the clock
	tree := treeOf(snap, lock)
	b := &builder{
		cfg: cfg, opts: opts, scope: sc, lock: lock, idx: newContentIndex(cfg),
		approvals: approvalsFor(cfg, lock, snap, tree, opts, now),
	}

	kept := sc.filter(snap.Items)
	components := b.itemComponents(kept)
	components = append(components, b.sourceComponents()...)
	servers, services := b.mcpEntries()
	components = append(components, servers...)
	outputs, err := b.outputComponents()
	if err != nil {
		return nil, err
	}
	components = append(components, outputs...)
	sort.SliceStable(components, func(i, j int) bool { return components[i].BOMRef < components[j].BOMRef })
	sort.SliceStable(services, func(i, j int) bool { return services[i].BOMRef < services[j].BOMRef })
	if err := uniqueRefs(components, services); err != nil {
		return nil, err
	}

	refs := make([]string, 0, len(components)+len(services))
	for i := range components {
		refs = append(refs, components[i].BOMRef)
	}
	for i := range services {
		refs = append(refs, services[i].BOMRef)
	}
	sort.Strings(refs)
	bom := &BOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  SpecVersion,
		SerialNumber: serialNumber(tree, refs, opts.scopeKey()),
		Version:      1,
		Metadata: Metadata{
			Tools:     Tools{Components: []Component{{Type: "application", Name: "ai-rulez", Version: toolVersion}}},
			Component: b.projectComponent(snap, tree),
		},
		Components:   components,
		Services:     services,
		Dependencies: b.dependencies(components, services, kept),
		Findings:     b.findings,
		LockPresent:  lock.HasContentPins(),
		tool:         toolVersion,
		tree:         tree,
		scope:        opts.scopeKey(),
	}
	if lock.HasContentPins() {
		bom.LockInSync = contentlock.Compare(lock, snap).InSync
	}
	if !opts.Timestamp.IsZero() {
		bom.created = opts.Timestamp.UTC().Format(time.RFC3339)
		bom.Metadata.Timestamp = bom.created
	}
	sortFindings(bom.Findings)
	return bom, nil
}

// serialNumber is the UUIDv5 URN over the lock tree, the component references and
// the slice options.
func serialNumber(tree string, refs []string, scopeKey string) string {
	name := tree + "\n" + strings.Join(refs, "\n")
	if scopeKey != "" {
		name += "\n" + scopeKey
	}
	return "urn:uuid:" + formatUUID(uuidV5Bytes(serialNamespace, name))
}

func approvalsFor(cfg *config.Config, lock *lockfile.File, snap *contentlock.Snapshot, tree string, opts Options, now time.Time) *approvalIndex {
	if opts.NoApprovals {
		return nil
	}
	return newApprovalIndex(cfg, lock, snap.Items, tree, opts.RedactReviewers, opts.RedactKey, now)
}

// builder holds what the component constructors share.
type builder struct {
	cfg       *config.Config
	opts      Options
	scope     *scope
	lock      *lockfile.File
	idx       *contentIndex
	approvals *approvalIndex
	findings  []Finding
}

func (b *builder) find(code, subject, format string, args ...any) {
	b.findings = append(b.findings, Finding{Code: code, Subject: subject, Message: fmt.Sprintf(format, args...)})
}

// treeOf is the digest the serial number is derived from: the tree of the lock
// when it pins content, otherwise the tree computed from the working copy and the
// remote pins the lock does hold.
func treeOf(snap *contentlock.Snapshot, lock *lockfile.File) string {
	if lock.HasContentPins() && lock.Tree != "" {
		return lock.Tree
	}
	computed := &lockfile.File{Item: snap.Items}
	if lock != nil {
		computed.Include, computed.Skill, computed.Source, computed.Served = lock.Include, lock.Skill, lock.Source, lock.Served
	}
	return contentlock.TreeOf(computed)
}

func (b *builder) projectComponent(snap *contentlock.Snapshot, tree string) Component {
	name := b.cfg.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(b.cfg.ConfigDir))
	}
	props := []Property{prop("tree", tree), prop("lock", "absent")}
	if b.lock != nil {
		props[1] = prop("lock", "present")
		if b.lock.HasContentPins() {
			inSync := contentlock.Compare(b.lock, snap).InSync
			props = append(props, prop("lock-in-sync", strconv.FormatBool(inSync)))
		}
	}
	if scopeName := b.cfg.LockScope(); scopeName != config.LockScopeAll {
		props = append(props, prop("lock-scope", scopeName))
	}
	if f := b.opts.files(); f != FilesNone {
		props = append(props, prop("files", f))
	}
	if b.opts.Profile != "" {
		props = append(props, prop("profile", b.opts.Profile))
	}
	if b.opts.Role != "" {
		props = append(props, prop("role", b.opts.Role))
	}
	if b.opts.IncludeOutputs {
		props = append(props, prop("outputs", "included"))
	}
	comp := Component{Type: "application", BOMRef: projectRef, Name: name}
	if sig := b.opts.Signature; sig != nil {
		props = append(props, prop("signature", sig.Status))
		if sig.Signer != "" {
			props = append(props, prop("signer", sig.Signer))
		}
		if sig.Issuer != "" {
			props = append(props, prop("signer-issuer", sig.Issuer))
		}
		if !sig.SignedAt.IsZero() {
			props = append(props, prop("signed-at", sig.SignedAt.UTC().Format(time.RFC3339)))
		}
		if sig.Code != "" {
			props = append(props, prop("signature-code", sig.Code))
		}
		comp.info = &info{signature: sig}
	}
	comp.Properties = sortProps(props)
	return comp
}

func prop(name, value string) Property { return Property{Name: PropertyPrefix + name, Value: value} }

func propOf(c *Component, name string) string {
	for _, p := range c.Properties {
		if p.Name == PropertyPrefix+name {
			return p.Value
		}
	}
	return ""
}

func sortProps(p []Property) []Property {
	sort.SliceStable(p, func(i, j int) bool { return p[i].Name < p[j].Name })
	return p
}

func itemRef(kind, domain, id string) string {
	return "ai-rulez:item:" + kind + ":" + domain + ":" + id
}

// itemComponents turns the lock's item digests (computed from the sources, with
// CRLF normalised) into components.
func (b *builder) itemComponents(items []lockfile.Item) []Component {
	out := make([]Component, 0, len(items))
	for i := range items {
		it := &items[i]
		if it.Kind == contentlock.KindSettings && it.ID == "mcp-servers" {
			continue // the MCP servers are listed below; a digest of their settings would also cover env and header values
		}
		props := []Property{prop("kind", it.Kind), prop("digest", it.Digest)}
		if it.Domain != "" {
			props = append(props, prop("domain", it.Domain))
		}
		if it.Path != "" {
			props = append(props, prop("path", it.Path))
		}
		if it.Owner != "" {
			props = append(props, prop("owner", it.Owner))
		}
		comp := Component{
			Type:    "data",
			BOMRef:  itemRef(it.Kind, it.Domain, it.ID),
			Group:   it.Kind,
			Name:    it.ID,
			Version: it.Version,
		}
		b.describeItem(&comp, it)
		comp.Properties = sortProps(props)
		b.approvals.annotate(&comp, it.Kind, it.Domain, it.ID)
		out = append(out, comp)
	}
	return out
}

// describeItem adds what the item's own file says (description, license) and its
// files. The text comes from untrusted frontmatter: it is length-capped and
// stripped of control and bidirectional characters.
func (b *builder) describeItem(comp *Component, it *lockfile.Item) {
	cf := b.idx.find(it)
	if cf == nil {
		return
	}
	inf := &info{}
	comp.info = inf
	if cf.Metadata != nil {
		comp.Description = sanitizeText(cf.Metadata.Extra["description"], maxDescriptionLen)
		if license := cf.Metadata.Extra["license"]; license != "" {
			comp.Licenses = licenseChoices(license)
			inf.declaredLicense = license
		}
	}
	files := b.opts.files()
	switch {
	case files == FilesAll, files == FilesSkills && it.Kind == contentlock.KindSkill:
		inf.files = b.idx.entries(cf, false)
	case it.Kind == contentlock.KindSkill:
		inf.files = b.idx.entries(cf, true) // the scripts a skill runs are always visible
	}
	comp.Components = fileComponents(comp.BOMRef, inf.files)
}

const maxDescriptionLen = 1024

// sourceComponents lists the remote includes, installed skills and skill sources
// of the configuration, with the commit and digest the lock pinned for each.
func (b *builder) sourceComponents() []Component {
	var out []Component
	add := func(kind, approvalKind, name, source, ref, subpath string) {
		var pin *lockfile.Entry
		if b.lock != nil {
			pin = b.lock.Find(kind, name)
		}
		comp := sourceComponent(kind, name, source, ref, subpath, pin)
		if propOf(&comp, "pinned") == "false" {
			b.find(CodeUnpinned, kind+" "+name, "the source is not pinned to a commit (ref %q); run `ai-rulez lock`", ref)
		}
		b.approvals.annotate(&comp, approvalKind, "", name)
		out = append(out, comp)
	}
	for i := range b.cfg.Includes {
		in := &b.cfg.Includes[i]
		add(lockfile.KindInclude, approval.KindInclude, in.Name, in.Source, in.RequestedRef(), in.Path)
	}
	for i := range b.cfg.InstalledSkills {
		sk := &b.cfg.InstalledSkills[i]
		if !b.scope.keepScoped(sk.Profiles) {
			continue
		}
		add(lockfile.KindSkill, approval.KindInstalledSkill, sk.Name, sk.Source, sk.RequestedRef(), sk.Path)
	}
	for i := range b.cfg.SkillSources {
		ss := &b.cfg.SkillSources[i]
		add(lockfile.KindSource, approval.KindSource, ss.Name, ss.URL, ss.RequestedRef(), ss.Path)
	}
	return out
}

func sourceComponent(kind, name, source, ref, subpath string, pin *lockfile.Entry) Component {
	props := []Property{prop("kind", "source"), prop("source-kind", kind)}
	version := ref
	commit := ""
	if ref != "" {
		props = append(props, prop("ref", ref))
	}
	if pin != nil {
		if pin.Commit != "" {
			commit = pin.Commit
			version = pin.Commit
			props = append(props, prop("commit", pin.Commit))
		}
		if pin.Digest != "" {
			props = append(props, prop("digest", pin.Digest))
		}
	}
	comp := Component{Type: "data", BOMRef: "ai-rulez:source:" + kind + ":" + name, Name: name, Version: version}
	if loc, ok := parseGitSource(source); ok {
		comp.PURL = sourcePURL(loc, name, version, subpath)
		comp.ExternalReferences = []ExternalReference{{Type: "vcs", URL: loc.HTTPS()}}
		props = append(props, prop("source-location", "git"))
		pinned := commit != "" || gitutil.IsCommitSHA(ref)
		props = append(props, prop("pinned", strconv.FormatBool(pinned)))
		if commit == "" && pinned {
			commit = ref
		}
		comp.info = &info{commit: commit, vcs: loc.HTTPS()}
	} else {
		props = append(props, prop("source-location", "local"))
	}
	if subpath != "" && !strings.Contains(subpath, "..") && !filepath.IsAbs(subpath) {
		props = append(props, prop("path", filepath.ToSlash(subpath)))
	}
	comp.Properties = sortProps(props)
	return comp
}

// mcpEntries splits the MCP servers into local ones (components, with a purl
// from the declared package or a heuristic over the launcher) and remote ones
// (services). Env and header values are never read; only the key names are listed.
func (b *builder) mcpEntries() ([]Component, []Service) {
	var comps []Component
	var services []Service
	for _, s := range b.cfg.EffectiveMCPServers() {
		if !b.scope.keepScoped(s.Profiles) {
			continue
		}
		props := []Property{prop("kind", "mcp-server"), prop("enabled", strconv.FormatBool(s.IsEnabled()))}
		if s.Transport != "" {
			props = append(props, prop("mcp-transport", s.Transport))
		}
		if len(s.Profiles) > 0 {
			props = append(props, prop("profiles", strings.Join(sortedCopy(s.Profiles), ",")))
		}
		if s.URL != "" {
			if len(s.Env) > 0 {
				props = append(props, prop("env-keys", strings.Join(slices.Sorted(maps.Keys(s.Env)), ",")))
			}
			if len(s.Headers) > 0 {
				props = append(props, prop("header-keys", strings.Join(slices.Sorted(maps.Keys(s.Headers)), ",")))
			}
			svc := Service{BOMRef: "ai-rulez:mcp:" + s.Name, Name: s.Name, Authenticated: len(s.Headers) > 0, TrustBoundary: true}
			if endpoint, ok := redactEndpoint(s.URL); ok {
				svc.Endpoints = []string{endpoint}
			}
			svc.Properties = sortProps(props)
			services = append(services, svc)
			continue
		}
		comp := Component{Type: "application", BOMRef: "ai-rulez:mcp:" + s.Name, Name: s.Name}
		if s.Command != "" {
			props = append(props, prop("mcp-command", filepath.Base(strings.ReplaceAll(s.Command, `\`, "/"))))
		}
		if len(s.Env) > 0 {
			props = append(props, prop("env-keys", strings.Join(slices.Sorted(maps.Keys(s.Env)), ",")))
		}
		props = append(props, b.coordinates(&comp, &s)...)
		comp.Properties = sortProps(props)
		comps = append(comps, comp)
	}
	return comps, services
}

// coordinates sets the purl and version of an MCP server component from its
// declared package or, failing that, from the launcher it uses, and returns the
// properties that say where they came from and whether they pin a release.
func (b *builder) coordinates(comp *Component, s *config.MCPServer) []Property {
	subject := "mcp-server " + s.Name
	var props []Property
	if s.Package != "" {
		typ, version, ok := purlParts(s.Package)
		if ok && !credentialsInPURL(s.Package) {
			comp.PURL = s.Package
			if v, err := url.PathUnescape(version); err == nil {
				comp.Version = v
			}
			pinned := version != "" && exactVersion(typ, version)
			props = append(props, prop("purl-source", "declared"), prop("pinned", strconv.FormatBool(pinned)))
			if !pinned {
				b.find(CodeUnpinned, subject, "the declared package %s does not name one release", redactPURL(s.Package))
			}
			return props
		}
		b.find(CodeUnknownCoords, subject, "the declared package is not a valid package URL without credentials; it is ignored")
	}
	l, ok := detectLauncher(s.Command, s.Args)
	if !ok {
		b.find(CodeUnknownCoords, subject, "no package URL can be derived from the command; declare `package` on the server")
		return props
	}
	comp.PURL, comp.Version = l.purl, l.version
	props = append(props, prop("purl-source", "heuristic"), prop("pinned", strconv.FormatBool(l.pinned)))
	if !l.pinned {
		if l.requested != "" {
			props = append(props, prop("requested-version", l.requested))
		}
		b.find(CodeUnpinned, subject, "%s", unpinnedReason(l))
	}
	return props
}

func unpinnedReason(l launcher) string {
	if l.requested == "" {
		return "the package has no version; pin it (for example name@1.2.3)"
	}
	return fmt.Sprintf("the requested version %q does not name one release", l.requested)
}

// redactPURL drops the qualifiers of a purl for a message.
func redactPURL(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		return p[:i]
	}
	return p
}

// outputComponents lists the generated files with their output digest (the
// lock's digest of the header-free rendering) when IncludeOutputs is set.
func (b *builder) outputComponents() ([]Component, error) {
	if !b.opts.IncludeOutputs {
		return nil, nil
	}
	var outs []contentlock.Output
	var err error
	if b.opts.Role != "" {
		outs, err = generator.LockRoleOutputs(b.cfg, b.opts.Role)
	} else {
		outs, err = generator.NewGenerator(b.cfg).LockOutputs(b.opts.Profile)
	}
	if err != nil {
		return nil, oops.Wrapf(err, "render the outputs to list")
	}
	comps := make([]Component, 0, len(outs))
	for _, out := range outs {
		digest, err := contentlock.TreeDigest("output", []contentlock.Leaf{{Path: out.Path, Mode: contentlock.ModeFor(out.Mode), Data: out.Data}})
		if err != nil {
			return nil, oops.Wrap(err)
		}
		comps = append(comps, Component{
			Type: componentFile, BOMRef: "ai-rulez:output:" + out.Path, Name: out.Path,
			Properties: sortProps([]Property{prop("kind", "output"), prop("output-digest", digest), prop("path", out.Path)}),
		})
	}
	return comps, nil
}

// dependencies builds the dependency graph: the project depends on every
// component and service; a role depends on the items it keeps; an agent or skill
// depends on the skills its `skills:` frontmatter names.
func (b *builder) dependencies(components []Component, services []Service, items []lockfile.Item) []Dependency {
	refs := make([]string, 0, len(components)+len(services))
	for i := range components {
		refs = append(refs, components[i].BOMRef)
	}
	for i := range services {
		refs = append(refs, services[i].BOMRef)
	}
	sort.Strings(refs)
	deps := []Dependency{{Ref: projectRef, DependsOn: refs}}

	present := map[string]string{}  // item key -> ref
	skills := map[string][]string{} // skill id -> refs
	for i := range items {
		it := &items[i]
		ref := itemRef(it.Kind, it.Domain, it.ID)
		if !has(refs, ref) {
			continue
		}
		if _, dup := present[itemKey(it.Kind, it.Domain, it.ID)]; !dup {
			present[itemKey(it.Kind, it.Domain, it.ID)] = ref
		}
		if it.Kind == contentlock.KindSkill {
			skills[baseID(it.ID)] = append(skills[baseID(it.ID)], ref)
		}
	}
	for i := range items {
		it := &items[i]
		ref := itemRef(it.Kind, it.Domain, it.ID)
		var on []string
		switch it.Kind {
		case contentlock.KindRole:
			on = b.roleDependencies(it.ID, present)
		case contentlock.KindSkill, contentlock.KindAgent:
			on = b.skillDependencies(it, skills)
		}
		on = uniqueSorted(on, ref)
		if len(on) > 0 && has(refs, ref) {
			deps = append(deps, Dependency{Ref: ref, DependsOn: on})
		}
	}
	sort.SliceStable(deps, func(i, j int) bool { return deps[i].Ref < deps[j].Ref })
	return deps
}

func (b *builder) roleDependencies(name string, present map[string]string) []string {
	res, err := b.cfg.ResolveRole(name)
	if err != nil {
		return nil
	}
	var on []string
	for i := range res.Items {
		if ref, ok := present[itemKey(res.Items[i].Kind, res.Items[i].Domain, res.Items[i].ID)]; ok {
			on = append(on, ref)
		}
	}
	return on
}

func (b *builder) skillDependencies(it *lockfile.Item, skills map[string][]string) []string {
	cf := b.idx.find(it)
	if cf == nil || cf.Metadata == nil {
		return nil
	}
	var on []string
	for _, name := range cf.Metadata.Skills {
		candidates := skills[strings.TrimSpace(name)]
		if len(candidates) == 0 {
			continue
		}
		pick := candidates[0]
		for _, ref := range candidates {
			if strings.HasPrefix(ref, "ai-rulez:item:skill:"+it.Domain+":") {
				pick = ref
				break
			}
		}
		on = append(on, pick)
	}
	return on
}

func has(sorted []string, s string) bool {
	i := sort.SearchStrings(sorted, s)
	return i < len(sorted) && sorted[i] == s
}

func uniqueSorted(in []string, drop string) []string {
	seen := map[string]bool{drop: true}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func uniqueRefs(components []Component, services []Service) error {
	seen := map[string]bool{projectRef: true}
	check := func(ref string) error {
		if seen[ref] {
			return oops.Errorf("duplicate bom-ref %q: two entries share a kind, domain and id", ref)
		}
		seen[ref] = true
		return nil
	}
	for i := range components {
		if err := check(components[i].BOMRef); err != nil {
			return err
		}
		for j := range components[i].Components {
			if err := check(components[i].Components[j].BOMRef); err != nil {
				return err
			}
		}
	}
	for i := range services {
		if err := check(services[i].BOMRef); err != nil {
			return err
		}
	}
	return nil
}

// uuidV5Bytes returns the RFC 4122 name-based (SHA-1) UUID of name in namespace.
func uuidV5Bytes(namespace [16]byte, name string) [16]byte {
	h := sha1.New() //nolint:gosec // see import
	h.Write(namespace[:])
	h.Write([]byte(name))
	var u [16]byte
	copy(u[:], h.Sum(nil)[:16])
	u[6] = (u[6] & 0x0f) | 0x50
	u[8] = (u[8] & 0x3f) | 0x80
	return u
}

func formatUUID(u [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}
