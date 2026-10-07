package generator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// A role's skill_mode becomes skillOverrides.<skill> in .claude/settings.json,
// merged key by key (see roles.go). When the document already holds a value for
// that skill that ai-rulez never wrote, the role replaces it. The previous value
// is kept in a machine-local ledger and put back once the role no longer sets the
// skill (a plain generate, or another role), so switching roles does not delete
// what the person wrote by hand.
//
// The ledger lives in <config dir>/local/, which is always gitignored, because
// the settings document and the claims that name its skills are machine-local
// too. The value is restored by editing the document before generation and
// dropping the role's claim for the skill, so generation then sees a consumer
// value and leaves it alone.

const (
	roleLedgerName   = ".role-skill-overrides.json"
	settingsRel      = ".claude/settings.json"
	keySkillOverride = "skillOverrides"
)

type roleLedger struct {
	// Prior maps a skill id to the value the document held before a role set it.
	Prior map[string]json.RawMessage `json:"prior"`
}

func (g *Generator) roleLedgerPath() string {
	return filepath.Join(g.manifestDir(), localSourceDirName, roleLedgerName)
}

func (g *Generator) readRoleLedger() roleLedger {
	data, err := safefs.ReadRegular(g.roleLedgerPath())
	if err != nil {
		return roleLedger{Prior: map[string]json.RawMessage{}}
	}
	var l roleLedger
	if json.Unmarshal(data, &l) != nil || l.Prior == nil {
		return roleLedger{Prior: map[string]json.RawMessage{}}
	}
	return l
}

func (g *Generator) writeRoleLedger(l roleLedger) error {
	path := g.roleLedgerPath()
	if len(l.Prior) == 0 {
		if _, err := os.Lstat(path); err != nil {
			return nil //nolint:nilerr // no ledger, nothing to remove
		}
		if err := safefs.EnsureParent(path); err != nil { // never remove through a linked directory
			return oops.Wrapf(err, "remove the role skill ledger")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return oops.Wrapf(err, "remove the role skill ledger")
		}
		return nil
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode the role skill ledger")
	}
	return oops.Wrapf(safefs.WriteFileAtomic(path, append(data, '\n')), "write the role skill ledger")
}

// roleSkillOverrides is the skillOverrides the role being rendered sets, empty
// for a plain generate.
func (g *Generator) roleSkillOverrides() map[string]string {
	if g.role == nil {
		return nil
	}
	res, err := g.config.ResolveRole(g.role.Name)
	if err != nil {
		return nil
	}
	return res.SkillOverrides
}

func (g *Generator) rendersClaudeSettings() bool {
	if g.userMode {
		return false
	}
	for i := range g.config.Presets {
		if g.config.Presets[i].GetName() == presetClaude {
			return true
		}
	}
	return false
}

// ReconcileRoleSkillOverrides warns when the role about to be rendered replaces a
// hand-written skillOverrides value, remembers that value, and restores a
// remembered value for every skill the role (or the plain run) no longer sets.
// Call it before Generate and not for a dry run or a check: it may edit
// .claude/settings.json and the manifests.
func (g *Generator) ReconcileRoleSkillOverrides() error {
	if !g.rendersClaudeSettings() {
		return nil
	}
	settingsPath := filepath.Join(g.config.BaseDir, filepath.FromSlash(settingsRel))
	doc, ok := readSkillOverrides(settingsPath)
	want := g.roleSkillOverrides()
	ledger := g.readRoleLedger()
	if !ok && len(ledger.Prior) == 0 {
		return nil
	}
	claims := g.previousMergedClaims()[settingsRel]
	changed := false

	for _, skill := range sortedStringKeys(want) {
		had, present := doc[skill]
		if !present || claimedSkill(claims, skill) || jsonEqualString(had, want[skill]) {
			continue
		}
		role := g.Role()
		g.log().Warn("Role "+role+" sets skillOverrides."+skill+" = "+want[skill]+" over the value you wrote in "+settingsRel+
			" ("+string(had)+"); it is put back when the role no longer sets this skill", "role", role, "skill", skill)
		if _, kept := ledger.Prior[skill]; !kept {
			ledger.Prior[skill] = had
			changed = true
		}
	}

	var restore []string
	for _, skill := range sortedRawKeys(ledger.Prior) {
		if _, still := want[skill]; !still {
			restore = append(restore, skill)
		}
	}
	for _, skill := range restore {
		if err := g.restoreSkillOverride(settingsPath, doc, claims, skill, ledger.Prior[skill]); err != nil {
			return err
		}
		delete(ledger.Prior, skill)
		changed = true
	}
	if !changed {
		return nil
	}
	return g.writeRoleLedger(ledger)
}

// restoreSkillOverride puts prior back as skillOverrides.<skill> when the
// document still holds the value the role wrote (a value the person changed
// since is theirs and stays), and drops the role's claim so generation treats the
// restored value as the consumer's.
func (g *Generator) restoreSkillOverride(settingsPath string, doc map[string]json.RawMessage,
	claims []jsonmerge.Claim, skill string, prior json.RawMessage,
) error {
	current, present := doc[skill]
	claim, claimed := skillClaim(claims, skill)
	if !present || !claimed || !claim.Matches(current) {
		return nil
	}
	var value any
	if err := json.Unmarshal(prior, &value); err != nil {
		return nil //nolint:nilerr // an unreadable ledger value is dropped, never written
	}
	result, err := jsonmerge.ApplyWith(g.config.ReadExisting, settingsPath, []jsonmerge.OwnedKey{
		{Path: []string{keySkillOverride}, Value: map[string]any{skill: value}, Members: true},
	})
	if err != nil {
		return oops.Wrapf(err, "restore skillOverrides.%s", skill)
	}
	if err := writeFileAtomic(settingsPath, []byte(result.Body)); err != nil {
		return oops.Wrapf(err, "write %s", settingsRel)
	}
	g.log().Info("Restored skillOverrides." + skill + " in " + settingsRel + " to the value you wrote")
	return g.dropSkillClaim(skill)
}

// dropSkillClaim removes the claim of skillOverrides.<skill> from both manifests.
func (g *Generator) dropSkillClaim(skill string) error {
	for _, path := range []string{g.manifestPath(), g.localManifestPath()} {
		m := g.readManifest(path)
		claims, ok := m.Merged[settingsRel]
		if !ok {
			continue
		}
		kept := make([]jsonmerge.Claim, 0, len(claims))
		for _, c := range claims {
			if len(c.Path) == 2 && c.Path[0] == keySkillOverride && c.Path[1] == skill {
				continue
			}
			kept = append(kept, c)
		}
		if len(kept) == len(claims) {
			continue
		}
		if len(kept) == 0 {
			delete(m.Merged, settingsRel)
		} else {
			m.Merged[settingsRel] = kept
		}
		if err := g.writeManifest(path, m.Files, m.Merged, m.Digests); err != nil {
			return oops.Wrapf(err, "update %s", filepath.Base(path))
		}
	}
	g.manifests = nil
	return nil
}

func readSkillOverrides(path string) (map[string]json.RawMessage, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // the project's own settings document
	if err != nil {
		return nil, false
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return nil, false // JSONC: left alone rather than parsed loosely
	}
	var entries map[string]json.RawMessage
	if raw, ok := root[keySkillOverride]; ok && json.Unmarshal(raw, &entries) == nil {
		return entries, true
	}
	return map[string]json.RawMessage{}, true
}

func skillClaim(claims []jsonmerge.Claim, skill string) (jsonmerge.Claim, bool) {
	for _, c := range claims {
		if len(c.Path) == 2 && c.Path[0] == keySkillOverride && c.Path[1] == skill {
			return c, true
		}
	}
	return jsonmerge.Claim{}, false
}

// claimedSkill reports whether a previous run recorded owning the skill's entry
// (or the whole map).
func claimedSkill(claims []jsonmerge.Claim, skill string) bool {
	if _, ok := skillClaim(claims, skill); ok {
		return true
	}
	for _, c := range claims {
		if len(c.Path) == 1 && c.Path[0] == keySkillOverride {
			return true
		}
	}
	return false
}

func jsonEqualString(raw json.RawMessage, s string) bool {
	want, err := json.Marshal(s)
	return err == nil && bytes.Equal(bytes.TrimSpace(raw), want)
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedRawKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
