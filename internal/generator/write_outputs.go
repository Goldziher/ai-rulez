package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

func (g *Generator) writeOutputs(outputs []config.OutputFile) error {
	g.previousFiles = nil
	g.skippedPaths = make(map[string]bool)
	defer func() { g.previousFiles = nil }()
	for _, output := range outputs {
		if err := g.writeOutput(output); err != nil {
			return oops.
				With("path", output.Path).
				Wrapf(err, "write output file")
		}
	}
	return nil
}

func (g *Generator) absOutputPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(g.config.BaseDir, path)
}

// writeOutput writes a single output file or creates a directory.
//
// Skip decision: we hash the freshly-rendered body and compare two values
// embedded in the existing file's header — the Source-Hash (changes when any
// source input changes) and the Content-Hash (changes when this output's body
// changes). The comparison is against the in-header hashes, never against the
// on-disk body, so skipping is robust to formatters or other tools that touch
// the body after we wrote it. We only skip when BOTH match: a Source-Hash
// mismatch alone forces a rewrite to refresh stale provenance metadata even
// when the body would round-trip identically.
//
// On write, output is normalized to end with exactly one trailing newline so
// formatters that enforce that convention (end-of-file-fixer, etc.) don't
// spuriously modify the file after generation.
func (g *Generator) writeOutput(output config.OutputFile) error {
	absPath := g.absOutputPath(output.Path)
	if rel := g.relSlash(absPath); g.linkedOutputs[rel] {
		// A link onto another generated path: the target is written under its own
		// name, the link stays the user's and is not recorded as ours.
		g.skippedPaths[rel] = true
		g.log().Debug("Skipped a symlinked output whose target is generated", "path", output.Path)
		return nil
	}
	// target is where a write really lands; absPath stays the lexical path that
	// ownership and manifest logic key on.
	target, viaLink, err := g.guardWrite(absPath)
	if err != nil {
		return err
	}

	if output.IsDir {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return oops.
				With("dir", absPath).
				Hint(fmt.Sprintf("Check directory permissions for: %s", absPath)).
				Wrapf(err, "create directory")
		}
		g.log().Debug("Created directory", "path", output.Path)
		return nil
	}

	// Raw mode: write bytes verbatim. Used for skill resources (references,
	// scripts, assets) where the standard header banner would corrupt the
	// payload (e.g. Python scripts) or break binary files.
	if output.RawContent != nil {
		return writeRawOutput(g.log(), target, viaLink, output)
	}

	finalContent := g.finalContent(output)

	if g.isUnmanagedRuleFile(absPath, finalContent) {
		g.skipUnmanagedRuleOutput(output, absPath)
		return nil
	}

	if g.canSkipWrite(absPath, output, finalContent) {
		if output.Sensitive && !viaLink {
			if err := os.Chmod(target, sensitiveFileMode); err != nil {
				return oops.With("path", absPath).Wrapf(err, "restrict permissions of a file carrying secrets")
			}
		}
		g.log().Debug("Skipped unchanged file", "path", output.Path)
		return nil
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}

	return g.writeFinalContent(output, absPath, target, finalContent)
}

// skipUnmanagedRuleOutput records and warns that output was not written because a
// hand-written file already sits at its path in a shared rules folder.
func (g *Generator) skipUnmanagedRuleOutput(output config.OutputFile, absPath string) {
	if g.skippedPaths == nil {
		g.skippedPaths = make(map[string]bool)
	}
	g.skippedPaths[filepath.ToSlash(g.convertToRelativePath(absPath))] = true
	if output.LocalOnly {
		g.log().Warn("Skipped existing hand-written file that collides with a machine-local rule file; "+
			"*.local.* names in rules folders are reserved for ai-rulez local rules, rename the file",
			"path", output.Path)
		return
	}
	g.log().Warn("Skipped existing hand-written rule file that collides with a generated rule; rename one of them"+
		g.unmanagedHint(),
		"path", output.Path, "rule", strings.TrimSuffix(filepath.Base(output.Path), filepath.Ext(output.Path)))
}

// writeFinalContent writes the rendered content of output to target. The file is
// replaced atomically, and owner-only when it carries secrets.
func (g *Generator) writeFinalContent(output config.OutputFile, absPath, target, finalContent string) error {
	if output.Sensitive {
		// Owner-only temp file renamed into place: the secret is never on disk
		// with a wider mode, and an existing world-readable file is replaced.
		if err := config.WriteFileAtomic(target, []byte(finalContent), sensitiveFileMode); err != nil {
			return oops.
				With("path", absPath).
				Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
				Wrapf(err, "write file")
		}
		g.log().Debug("Wrote file", "path", output.Path, "size", len(finalContent), "mode", sensitiveFileMode)
		return nil
	}

	// Temp file + rename so a crash never leaves a truncated hand-authored file;
	// keeps an existing file's mode and writes through symlinks.
	if err := writeFileAtomic(target, []byte(finalContent)); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}

	g.log().Debug("Wrote file", "path", output.Path, "size", len(finalContent))
	return nil
}

// isNestedAgentsMD reports whether rel is an AGENTS.md below the project root
// (the baz nested files, monorepo scope files): hand-written ones are common.
func isNestedAgentsMD(rel string) bool {
	return strings.HasSuffix(rel, "/AGENTS.md")
}

// isUnmanagedRuleFile reports whether absPath is an existing file inside a
// shared rules folder that ai-rulez did not write. A file counts as ours when
// any of these hold: it is in the previous generated manifest, it stores a
// Content-Hash, it carries a generated-file banner, or its bytes already equal
// wantContent (so a fresh clone with no manifest and hashes = "none" does not
// mistake our own output for a hand-written file).
func (g *Generator) isUnmanagedRuleFile(absPath, wantContent string) bool {
	rel := filepath.ToSlash(g.convertToRelativePath(absPath))
	if !g.config.InRulesDir(rel) && !isNestedAgentsMD(rel) {
		return false
	}
	info, err := g.config.StatExisting(absPath)
	if err != nil || info.IsDir() {
		return false
	}
	if g.previousFiles == nil {
		g.previousFiles = make(map[string]bool)
		for _, f := range g.previousManifestFiles() {
			g.previousFiles[filepath.ToSlash(f)] = true
		}
	}
	if g.previousFiles[rel] {
		return false
	}
	if contentHash, _, _ := g.scanHashes(absPath); contentHash != "" {
		return false
	}
	data, err := g.config.ReadExisting(absPath)
	if err != nil {
		return false
	}
	if string(data) == wantContent {
		return false
	}
	return !hasGeneratedBanner(absPath, data)
}

// unmanagedHint explains why a generated file may have been mistaken for a
// hand-written one when the header carries no recognizable marker.
func (g *Generator) unmanagedHint() string {
	if g.config.GetHeaderHashes() == config.HeaderHashesNone || g.config.Header.GetCustomHeader() != "" {
		return " (generated files carry no recognizable header with [header] hashes = \"none\" or custom header text;" +
			" if this file was generated by ai-rulez, delete it and regenerate)"
	}
	return ""
}

// generatedBannerMarkers are the strings generated headers and rule banners carry.
var generatedBannerMarkers = [...]string{"GENERATED FILE", "Generated by ai-rulez"}

// extHTML is the only markup extension the banner check treats like markdown.
const extHTML = ".html"

// bannerScanLimit bounds how much of a file the banner check reads.
const bannerScanLimit = 16 * 1024

// frontmatterEnd locates a leading YAML frontmatter block. open reports that
// the content starts with a "---" line (LF or CRLF); end is the offset just past
// the closing "---" line, or 0 when the block is not closed. One helper serves
// the banner check, hash injection and header stripping, so files with CRLF line
// endings (a checkout with autocrlf, an editor that converts them) are read the
// same way as the LF files ai-rulez writes.
func frontmatterEnd(s string) (end int, open bool) {
	var first int
	switch {
	case strings.HasPrefix(s, "---\n"):
		first = len("---\n")
	case strings.HasPrefix(s, "---\r\n"):
		first = len("---\r\n")
	default:
		return 0, false
	}
	for pos := first; pos < len(s); {
		nl := strings.IndexByte(s[pos:], '\n')
		line, next := s[pos:], len(s)
		if nl >= 0 {
			line, next = s[pos:pos+nl], pos+nl+1
		}
		if strings.TrimSuffix(line, "\r") == frontmatterFence {
			return next, true
		}
		pos = next
	}
	return 0, true
}

// skipEOL drops one or two line breaks (LF or CRLF) from the start of s: the
// blank line that follows a banner.
func skipEOL(s string) string {
	for i := 0; i < 2; i++ {
		switch {
		case strings.HasPrefix(s, "\r\n"):
			s = s[2:]
		case strings.HasPrefix(s, "\n"):
			s = s[1:]
		default:
			return s
		}
	}
	return s
}

// hasGeneratedBanner reports whether data starts with a generated-file banner:
// the first comment after the optional frontmatter. For markdown that is an HTML
// comment, for any other extension also a leading block of line comments. A
// marker deeper in the file (documentation that quotes the banner, a hand-written
// rule that mentions it) does not count.
func hasGeneratedBanner(path string, data []byte) bool {
	if len(data) > bannerScanLimit {
		data = data[:bannerScanLimit]
	}
	text := string(data)
	if end, _ := frontmatterEnd(text); end > 0 {
		text = text[end:]
	}
	text = strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(text, "<!--") {
		if end := strings.Index(text, "-->"); end >= 0 {
			text = text[:end]
		}
		return containsBannerMarker(text)
	}
	if isMarkdownRuleExt(path) || strings.EqualFold(filepath.Ext(path), extHTML) {
		return false
	}
	var header strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, ";") {
			break
		}
		header.WriteString(trimmed + "\n")
	}
	return containsBannerMarker(header.String())
}

// isMarkdownRuleExt reports whether path has a markdown-family extension, the
// files whose headers are HTML comments.
func isMarkdownRuleExt(path string) bool {
	return slices.Contains(markdownRuleExts, strings.ToLower(filepath.Ext(path)))
}

var markdownRuleExts = strings.Fields(".md .mdc .markdown .mdx")

func containsBannerMarker(header string) bool {
	for _, marker := range generatedBannerMarkers {
		if strings.Contains(header, marker) {
			return true
		}
	}
	return false
}

// finalContent is the exact text writeOutput puts on disk for a rendered
// output: the body plus the freshness lines the header hash mode asks for,
// normalized to a single trailing newline.
func (g *Generator) finalContent(output config.OutputFile) string {
	contentHash := templates.HashContent(stripHeader(output.Content, output.Path))
	sourceHash := g.sourceHashFor(output)
	switch g.config.GetHeaderHashes() {
	case config.HeaderHashesNone:
		contentHash, sourceHash = "", ""
	case config.HeaderHashesContent:
		sourceHash = ""
	}
	return normalizeTrailingNewline(injectHashesIn(g.config.RulesDirs, output.Content, output.Path, contentHash, sourceHash))
}

// canSkipWrite reports whether the file on disk already matches what would be
// written.
//
// In "full" mode the comparison is the in-header Content-Hash plus Source-Hash,
// and the on-disk body must still hash to its own Content-Hash: a body edited by
// hand (or reformatted) is rewritten, since generated files are never edited.
// "content" and "none" have no Source-Hash to carry header changes (style, text,
// config directory), and "none" has no hash at all, so they compare the whole
// rendered file. With [header] timestamp enabled the Generated: text is ignored
// in that comparison, otherwise every run would rewrite every file.
func (g *Generator) canSkipWrite(absPath string, output config.OutputFile, finalContent string) bool {
	if g.config.GetHeaderHashes() == config.HeaderHashesFull && finalCarriesHash(finalContent) {
		contentHash := templates.HashContent(stripHeader(output.Content, output.Path))
		existingContentHash, existingSourceHash, legacy := g.scanHashes(absPath)
		if legacy && g.config.InRulesDir(output.Path) {
			// Hashes in the frontmatter are the pre-banner layout: rewrite once.
			return false
		}
		if existingContentHash == "" || existingContentHash != contentHash ||
			existingSourceHash != g.sourceHashFor(output) {
			return false
		}
		// The header hashes alone would leave a hand-edited body in place for ever
		// (and --check failing): only a body that still matches its Content-Hash may
		// be skipped, otherwise generate repairs the file.
		existing, err := g.config.ReadExisting(absPath)
		return err == nil && !g.bodyEdited(string(existing), absPath)
	}
	existing, err := g.config.ReadExisting(absPath)
	if err != nil {
		return false
	}
	if g.config.ShowHeaderTimestamp() {
		return equalIgnoringHeaderStamp(string(existing), finalContent, output.Path)
	}
	return string(existing) == finalContent
}

// finalCarriesHash reports whether the rendered file holds a Content-Hash line.
// Formats with no comment syntax to carry one (strict JSON settings documents,
// merged MCP files) never do, so in "full" mode there is no stored hash to
// compare and canSkipWrite falls back to the whole rendered file.
func finalCarriesHash(finalContent string) bool {
	for _, line := range strings.Split(finalContent, "\n") {
		if c, _ := hashFromLine(line); c != "" {
			return true
		}
	}
	return false
}

// equalIgnoringHeaderStamp compares two renderings of outputPath, ignoring the
// Generated: stamp in the header only. Bodies must match exactly, so a body line
// that happens to start with "Generated: " is still compared.
func equalIgnoringHeaderStamp(existing, final, outputPath string) bool {
	existingBody, finalBody := stripHeader(existing, outputPath), stripHeader(final, outputPath)
	if existingBody != finalBody {
		return false
	}
	existingHeader := strings.TrimSuffix(existing, existingBody)
	finalHeader := strings.TrimSuffix(final, finalBody)
	return generatedStampPattern.ReplaceAllString(existingHeader, "") ==
		generatedStampPattern.ReplaceAllString(finalHeader, "")
}

// generatedStampPattern matches the header timestamp in both its inline
// (" | Generated: ...") and standalone ("Generated: ...") forms.
var generatedStampPattern = regexp.MustCompile(`(?m)(?: \| )?Generated: [^\n]*$`)

// writeRawOutput writes an OutputFile with non-nil RawContent verbatim,
// preserving Mode (defaulting to 0o644). Skips the write when both the
// existing bytes and mode already match on disk so unchanged bundled
// assets don't dirty the working tree on every regeneration.
//
// The bytes go to a temp file renamed into place, so a crash or a full disk never
// leaves a truncated script and a path hard-linked elsewhere is replaced, not
// written through. The mode is applied to a file this call creates, and to an
// existing regular file that is not reached through a symlink; a file reached
// through a link keeps the mode it has, since it is not ours.
func writeRawOutput(log logger.Logger, absPath string, viaLink bool, output config.OutputFile) error {
	mode := output.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if output.Sensitive {
		mode &= sensitiveFileMode
		if mode == 0 {
			mode = sensitiveFileMode
		}
	}

	if rawWriteCanSkip(absPath, output.RawContent, mode) {
		log.Debug("Skipped unchanged raw file", "path", output.Path)
		return nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}
	writeMode := mode
	if viaLink {
		if info, err := os.Stat(absPath); err == nil {
			writeMode = info.Mode().Perm()
		}
	}
	if err := config.WriteFileAtomic(absPath, output.RawContent, writeMode); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}
	log.Debug("Wrote raw file", "path", output.Path, "size", len(output.RawContent), "mode", writeMode)
	return nil
}

func rawWriteCanSkip(absPath string, payload []byte, mode os.FileMode) bool {
	return rawWriteCanSkipWith(os.ReadFile, os.Stat, absPath, payload, mode)
}

// rawWriteCanSkip is rawWriteCanSkip reading the file from the project's workspace.
func (g *Generator) rawWriteCanSkip(absPath string, payload []byte, mode os.FileMode) bool {
	return rawWriteCanSkipWith(g.config.ReadExisting, g.config.StatExisting, absPath, payload, mode)
}

func rawWriteCanSkipWith(read jsonmerge.Reader, stat func(string) (os.FileInfo, error), absPath string, payload []byte, mode os.FileMode) bool {
	existing, err := read(absPath)
	if err != nil || !bytes.Equal(existing, payload) {
		return false
	}
	info, err := stat(absPath)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true // Windows reports 0o666 for every file: the mode cannot drift
	}
	return info.Mode().Perm() == mode
}

// normalizeTrailingNewline ensures the file ends with exactly one '\n'.
// Empty content stays empty.
func normalizeTrailingNewline(content string) string {
	if content == "" {
		return content
	}
	trimmed := strings.TrimRight(content, "\n")
	return trimmed + "\n"
}
