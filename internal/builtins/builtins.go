package builtins

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/parser"
)

//go:embed universal languages bindings
var content embed.FS

// Category represents a builtin category
type Category string

const (
	CategoryUniversal Category = "universal"
	CategoryLanguage  Category = "language"
	CategoryBinding   Category = "binding"
)

// BuiltinDomain describes an available builtin domain
type BuiltinDomain struct {
	Name        string
	Category    Category
	AutoInclude bool
	Description string
}

// autoIncludeDomains are included by default unless explicitly excluded with "!" prefix
var autoIncludeDomains = map[string]bool{
	"ai-governance":    true,
	"agent-delegation": true,
	"code-quality":     true,
	"testing":          true,
	"git-workflow":     true,
	"security":         true,
	"token-efficiency": true,
}

// registry maps builtin names to their metadata
var registry = map[string]BuiltinDomain{
	// Universal
	"ai-governance":    {Name: "ai-governance", Category: CategoryUniversal, AutoInclude: true, Description: "AI agent behavior governance"},
	"security":         {Name: "security", Category: CategoryUniversal, AutoInclude: true, Description: "Security best practices and OWASP reference"},
	"git-workflow":     {Name: "git-workflow", Category: CategoryUniversal, AutoInclude: true, Description: "Git workflow and commit conventions"},
	"code-quality":     {Name: "code-quality", Category: CategoryUniversal, AutoInclude: true, Description: "Code readability, error handling, and complexity"},
	"testing":          {Name: "testing", Category: CategoryUniversal, AutoInclude: true, Description: "Testing conventions and best practices"},
	"token-efficiency": {Name: "token-efficiency", Category: CategoryUniversal, AutoInclude: true, Description: "Output efficiency and task automation"},
	"documentation":    {Name: "documentation", Category: CategoryUniversal, Description: "Documentation standards and maintenance"},
	"default-commands": {Name: "default-commands", Category: CategoryUniversal, Description: "Built-in slash commands (/iterate, /parallelize)"},
	"agent-delegation": {Name: "agent-delegation", Category: CategoryUniversal, AutoInclude: true, Description: "Agent delegation instructions and listing in generated outputs"},
	"cicd":             {Name: "cicd", Category: CategoryUniversal, Description: "CI/CD pipeline standards and GitHub workflow conventions"},
	"observability":    {Name: "observability", Category: CategoryUniversal, Description: "Logging, metrics, health checks, and observability standards"},
	"docker":           {Name: "docker", Category: CategoryUniversal, Description: "Container build, security, and deployment best practices"},

	// Languages
	"rust":       {Name: "rust", Category: CategoryLanguage, Description: "Rust language conventions"},
	"python":     {Name: "python", Category: CategoryLanguage, Description: "Python language conventions"},
	"typescript": {Name: "typescript", Category: CategoryLanguage, Description: "TypeScript language conventions"},
	"go":         {Name: "go", Category: CategoryLanguage, Description: "Go language conventions"},
	"java":       {Name: "java", Category: CategoryLanguage, Description: "Java language conventions"},
	"ruby":       {Name: "ruby", Category: CategoryLanguage, Description: "Ruby language conventions"},
	"php":        {Name: "php", Category: CategoryLanguage, Description: "PHP language conventions"},
	"elixir":     {Name: "elixir", Category: CategoryLanguage, Description: "Elixir language conventions"},
	"csharp":     {Name: "csharp", Category: CategoryLanguage, Description: "C# language conventions"},
	"r":          {Name: "r", Category: CategoryLanguage, Description: "R language conventions"},

	// Bindings
	"pyo3":       {Name: "pyo3", Category: CategoryBinding, Description: "PyO3 Rust-Python bindings"},
	"napi-rs":    {Name: "napi-rs", Category: CategoryBinding, Description: "NAPI-RS Rust-Node.js bindings"},
	"magnus":     {Name: "magnus", Category: CategoryBinding, Description: "Magnus Rust-Ruby bindings"},
	"ext-php-rs": {Name: "ext-php-rs", Category: CategoryBinding, Description: "ext-php-rs Rust-PHP bindings"},
	"rustler":    {Name: "rustler", Category: CategoryBinding, Description: "Rustler Rust-Elixir NIF bindings"},
	"wasm":       {Name: "wasm", Category: CategoryBinding, Description: "WebAssembly/wasm-bindgen bindings"},
	"jni-rs":     {Name: "jni-rs", Category: CategoryBinding, Description: "JNI Rust-Java/JVM bindings"},
	"extendr":    {Name: "extendr", Category: CategoryBinding, Description: "extendr Rust-R bindings"},
	"cgo":        {Name: "cgo", Category: CategoryBinding, Description: "cgo Go-C/Rust FFI bindings"},
	"vite-plus":  {Name: "vite-plus", Category: CategoryBinding, Description: "Vite+ unified TypeScript toolchain"},
}

// Get returns the BuiltinDomain for the given name, or false if not found.
func Get(name string) (BuiltinDomain, bool) {
	d, ok := registry[name]
	return d, ok
}

// IsValid returns true if the given name is a valid builtin (ignoring "!" prefix)
func IsValid(name string) bool {
	clean := strings.TrimPrefix(name, "!")
	_, ok := registry[clean]
	return ok
}

// IsAutoInclude returns true if the domain is auto-included by default
func IsAutoInclude(name string) bool {
	return autoIncludeDomains[name]
}

// List returns all available builtin domains sorted by category then name
func List() []BuiltinDomain {
	domains := make([]BuiltinDomain, 0, len(registry))
	for _, d := range registry {
		domains = append(domains, d)
	}
	sort.Slice(domains, func(i, j int) bool {
		if domains[i].Category != domains[j].Category {
			return domains[i].Category < domains[j].Category
		}
		return domains[i].Name < domains[j].Name
	})
	return domains
}

// ResolveAll returns all available builtin names sorted.
func ResolveAll() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveBuiltins takes the user's builtins list and returns the final set of builtin names to load.
// Auto-includes are added unless explicitly excluded with "!" prefix.
func ResolveBuiltins(builtins []string) []string {
	// Track explicit exclusions
	excluded := make(map[string]bool)
	explicit := make(map[string]bool)

	for _, name := range builtins {
		if strings.HasPrefix(name, "!") {
			excluded[strings.TrimPrefix(name, "!")] = true
		} else {
			explicit[name] = true
		}
	}

	// Start with auto-includes (unless excluded)
	result := make(map[string]bool)
	for name := range autoIncludeDomains {
		if !excluded[name] {
			result[name] = true
		}
	}

	// Add explicit includes
	for name := range explicit {
		if !excluded[name] {
			result[name] = true
		}
	}

	// Convert to sorted slice
	names := make([]string, 0, len(result))
	for name := range result {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// embeddedPath returns the embedded FS path for a builtin domain.
// NOTE: embed.FS always uses forward slashes for paths, even on Windows,
// so we must use the path package (not filepath) here.
func embeddedPath(name string) string {
	if d, ok := registry[name]; ok {
		switch d.Category {
		case CategoryUniversal:
			return path.Join("universal", name)
		case CategoryLanguage:
			return path.Join("languages", name)
		case CategoryBinding:
			return path.Join("bindings", name)
		}
	}
	return ""
}

// ContentEntry represents a single content file from the embedded FS
type ContentEntry struct {
	Name     string // filename without extension
	Path     string // relative path within the domain
	Content  string // file content
	Type     string // "rules", "context", "skills", "agents", "commands"
	Priority string // parsed from frontmatter if present
}

// LoadDomainContent loads all content files for a given builtin domain name.
// Returns nil if the domain doesn't exist or has no content.
func LoadDomainContent(name string) ([]ContentEntry, error) {
	basePath := embeddedPath(name)
	if basePath == "" {
		return nil, nil
	}

	var entries []ContentEntry

	contentTypes := []string{"rules", "context", "skills", "agents", "commands"}
	for _, contentType := range contentTypes {
		// Use path.Join (forward slashes) so embed.FS can find files on all platforms.
		dirPath := path.Join(basePath, contentType)
		dirEntries, err := fs.ReadDir(content, dirPath)
		if err != nil {
			// Directory doesn't exist — skip
			continue
		}

		for _, entry := range dirEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			filePath := path.Join(dirPath, entry.Name())
			data, err := content.ReadFile(filePath)
			if err != nil {
				continue
			}

			// All builtin content files use .md extension; trim it directly.
			entryName := strings.TrimSuffix(entry.Name(), ".md")
			rawContent := string(data)

			// Parse frontmatter for priority
			var priority string
			meta, _ := parser.ParseFrontmatterNonFatal(rawContent)
			if meta != nil && meta.Priority != "" {
				priority = meta.Priority
			}

			entries = append(entries, ContentEntry{
				Name:     entryName,
				Path:     filePath,
				Content:  rawContent,
				Type:     contentType,
				Priority: priority,
			})
		}
	}

	return entries, nil
}
