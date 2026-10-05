package contentlock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
)

// Item kinds recorded in the lock, besides the role-selectable ones.
const (
	KindRule     = "rule"
	KindContext  = "context"
	KindSkill    = "skill"
	KindAgent    = "agent"
	KindCommand  = "command"
	KindCheck    = "check"
	KindHook     = "hook"
	KindRole     = "role"
	KindSettings = "settings"
)

// Output is a generated file to pin. Data must be the rendering with the
// Generated stamp removed and before any Content-Hash or Source-Hash line is
// injected, so the digest is the same for every [header] hashes mode.
type Output struct {
	// Path is relative to the project root, "/"-separated.
	Path string
	Mode uint32
	Data []byte
}

// Options select what is pinned.
type Options struct {
	// Scope is config.LockScopeAll (default) or config.LockScopeSkills.
	Scope string
	// Outputs are pinned when IncludeOutputs is set.
	Outputs        []Output
	IncludeOutputs bool
	// ToolVersion is the running ai-rulez version.
	ToolVersion string
	// Profile is the profile Outputs were rendered for.
	Profile string
	// SourcesOnly makes Compare ignore the output pins (and the settings that
	// govern them), for a check that must not render anything.
	SourcesOnly bool
}

// Snapshot is the computed content pins.
type Snapshot struct {
	Options Options
	Items   []lockfile.Item
	Outputs []lockfile.OutputPin
	// Problems are things that could not be pinned (for example a hook script
	// outside the project). Compare reports each as a lock-scope change, so a
	// check fails on them instead of the pinning aborting.
	Problems []string
}

func (o Options) scope() string {
	if o.Scope == "" {
		return config.LockScopeAll
	}
	return o.Scope
}

// Compute digests the authored content of cfg (and Options.Outputs).
func Compute(cfg *config.Config, opts Options) (*Snapshot, error) {
	snap := &Snapshot{Options: opts}
	c := &collector{cfg: cfg, scope: opts.scope()}
	if err := c.collect(); err != nil {
		return nil, err
	}
	snap.Items = c.items
	snap.Problems = c.problems
	sort.SliceStable(snap.Items, func(i, j int) bool { return itemLess(snap.Items[i], snap.Items[j]) })
	disambiguate(snap.Items)
	if opts.IncludeOutputs {
		for _, out := range opts.Outputs {
			digest, err := TreeDigest("output", []Leaf{{Path: out.Path, Mode: ModeFor(out.Mode), Data: out.Data}})
			if err != nil {
				return nil, err
			}
			snap.Outputs = append(snap.Outputs, lockfile.OutputPin{Path: out.Path, Digest: digest})
		}
		sort.Slice(snap.Outputs, func(i, j int) bool { return snap.Outputs[i].Path < snap.Outputs[j].Path })
	}
	return snap, nil
}

func itemLess(a, b lockfile.Item) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Domain != b.Domain {
		return a.Domain < b.Domain
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	return a.Path < b.Path
}

// disambiguate suffixes the id of a second item with the same kind, domain and
// id ("#2"), in path order, so every item has a unique key.
func disambiguate(items []lockfile.Item) {
	seen := map[string]int{}
	for i := range items {
		key := items[i].Key()
		seen[key]++
		if n := seen[key]; n > 1 {
			items[i].ID += "#" + strconv.Itoa(n)
		}
	}
}

type collector struct {
	cfg   *config.Config
	scope string
	items []lockfile.Item
	// problems are unpinnable declarations, see Snapshot.Problems.
	problems []string
}

func (c *collector) wants(kind string) bool {
	return c.scope == config.LockScopeAll || kind == KindSkill
}

func (c *collector) collect() error {
	if c.cfg.Content != nil {
		if err := c.collectFiles("", c.cfg.Content.Rules, c.cfg.Content.Context, c.cfg.Content.Skills, c.cfg.Content.Agents, c.cfg.Content.Commands, c.cfg.Content.Checks); err != nil {
			return err
		}
		for _, name := range sortedDomainNames(c.cfg.Content.Domains) {
			d := c.cfg.Content.Domains[name]
			if d == nil || d.FromInclude || d.Builtin {
				continue // remote includes are pinned by their own digest; builtins by the tool version
			}
			if err := c.collectFiles(name, d.Rules, d.Context, d.Skills, d.Agents, d.Commands, d.Checks); err != nil {
				return err
			}
		}
	}
	if c.scope != config.LockScopeAll {
		return nil
	}
	return c.collectDeclared()
}

func sortedDomainNames(m map[string]*config.Domain) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (c *collector) collectFiles(domain string, rules, contexts, skills, agents, commands, checks []config.ContentFile) error {
	for _, group := range []struct {
		kind  string
		files []config.ContentFile
	}{{KindRule, rules}, {KindContext, contexts}, {KindSkill, skills}, {KindAgent, agents}, {KindCommand, commands}, {KindCheck, checks}} {
		if !c.wants(group.kind) {
			continue
		}
		for i := range group.files {
			if err := c.addFile(group.kind, domain, &group.files[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// local reports whether the file is an authored file inside the configuration
// directory (not built in, not from a remote include, not outside the project).
func (c *collector) local(cf *config.ContentFile) bool {
	if cf.Path == "" || strings.Contains(cf.Path, "://") {
		return false
	}
	_, ok := c.relToConfig(cf.Path)
	return ok
}

func (c *collector) relToConfig(p string) (string, bool) {
	if c.cfg.ConfigDir == "" {
		return filepath.ToSlash(p), true
	}
	rel, err := filepath.Rel(c.cfg.ConfigDir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func itemID(kind string, cf *config.ContentFile) string {
	if kind == KindSkill {
		return config.SkillID(*cf)
	}
	return cf.Name
}

func (c *collector) addFile(kind, domain string, cf *config.ContentFile) error {
	if !c.local(cf) {
		return nil
	}
	id := itemID(kind, cf)
	primary, err := os.ReadFile(cf.Path)
	if err != nil {
		return oops.With("path", cf.Path).Wrapf(err, "read %s for the lock", kind)
	}
	mode := ModeRegular
	if info, statErr := os.Stat(cf.Path); statErr == nil {
		mode = fileMode(cf.Path, info)
	}
	leaves := []Leaf{{Path: filepath.Base(cf.Path), Mode: mode, Data: primary}}
	dir := filepath.Dir(cf.Path)
	for _, res := range cf.Resources {
		data, resMode := res.Content, ModeFor(uint32(res.Mode.Perm()))
		abs := filepath.Join(dir, filepath.FromSlash(res.RelPath))
		if disk, readErr := os.ReadFile(abs); readErr == nil {
			data = disk
			if info, statErr := os.Stat(abs); statErr == nil {
				resMode = fileMode(abs, info)
			}
		}
		leaves = append(leaves, Leaf{Path: res.RelPath, Mode: resMode, Data: data})
	}
	digest, err := TreeDigest(kind, leaves)
	if err != nil {
		return oops.With("path", cf.Path).Wrap(err)
	}
	rel, _ := c.relToConfig(cf.Path)
	if kind == KindSkill || len(cf.Resources) > 0 {
		rel = dirOf(rel)
	}
	item := lockfile.Item{Kind: kind, ID: id, Domain: domain, Path: rel, Digest: digest}
	if cf.Metadata != nil {
		item.Owner = strings.TrimSpace(cf.Metadata.Extra["owner"])
		item.Version = strings.TrimSpace(cf.Metadata.Extra["version"])
	}
	c.items = append(c.items, item)
	return nil
}

// dirOf returns the directory of a slash path: a skill is pinned by its directory.
func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return p
}

// collectDeclared pins what config.toml declares: hooks, roles and the
// settings sources that end up in the merged settings document.
func (c *collector) collectDeclared() error {
	ordinal := map[string]int{}
	for i := range c.cfg.Hooks {
		g := &c.cfg.Hooks[i]
		matcher := g.Matcher
		if matcher == "" {
			matcher = "*"
		}
		base := g.Event + ":" + matcher
		id := fmt.Sprintf("%s:%d", base, ordinal[base])
		ordinal[base]++
		data, err := canonicalJSON(g)
		if err != nil {
			return err
		}
		leaves := []Leaf{{Path: "hook.json", Mode: ModeRegular, Data: data}}
		for _, action := range g.Hooks {
			if action.Script == "" {
				continue
			}
			rel := filepath.ToSlash(filepath.Clean(action.Script))
			if filepath.IsAbs(action.Script) || rel == ".." || strings.HasPrefix(rel, "../") {
				// Never read outside the project: pin the declaration only.
				c.problems = append(c.problems, fmt.Sprintf("hook %s script %q is outside the project, so its content cannot be pinned; move it into the project", id, action.Script))
				leaf := Leaf{Path: "outside/" + outsideName(rel), Mode: ModeRegular, Data: []byte(action.Script)}
				if !containsPath(leaves, leaf.Path) {
					leaves = append(leaves, leaf)
				}
				continue
			}
			abs := filepath.Join(c.cfg.BaseDir, filepath.FromSlash(rel))
			leaf := Leaf{Path: "script/" + strings.TrimPrefix(rel, "./"), Mode: ModeRegular}
			if disk, readErr := os.ReadFile(abs); readErr == nil {
				leaf.Data = disk
				if info, statErr := os.Stat(abs); statErr == nil {
					leaf.Mode = fileMode(abs, info)
				}
			} else {
				leaf.Path = "missing/" + strings.TrimPrefix(rel, "./") // reported by validate --strict (AR504)
			}
			if !containsPath(leaves, leaf.Path) {
				leaves = append(leaves, leaf)
			}
		}
		digest, err := TreeDigest(KindHook, leaves)
		if err != nil {
			return err
		}
		c.items = append(c.items, lockfile.Item{Kind: KindHook, ID: id, Digest: digest})
	}
	for i := range c.cfg.Roles {
		data, err := canonicalJSON(&c.cfg.Roles[i])
		if err != nil {
			return err
		}
		digest, err := TreeDigest(KindRole, []Leaf{{Path: "role.json", Mode: ModeRegular, Data: data}})
		if err != nil {
			return err
		}
		c.items = append(c.items, lockfile.Item{Kind: KindRole, ID: c.cfg.Roles[i].Name, Digest: digest})
	}
	return c.collectSettings()
}

func (c *collector) collectSettings() error {
	add := func(id string, v any) error {
		data, err := canonicalJSON(v)
		if err != nil {
			return err
		}
		digest, err := TreeDigest(KindSettings, []Leaf{{Path: id + ".json", Mode: ModeRegular, Data: data}})
		if err != nil {
			return err
		}
		c.items = append(c.items, lockfile.Item{Kind: KindSettings, ID: id, Digest: digest})
		return nil
	}
	if !c.cfg.Permissions.IsEmpty() {
		if err := add("permissions", c.cfg.Permissions); err != nil {
			return err
		}
	}
	if managed := c.cfg.ManagedClaudeSettings(); !managed.IsEmpty() {
		if err := add("claude-managed", managed); err != nil {
			return err
		}
	}
	if len(c.cfg.MCPServersRaw) > 0 {
		// As written in the configuration (placeholders unresolved), by name.
		servers := append([]config.MCPServer(nil), c.cfg.MCPServersRaw...)
		sort.SliceStable(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
		if err := add("mcp-servers", servers); err != nil {
			return err
		}
	}
	return nil
}

// outsideName flattens a path outside the project into one valid leaf segment.
func outsideName(rel string) string {
	return strings.NewReplacer("/", "_", ":", "_", `\`, "_").Replace(strings.Trim(rel, "./"))
}

func containsPath(leaves []Leaf, p string) bool {
	for _, l := range leaves {
		if l.Path == p {
			return true
		}
	}
	return false
}

// canonicalJSON renders a config value as compact JSON. Struct fields keep their
// declaration order and map keys are sorted by encoding/json, so the bytes are
// deterministic.
func canonicalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, oops.Wrapf(err, "encode a declared item for the lock")
	}
	return data, nil
}
