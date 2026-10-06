package lint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// ServedFile is one file of a skill that is about to be served.
type ServedFile struct {
	// Path is the slash-separated path relative to the skill directory.
	Path    string
	Content []byte
}

// MaxServedScanBytes bounds the size of a single file that is scanned.
const MaxServedScanBytes = 512 * 1024

// CodeServedUnscannable is reported for a served file the security scan cannot
// read: binary (a NUL byte or invalid UTF-8) or larger than MaxServedScanBytes.
// A SKILL.md that cannot be scanned is an error at every trust level, as it is
// what an agent reads first; for any other file the server decides (see
// UnscannableReason).
const CodeServedUnscannable = "AR989"

func init() {
	registerRules(RuleInfo{CodeServedUnscannable, "served-file-unscannable", SeverityWarning,
		"a skill file the server would serve is binary or larger than 512 KiB, so the security scan cannot read it; reported for authored skills too. The server does not serve such a file from a remote source (trust=error) and refuses a skill whose SKILL.md is one"})
	registerRuleDocs(map[string]RuleDoc{
		CodeServedUnscannable: {
			Why:  "The security scan only reads text of bounded size, so a binary or oversized served file would reach the agent unscanned; the server refuses it from a remote source and refuses a skill whose SKILL.md is one.",
			Bad:  "A served skill that bundles a compiled binary or a 2 MiB text dump next to SKILL.md",
			Good: "Keep served skill files small UTF-8 text; ship binaries outside the served skill",
		},
	})
}

// UnscannableReason says why the security scan cannot read content, or returns
// "" when it can: the scan only reads text, and bounds the size it reads.
func UnscannableReason(content []byte) string {
	switch {
	case len(content) > MaxServedScanBytes:
		return fmt.Sprintf("is larger than %d KiB", MaxServedScanBytes/1024)
	case bytes.IndexByte(content, 0) >= 0:
		return "contains a NUL byte"
	case !utf8.Valid(content):
		return "is not valid UTF-8"
	}
	return ""
}

// ScanServed runs the security rules (AR001 to AR009) over the files of one
// skill before a server hands it to an agent. Findings keep their own severity
// unless level is config.TrustError, which turns every finding into an error.
// Inline `ai-rulez-lint-ignore` comments are not honored: the skill's author
// must not be able to silence the check on their own content. A file the scan
// cannot read (see UnscannableReason) is reported as CodeServedUnscannable, not
// silently passed: an error for SKILL.md, otherwise a warning that the caller
// turns into excluding the file from what it serves.
func ScanServed(cfg *config.Config, name string, files []ServedFile, level string, opts ...Option) []Finding {
	top := ""
	if cfg != nil {
		top, _ = filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	r := &runner{cfg: cfg, tree: &Tree{Top: top}, docs: map[string]doc{}, noInlineIgnore: true}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	r.resolveSettings()
	if level == config.TrustError {
		r.forceSev = SeverityError
	}

	var out []Finding
	for _, f := range files {
		if reason := UnscannableReason(f.Content); reason != "" {
			sev := SeverityWarning
			if strings.EqualFold(f.Path, "SKILL.md") {
				sev = SeverityError
			}
			out = append(out, Finding{
				Code: CodeServedUnscannable, Name: "served-file-unscannable", Severity: sev,
				File: "skill://" + name + "/" + f.Path, Message: "the file cannot be security-scanned: it " + reason,
			})
			continue
		}
		abs := filepath.Join(string(os.PathSeparator), "ai-rulez-served", name, filepath.FromSlash(f.Path))
		r.findings = nil
		raw := string(f.Content)
		r.docs[abs] = parseDoc(raw)
		r.securityScan(abs, raw)
		if strings.EqualFold(f.Path, "SKILL.md") {
			it := &item{kind: kindSkill, abs: abs}
			r.checkToolBreadth(it, parseFrontmatterDoc(r.docs[abs]))
		}
		for _, fd := range r.findings {
			fd.File = "skill://" + name + "/" + f.Path
			fd.Root = ""
			out = append(out, fd)
		}
	}
	return out
}
