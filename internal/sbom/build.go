package sbom

import (
	"crypto/sha1" //nolint:gosec // UUIDv5 is defined over SHA-1; it is an identifier, not a security hash
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const projectRef = "ai-rulez:project"

// serialNamespace is the UUIDv5 namespace of ai-rulez SBOM serial numbers:
// UUIDv5(DNS namespace, "sbom.ai-rulez").
var serialNamespace = uuidV5Bytes([16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}, "sbom.ai-rulez")

// Build assembles the CycloneDX document of cfg. toolVersion is the running
// ai-rulez release. Nothing is rendered or written.
func Build(cfg *config.Config, toolVersion string) (*BOM, error) {
	snap, err := govview.Snapshot(cfg, "", true, toolVersion)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}

	components := itemComponents(snap.Items)
	sources := sourceComponents(cfg, lock)
	servers, services := mcpEntries(cfg)
	components = append(components, sources...)
	components = append(components, servers...)
	sort.SliceStable(components, func(i, j int) bool { return components[i].BOMRef < components[j].BOMRef })
	sort.SliceStable(services, func(i, j int) bool { return services[i].BOMRef < services[j].BOMRef })
	if err := uniqueRefs(components, services); err != nil {
		return nil, err
	}

	tree := treeOf(snap, lock)
	refs := make([]string, 0, len(components)+len(services))
	for i := range components {
		refs = append(refs, components[i].BOMRef)
	}
	for i := range services {
		refs = append(refs, services[i].BOMRef)
	}

	sort.Strings(refs)
	return &BOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  SpecVersion,
		SerialNumber: "urn:uuid:" + formatUUID(uuidV5Bytes(serialNamespace, tree+"\n"+strings.Join(refs, "\n"))),
		Version:      1,
		Metadata: Metadata{
			Tools:     Tools{Components: []Component{{Type: "application", Name: "ai-rulez", Version: toolVersion}}},
			Component: projectComponent(cfg, lock, snap, tree),
		},
		Components:   components,
		Services:     services,
		Dependencies: []Dependency{{Ref: projectRef, DependsOn: refs}},
	}, nil
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

func projectComponent(cfg *config.Config, lock *lockfile.File, snap *contentlock.Snapshot, tree string) Component {
	name := cfg.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(cfg.ConfigDir))
	}
	props := []Property{prop("tree", tree), prop("lock", "absent")}
	if lock != nil {
		props[1] = prop("lock", "present")
		if lock.HasContentPins() {
			inSync := contentlock.Compare(lock, snap).InSync
			props = append(props, prop("lock-in-sync", strconv.FormatBool(inSync)))
		}
	}
	return Component{Type: "application", BOMRef: projectRef, Name: name, Properties: sortProps(props)}
}

func prop(name, value string) Property { return Property{Name: PropertyPrefix + name, Value: value} }

func sortProps(p []Property) []Property {
	sort.SliceStable(p, func(i, j int) bool { return p[i].Name < p[j].Name })
	return p
}

// itemComponents turns the lock's item digests (computed from the sources, with
// CRLF normalised) into components.
func itemComponents(items []lockfile.Item) []Component {
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
		out = append(out, Component{
			Type:       "data",
			BOMRef:     "ai-rulez:item:" + it.Kind + ":" + it.Domain + ":" + it.ID,
			Group:      it.Kind,
			Name:       it.ID,
			Version:    it.Version,
			Properties: sortProps(props),
		})
	}
	return out
}

// sourceComponents lists the remote includes, installed skills and skill sources
// of the configuration, with the commit and digest the lock pinned for each.
func sourceComponents(cfg *config.Config, lock *lockfile.File) []Component {
	var out []Component
	add := func(kind, name, source, ref, subpath string) {
		var pin *lockfile.Entry
		if lock != nil {
			pin = lock.Find(kind, name)
		}
		out = append(out, sourceComponent(kind, name, source, ref, subpath, pin))
	}
	for i := range cfg.Includes {
		in := &cfg.Includes[i]
		add(lockfile.KindInclude, in.Name, in.Source, in.RequestedRef(), in.Path)
	}
	for i := range cfg.InstalledSkills {
		sk := &cfg.InstalledSkills[i]
		add(lockfile.KindSkill, sk.Name, sk.Source, sk.RequestedRef(), sk.Path)
	}
	for i := range cfg.SkillSources {
		ss := &cfg.SkillSources[i]
		add(lockfile.KindSource, ss.Name, ss.URL, ss.RequestedRef(), ss.Path)
	}
	return out
}

func sourceComponent(kind, name, source, ref, subpath string, pin *lockfile.Entry) Component {
	props := []Property{prop("kind", "source"), prop("source-kind", kind)}
	version := ref
	if ref != "" {
		props = append(props, prop("ref", ref))
	}
	if pin != nil {
		if pin.Commit != "" {
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
	} else {
		props = append(props, prop("source-location", "local"))
	}
	if subpath != "" && !strings.Contains(subpath, "..") && !filepath.IsAbs(subpath) {
		props = append(props, prop("path", filepath.ToSlash(subpath)))
	}
	comp.Properties = sortProps(props)
	return comp
}

// mcpEntries splits the MCP servers into local ones (components, with a
// heuristic purl when the launcher is recognised) and remote ones (services).
// Env and header values are never read; only the key names are listed.
func mcpEntries(cfg *config.Config) ([]Component, []Service) {
	var comps []Component
	var services []Service
	for _, s := range cfg.EffectiveMCPServers() {
		props := []Property{prop("kind", "mcp-server"), prop("enabled", strconv.FormatBool(s.IsEnabled()))}
		if s.Transport != "" {
			props = append(props, prop("mcp-transport", s.Transport))
		}
		if len(s.Profiles) > 0 {
			props = append(props, prop("profiles", strings.Join(sortedCopy(s.Profiles), ",")))
		}
		if s.URL != "" {
			if len(s.Env) > 0 {
				props = append(props, prop("env-keys", strings.Join(sortedKeys(s.Env), ",")))
			}
			if len(s.Headers) > 0 {
				props = append(props, prop("header-keys", strings.Join(sortedKeys(s.Headers), ",")))
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
			props = append(props, prop("env-keys", strings.Join(sortedKeys(s.Env), ",")))
		}
		if l, ok := detectLauncher(s.Command, s.Args); ok {
			comp.PURL, comp.Version = l.purl, l.version
			props = append(props, prop("purl-source", "heuristic"))
		}
		comp.Properties = sortProps(props)
		comps = append(comps, comp)
	}
	return comps, services
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
