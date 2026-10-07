package sbom

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Codes of the SBOM findings (the AR75x block of docs/strict-validation.md).
// They are reported by `ai-rulez sbom`; the registry in internal/lint lists them.
const (
	CodeUnpinned      = "AR750"
	CodeUnknownCoords = "AR751"
	CodeLockStale     = "AR752"
	CodeDrift         = "AR753"
)

// Names of the codes.
var codeNames = map[string]string{
	CodeUnpinned:      "sbom-component-unpinned",
	CodeUnknownCoords: "sbom-coordinates-unknown",
	CodeLockStale:     "sbom-lock-out-of-sync",
	CodeDrift:         "sbom-drift",
}

// CodeName is the rule name of an AR75x code.
func CodeName(code string) string { return codeNames[code] }

// Finding is one thing the document cannot say precisely.
type Finding struct {
	Code string
	// Subject names the component, for example "mcp-server fs".
	Subject string
	Message string
}

// String renders the finding as one line.
func (f Finding) String() string {
	return fmt.Sprintf("%s %s: %s: %s", f.Code, codeNames[f.Code], f.Subject, f.Message)
}

func sortFindings(f []Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		a, b := f[i], f[j]
		switch {
		case a.Code != b.Code:
			return a.Code < b.Code
		case a.Subject != b.Subject:
			return a.Subject < b.Subject
		}
		return a.Message < b.Message
	})
}

var (
	exactSemver = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	exactPEP440 = regexp.MustCompile(`^\d+(\.\d+)+([.-]?(a|b|rc|post|dev)\d*)*$`)
	commitSHA   = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

// exactVersion reports whether v names one release of a package of the given
// purl type: a range, a tag such as "latest" or a bare major is not exact.
func exactVersion(typ, v string) bool {
	switch typ {
	case "pypi":
		return exactPEP440.MatchString(v)
	case "oci":
		algo, hex, ok := strings.Cut(v, ":")
		return ok && algo != "" && hex != ""
	case "golang":
		return strings.HasPrefix(v, "v") && exactSemver.MatchString(v) || pseudoVersion.MatchString(v)
	}
	return exactSemver.MatchString(v)
}

var pseudoVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+-(0\.)?\d{14}-[0-9a-f]{12}$`)

// purlParts splits a package URL into its type and version. ok is false when it
// is not a pkg: URL with a type and a name.
func purlParts(p string) (typ, version string, ok bool) {
	rest, found := strings.CutPrefix(p, "pkg:")
	if !found {
		return "", "", false
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest = rest[:i]
	}
	typ, path, found := strings.Cut(rest, "/")
	if !found || typ == "" || strings.Trim(path, "/") == "" {
		return "", "", false
	}
	if i := strings.LastIndexByte(path, '@'); i > strings.LastIndexByte(path, '/') {
		version = path[i+1:]
	}
	return typ, version, true
}

// credentialsInPURL reports a user:password or token@host inside a purl
// qualifier; a declared package must never carry one into the document.
func credentialsInPURL(p string) bool {
	_, q, found := strings.Cut(p, "?")
	if !found {
		return false
	}
	return purlCredentials.MatchString(q)
}

var purlCredentials = regexp.MustCompile(`://[^/?#&\s]*@`)
