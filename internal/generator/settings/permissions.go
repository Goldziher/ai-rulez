package settings

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// PermAction is what a permission rule decides.
type PermAction string

// Actions of a Claude permission list.
const (
	ActionAllow PermAction = "allow"
	ActionAsk   PermAction = "ask"
	ActionDeny  PermAction = "deny"
)

// permEntry is one configured rule with its list.
type permEntry struct {
	Action PermAction
	Rule   Rule
}

// translation is the input of one harness's permission translator: the parsed
// [permissions] and where the target document lives.
type translation struct {
	cfg     *config.Config
	harness string
	docPath string
	entries []permEntry
}

// permissionDialect renders [permissions] for one harness's native surface.
type permissionDialect struct {
	// build returns the owned keys of the target document.
	build func(t *translation) ([]jsonmerge.OwnedKey, error)
	// required lists arrays the harness's schema requires whenever it writes any
	// permission (Cursor: permissions.allow and permissions.deny). One build left
	// out is added empty, owned like any array ai-rulez created.
	required [][]string
}

// permissionDialects is one map literal so every dialect exists before any
// init(); each file permissions_<harness>.go contributes through
// registerPermissionDialect from a var initialiser rather than init().
var permissionDialects = map[string]permissionDialect{}

func registerPermissionDialect(name string, build func(t *translation) ([]jsonmerge.OwnedKey, error)) struct{} {
	permissionDialects[name] = permissionDialect{build: build}
	return struct{}{}
}

// registerPermissionDialectRequiring is registerPermissionDialect for a harness
// whose document must hold the given array paths whenever it holds any permission.
func registerPermissionDialectRequiring(name string, build func(t *translation) ([]jsonmerge.OwnedKey, error), required ...[]string) struct{} {
	permissionDialects[name] = permissionDialect{build: build, required: required}
	return struct{}{}
}

// withRequiredArrays adds, for every required path no key addresses, the array
// empty (or as the document and earlier runs hold it), when any key is written.
func (d permissionDialect) withRequiredArrays(t *translation, keys []jsonmerge.OwnedKey) []jsonmerge.OwnedKey {
	if len(keys) == 0 {
		return keys
	}
	for _, path := range d.required {
		if !slices.ContainsFunc(keys, func(k jsonmerge.OwnedKey) bool { return equalPath(k.Path, path) }) {
			key := docArrayKey(t.cfg, t.docPath, path, nil)
			key.Created = true
			keys = append(keys, key)
		}
	}
	return keys
}

// IsPermissionDialect reports whether name is a known permission dialect.
func IsPermissionDialect(name string) bool {
	_, ok := permissionDialects[name]
	return ok
}

// PermissionDialectNames lists the known dialects, sorted.
func PermissionDialectNames() []string {
	names := make([]string, 0, len(permissionDialects))
	for name := range permissionDialects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PermissionKeys renders the top-level [permissions] for a dialect into the owned
// keys of the harness document at docPath (empty for a fresh one, e.g. clean's
// fallback claims). It returns nil when the configuration declares no permission
// or the run renders a [[scopes]] subdirectory, where project-wide settings do
// not belong. A rule the harness cannot express is skipped with a warning; a
// rule is never approximated into a broader one.
func PermissionKeys(cfg *config.Config, dialect, docPath string) ([]jsonmerge.OwnedKey, error) {
	if cfg == nil || cfg.Permissions.IsEmpty() || inScopeRun(cfg) {
		return nil, nil
	}
	d, ok := permissionDialects[dialect]
	if !ok {
		return nil, fmt.Errorf("unknown permissions dialect %q (known: %s)", dialect, strings.Join(PermissionDialectNames(), ", "))
	}
	t := newTranslation(cfg, dialect, docPath)
	keys, err := d.build(t)
	if err != nil {
		return nil, err
	}
	return d.withRequiredArrays(t, keys), nil
}

// newTranslation parses cfg.Permissions for a harness.
func newTranslation(cfg *config.Config, harness, docPath string) *translation {
	t := &translation{cfg: cfg, harness: harness, docPath: docPath}
	t.entries = t.parse()
	return t
}

// applicable reports whether [permissions] renders for this run at all.
func applicable(cfg *config.Config) bool {
	return cfg != nil && !cfg.Permissions.IsEmpty() && !inScopeRun(cfg)
}

// parse parses every configured rule. A rule that does not parse is dropped like
// any other the harness cannot express.
func (t *translation) parse() []permEntry {
	var out []permEntry
	lists := []struct {
		action PermAction
		rules  []string
	}{{ActionAllow, t.cfg.Permissions.Allow}, {ActionAsk, t.cfg.Permissions.Ask}, {ActionDeny, t.cfg.Permissions.Deny}}
	for _, list := range lists {
		seen := map[string]bool{}
		for _, raw := range list.rules {
			rule, err := ParseRule(raw)
			if err != nil {
				t.dropRaw(list.action, raw, err.Error())
				continue
			}
			key := rule.Raw
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, permEntry{Action: list.action, Rule: rule})
		}
	}
	return out
}

// only returns the entries of one list, in declaration order.
func (t *translation) only(action PermAction) []permEntry {
	var out []permEntry
	for _, e := range t.entries {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// drop reports a rule the harness cannot express. An allow or ask rule that is
// skipped is a narrower result than declared; a skipped deny is a hole, so it is
// reported as a security problem.
func (t *translation) drop(e permEntry, why string) {
	t.dropRaw(e.Action, e.Rule.Raw, why)
}

func (t *translation) dropRaw(action PermAction, raw, why string) {
	if action == ActionDeny {
		t.cfg.Diag.Warn(fmt.Sprintf("SECURITY: [permissions] deny rule %q is NOT enforced by %s: %s", raw, t.harness, why),
			"severity", "error", "hint", "enforce it another way (sandbox, hook) or remove the harness from the project")
		return
	}
	t.cfg.Diag.Warn(fmt.Sprintf("[permissions] %s rule %q not generated for %s: %s", action, raw, t.harness, why))
}

// askUnsupported reports, once, that the harness has no ask list: anything not
// allowed already prompts there, so an ask rule only matters under a broader allow.
func (t *translation) askUnsupported() {
	if len(t.only(ActionAsk)) == 0 {
		return
	}
	t.cfg.Diag.Warn(fmt.Sprintf("[permissions] ask rules are not generated for %s: it has no ask list and prompts for whatever is not allowed", t.harness))
}

// noDenySurface reports that the harness has no way to deny anything, once per
// harness, when the configuration declares deny rules.
func (t *translation) noDenySurface(why string) {
	for _, e := range t.only(ActionDeny) {
		t.drop(e, why)
	}
}

// docTree reads the target document into generic Go values; nil when it is
// absent or unparsable (the merge reports a broken document itself).
func docTree(cfg *config.Config, docPath string) map[string]any {
	if docPath == "" {
		return nil
	}
	data, err := cfg.ReadExisting(docPath)
	if err != nil {
		return nil
	}
	var tree map[string]any
	switch {
	case strings.HasSuffix(docPath, ".toml"):
		if toml.Unmarshal(data, &tree) != nil {
			return nil
		}
	case strings.HasSuffix(docPath, ".yaml"), strings.HasSuffix(docPath, ".yml"):
		if yaml.Unmarshal(data, &tree) != nil {
			return nil
		}
	default:
		var err error
		if tree, err = jsonmerge.DecodeTolerantTree(string(data)); err != nil {
			return nil
		}
	}
	return tree
}

// canonical is the comparison form of a document value: its JSON text.
func canonical(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	var generic any
	if json.Unmarshal(raw, &generic) == nil {
		if raw2, err := json.Marshal(generic); err == nil {
			return string(raw2)
		}
	}
	return string(raw)
}

func containsValue(list []any, v any) bool {
	want := canonical(v)
	return slices.ContainsFunc(list, func(item any) bool { return canonical(item) == want })
}

// docArrayKey is arrayKey for a document of any format: the owned key of an
// array in which ai-rulez owns only the elements in ours. The array keeps the
// consumer's elements in their order and appends the missing elements of ours;
// elements an earlier run added and this one no longer wants leave with it, and
// only the elements ai-rulez added or claimed earlier are recorded as its own.
func docArrayKey(cfg *config.Config, docPath string, path []string, ours []any) jsonmerge.OwnedKey {
	var existing []any
	if v, ok := jsonmerge.LookupTree(docTree(cfg, docPath), path); ok {
		existing, _ = v.([]any) // a non-array value is the consumer's; the merge reports it
	}
	value, claimed := planElements(previousElementClaims(cfg, docPath, path), existing, ours)
	if value == nil {
		value = []any{}
	}
	if claimed == nil {
		claimed = []any{}
	}
	return jsonmerge.OwnedKey{Path: path, Value: value, Elements: claimed}
}

// docMembersKey owns the given entries of the map at path one by one. An entry
// the document already holds with another value that no earlier run wrote is the
// consumer's and is left as it is (ai-rulez never overrides a rule of theirs, in
// particular never relaxes a deny); a warning says so.
func docMembersKey(t *translation, path []string, entries map[string]any) (jsonmerge.OwnedKey, bool) {
	var existing map[string]any
	if v, ok := jsonmerge.LookupTree(docTree(t.cfg, t.docPath), path); ok {
		existing, _ = v.(map[string]any)
	}
	previous := t.cfg.Run.PreviousClaims(documentRel(t.cfg, t.docPath))
	value := make(map[string]any, len(entries))
	for name, entry := range entries {
		had, ok := existing[name]
		if ok && canonical(had) != canonical(entry) && !claimedPath(previous, append(slices.Clone(path), name)) {
			t.cfg.Diag.Warn(fmt.Sprintf("[permissions] %s already sets %s.%s; the existing value is kept",
				t.harness, strings.Join(path, "."), name))
			continue
		}
		value[name] = entry
	}
	if len(value) == 0 {
		return jsonmerge.OwnedKey{}, false
	}
	return jsonmerge.OwnedKey{Path: path, Value: value, Members: true}, true
}

// scalarKey owns one scalar setting. A value the document already holds that
// differs from ours and that no earlier run wrote is the consumer's and is kept:
// ai-rulez never overrides a setting of theirs, in particular never relaxes one.
func scalarKey(t *translation, path []string, value any) (jsonmerge.OwnedKey, bool) {
	if v, ok := jsonmerge.LookupTree(docTree(t.cfg, t.docPath), path); ok &&
		!claimedPath(t.cfg.Run.PreviousClaims(documentRel(t.cfg, t.docPath)), path) {
		if canonical(v) != canonical(value) {
			t.cfg.Diag.Warn(fmt.Sprintf("[permissions] %s already sets %s; the existing value is kept", t.harness, strings.Join(path, ".")))
		}
		return jsonmerge.OwnedKey{}, false // an identical value is theirs too
	}
	return jsonmerge.OwnedKey{Path: path, Value: value}, true
}

func claimedPath(claims []jsonmerge.Claim, path []string) bool {
	return slices.ContainsFunc(claims, func(c jsonmerge.Claim) bool { return equalPath(c.Path, path) })
}

// stringElements converts rule strings to array elements.
func stringElements(rules []string) []any {
	out := make([]any, 0, len(rules))
	for _, r := range rules {
		out = append(out, r)
	}
	return out
}

// PermissionHarnesses lists the harnesses [permissions] is translated for, sorted:
// claude (see ClaudeKeys), codex (CodexRules) and every registered dialect.
func PermissionHarnesses() []string {
	names := append([]string{config.HarnessClaude, config.HarnessCodex}, PermissionDialectNames()...)
	sort.Strings(names)
	return slices.Compact(names)
}

// SupportsPermissions reports whether [permissions] is translated for the preset.
func SupportsPermissions(preset string) bool {
	return slices.Contains(PermissionHarnesses(), preset)
}

// userOnlyPermissionHarnesses read their permission settings from a user-level
// file only; their sidecars are `user_only`.
var userOnlyPermissionHarnesses = []string{"zed", "hermes", "kimi"}

// PermissionsUserOnly reports whether the harness reads its permissions from a
// user-level file, so a project run writes none for it.
func PermissionsUserOnly(preset string) bool {
	return slices.Contains(userOnlyPermissionHarnesses, preset)
}
