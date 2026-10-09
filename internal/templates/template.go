package templates

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/zeebo/blake3"
)

type TemplateData struct {
	ProjectName   string
	Timestamp     time.Time
	RuleCount     int
	SectionCount  int
	AgentCount    int
	ConfigFile    string
	OutputFile    string
	Config        *config.Config // Config for accessing header style
	StyleOverride string         // If set, overrides Config.GetHeaderStyle()
}

// HashContent computes a BLAKE3 hash of the given content and returns it
// in the format "blake3:<hex>".
func HashContent(content string) string {
	h := blake3.Sum256([]byte(content))
	return fmt.Sprintf("blake3:%x", h)
}

// ContentHasher computes HashContent of a text written to it in pieces, without
// holding the text: the same digest as HashContent of the pieces joined.
type ContentHasher struct{ h *blake3.Hasher }

// NewContentHasher starts an empty hash.
func NewContentHasher() *ContentHasher { return &ContentHasher{h: blake3.New()} }

// WriteString adds s to the hashed text.
func (c *ContentHasher) WriteString(s string) { _, _ = c.h.WriteString(s) } //nolint:errcheck // a hash never fails

// Printf adds the formatted text to the hashed text.
func (c *ContentHasher) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.h, format, args...) //nolint:errcheck // a hash never fails
}

// WriteBytes adds p to the hashed text.
func (c *ContentHasher) WriteBytes(p []byte) { _, _ = c.h.Write(p) } //nolint:errcheck // a hash never fails

// Sum returns the digest in the format of HashContent.
func (c *ContentHasher) Sum() string { return fmt.Sprintf("blake3:%x", c.h.Sum(nil)) }

// GeneratorSchemaVersion bumps when the rendering logic changes in a way that
// would alter output for the same source inputs (e.g., template restructuring,
// header layout changes, fixed bugs in serialization). Bumping invalidates all
// stored Source-Hash values, forcing one round of regeneration before the skip
// mechanism can re-engage.
const GeneratorSchemaVersion = "v8"

type commentStyle int

const (
	commentStyleHTML commentStyle = iota
	commentStyleHash
	commentStyleSlash
	commentStyleSemicolon
)

func GenerateHeader(data *TemplateData) string {
	lines := buildHeaderLines(data)
	style := determineCommentStyle(data.OutputFile)

	switch style {
	case commentStyleHTML:
		return wrapWithHTMLComment(lines)
	case commentStyleSlash:
		return wrapWithLinePrefix(lines, "// ")
	case commentStyleSemicolon:
		return wrapWithLinePrefix(lines, "; ")
	default:
		return wrapWithLinePrefix(lines, "# ")
	}
}

func determineCommentStyle(outputPath string) commentStyle {
	ext := strings.ToLower(filepath.Ext(outputPath))

	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		return commentStyleHTML
	case ".json", ".jsonc":
		return commentStyleSlash
	case ".ini":
		return commentStyleSemicolon
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cc", ".cpp", ".cs":
		return commentStyleSlash
	default:
		return commentStyleHash
	}
}

// timestampSuffix renders the inline "| Generated: …" fragment used by the
// compact header, collapsing to nothing when timestamps are disabled.
func timestampSuffix(timestamp string) string {
	if timestamp == "" {
		return ""
	}
	return " | Generated: " + timestamp
}

// timestampLines renders the standalone "Generated: …" banner line, or no line
// at all when timestamps are disabled.
func timestampLines(timestamp string) []string {
	if timestamp == "" {
		return nil
	}
	return []string{"Generated: " + timestamp}
}

// configDirLabel returns the project-relative config directory to show in
// generated headers (e.g. ".ai-rulez" or ".config/ai-rulez"), defaulting to
// ".ai-rulez" when the config carries no directory name (previews, V2 configs).
func configDirLabel(data *TemplateData) string {
	if data != nil && data.Config != nil {
		if name := strings.TrimSpace(data.Config.ConfigDirName); name != "" {
			return filepath.ToSlash(name)
		}
	}
	return ".ai-rulez"
}

func buildDetailedHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	dir := configDirLabel(data) + "/"
	banner := slices.Concat([]string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY",
		"Project: " + data.ProjectName,
	}, timestampLines(timestamp), []string{
		"Source: " + dir + configPath,
		"Target: " + outputPath,
		"Content: rules=" + fmt.Sprint(data.RuleCount) + ", sections=" + fmt.Sprint(data.SectionCount) + ", agents=" + fmt.Sprint(data.AgentCount),
	})

	banner = append(banner, "",
		"WHAT IS AI-RULEZ",
		"AI-Rulez is a directory-based AI governance tool. All configuration lives in",
		"the "+dir+" directory. This file is auto-generated from source files.",
		"",
		".AI-RULEZ FOLDER ORGANIZATION",
		"Root content (always included):",
		"  "+dir+"config.toml    Main configuration (presets, profiles)",
		"  "+dir+"rules/         Mandatory rules for AI assistants",
		"  "+dir+"context/       Reference documentation",
		"  "+dir+"skills/        Specialized AI prompts",
		"  "+dir+"agents/        Agent definitions",
		"",
		"Domain content (profile-specific):",
		"  "+dir+"domains/{name}/rules/    Domain-specific rules",
		"  "+dir+"domains/{name}/context/  Domain-specific documentation",
		"  "+dir+"domains/{name}/skills/   Domain-specific AI prompts",
		"",
		"Profiles in config.toml control which domains are included.",
		"",
		"INSTRUCTIONS FOR AI AGENTS",
		"1. NEVER edit this file ("+outputPath+") - it is auto-generated",
		"",
		"2. ALWAYS edit files in "+dir+" instead:",
		"   - Add/modify rules: "+dir+"rules/*.md",
		"   - Add/modify context: "+dir+"context/*.md",
		"   - Update config: "+dir+"config.toml",
		"   - Domain-specific: "+dir+"domains/{name}/rules/*.md",
		"",
		"3. PREFER using the MCP Server (if available):",
		"   Command: npx -y ai-rulez@latest mcp",
		"   Provides safe CRUD tools for reading and modifying "+dir+" content",
		"",
		"4. After making changes: ai-rulez generate",
		"",
		"5. Complete workflow:",
		"   a. Edit source files in "+dir,
		"   b. Run: ai-rulez generate",
		"   c. Commit both "+dir+" and generated files",
		"",
		"Documentation: https://github.com/Goldziher/ai-rulez",
	)
	return banner
}

func buildCompactHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	dir := configDirLabel(data) + "/"
	banner := []string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT",
		"Project: " + data.ProjectName + timestampSuffix(timestamp),
		"Source: " + dir + configPath + " | Target: " + outputPath,
		"Content: " + fmt.Sprintf("rules=%d, sections=%d, agents=%d", data.RuleCount, data.SectionCount, data.AgentCount),
	}

	banner = append(banner,
		"",
		"WHAT IS AI-RULEZ: Directory-based AI governance. Config in "+dir,
		"",
		"STRUCTURE:",
		"  "+dir+"config.toml, rules/, context/, skills/, agents/ (root)",
		"  "+dir+"domains/{name}/ (profile-specific)",
		"",
		"AI AGENT INSTRUCTIONS:",
		"✗ NEVER edit "+outputPath+" (auto-generated)",
		"✓ EDIT "+dir+"rules/*.md, "+dir+"context/*.md, "+dir+"config.toml",
		"✓ USE MCP server: npx -y ai-rulez@latest mcp (provides CRUD tools)",
		"✓ REGENERATE: ai-rulez generate",
		"✓ COMMIT: both "+dir+" and generated files",
		"",
		"Docs: https://github.com/Goldziher/ai-rulez",
	)
	return banner
}

func buildMinimalHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	dir := configDirLabel(data) + "/"
	banner := slices.Concat([]string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT",
		"Project: " + data.ProjectName,
	}, timestampLines(timestamp), []string{
		"Source: " + dir + configPath,
	})

	banner = append(banner,
		"",
		"NEVER edit this file - modify "+dir+" content instead",
		"Use MCP server: npx -y ai-rulez@latest mcp",
		"Regenerate: ai-rulez generate",
		"",
		"Docs: https://github.com/Goldziher/ai-rulez",
	)
	return banner
}

func buildHeaderLines(data *TemplateData) []string {
	configPath := strings.TrimSpace(data.ConfigFile)
	if configPath == "" {
		configPath = "config.toml"
	}

	outputPath := strings.TrimSpace(data.OutputFile)
	if outputPath == "" {
		outputPath = "(preview output)"
	}

	// An author-supplied header (header.text) replaces the prose generated from
	// header.style. It is wrapped in the output's comment syntax by
	// GenerateHeader and still has the freshness lines injected, so the style
	// is irrelevant when text is set.
	if data.Config != nil {
		if custom := data.Config.Header.GetCustomHeader(); strings.TrimSpace(custom) != "" {
			return splitHeaderLines(custom)
		}
	}

	// An empty timestamp means the "Generated:" line is omitted entirely, which is
	// the default: a per-run value makes output non-reproducible and lets two
	// otherwise identical sibling files disagree. A nil Config takes the same
	// default, so the header a preview renders matches the one generation writes.
	timestamp := ""
	if data.Config.ShowHeaderTimestamp() {
		timestamp = data.Timestamp.Format("2006-01-02 15:04:05")
	}

	headerStyle := "detailed"
	if data.StyleOverride != "" {
		headerStyle = data.StyleOverride
	} else if data.Config != nil {
		headerStyle = data.Config.GetHeaderStyle()
	}

	switch headerStyle {
	case "compact":
		return buildCompactHeader(configPath, outputPath, timestamp, data)
	case "minimal":
		return buildMinimalHeader(configPath, outputPath, timestamp, data)
	default:
		return buildDetailedHeader(configPath, outputPath, timestamp, data)
	}
}

// splitHeaderLines turns author-supplied header text into the line slice the
// comment-style wrappers consume. Line endings are normalized and surrounding
// blank lines trimmed so a TOML `"""` block does not introduce stray newlines,
// while interior indentation and blank lines are preserved verbatim.
func splitHeaderLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.Trim(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func wrapWithHTMLComment(lines []string) string {
	var builder strings.Builder
	builder.WriteString("<!--\n")
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteString("-->\n\n")
	return builder.String()
}

func wrapWithLinePrefix(lines []string, prefix string) string {
	if prefix == "" {
		prefix = "# "
	}

	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(prefix)
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
	return builder.String()
}

// RuleBanner returns the one-line banner for a generated rule file, wrapped as
// a multi-line HTML comment so the generator can inject its hash lines before
// the closing marker:
//
//	<!--
//	Generated by ai-rulez from <source>. Edit the source, not this file.
//	-->
func RuleBanner(sourcePath string) string {
	line := "Generated by ai-rulez"
	if sourcePath = sanitizeBannerText(sourcePath); sourcePath != "" {
		line += " from " + sourcePath
	}
	line += ". Edit the source, not this file."
	return wrapWithHTMLComment([]string{line})
}

// sanitizeBannerText keeps a path from breaking out of the HTML comment: line
// breaks become spaces and "--" is split so "-->" can never appear.
func sanitizeBannerText(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	return s
}
