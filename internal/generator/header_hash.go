package generator

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// extractContentHash returns just the Content-Hash header value (or empty if absent).
// Kept for tests and consumers that don't care about Source-Hash.
func extractContentHash(filePath string) string {
	contentHash, _ := extractStoredHashes(filePath)
	return contentHash
}

// maxHeaderLines bounds how far extractStoredHashes reads past a frontmatter
// block (or into a file without one).
const maxHeaderLines = 60

// extractStoredHashes scans the header of an existing file and returns the
// Content-Hash and Source-Hash values found there (empty strings if missing).
// A file starting with a YAML frontmatter block is scanned through the closing
// "---" however long the block is, then up to maxHeaderLines more lines for the
// banner; other files are scanned for maxHeaderLines lines.
func extractStoredHashes(filePath string) (contentHash, sourceHash string) {
	contentHash, sourceHash, _ = scanStoredHashes(filePath)
	return contentHash, sourceHash
}

// scanStoredHashes is extractStoredHashes that also reports whether a hash was
// found inside the frontmatter block (the layout older versions used for
// rules-folder files). Lines of any length are handled and CRLF is accepted.
func scanStoredHashes(filePath string) (contentHash, sourceHash string, inFrontmatterBlock bool) {
	return scanHashesFrom(func(p string) (io.ReadCloser, error) { return os.Open(p) }, filePath)
}

// opener opens an existing file: os.Open, or the project's workspace.
type opener func(path string) (io.ReadCloser, error)

// scanHashes is scanStoredHashes reading the file from the project's workspace.
func (g *Generator) scanHashes(filePath string) (contentHash, sourceHash string, inFrontmatterBlock bool) {
	return scanHashesFrom(g.opener(), filePath)
}

func (g *Generator) opener() opener {
	return func(p string) (io.ReadCloser, error) { return g.config.OpenExisting(p) }
}

func scanHashesFrom(open opener, filePath string) (contentHash, sourceHash string, inFrontmatterBlock bool) {
	file, err := open(filePath)
	if err != nil {
		return "", "", false
	}
	defer file.Close()

	st := hashScan{budget: maxHeaderLines}
	reader := bufio.NewReader(file)
	for first := true; ; first = false {
		raw, readErr := reader.ReadString('\n')
		if raw == "" && readErr != nil {
			break // EOF, or a read error: end with what was found
		}
		if st.line(strings.TrimRight(raw, "\r\n"), first) || readErr != nil {
			break
		}
	}
	return st.contentHash, st.sourceHash, st.inBlock
}

// hashScan is the state of scanStoredHashes.
type hashScan struct {
	contentHash, sourceHash string
	inFrontmatter           bool // currently inside the frontmatter block
	inBlock                 bool // a hash was found inside the frontmatter block
	budget                  int  // lines left outside the frontmatter block
}

// line consumes one line and reports whether the scan is finished.
func (h *hashScan) line(raw string, first bool) bool {
	switch {
	case first && raw == "---":
		h.inFrontmatter = true
		return false
	case h.inFrontmatter && raw == "---":
		h.inFrontmatter = false
		return false
	case !h.inFrontmatter:
		h.budget--
		if h.budget < 0 {
			return true
		}
	}
	c, s := hashFromLine(raw)
	if c != "" {
		h.contentHash = c
	}
	if s != "" {
		h.sourceHash = s
	}
	if (c != "" || s != "") && h.inFrontmatter {
		h.inBlock = true
	}
	return h.contentHash != "" && h.sourceHash != ""
}

// hashFromLine extracts a Content-Hash or Source-Hash value from one header
// line, regardless of comment style (HTML, hash, slash, semicolon).
func hashFromLine(raw string) (contentHash, sourceHash string) {
	stripped := strings.TrimSpace(raw)
	for _, prefix := range []string{"<!--", "-->", "//", "#", ";", "/*", "*/"} {
		stripped = strings.TrimPrefix(stripped, prefix)
	}
	stripped = strings.TrimSpace(stripped)
	switch {
	case strings.HasPrefix(stripped, "Content-Hash: "):
		return strings.TrimPrefix(stripped, "Content-Hash: "), ""
	case strings.HasPrefix(stripped, "Source-Hash: "):
		return "", strings.TrimPrefix(stripped, "Source-Hash: ")
	}
	return "", ""
}

// stripHeader removes the header comment from generated content, returning
// only the body. The header format depends on the output file extension.
//
// Detection runs in the same priority order as injectHashes so that the body
// passed to the body hash is symmetric with the body the injection sees.
// Files may have multiple header layers (e.g. devin rule files have
// trigger frontmatter THEN a generated-file banner) — we strip them all,
// otherwise the banner's per-run timestamp would leak into the body hash.
func stripHeader(content, outputPath string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	// 1. YAML frontmatter (skill/agent files, plus rule files that prepend
	// trigger frontmatter). Strip and continue — there may be a banner after.
	if end, _ := frontmatterEnd(content); end > 0 {
		// Drop a single leading blank line so banner detection works against
		// either "---\n\n<!--" or "---\n<!--" forms.
		content = strings.TrimPrefix(strings.TrimPrefix(content[end:], "\r"), "\n")
	}

	switch ext {
	case ".mdc", ".md", ".markdown", ".mdx", ".html":
		// Only a banner at the very start (after the frontmatter) is a header: a
		// "-->" further down belongs to the body (fenced HTML, comments) and
		// stripping up to it would truncate the hashed body. "# Heading" is a
		// heading in markdown, not a line comment, so no line-comment stripping.
		if !strings.HasPrefix(content, "<!--") {
			return content
		}
		if idx := strings.Index(content, "-->"); idx >= 0 {
			return skipEOL(content[idx+len("-->"):])
		}
		return content
	default:
		// Line-prefix comments (#, //, ;) — also covers .mdc which uses
		// "# title\n\nbody" with the title acting as a heading-shaped header.
		lines := strings.Split(content, "\n")
		i := 0
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				i++
				break
			}
			isComment := strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, ";")
			if !isComment {
				break
			}
			i++
		}
		if i >= len(lines) {
			return ""
		}
		return strings.Join(lines[i:], "\n")
	}
}

// injectContentHash inserts a Content-Hash line into the header of the
// generated content. Backward-compatible thin wrapper around injectHashes.
func injectContentHash(content, outputPath, hash string) string {
	return injectHashes(content, outputPath, hash, "")
}

// injectHashes inserts Content-Hash and (optionally) Source-Hash lines into
// the header of the generated content. It locates the header closing marker
// and inserts the hash lines before it. If sourceHash is empty, only
// Content-Hash is injected (e.g., from older callers).
func injectHashes(content, outputPath, contentHash, sourceHash string) string {
	return injectHashesIn(nil, content, outputPath, contentHash, sourceHash)
}

// injectHashesIn is injectHashes for a project whose rules folders are rd (nil:
// the built-in ones).
func injectHashesIn(rd *config.RulesDirSet, content, outputPath, contentHash, sourceHash string) string {
	if contentHash == "" {
		return content
	}
	ext := strings.ToLower(filepath.Ext(outputPath))

	hashBlock := func(linePrefix string) string { return hashLines(linePrefix, contentHash, sourceHash) }

	// 0. Native rules folders: the tools' frontmatter parsers are not documented
	// to tolerate YAML comments (a failed parse can turn a scoped rule global or
	// drop it), so the hashes go into the HTML banner after the frontmatter.
	if out, ok := injectIntoRulesDirBanner(rd, content, outputPath, hashBlock("")); ok {
		return out
	}

	// 1. YAML frontmatter (skill/agent files): inject as YAML comment lines
	// inside the frontmatter, before the closing "---". YAML parsers ignore
	// comments, so consumers see the same parsed fields.
	if strings.HasPrefix(content, "---\n") {
		rest := content[len("---\n"):]
		if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
			closeIdx := len("---\n") + idx
			return content[:closeIdx] + "\n" + hashBlock("# ") + content[closeIdx:]
		}
	}

	// 2. HTML comment banner — only for true markdown/HTML extensions.
	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		marker := "\n-->\n"
		if idx := strings.Index(content, marker); idx >= 0 {
			return content[:idx] + "\n" + hashBlock("") + marker + content[idx+len(marker):]
		}
		return content
	}

	// 3. Line-prefix comments (yaml, ini, .mdc title-as-header, etc).
	prefix := "# "
	switch ext {
	case ".json", ".jsonc", ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cc", ".cpp", ".cs":
		prefix = "// "
	case ".ini":
		prefix = "; "
	}

	// Only a comment block at the very start of the file is a header (the same
	// shape stripHeader removes). A "# heading" line deeper in the file, such as
	// inside a TOML multi-line string or a YAML block scalar, is body: hashes
	// injected there are never stripped, so the file would read as hand-edited.
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if isLineComment(line) {
			continue
		}
		if strings.TrimSpace(line) == "" && i > 0 {
			hashLines := strings.Split(hashBlock(prefix), "\n")
			result := make([]string, 0, len(lines)+len(hashLines))
			result = append(result, lines[:i]...)
			result = append(result, hashLines...)
			result = append(result, lines[i:]...)
			return strings.Join(result, "\n")
		}
		break
	}
	return content
}

// isLineComment reports whether line starts a #, // or ; comment.
func isLineComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, ";")
}

// hashLines renders the Content-Hash line and, when set, the Source-Hash line,
// each starting with linePrefix.
func hashLines(linePrefix, contentHash, sourceHash string) string {
	var b strings.Builder
	b.WriteString(linePrefix)
	b.WriteString("Content-Hash: ")
	b.WriteString(contentHash)
	if sourceHash != "" {
		b.WriteByte('\n')
		b.WriteString(linePrefix)
		b.WriteString("Source-Hash: ")
		b.WriteString(sourceHash)
	}
	return b.String()
}

// injectIntoRulesDirBanner is injectIntoBanner for files in a native rules
// folder; it reports false for any other output. A markdown rules-folder file
// without a banner (a provider spec that is not split writes such files) gets a
// minimal banner carrying the hashes, right after its frontmatter: the hashes
// must not go into the frontmatter, and without them the file would be
// rewritten on every run and could not be told apart from a hand-written one.
// Other extensions report false and take the generic fallback.
func injectIntoRulesDirBanner(rd *config.RulesDirSet, content, outputPath, block string) (string, bool) {
	if !rd.In(outputPath) {
		return content, false
	}
	if out, ok := injectIntoBanner(content, block); ok {
		return out, true
	}
	if !isMarkdownRuleExt(outputPath) {
		return content, false
	}
	banner, ok := injectIntoBanner(templates.RuleBanner(""), block)
	if !ok {
		return content, false
	}
	end, open := frontmatterEnd(content)
	switch {
	case end > 0:
		return content[:end] + banner + strings.TrimPrefix(content[end:], "\n"), true
	case open:
		return content, false
	}
	return banner + content, true
}

// injectIntoBanner adds block before the closing "-->" of the HTML comment
// banner that follows the optional frontmatter. It reports false when the
// content has no such banner.
func injectIntoBanner(content, block string) (string, bool) {
	offset, open := frontmatterEnd(content)
	if open && offset == 0 {
		return content, false
	}
	rest := content[offset:]
	if !strings.HasPrefix(strings.TrimLeft(rest, "\r\n"), "<!--") {
		return content, false
	}
	idx := strings.Index(rest, "\n-->")
	if idx < 0 {
		return content, false
	}
	at := offset + idx
	eol := "\n"
	if at > 0 && content[at-1] == '\r' {
		at--
		eol = "\r\n"
	}
	return content[:at] + eol + block + content[at:], true
}
