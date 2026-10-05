package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// ServedFile is one file of a skill that is about to be served.
type ServedFile struct {
	// Path is the slash-separated path relative to the skill directory.
	Path    string
	Content []byte
}

// maxServedScanBytes bounds the size of a single file that is scanned; larger
// files are not text an agent would read.
const maxServedScanBytes = 512 * 1024

// ScanServed runs the security rules (AR001 to AR009) over the files of one
// skill before a server hands it to an agent. Findings keep their own severity
// unless level is config.TrustError, which turns every finding into an error.
// Inline `ai-rulez-lint-ignore` comments are not honored: the skill's author
// must not be able to silence the check on their own content. Binary files and
// files over 512 KiB are skipped, as they are for generation.
func ScanServed(cfg *config.Config, name string, files []ServedFile, level string) []Finding {
	top := ""
	if cfg != nil {
		top, _ = filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	r := &runner{cfg: cfg, tree: &Tree{Top: top}, docs: map[string]doc{}, noInlineIgnore: true}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	r.cwd, _ = os.Getwd() //nolint:errcheck // display paths fall back to absolute
	r.resolveSettings()
	if level == config.TrustError {
		r.forceSev = SeverityError
	}

	var out []Finding
	for _, f := range files {
		if len(f.Content) > maxServedScanBytes || !utf8.Valid(f.Content) || bytes.IndexByte(f.Content, 0) >= 0 {
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
