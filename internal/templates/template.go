package templates

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
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

func buildDetailedHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	banner := []string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY",
		"Project: " + data.ProjectName,
		"Generated: " + timestamp,
		"Source: .ai-rulez/" + configPath,
		"Target: " + outputPath,
		"Content: rules=" + fmt.Sprint(data.RuleCount) + ", sections=" + fmt.Sprint(data.SectionCount) + ", agents=" + fmt.Sprint(data.AgentCount),
	}

	banner = append(banner, "",
		"WHAT IS AI-RULEZ",
		"AI-Rulez is a directory-based AI governance tool. All configuration lives in",
		"the .ai-rulez/ directory. This file is auto-generated from source files.",
		"",
		".AI-RULEZ FOLDER ORGANIZATION",
		"Root content (always included):",
		"  .ai-rulez/config.yaml    Main configuration (presets, profiles)",
		"  .ai-rulez/rules/         Mandatory rules for AI assistants",
		"  .ai-rulez/context/       Reference documentation",
		"  .ai-rulez/skills/        Specialized AI prompts",
		"  .ai-rulez/agents/        Agent definitions",
		"",
		"Domain content (profile-specific):",
		"  .ai-rulez/domains/{name}/rules/    Domain-specific rules",
		"  .ai-rulez/domains/{name}/context/  Domain-specific documentation",
		"  .ai-rulez/domains/{name}/skills/   Domain-specific AI prompts",
		"",
		"Profiles in config.yaml control which domains are included.",
		"",
		"INSTRUCTIONS FOR AI AGENTS",
		"1. NEVER edit this file ("+outputPath+") - it is auto-generated",
		"",
		"2. ALWAYS edit files in .ai-rulez/ instead:",
		"   - Add/modify rules: .ai-rulez/rules/*.md",
		"   - Add/modify context: .ai-rulez/context/*.md",
		"   - Update config: .ai-rulez/config.yaml",
		"   - Domain-specific: .ai-rulez/domains/{name}/rules/*.md",
		"",
		"3. PREFER using the MCP Server (if available):",
		"   Command: npx -y ai-rulez@latest mcp",
		"   Provides safe CRUD tools for reading and modifying .ai-rulez/ content",
		"",
		"4. After making changes: ai-rulez generate",
		"",
		"5. Complete workflow:",
		"   a. Edit source files in .ai-rulez/",
		"   b. Run: ai-rulez generate",
		"   c. Commit both .ai-rulez/ and generated files",
		"",
		"Documentation: https://github.com/Goldziher/ai-rulez",
	)
	return banner
}

func buildCompactHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	banner := []string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT",
		"Project: " + data.ProjectName + " | Generated: " + timestamp,
		"Source: .ai-rulez/" + configPath + " | Target: " + outputPath,
		"Content: " + fmt.Sprintf("rules=%d, sections=%d, agents=%d", data.RuleCount, data.SectionCount, data.AgentCount),
	}

	banner = append(banner,
		"",
		"WHAT IS AI-RULEZ: Directory-based AI governance. Config in .ai-rulez/",
		"",
		"STRUCTURE:",
		"  .ai-rulez/config.yaml, rules/, context/, skills/, agents/ (root)",
		"  .ai-rulez/domains/{name}/ (profile-specific)",
		"",
		"AI AGENT INSTRUCTIONS:",
		"✗ NEVER edit "+outputPath+" (auto-generated)",
		"✓ EDIT .ai-rulez/rules/*.md, .ai-rulez/context/*.md, .ai-rulez/config.yaml",
		"✓ USE MCP server: npx -y ai-rulez@latest mcp (provides CRUD tools)",
		"✓ REGENERATE: ai-rulez generate",
		"✓ COMMIT: both .ai-rulez/ and generated files",
		"",
		"Docs: https://github.com/Goldziher/ai-rulez",
	)
	return banner
}

func buildMinimalHeader(configPath, outputPath, timestamp string, data *TemplateData) []string {
	banner := []string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT",
		"Project: " + data.ProjectName,
		"Generated: " + timestamp,
		"Source: .ai-rulez/" + configPath,
	}

	banner = append(banner,
		"",
		"NEVER edit this file - modify .ai-rulez/ content instead",
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
		configPath = "config.yaml"
	}

	outputPath := strings.TrimSpace(data.OutputFile)
	if outputPath == "" {
		outputPath = "(preview output)"
	}

	timestamp := data.Timestamp.Format("2006-01-02 15:04:05")

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
