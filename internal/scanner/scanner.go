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

// Scanner scans content from .ai-rulez/ directories and builds an in-memory content tree
type Scanner struct {
	baseDir string
	config  *config.ConfigV3
}

// NewScanner creates a new Scanner for the given base directory and config
func NewScanner(baseDir string, cfg *config.ConfigV3) *Scanner {
	return &Scanner{
		baseDir: baseDir,
		config:  cfg,
	}
}

// ScanProfile scans content for a specific profile and returns a merged ContentTree
// This follows the V3 design: root content + domain content with namespacing and collision handling
//
//nolint:gocyclo // Complex logic, acceptable for this use case
func (s *Scanner) ScanProfile(profileName string) (*config.ContentTreeV3, error) {
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

	rootCommands, err := s.scanMarkdownFiles(filepath.Join(aiRulezDir, "commands"))
	if err != nil {
		return nil, oops.
			With("path", filepath.Join(aiRulezDir, "commands")).
			Wrapf(err, "scan root commands directory")
	}
	logger.Info("Scanned root commands", "count", len(rootCommands))

	// Build collision tracking maps (basename -> [root, domain1, domain2, ...])
	// For skills, use the skill name (directory name) instead of file basename
	rulesMap := make(map[string][]string)
	contextMap := make(map[string][]string)
	skillsMap := make(map[string][]string)
	agentsMap := make(map[string][]string)
	commandsMap := make(map[string][]string)

	// Track root files
	for _, file := range rootRules {
		basename := filepath.Base(file.Path)
		rulesMap[basename] = append(rulesMap[basename], "root")
	}
	for _, file := range rootContext {
		basename := filepath.Base(file.Path)
		contextMap[basename] = append(contextMap[basename], "root")
	}
	for _, file := range rootSkills {
		// For skills, use the skill name (not the file basename)
		skillsMap[file.Name] = append(skillsMap[file.Name], "root")
	}
	for _, file := range rootAgents {
		basename := filepath.Base(file.Path)
		agentsMap[basename] = append(agentsMap[basename], "root")
	}
	for _, file := range rootCommands {
		basename := filepath.Base(file.Path)
		commandsMap[basename] = append(commandsMap[basename], "root")
	}

	// Scan domains and apply namespacing
	domainMap := make(map[string]*config.DomainV3)
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

		domainCommands, err := s.scanMarkdownFiles(filepath.Join(domainPath, "commands"))
		if err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", filepath.Join(domainPath, "commands")).
				Wrapf(err, "scan domain commands directory")
		}
		logger.Info("Scanned domain commands", "domain", domainName, "count", len(domainCommands))

		// Track domain files for collision detection
		for _, file := range domainRules {
			basename := filepath.Base(file.Path)
			rulesMap[basename] = append(rulesMap[basename], domainName)
		}
		for _, file := range domainContext {
			basename := filepath.Base(file.Path)
			contextMap[basename] = append(contextMap[basename], domainName)
		}
		for _, file := range domainSkills {
			// For skills, use the skill name (not the file basename)
			skillsMap[file.Name] = append(skillsMap[file.Name], domainName)
		}
		for _, file := range domainAgents {
			basename := filepath.Base(file.Path)
			agentsMap[basename] = append(agentsMap[basename], domainName)
		}
		for _, file := range domainCommands {
			basename := filepath.Base(file.Path)
			commandsMap[basename] = append(commandsMap[basename], domainName)
		}

		// Apply namespacing to domain content
		s.applyNamespacing(domainRules, domainName)
		s.applyNamespacing(domainContext, domainName)
		s.applySkillNamespacing(domainSkills, domainName)
		s.applyNamespacing(domainAgents, domainName)
		s.applyNamespacing(domainCommands, domainName)

		// Store domain
		domainMap[domainName] = &config.DomainV3{
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
	finalRules := s.resolveCollisions(rootRules, allRules, rulesMap)
	finalContext := s.resolveCollisions(rootContext, allContext, contextMap)
	finalSkills := s.resolveCollisions(rootSkills, allSkills, skillsMap)
	finalAgents := s.resolveCollisions(rootAgents, allAgents, agentsMap)
	finalCommands := s.resolveCollisions(rootCommands, allCommands, commandsMap)

	// Sort by priority (highest first)
	s.sortByPriority(finalRules)
	s.sortByPriority(finalContext)
	s.sortByPriority(finalSkills)
	s.sortByPriority(finalAgents)
	s.sortByPriority(finalCommands)

	logger.Info("Final commands after collision resolution", "count", len(finalCommands))

	return &config.ContentTreeV3{
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
			skillPath = filepath.Join(skillsDir, entry.Name(), "SKILL.md")
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

	// Convert parser.MetadataV3 to config.MetadataV3
	var metadata *config.MetadataV3
	if parserMetadata != nil {
		metadata = &config.MetadataV3{
			Priority: parserMetadata.Priority,
			Targets:  parserMetadata.Targets,
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
				logger.Warn(
					fmt.Sprintf("domain %s file overrides root file", contentType),
					"filename", filename,
					"domains", strings.Join(domains, ", "),
				)
			} else if len(domains) > 1 {
				// Multiple domains have the same filename (last one wins)
				logger.Warn(
					fmt.Sprintf("multiple domains have same %s file", contentType),
					"filename", filename,
					"domains", strings.Join(domains, ", "),
					"note", "last domain wins",
				)
			}
		}
	}
}

// resolveCollisions resolves filename collisions: domain content overrides root content
// For skills, we use the skill name (not basename) as the collision key
func (s *Scanner) resolveCollisions(rootFiles, domainFiles []config.ContentFile, collisionMap map[string][]string) []config.ContentFile {
	result := make([]config.ContentFile, 0)

	// Add root files that don't have collisions
	for _, file := range rootFiles {
		// For skills, use the skill name; for others, use basename
		var key string
		if strings.HasSuffix(file.Path, "SKILL.md") {
			key = file.Name
		} else {
			key = filepath.Base(file.Path)
		}

		sources := collisionMap[key]

		// Only include root file if it's the only source
		if len(sources) == 1 && sources[0] == "root" {
			result = append(result, file)
		}
	}

	// Add all domain files (they override root)
	result = append(result, domainFiles...)

	return result
}

// sortByPriority sorts content files by priority (highest first)
func (s *Scanner) sortByPriority(files []config.ContentFile) {
	sort.SliceStable(files, func(i, j int) bool {
		// Get priority from metadata, defaulting to medium
		priI := config.PriorityMedium
		if files[i].Metadata != nil {
			priI = files[i].Metadata.GetPriority()
		}

		priJ := config.PriorityMedium
		if files[j].Metadata != nil {
			priJ = files[j].Metadata.GetPriority()
		}

		// Sort by priority value (higher first)
		return priI.ToInt() > priJ.ToInt()
	})
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
