package sbom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Drift compares a committed document with the one just built and lists the
// differences, one line each (empty when they agree). Both are JSON in the same
// format. The tool version is ignored (CycloneDX metadata.tools, SPDX
// creationInfo.creators): upgrading ai-rulez must not break a check on its own.
// Elements are compared by identity (bom-ref, SPDXID, dependency ref), so the
// report names what was added, removed or changed.
func Drift(committed, current []byte) ([]string, error) {
	if bytes.Equal(committed, current) {
		return nil, nil
	}
	var a, b map[string]any
	if err := json.Unmarshal(committed, &a); err != nil {
		return nil, oops.Wrapf(err, "the committed SBOM is not valid JSON")
	}
	if err := json.Unmarshal(current, &b); err != nil {
		return nil, oops.Wrapf(err, "the generated SBOM is not valid JSON")
	}
	ignoreToolVersion(a)
	ignoreToolVersion(b)
	var diffs []string
	for _, k := range unionKeys(a, b) {
		diffs = append(diffs, diffValue(k, a[k], b[k])...)
	}
	return diffs, nil
}

func ignoreToolVersion(doc map[string]any) {
	if md, ok := doc["metadata"].(map[string]any); ok {
		delete(md, "tools")
	}
	if ci, ok := doc["creationInfo"].(map[string]any); ok {
		delete(ci, "creators")
	}
}

// identityKeys name the field that identifies an element of an array.
var identityKeys = []string{"bom-ref", "SPDXID", "ref", "licenseId", "name"}

func diffValue(path string, old, cur any) []string {
	if equalJSON(old, cur) {
		return nil
	}
	oldArr, okOld := old.([]any)
	curArr, okCur := cur.([]any)
	if (okOld || old == nil) && (okCur || cur == nil) {
		return diffArray(path, oldArr, curArr)
	}
	oldMap, okOld := old.(map[string]any)
	curMap, okCur := cur.(map[string]any)
	if okOld && okCur {
		var out []string
		for _, k := range unionKeys(oldMap, curMap) {
			out = append(out, diffValue(path+"."+k, oldMap[k], curMap[k])...)
		}
		return out
	}
	return []string{fmt.Sprintf("%s changed: %s -> %s", path, short(old), short(cur))}
}

func diffArray(path string, old, cur []any) []string {
	oldBy, oldOK := indexByIdentity(old)
	curBy, curOK := indexByIdentity(cur)
	if !oldOK || !curOK {
		return []string{fmt.Sprintf("%s changed (%d -> %d entries)", path, len(old), len(cur))}
	}
	var out []string
	for _, id := range unionKeys(oldBy, curBy) {
		o, hadOld := oldBy[id]
		c, hasCur := curBy[id]
		switch {
		case !hadOld:
			out = append(out, fmt.Sprintf("%s: added %s", path, id))
		case !hasCur:
			out = append(out, fmt.Sprintf("%s: removed %s", path, id))
		case !equalJSON(o, c):
			out = append(out, fmt.Sprintf("%s: changed %s", path, id))
		}
	}
	return out
}

func indexByIdentity(items []any) (map[string]any, bool) {
	out := map[string]any{}
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, false
		}
		id := identityOf(m)
		if id == "" {
			return nil, false
		}
		if _, dup := out[id]; dup {
			id = fmt.Sprintf("%s#%d", id, i)
		}
		out[id] = it
	}
	return out, true
}

// identityOf names an array element: its id field, or for a relationship its
// three ends.
func identityOf(m map[string]any) string {
	for _, k := range identityKeys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	if e, ok := m["spdxElementId"].(string); ok {
		return fmt.Sprintf("%s %v %v", e, m["relationshipType"], m["relatedSpdxElement"])
	}
	return ""
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a) //nolint:errcheck // values come from json.Unmarshal
	y, _ := json.Marshal(b) //nolint:errcheck // values come from json.Unmarshal
	return bytes.Equal(x, y)
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]V{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func short(v any) string {
	data, _ := json.Marshal(v) //nolint:errcheck // values come from json.Unmarshal
	s := string(data)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return strings.ReplaceAll(s, "\n", " ")
}
