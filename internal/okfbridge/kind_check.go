package okfbridge

import (
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// Check runs every OKF conformance and hygiene check on a bundle, including
// the x-ai-rulez kind check KindFindings adds to okf.Check. It is what
// `ai-rulez okf validate` and the okf_validate tool report.
func Check(b *okf.Bundle) []okf.Finding {
	return withKindFindings(okf.Check(b), b)
}

// withKindFindings merges the kind findings into findings, in the path, line
// and code order okf.Bundle.Validate uses.
func withKindFindings(findings []okf.Finding, b *okf.Bundle) []okf.Finding {
	extra := KindFindings(b)
	if len(extra) == 0 {
		return findings
	}
	out := append(append([]okf.Finding{}, findings...), extra...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// KindFindings reports, as AR9B1, the concepts whose x-ai-rulez.kind is not a
// content kind (an error) or names a different kind than the directory the
// concept sits in (a warning). A configuration directory classifies by
// directory and ignores the value, so a mismatch is misleading rather than
// harmful. import okf, which does read the value, maps an unknown one by type.
func KindFindings(b *okf.Bundle) []okf.Finding {
	var out []okf.Finding
	for _, p := range b.ConceptPaths() {
		c := b.Concepts[p]
		if c.Frontmatter.Err != nil {
			continue
		}
		ext := readExt(c.Frontmatter)
		if !ext.present || ext.kind == "" || ext.kind == string(kindResource) {
			continue
		}
		kind, ok := kindFromName(ext.kind)
		if !ok {
			out = append(out, okf.NewFinding(okf.CodeTypeInvalid, p, 1,
				"x-ai-rulez kind %q is unknown (use %s)", ext.kind, allowedKindNames()))
			continue
		}
		dir, inKindDir := kindDirOf(p)
		if inKindDir && dir != kind {
			f := okf.NewFinding(okf.CodeTypeInvalid, p, 1,
				"x-ai-rulez kind %q disagrees with its directory %s/; a configuration directory classifies by directory and ignores the kind",
				ext.kind, dir)
			f.Severity = okf.SeverityWarning
			out = append(out, f)
		}
	}
	return out
}

// allowedKindNames lists the x-ai-rulez kind values for a message.
func allowedKindNames() string {
	names := make([]string, 0, len(AllKinds)+1)
	for _, k := range AllKinds {
		names = append(names, string(k))
	}
	names = append(names, string(kindResource))
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// kindDirOf returns the kind of the content directory a concept path sits in,
// directly under the bundle root or under domains/<name>/. The second result is
// false for a path outside every content directory.
func kindDirOf(p string) (Kind, bool) {
	parts := strings.Split(p, "/")
	if len(parts) > 1 && parts[0] == dirDomains {
		if len(parts) < 4 {
			return "", false
		}
		parts = parts[2:]
	}
	if len(parts) < 2 {
		return "", false
	}
	for _, k := range AllKinds {
		if string(k) == parts[0] {
			return k, true
		}
	}
	return "", false
}
