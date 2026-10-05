package providers

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Dialects of the `checks` sidecar: the YAML review-guideline files that tools
// document as hand-written, so ai-rulez owns only the entries it renders.
const (
	// ChecksDialectAugment is .augment/code_review_guidelines.yaml: an `areas`
	// map, one area per check, each owned by its name.
	ChecksDialectAugment = "augment"
	// ChecksDialectGitLabDuo is .gitlab/duo/mr-review-instructions.yaml: an
	// `instructions` list of {name, instructions} groups, each owned by value.
	ChecksDialectGitLabDuo = "gitlab-duo"
)

func isChecksDialect(d string) bool {
	return d == ChecksDialectAugment || d == ChecksDialectGitLabDuo
}

// checkText is the review text of a check: its body, else its description, else
// its name (Augment and GitLab both require non-empty text).
func checkText(check *config.ContentFile) string {
	if body := strings.TrimSpace(check.Content); body != "" {
		return body
	}
	if desc := config.CheckDescription(check); desc != "" {
		return desc
	}
	return check.Name
}

// augmentSeverity maps a check severity onto Augment's high|medium|low scale:
// critical has no band above high, and an unset severity is medium.
func augmentSeverity(check *config.ContentFile) string {
	switch config.CheckSeverity(check) {
	case "critical", "high":
		return "high"
	case "low":
		return "low"
	}
	return "medium"
}

// augmentAreas builds one area per check, keyed by the check name.
func augmentAreas(checks []config.ContentFile) map[string]any {
	areas := make(map[string]any, len(checks))
	for i := range checks {
		check := &checks[i]
		desc := config.CheckDescription(check)
		if desc == "" {
			desc = check.Name
		}
		areas[check.Name] = map[string]any{
			SectionRootDescription: desc,
			"globs":                []any{"**"},
			"rules": []any{map[string]any{
				"id":                   check.Name,
				SectionRootDescription: checkText(check),
				"severity":             augmentSeverity(check),
			}},
		}
	}
	return areas
}

// gitlabInstructions builds one instruction group per check.
func gitlabInstructions(checks []config.ContentFile) []any {
	groups := make([]any, 0, len(checks))
	for i := range checks {
		groups = append(groups, map[string]any{keyName: checks[i].Name, "instructions": checkText(&checks[i])})
	}
	return groups
}

// checksSidecarItems are the checks a checks sidecar renders for this provider.
func (g *Generator) checksSidecarItems(content *config.ContentTree) []config.ContentFile {
	return presets.ChecksForPreset(checkItems(content), g.Spec.Name)
}

// renderChecksSidecar merges the checks into the YAML review document at
// outputPath.
func (g *Generator) renderChecksSidecar(sc *SidecarSpec, checks []config.ContentFile, cfg *config.Config, outputPath string) (sidecarRender, error) {
	var owned []jsonmerge.OwnedKey
	switch sc.Dialect {
	case ChecksDialectAugment:
		areas, aerr := claimableAugmentAreas(checks, cfg, outputPath, sc.Path)
		if aerr != nil {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(aerr)
		}
		if len(areas) > 0 {
			owned = []jsonmerge.OwnedKey{{Path: []string{"areas"}, Value: areas, Members: true}}
		}
	case ChecksDialectGitLabDuo:
		key, ok, err := gitlabOwnedKey(checks, cfg, outputPath, sc.Path)
		if err != nil {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		if !ok {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).
				Errorf("%s: `instructions` is not a list; leaving the document alone", sc.Path)
		}
		owned = []jsonmerge.OwnedKey{key}
	default:
		return sidecarRender{}, fmt.Errorf("unknown checks dialect %q", sc.Dialect)
	}
	return mergeDocument(outputPath, sc.DocFormat(), owned)
}

// sameYAMLValue reports whether two values are the same document value.
func sameYAMLValue(a, b any) bool {
	return jsonmerge.Digest(a) == jsonmerge.Digest(b)
}

// readYAMLMember returns the top-level member of the YAML document at path, and
// whether the document has it. A missing or unparseable document has none (the
// merge itself reports a document that does not parse).
func readYAMLMember(path, member string) (value any, present bool, err error) {
	text, found, rerr := jsonmerge.ReadExisting(path)
	if rerr != nil {
		return nil, false, rerr //nolint:wrapcheck // already contextual
	}
	if !found {
		return nil, false, nil
	}
	var doc map[string]any
	if parseErr := yaml.Unmarshal([]byte(text), &doc); parseErr != nil { //nolint:nilerr // The merge reports invalid documents; this probe claims no existing member.
		return nil, false, nil //nolint:nilerr // The merge reports invalid documents; this probe claims no member.
	}
	value, present = doc[member]
	return value, present && value != nil, nil
}

func previousChecksClaims(cfg *config.Config, outputPath string) []jsonmerge.Claim {
	if cfg == nil || cfg.Run == nil {
		return nil
	}
	return cfg.Run.PreviousClaims(projectRelativePath(cfg, outputPath))
}

// claimableAugmentAreas drops the areas the user already has under a check's
// name. An area ai-rulez wrote on an earlier run (the previous record names it)
// and has not been edited since, or one that equals what it would write, is ours
// to maintain; any other area of that name is the user's, and silently replacing it would destroy hand-written
// guidelines, so the check is not written and the clash is reported.
func claimableAugmentAreas(checks []config.ContentFile, cfg *config.Config, outputPath, sidecarPath string) (map[string]any, error) {
	areas := augmentAreas(checks)
	value, present, err := readYAMLMember(outputPath, "areas")
	if err != nil {
		return nil, err
	}
	existing, isMap := value.(map[string]any)
	if !present || !isMap {
		return areas, nil
	}
	claimed := map[string]jsonmerge.Claim{}
	for _, claim := range previousChecksClaims(cfg, outputPath) {
		if len(claim.Path) == 2 && claim.Path[0] == "areas" {
			claimed[claim.Path[1]] = claim
		}
	}
	for name, area := range areas {
		current, exists := existing[name]
		if !exists || sameYAMLValue(current, area) {
			continue
		}
		// The record guards what ai-rulez wrote: an area it still holds is ours to
		// update, one the user has edited since is theirs.
		if claim, wasClaimed := claimed[name]; wasClaimed && (claim.Sum == "" || claim.Sum == jsonmerge.Digest(current)) {
			continue
		}
		rulefiles.Warn(fmt.Sprintf("%s already has an area %q that ai-rulez did not write, so the check of that name is not written there",
			sidecarPath, name), "hint", "rename the area or the check")
		delete(areas, name)
	}
	return areas, nil
}

// gitlabOwnedKey returns the owned `instructions` list: the document's groups
// with the groups ai-rulez wrote last run replaced in place (or dropped, when the
// check is gone) and the new groups appended. A group the user wrote under a
// check's name stays and the check is not written: only a group that equals the
// previous record, or what ai-rulez would write now, is ours to replace. ok is
// false when the member exists but is not a list. The YAML merge keeps the source
// text of every group that did not change, so the user's comments and key order
// survive.
func gitlabOwnedKey(checks []config.ContentFile, cfg *config.Config, outputPath, sidecarPath string,
) (key jsonmerge.OwnedKey, ok bool, err error) {
	path := []string{"instructions"}
	value, present, err := readYAMLMember(outputPath, path[0])
	if err != nil {
		return key, false, err
	}
	var existing []any
	if present {
		list, isList := value.([]any)
		if !isList {
			return key, false, nil
		}
		existing = list
	}

	var previous []any
	for _, claim := range previousChecksClaims(cfg, outputPath) {
		if len(claim.Path) == 1 && claim.Path[0] == path[0] {
			previous = append(previous, claim.Elements...)
		}
	}

	oursByName := make(map[string]any, len(checks))
	for _, group := range gitlabInstructions(checks) {
		object, isMap := group.(map[string]any)
		if !isMap {
			return jsonmerge.OwnedKey{}, false, fmt.Errorf("invalid generated instruction group")
		}
		name, isString := object[keyName].(string)
		if !isString {
			return jsonmerge.OwnedKey{}, false, fmt.Errorf("generated instruction group has no name")
		}
		oursByName[name] = group
	}
	entries, placed, skipped := reconcileGitlabGroups(existing, previous, oursByName, sidecarPath)
	claimed := make([]any, 0, len(oursByName))
	for _, check := range checks {
		name := check.Name
		if skipped[name] {
			continue
		}
		if !placed[name] {
			entries = append(entries, oursByName[name])
		}
		claimed = append(claimed, oursByName[name])
	}
	return jsonmerge.OwnedKey{Path: path, Value: entries, Elements: claimed}, true, nil
}

func reconcileGitlabGroups(existing, previous []any, oursByName map[string]any, sidecarPath string) (entries []any, placed, skipped map[string]bool) {
	placed, skipped = map[string]bool{}, map[string]bool{}
	entries = make([]any, 0, len(existing)+len(oursByName))
	for _, element := range existing {
		name := gitlabGroupName(element)
		ours, isOurs := oursByName[name]
		switch {
		case isOurs && name != "" && (jsonmerge.ElementsContain(element, previous) || sameYAMLValue(element, ours)):
			if !placed[name] {
				entries = append(entries, ours)
				placed[name] = true
			}
		case isOurs && name != "":
			if !skipped[name] {
				skipped[name] = true
				rulefiles.Warn(fmt.Sprintf("%s already has an instruction group %q that ai-rulez did not write, "+
					"so the check of that name is not written there", sidecarPath, name), "hint", "rename the group or the check")
			}
			entries = append(entries, element)
		case jsonmerge.ElementsContain(element, previous):
			// written by an earlier run for a check that is gone
		default:
			entries = append(entries, element)
		}
	}
	return entries, placed, skipped
}

func gitlabGroupName(element any) string {
	if group, isMap := element.(map[string]any); isMap {
		if name, isString := group[keyName].(string); isString {
			return name
		}
	}
	return ""
}
