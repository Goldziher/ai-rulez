package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/parser"
	"github.com/samber/oops"
)

// Marker files that make a directory a skill or a command.
const (
	skillMarkerFile   = "SKILL.md"
	commandMarkerFile = "COMMAND.md"
)

// basenameKey keys rules, context and agents, which exist only as flat files and
// so compete for an output named after the file.
func basenameKey(file config.ContentFile) string {
	return filepath.Base(file.Path)
}

// nameKey keys skills and commands, which compete for an output named after the
// item rather than the file. Both support a flat and a directory form, and the
// two forms must produce the same key: commands/deploy.md and
// commands/deploy/COMMAND.md render to the same .claude/skills/deploy/SKILL.md,
// so keying one by its basename would hide the collision and emit both to that
// one path. Name is the filename stem for a flat item and the directory name for
// a directory one, which is exactly the identifier the generator resolves.
//
// Safe to call on root items only, which is all resolveCollisions keys: domain
// items have their Name prefixed with the domain by applyNamespacing, after the
// collision map is built.
func nameKey(file config.ContentFile) string {
	return file.Name
}

// Scanner scans content from .ai-rulez/ directories and builds an in-memory content tree
type Scanner struct {
	baseDir string
	config  *config.Config
}

// NewScanner creates a new Scanner for the given base directory and config
func NewScanner(baseDir string, cfg *config.Config) *Scanner {
	return &Scanner{
		baseDir: baseDir,
		config:  cfg,
	}
}

// ScanProfile scans content for a specific profile and returns a merged ContentTree
// This follows the design: root content + domain content with namespacing and collision handling
//
//nolint:gocyclo // Complex logic, acceptable for this use case
func (s *Scanner) ScanProfile(profileName string) (*config.ContentTree, error) {
	// Validate profile exists
	if profileName != "" && !s.config.HasProfile(profileName) {
		return nil, oops.
			With("profile", profileName).
			With("available_profiles", getProfileNames(s.config.Profiles)).
			Hint(fmt.Sprintf("Available profiles: %s\nCreate the profile in config.yaml or use an existing profile", strings.Join(getProfileNames(s.config.Profiles), ", "))).
			Errorf("profile not found: %s", profileName)
	}

	// Get domains for this profile
	domains := s.config.GetProfileDomains(profileName)

	// Validate all domains exist
	if err := s.validateDomains(domains); err != nil {
		return nil, err
	}

	aiRulezDir := filepath.Join(s.baseDir, ".ai-rulez")

	// Scan root content
	rootRules, err := s.scanMarkdownFiles(filepath.Join(aiRulezDir, "rules"))
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "rules")).
			Wrapf(err, "scan root rules directory")
	}

	rootContext, err := s.scanMarkdownFiles(filepath.Join(aiRulezDir, "context"))
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "context")).
			Wrapf(err, "scan root context directory")
	}

	rootSkills, err := s.scanSkills(filepath.Join(aiRulezDir, "skills"), "")
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "skills")).
			Wrapf(err, "scan root skills directory")
	}

	rootAgents, err := s.scanMarkdownFiles(filepath.Join(aiRulezDir, "agents"))
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "agents")).
			Wrapf(err, "scan root agents directory")
	}

	rootCommands, err := s.scanCommands(filepath.Join(aiRulezDir, "commands"))
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "commands")).
			Wrapf(err, "scan root commands directory")
	}
	logger.Info("Scanned root commands", "count", len(rootCommands))

	// Build collision tracking maps (key -> [root, domain1, domain2, ...]).
	// Each map is populated through the same key function resolveCollisions later
	// reads it with, so the two sides cannot drift apart.
	rulesMap := make(map[string][]string)
	contextMap := make(map[string][]string)
	skillsMap := make(map[string][]string)
	agentsMap := make(map[string][]string)
	commandsMap := make(map[string][]string)

	trackSources := func(into map[string][]string, files []config.ContentFile, key func(config.ContentFile) string, source string) {
		for _, file := range files {
			itemKey := key(file)
			into[itemKey] = append(into[itemKey], source)
		}
	}

	// Track root files
	trackSources(rulesMap, rootRules, basenameKey, "root")
	trackSources(contextMap, rootContext, basenameKey, "root")
	trackSources(skillsMap, rootSkills, nameKey, "root")
	trackSources(agentsMap, rootAgents, basenameKey, "root")
	trackSources(commandsMap, rootCommands, nameKey, "root")

	// Scan domains and apply namespacing
	domainMap := make(map[string]*config.Domain)
	allRules := make([]config.ContentFile, 0)
	allContext := make([]config.ContentFile, 0)
	allSkills := make([]config.ContentFile, 0)
	allAgents := make([]config.ContentFile, 0)
	allCommands := make([]config.ContentFile, 0)

	for _, domainName := range domains {
		domainPath := filepath.Join(aiRulezDir, "domains", domainName)

		// Scan domain content
		domainRules, err := s.scanMarkdownFiles(filepath.Join(domainPath, "rules"))
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "rules")).
				Wrapf(err, "scan domain rules directory")
		}

		domainContext, err := s.scanMarkdownFiles(filepath.Join(domainPath, "context"))
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "context")).
				Wrapf(err, "scan domain context directory")
		}

		domainSkills, err := s.scanSkills(filepath.Join(domainPath, "skills"), domainName)
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "skills")).
				Wrapf(err, "scan domain skills directory")
		}

		domainAgents, err := s.scanMarkdownFiles(filepath.Join(domainPath, "agents"))
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "agents")).
				Wrapf(err, "scan domain agents directory")
		}

		domainCommands, err := s.scanCommands(filepath.Join(domainPath, "commands"))
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "commands")).
				Wrapf(err, "scan domain commands directory")
		}
		logger.Debug("Scanned domain commands", "domain", domainName, "count", len(domainCommands))

		// Track domain files for collision detection
		trackSources(rulesMap, domainRules, basenameKey, domainName)
		trackSources(contextMap, domainContext, basenameKey, domainName)
		trackSources(skillsMap, domainSkills, nameKey, domainName)
		trackSources(agentsMap, domainAgents, basenameKey, domainName)
		trackSources(commandsMap, domainCommands, nameKey, domainName)

		// Apply namespacing to domain content
		s.applyNamespacing(domainRules, domainName)
		s.applyNamespacing(domainContext, domainName)
		s.applySkillNamespacing(domainSkills, domainName)
		s.applyNamespacing(domainAgents, domainName)
		s.applyNamespacing(domainCommands, domainName)

		// Store domain
		domainMap[domainName] = &config.Domain{
			Name:     domainName,
			Rules:    domainRules,
			Context:  domainContext,
			Skills:   domainSkills,
			Agents:   domainAgents,
			Commands: domainCommands,
		}

		// Collect all domain content
		allRules = append(allRules, domainRules...)
		allContext = append(allContext, domainContext...)
		allSkills = append(allSkills, domainSkills...)
		allAgents = append(allAgents, domainAgents...)
		allCommands = append(allCommands, domainCommands...)
	}

	// Log collision warnings
	s.logCollisions(rulesMap, "rules")
	s.logCollisions(contextMap, "context")
	s.logCollisions(skillsMap, "skills")
	s.logCollisions(agentsMap, "agents")
	s.logCollisions(commandsMap, "commands")

	// Handle collisions: domain content overrides root content
	finalRules := s.resolveCollisions(rootRules, allRules, rulesMap, basenameKey)
	finalContext := s.resolveCollisions(rootContext, allContext, contextMap, basenameKey)
	finalSkills := s.resolveCollisions(rootSkills, allSkills, skillsMap, nameKey)
	finalAgents := s.resolveCollisions(rootAgents, allAgents, agentsMap, basenameKey)
	finalCommands := s.resolveCollisions(rootCommands, allCommands, commandsMap, nameKey)

	// Sort alphabetically by name for stable, idempotent output.
	// Priority is preserved in each item's metadata and rendered alongside the name.
	sortByName(finalRules)
	sortByName(finalContext)
	sortByName(finalSkills)
	sortByName(finalAgents)
	sortByName(finalCommands)

	logger.Info("Final commands after collision resolution", "count", len(finalCommands))

	return &config.ContentTree{
		Rules:    finalRules,
		Context:  finalContext,
		Skills:   finalSkills,
		Agents:   finalAgents,
		Commands: finalCommands,
		Domains:  domainMap,
	}, nil
}

// validateDomains checks that all referenced domains exist in the filesystem
func (s *Scanner) validateDomains(domains []string) error {
	aiRulezDir := filepath.Join(s.baseDir, ".ai-rulez")
	domainsDir := filepath.Join(aiRulezDir, "domains")

	for _, domainName := range domains {
		domainPath := filepath.Join(domainsDir, domainName)
		if info, err := os.Stat(domainPath); err != nil {
			if os.IsNotExist(err) {
				return oops.
					With("domain", domainName).
					With("path", domainPath).
					Hint(fmt.Sprintf("Create the domain directory: mkdir -p %s\nOr remove the domain from the profile in config.yaml", domainPath)).
					Errorf("domain directory not found: %s", domainName)
			}
			return oops.
				With("domain", domainName).
				With("path", domainPath).
				Wrapf(err, "stat domain directory")
		} else if !info.IsDir() {
			return oops.
				With("domain", domainName).
				With("path", domainPath).
				Hint(fmt.Sprintf("Remove the file and create a directory: rm %s && mkdir -p %s", domainPath, domainPath)).
				Errorf("domain path exists but is not a directory: %s", domainName)
		}
	}

	return nil
}

// scanMarkdownFiles scans a directory for .md files (non-recursive)
func (s *Scanner) scanMarkdownFiles(dir string) ([]config.ContentFile, error) {
	// If directory doesn't exist, return empty slice (not an error)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []config.ContentFile{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, oops.
			With("path", dir).
			Wrapf(err, "read directory")
	}

	var files []config.ContentFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		contentFile, err := s.loadContentFile(filePath)
		if err != nil {
			// Log warning but continue
			logger.Warn("failed to load content file", "path", filePath, "error", err)
			continue
		}

		files = append(files, contentFile)
	}

	return files, nil
}

// scanSkills scans a skills directory for SKILL.md files in subdirectories
func (s *Scanner) scanSkills(skillsDir, domainName string) ([]config.ContentFile, error) {
	// If directory doesn't exist, return empty slice (not an error)
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return []config.ContentFile{}, nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, oops.
			With("path", skillsDir).
			Wrapf(err, "read skills directory")
	}

	var skills []config.ContentFile
	for _, entry := range entries {
		var skillPath string
		var contentFile config.ContentFile
		var err error

		if entry.IsDir() {
			// Directory structure: skills/name/SKILL.md
			skillPath = filepath.Join(skillsDir, entry.Name(), skillMarkerFile)
			if _, err := os.Stat(skillPath); os.IsNotExist(err) {
				// No SKILL.md file, skip this directory
				continue
			}

			contentFile, err = s.loadContentFile(skillPath)
			if err != nil {
				// Log warning but continue
				logger.Warn("failed to load skill file", "path", skillPath, "error", err)
				continue
			}

			// For skills in directories, use the directory name as the skill name
			contentFile.Name = entry.Name()
		} else {
			// Flat file structure: skills/name.md (for bare structure includes)
			if !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			skillPath = filepath.Join(skillsDir, entry.Name())
			contentFile, err = s.loadContentFile(skillPath)
			if err != nil {
				logger.Warn("failed to load skill file", "path", skillPath, "error", err)
				continue
			}

			// For flat skill files, use filename without extension as skill name (if not set in frontmatter)
			if contentFile.Name == "" {
				contentFile.Name = strings.TrimSuffix(entry.Name(), ".md")
			}
		}

		skills = append(skills, contentFile)
	}

	return skills, nil
}

// scanCommands scans a commands directory for .md files (flat form) and
// COMMAND.md files in subdirectories (directory form with optional resources/).
// Mirrors the structure of scanSkills to support bundled reference material.
func (s *Scanner) scanCommands(commandsDir string) ([]config.ContentFile, error) {
	// If directory doesn't exist, return empty slice (not an error)
	if _, err := os.Stat(commandsDir); os.IsNotExist(err) {
		return []config.ContentFile{}, nil
	}

	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		return nil, oops.
			With("path", commandsDir).
			Wrapf(err, "read commands directory")
	}

	var commands []config.ContentFile
	for _, entry := range entries {
		var commandPath string
		var contentFile config.ContentFile
		var err error

		if entry.IsDir() {
			// Directory structure: commands/name/COMMAND.md
			commandPath = filepath.Join(commandsDir, entry.Name(), commandMarkerFile)
			if _, err := os.Stat(commandPath); os.IsNotExist(err) {
				// No COMMAND.md file, skip this directory
				continue
			}

			contentFile, err = s.loadContentFile(commandPath)
			if err != nil {
				// Log warning but continue
				logger.Warn("failed to load command file", "path", commandPath, "error", err)
				continue
			}

			// For commands in directories, use the directory name as the command name
			contentFile.Name = entry.Name()

			// Load command resources (references/, scripts/, assets/)
			commandRoot := filepath.Join(commandsDir, entry.Name())
			resources, resErr := config.LoadResources(commandRoot, config.ItemKindCommand)
			if resErr != nil {
				logger.Warn("Failed to load command resources", "command", entry.Name(), "error", resErr)
			}
			contentFile.Resources = resources
		} else {
			// Flat file structure: commands/name.md
			if !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			commandPath = filepath.Join(commandsDir, entry.Name())
			contentFile, err = s.loadContentFile(commandPath)
			if err != nil {
				logger.Warn("failed to load command file", "path", commandPath, "error", err)
				continue
			}
			// loadContentFile already names a flat file after its filename stem.
		}

		commands = append(commands, contentFile)
	}

	return commands, nil
}

// loadContentFile loads a content file and parses optional frontmatter
func (s *Scanner) loadContentFile(path string) (config.ContentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.ContentFile{}, oops.
			With("path", path).
			Wrapf(err, "read content file")
	}

	content := string(data)
	filename := filepath.Base(path)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Parse frontmatter using canonical parser
	// Use non-fatal version to avoid blocking on parse errors
	parserMetadata, actualContent := parser.ParseFrontmatterNonFatal(content)

	// Convert parser.Metadata to config.Metadata.
	// Sort list-valued fields alphabetically so output ordering is stable
	// regardless of how the user wrote them in source frontmatter.
	var metadata *config.Metadata
	if parserMetadata != nil {
		metadata = &config.Metadata{
			Priority: parserMetadata.Priority,
			Targets:  sortedCopy(parserMetadata.Targets),
			Tools:    sortedCopy(parserMetadata.Tools),
			Skills:   sortedCopy(parserMetadata.Skills),
			Keywords: sortedCopy(parserMetadata.Keywords),
			Extra:    parserMetadata.Extra,
		}
	}

	return config.ContentFile{
		Name:     name,
		Path:     path,
		Content:  actualContent,
		Metadata: metadata,
	}, nil
}

// applyNamespacing prefixes domain name to content file names
func (s *Scanner) applyNamespacing(files []config.ContentFile, domainName string) {
	for i := range files {
		// Prefix the Name field with "Domain: "
		files[i].Name = fmt.Sprintf("%s: %s", domainName, files[i].Name)
	}
}

// applySkillNamespacing prefixes domain name to skill IDs (different format than regular namespacing)
func (s *Scanner) applySkillNamespacing(files []config.ContentFile, domainName string) {
	for i := range files {
		// Prefix the Name field with "domain-" (e.g., "backend-api-expert")
		files[i].Name = fmt.Sprintf("%s-%s", domainName, files[i].Name)
	}
}

// logCollisions logs warnings for filename collisions
func (s *Scanner) logCollisions(collisionMap map[string][]string, contentType string) {
	for filename, sources := range collisionMap {
		if len(sources) > 1 {
			// Check if root is in the sources
			hasRoot := false
			domains := []string{}
			for _, source := range sources {
				if source == "root" {
					hasRoot = true
				} else {
					domains = append(domains, source)
				}
			}

			if hasRoot && len(domains) > 0 {
				// Domain-overrides-root is intentional design (see docs/profiles-and-domains).
				// Surface only when the user opts into debug.
				logger.Debug(
					fmt.Sprintf("domain %s file overrides root file", contentType),
					"filename", filename,
					"domains", strings.Join(domains, ", "),
				)
			} else if len(domains) > 1 {
				// Multiple domains sharing a filename (last one wins) is also by design.
				logger.Debug(
					fmt.Sprintf("multiple domains have same %s file", contentType),
					"filename", filename,
					"domains", strings.Join(domains, ", "),
					"note", "last domain wins",
				)
			}
		}
	}
}

// resolveCollisions resolves collisions: domain content overrides root content.
// key must be the same function that populated collisionMap.
func (s *Scanner) resolveCollisions(rootFiles, domainFiles []config.ContentFile, collisionMap map[string][]string, key func(config.ContentFile) string) []config.ContentFile {
	result := make([]config.ContentFile, 0)

	// Add root files that don't have collisions
	for _, file := range rootFiles {
		sources := collisionMap[key(file)]

		// Only include root file if it's the only source
		if len(sources) == 1 && sources[0] == "root" {
			result = append(result, file)
		}
	}

	// Add all domain files (they override root)
	result = append(result, domainFiles...)

	return result
}

// sortByName sorts content files alphabetically by Name.
// We sort by name (not priority) so generated output is deterministic and idempotent —
// the skip-on-content-hash mechanism in the generator only triggers when output is byte-stable.
func sortByName(files []config.ContentFile) {
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
}

// sortedCopy returns a sorted copy of the input slice without mutating it.
// Returns nil for empty input so omitempty yaml/json tags drop the field.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// Helper functions

func getProfileNames(profiles map[string][]string) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
