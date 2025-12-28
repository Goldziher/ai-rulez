package templates

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type PrioritizedNamed interface {
	GetPriority() config.Priority
	GetName() string
}

type TitledItem interface {
	GetTitle() string
	GetPriority() config.Priority
}

func sortByTitlePriority[T TitledItem](items []T) {
	slices.SortFunc(items, func(a, b T) int {
		if a.GetPriority() != b.GetPriority() {
			return cmp.Compare(b.GetPriority().ToInt(), a.GetPriority().ToInt())
		}
		return cmp.Compare(a.GetTitle(), b.GetTitle())
	})
}

var (
	templateCache = sync.Map{}
	bufferPool    = sync.Pool{
		New: func() interface{} {
			return &strings.Builder{}
		},
	}
)

type contentItem struct {
	Type     string
	Title    string
	Priority int
	Content  string
	IsRule   bool
}

func (c contentItem) GetTitle() string             { return c.Title }
func (c contentItem) GetPriority() config.Priority { return config.IntToPriority(c.Priority) }
func (c contentItem) PriorityString() string       { return string(config.IntToPriority(c.Priority)) }

type TemplateData struct {
	ProjectName    string
	Version        string
	Description    string
	Rules          []config.Rule
	Sections       []config.Section
	Agents         []config.Agent
	MCPServers     []config.MCPServer
	Commands       []config.Command
	AllContent     []contentItem
	Timestamp      time.Time
	RuleCount      int
	SectionCount   int
	AgentCount     int
	MCPServerCount int
	CommandCount   int
	ConfigFile     string
	OutputFile     string
}

func NewTemplateData(cfg *config.Config) *TemplateData {
	return NewTemplateDataForOutput(cfg, "")
}

func NewTemplateDataForOutput(cfg *config.Config, outputPath string) *TemplateData {
	allRules := cfg.Rules
	allSections := cfg.Sections
	allAgents := cfg.Agents
	allMCPServers := cfg.MCPServers
	allCommands := cfg.Commands

	if outputPath != "" {
		allRules, allSections, allAgents = filterTargetedContent(cfg, outputPath, allRules, allSections, allAgents)
		allMCPServers, allCommands = filterMCPAndCommands(cfg, outputPath, allMCPServers, allCommands)
	} else {
		allMCPServers, allCommands = filterDisabledItems(allMCPServers, allCommands)
	}

	sortedRules := make([]config.Rule, len(allRules))
	copy(sortedRules, allRules)

	sortedSections := make([]config.Section, len(allSections))
	copy(sortedSections, allSections)

	allContent := make([]contentItem, 0, len(allRules)+len(allSections))

	for _, rule := range allRules {
		allContent = append(allContent, contentItem{
			Type:     "rule",
			Title:    rule.Name,
			Priority: rule.Priority.ToInt(),
			Content:  rule.Content,
			IsRule:   true,
		})
	}

	for _, section := range allSections {
		allContent = append(allContent, contentItem{
			Type:     "section",
			Title:    section.Name,
			Priority: section.Priority.ToInt(),
			Content:  section.Content,
			IsRule:   false,
		})
	}

	sortContent(allContent)

	slices.SortFunc(sortedRules, func(a, b config.Rule) int {
		if a.GetPriority() != b.GetPriority() {
			return cmp.Compare(b.GetPriority().ToInt(), a.GetPriority().ToInt())
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})

	slices.SortFunc(sortedSections, func(a, b config.Section) int {
		if a.GetPriority() != b.GetPriority() {
			return cmp.Compare(b.GetPriority().ToInt(), a.GetPriority().ToInt())
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})

	sortedAgents := make([]config.Agent, len(allAgents))
	copy(sortedAgents, allAgents)

	slices.SortFunc(sortedAgents, func(a, b config.Agent) int {
		if a.GetPriority() != b.GetPriority() {
			return cmp.Compare(b.GetPriority().ToInt(), a.GetPriority().ToInt())
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})

	sortedMCPServers := make([]config.MCPServer, len(allMCPServers))
	copy(sortedMCPServers, allMCPServers)

	sortedCommands := make([]config.Command, len(allCommands))
	copy(sortedCommands, allCommands)

	return &TemplateData{
		ProjectName:    cfg.Metadata.Name,
		Version:        cfg.Metadata.Version,
		Description:    cfg.Metadata.Description,
		Rules:          sortedRules,
		Sections:       sortedSections,
		Agents:         sortedAgents,
		MCPServers:     sortedMCPServers,
		Commands:       sortedCommands,
		AllContent:     allContent,
		Timestamp:      time.Now(),
		RuleCount:      len(allRules),
		SectionCount:   len(allSections),
		AgentCount:     len(allAgents),
		MCPServerCount: len(allMCPServers),
		CommandCount:   len(allCommands),
		OutputFile:     outputPath,
	}
}

type Renderer struct {
	templates map[string]*template.Template
}

func NewRenderer() *Renderer {
	r := &Renderer{
		templates: make(map[string]*template.Template),
	}

	r.registerBuiltinTemplates()

	return r
}

func (r *Renderer) Render(format string, data *TemplateData) (string, error) {
	if content, handled, err := RenderSpecialBuiltin(format, data); err != nil {
		return "", fmt.Errorf("failed to render %s template: %w", format, err)
	} else if handled {
		return content, nil
	}

	tmpl, exists := r.templates[format]
	if !exists {
		return "", fmt.Errorf("unknown template format: %s", format)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", format, err)
	}

	return buf.String(), nil
}

func (r *Renderer) RegisterTemplate(format, templateStr string) error {
	tmpl, err := template.New(format).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template for %s: %w", format, err)
	}

	r.templates[format] = tmpl
	return nil
}

func (r *Renderer) GetSupportedFormats() []string {
	formats := make([]string, 0, len(r.templates))
	for format := range r.templates {
		formats = append(formats, format)
	}
	return formats
}

func (r *Renderer) registerBuiltinTemplates() {
	builtinTemplates := getBuiltinTemplates()

	for name, template := range builtinTemplates {
		if err := r.RegisterTemplate(name, template); err != nil {
			panic(fmt.Sprintf("Failed to register %s template: %v", name, err))
		}
	}
}

func ValidateTemplate(templateStr string) error {
	_, err := template.New("validation").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}
	return nil
}

func RenderString(templateStr string, data *TemplateData) (string, error) {
	hash := sha256.Sum256([]byte(templateStr))
	cacheKey := fmt.Sprintf("inline_%x", hash[:8])

	var tmpl *template.Template
	if cached, ok := templateCache.Load(cacheKey); ok {
		tmpl = cached.(*template.Template)
	} else {
		parsed, err := template.New("inline").Parse(templateStr)
		if err != nil {
			return "", fmt.Errorf("failed to parse template: %w", err)
		}
		templateCache.Store(cacheKey, parsed)
		tmpl = parsed
	}

	buf := bufferPool.Get().(*strings.Builder)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	if err := tmpl.Execute(buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func ExecuteTemplate(templateStr string, data interface{}) (string, error) {
	hash := sha256.Sum256([]byte(templateStr))
	cacheKey := fmt.Sprintf("exec_%x", hash[:8])

	var tmpl *template.Template
	if cached, ok := templateCache.Load(cacheKey); ok {
		tmpl = cached.(*template.Template)
	} else {
		parsed, err := template.New("inline").Parse(templateStr)
		if err != nil {
			return "", fmt.Errorf("failed to parse template: %w", err)
		}
		templateCache.Store(cacheKey, parsed)
		tmpl = parsed
	}

	buf := bufferPool.Get().(*strings.Builder)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	if err := tmpl.Execute(buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
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

func sortContent(items []contentItem) {
	sortByTitlePriority(items)
}

// filterTargetedContent filters rules, sections, and agents for a specific output path
func filterTargetedContent(cfg *config.Config, outputPath string, allRules []config.Rule, allSections []config.Section, allAgents []config.Agent) ([]config.Rule, []config.Section, []config.Agent) {
	var err error
	allRules, err = config.FilterRules(allRules, outputPath, nil)
	if err != nil {
		allRules = cfg.Rules
	}

	allSections, err = config.FilterSections(allSections, outputPath, nil)
	if err != nil {
		allSections = cfg.Sections
	}

	allAgents, err = config.FilterAgents(allAgents, outputPath, nil)
	if err != nil {
		allAgents = cfg.Agents
	}

	return allRules, allSections, allAgents
}

// filterMCPAndCommands filters MCP servers and commands for a specific output path
func filterMCPAndCommands(cfg *config.Config, outputPath string, allMCPServers []config.MCPServer, allCommands []config.Command) ([]config.MCPServer, []config.Command) {
	var err error
	allMCPServers, err = config.FilterMCPServers(allMCPServers, outputPath, nil)
	if err != nil {
		allMCPServers = cfg.MCPServers
	}

	// For MCP templates, include all servers (disabled ones need to be marked as disabled)
	// For regular templates, filter out disabled servers
	if !isMCPTemplate(outputPath, cfg) {
		allMCPServers = filterDisabledMCPServers(allMCPServers)
	}

	allCommands, err = config.FilterCommands(allCommands, outputPath, nil)
	if err != nil {
		allCommands = cfg.Commands
	}

	// For command-aware templates, include all commands (disabled ones need to be marked)
	// For regular templates, filter out disabled commands
	if !isCommandStatusTemplate(outputPath, cfg) {
		allCommands = filterDisabledCommands(allCommands)
	}

	return allMCPServers, allCommands
}

// filterDisabledItems filters out disabled MCP servers and commands when no specific output path is given
func filterDisabledItems(allMCPServers []config.MCPServer, allCommands []config.Command) ([]config.MCPServer, []config.Command) {
	allMCPServers = filterDisabledMCPServers(allMCPServers)
	allCommands = filterDisabledCommands(allCommands)
	return allMCPServers, allCommands
}

// filterDisabledMCPServers removes disabled MCP servers from the slice
func filterDisabledMCPServers(servers []config.MCPServer) []config.MCPServer {
	enabledServers := make([]config.MCPServer, 0, len(servers))
	for i := range servers {
		if servers[i].IsEnabled() {
			enabledServers = append(enabledServers, servers[i])
		}
	}
	return enabledServers
}

// filterDisabledCommands removes disabled commands from the slice
func filterDisabledCommands(commands []config.Command) []config.Command {
	enabledCommands := make([]config.Command, 0, len(commands))
	for i := range commands {
		if commands[i].IsEnabled() {
			enabledCommands = append(enabledCommands, commands[i])
		}
	}
	return enabledCommands
}

// isMCPTemplate checks if the given output path corresponds to an MCP template
func isMCPTemplate(outputPath string, cfg *config.Config) bool {
	if outputPath == "" || cfg == nil {
		return false
	}

	// Find the output with this path
	for _, output := range cfg.Outputs {
		if output.Path == outputPath {
			tmpl, err := output.GetTemplate()
			if err != nil || tmpl == nil {
				return false
			}

			// Check if this is an MCP template
			switch tmpl.Value {
			case "claude-code-mcp", "cursor-mcp", "windsurf-mcp", "vscode-mcp", "continuedev-mcp", "cline-mcp", "gemini-mcp":
				return true
			}
		}
	}

	return false
}

// isCommandStatusTemplate checks if the template displays command status information
func isCommandStatusTemplate(outputPath string, cfg *config.Config) bool {
	if outputPath == "" || cfg == nil {
		return false
	}

	// Find the output with this path
	for _, output := range cfg.Outputs {
		if output.Path == outputPath {
			tmpl, err := output.GetTemplate()
			if err != nil || tmpl == nil {
				return false
			}

			// Check if template contains references to command status
			if strings.Contains(tmpl.Value, ".IsEnabled") || strings.Contains(tmpl.Value, "Enabled:") {
				return true
			}
		}
	}

	return false
}

func buildHeaderLines(data *TemplateData) []string {
	configPath := strings.TrimSpace(data.ConfigFile)
	if configPath == "" {
		configPath = "ai-rulez.yaml"
	}

	outputPath := strings.TrimSpace(data.OutputFile)
	if outputPath == "" {
		outputPath = "(preview output)"
	}

	timestamp := data.Timestamp.Format("2006-01-02 15:04:05")

	lines := []string{
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY",
		"Project: " + data.ProjectName,
		"Generated: " + timestamp,
		"Source of truth: " + configPath,
		"Target file: " + outputPath,
		"Content summary: rules=" + fmt.Sprint(data.RuleCount) + ", sections=" + fmt.Sprint(data.SectionCount) + ", agents=" + fmt.Sprint(data.AgentCount),
		"",
		"UPDATE WORKFLOW",
		"1. Modify " + configPath,
		"2. Run `ai-rulez generate` to refresh generated files",
		"3. Commit regenerated outputs together with the config changes",
		"",
		"AI ASSISTANT SAFEGUARDS",
		"- Treat " + configPath + " as the canonical configuration",
		"- Never overwrite " + outputPath + " manually; regenerate instead",
		"- Surface changes as patches to " + configPath + " (include doc/test updates)",
		"",
		"Need help? /capability-plan or https://github.com/Goldziher/ai-rulez",
	}

	return lines
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
