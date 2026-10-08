package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// ScanContentTree scans all content directories and returns a populated ContentTree.
// Root content goes into the top-level slices; domain content goes only into the Domains map.
// This keeps the two layers separate so callers (e.g. include sources) can merge without duplication.
func ScanContentTree(configDir string) (*ContentTree, error) {
	return ScanContentTreeWith(configDir, nil)
}

// ScanContentTreeWith is ScanContentTree with extra bundle_exclude patterns
// applied to the resources of every skill and command.
func ScanContentTreeWith(configDir string, bundleExclude []string) (*ContentTree, error) {
	return scanContentTree(newIncludeScanner(context.Background(), osView(configDir)), configDir, bundleExclude)
}

// ScanContentTreeContext is ScanContentTree whose git questions (which files a
// work tree ignores) run through the runner ctx carries (runner.WithContext);
// without one it runs real git.
func ScanContentTreeContext(ctx context.Context, configDir string) (*ContentTree, error) {
	s := newIncludeScanner(ctx, osView(configDir))
	s.git = gitutil.New(runner.FromContext(ctx))
	s.log = logger.FromContext(ctx)
	return scanContentTree(s, configDir, nil)
}

// ScanContentTreeIn is ScanContentTreeContext reading through v, for content that
// lives inside a workspace (a local include) rather than in a directory of the
// real file system. configDir is an absolute path below v's root. Symlinks are
// never followed, as for any included content.
func ScanContentTreeIn(ctx context.Context, v workspace.View, configDir string) (*ContentTree, error) {
	s := newIncludeScanner(ctx, v)
	s.git = gitutil.New(runner.FromContext(ctx))
	s.log = logger.FromContext(ctx)
	return scanContentTree(s, configDir, nil)
}

// scanContentTree scans configDir under the symlink policy held by s.
func scanContentTree(s *contentScanner, configDir string, bundleExclude []string) (*ContentTree, error) {
	if !s.admitTreeRoot(configDir) {
		return &ContentTree{Domains: make(map[string]*Domain)}, nil
	}
	tree := &ContentTree{
		Domains: make(map[string]*Domain),
	}

	// Scan root rules/
	rulesPath := filepath.Join(configDir, rulesDir)
	var rules []ContentFile
	var err error
	if rules, err = s.markdownFiles(rulesPath); err != nil {
		return nil, oops.
			With("path", rulesPath).
			Wrapf(err, "scan rules directory")
	}
	tree.Rules = rules

	// Scan root context/
	contextPath := filepath.Join(configDir, contextDir)
	var contextFiles []ContentFile
	if contextFiles, err = s.markdownFiles(contextPath); err != nil {
		return nil, oops.
			With("path", contextPath).
			Wrapf(err, "scan context directory")
	}
	tree.Context = contextFiles

	// Scan root skills/
	skillsPath := filepath.Join(configDir, skillsDir)
	var skills []ContentFile
	if skills, err = s.skills(skillsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", skillsPath).
			Wrapf(err, "scan skills directory")
	}
	tree.Skills = skills

	// Scan root agents/
	agentsPath := filepath.Join(configDir, agentsDir)
	var agents []ContentFile
	if agents, err = s.agents(agentsPath); err != nil {
		return nil, oops.
			With("path", agentsPath).
			Wrapf(err, "scan agents directory")
	}
	tree.Agents = agents
	s.logger().Debug("Scanned agents directory", "path", agentsPath, "count", len(agents))

	// Scan root commands/
	commandsPath := filepath.Join(configDir, commandsDir)
	var commands []ContentFile
	if commands, err = s.commands(commandsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", commandsPath).
			Wrapf(err, "scan commands directory")
	}
	tree.Commands = commands
	s.logger().Debug("Scanned commands directory", "path", commandsPath, "count", len(commands))

	// Scan root checks/
	checksPath := filepath.Join(configDir, checksDir)
	var checks []ContentFile
	if checks, err = s.markdownFiles(checksPath); err != nil {
		return nil, oops.
			With("path", checksPath).
			Wrapf(err, "scan checks directory")
	}
	tree.Checks = checks

	// Scan domains/
	domainsPath := filepath.Join(configDir, domainsDir)
	var domains map[string]*Domain
	if domains, err = s.domains(domainsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", domainsPath).
			Wrapf(err, "scan domains directory")
	}
	tree.Domains = domains

	return tree, nil
}

// ScanLocalContentTree scans machine-local override content from
// <configDir>/local/, which mirrors the shared layout: rules, context, skills,
// agents, commands and domains. The tree is kept separate from the committed
// content tree so local content is only ever written to gitignored outputs.
func ScanLocalContentTree(configDir string) (*ContentTree, error) {
	return ScanLocalContentTreeWith(configDir, nil)
}

// ScanLocalContentTreeWith is ScanLocalContentTree with extra bundle_exclude patterns.
func ScanLocalContentTreeWith(configDir string, bundleExclude []string) (*ContentTree, error) {
	return scanLocalContentTree(newIncludeScanner(context.Background(), osView(configDir)), configDir, bundleExclude)
}

func scanLocalContentTree(s *contentScanner, configDir string, bundleExclude []string) (*ContentTree, error) {
	localBase := filepath.Join(configDir, localDir)
	tree, err := scanContentTree(s, localBase, bundleExclude)
	if err != nil {
		return nil, oops.With("path", localBase).Wrapf(err, "scan local content")
	}
	return tree, nil
}

// scanMarkdownFiles scans a directory for .md files and returns ContentFile entries
func (s *contentScanner) markdownFiles(dir string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var files []ContentFile

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if isDir, ok := s.entryInfo(filePath, entry); !ok || isDir {
			continue
		}
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		files = append(files, contentFile)
	}

	return files, nil
}

// scanSkills scans the skills/ directory for SKILL.md files in subdirectories
func (s *contentScanner) skills(skillsDir string, bundleExclude []string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(skillsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var skills []ContentFile

	for _, entry := range entries {
		skillRoot := filepath.Join(skillsDir, entry.Name())
		if isDir, ok := s.entryInfo(skillRoot, entry); !ok || !isDir {
			continue
		}

		skillPath := filepath.Join(skillRoot, skillMarkerFile)
		if _, err := s.v.Lstat(skillPath); os.IsNotExist(err) {
			// No SKILL.md file, skip this directory
			continue
		}

		contentFile, err := s.loadFile(skillPath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		// A frontmatter that never closes would be served as body text with no
		// name or description; treat it as malformed like an unparseable one.
		if contentFile.Metadata == nil && !contentFile.MalformedFrontmatter && hasUnclosedFrontmatter(contentFile.Content) {
			s.logger().Warn("Ignoring skill frontmatter with no closing '---'", "skill", entry.Name(), "path", skillPath)
			contentFile.MalformedFrontmatter = true
		}

		// Override the name with the directory name instead of filename
		contentFile.Name = entry.Name()

		// Load skill supporting files (references/, scripts/, assets/) so
		// presets can preserve the canonical Agent Skills layout instead of
		// concatenating everything into SKILL.md.
		resources, resErr := s.loadResources(skillRoot, ItemKindSkill, bundleExclude)
		if resErr != nil {
			s.logger().Warn("Failed to load skill resources", "skill", entry.Name(), "error", resErr)
		}
		contentFile.Resources = resources

		skills = append(skills, contentFile)
	}

	return skills, nil
}

// scanCommands scans the commands/ directory for .md files (flat form) and
// COMMAND.md files in subdirectories (directory form with optional resources/).
// Mirrors the structure of scanSkills to support bundled reference material.
func scanCommands(commandsDir string) ([]ContentFile, error) {
	return newIncludeScanner(context.Background(), osView(commandsDir)).commands(commandsDir, nil)
}

func (s *contentScanner) commands(commandsDir string, bundleExclude []string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(commandsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var commands []ContentFile

	for _, entry := range entries {
		entryPath := filepath.Join(commandsDir, entry.Name())
		isDir, ok := s.entryInfo(entryPath, entry)
		if !ok {
			continue
		}
		if isDir {
			// Directory structure: commands/name/COMMAND.md
			commandRoot := entryPath
			commandPath := filepath.Join(commandRoot, commandMarkerFile)
			if _, err := s.v.Lstat(commandPath); os.IsNotExist(err) {
				// No COMMAND.md file, skip this directory
				continue
			}

			contentFile, err := s.loadFile(commandPath)
			if err != nil {
				// Non-fatal: one unreadable command must not fail the whole
				// load, but dropping it without a diagnostic makes an
				// unreadable COMMAND.md indistinguishable from a missing one.
				s.logger().Warn("failed to load command file", "path", commandPath, "error", err)
				continue
			}

			// Override the name with the directory name instead of filename
			contentFile.Name = entry.Name()

			// Load command supporting files (references/, scripts/, assets/) so
			// presets can preserve the canonical layout instead of concatenating
			// everything into COMMAND.md.
			resources, resErr := s.loadResources(commandRoot, ItemKindCommand, bundleExclude)
			if resErr != nil {
				s.logger().Warn("Failed to load command resources", "command", entry.Name(), "error", resErr)
			}
			contentFile.Resources = resources

			commands = append(commands, contentFile)
			continue
		}

		// Flat file structure: commands/name.md
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := entryPath
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			s.logger().Warn("failed to load command file", "path", filePath, "error", err)
			continue
		}

		commands = append(commands, contentFile)
	}

	return commands, nil
}

// scanAgents scans the agents/ directory for .md files
func (s *contentScanner) agents(agentsPath string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(agentsPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var agents []ContentFile

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(agentsPath, entry.Name())
		if isDir, ok := s.entryInfo(filePath, entry); !ok || isDir {
			continue
		}
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		agents = append(agents, contentFile)
	}

	return agents, nil
}

// scanDomains scans the domains/ directory and returns a map of domain name to Domain
func (s *contentScanner) domains(domainsDir string, bundleExclude []string) (map[string]*Domain, error) {
	entries, ok, err := s.dirEntries(domainsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return make(map[string]*Domain), nil
	}

	domains := make(map[string]*Domain)

	for _, entry := range entries {
		domainName := entry.Name()
		domainPath := filepath.Join(domainsDir, domainName)
		if isDir, ok := s.entryInfo(domainPath, entry); !ok || !isDir {
			continue
		}

		domain := &Domain{
			Name: domainName,
		}

		// Scan domain/rules/
		rulesPath := filepath.Join(domainPath, rulesDir)
		var rules []ContentFile
		var err error
		if rules, err = s.markdownFiles(rulesPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", rulesPath).
				Wrapf(err, "scan domain rules")
		}
		domain.Rules = rules

		// Scan domain/context/
		contextPath := filepath.Join(domainPath, contextDir)
		var contextFiles []ContentFile
		if contextFiles, err = s.markdownFiles(contextPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", contextPath).
				Wrapf(err, "scan domain context")
		}
		domain.Context = contextFiles

		// Scan domain/skills/
		skillsPath := filepath.Join(domainPath, skillsDir)
		var skills []ContentFile
		if skills, err = s.skills(skillsPath, bundleExclude); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", skillsPath).
				Wrapf(err, "scan domain skills")
		}
		domain.Skills = skills

		// Scan domain/agents/
		agentsPath := filepath.Join(domainPath, agentsDir)
		var agentsContent []ContentFile
		if agentsContent, err = s.agents(agentsPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", agentsPath).
				Wrapf(err, "scan domain agents")
		}
		domain.Agents = agentsContent

		// Scan domain/commands/
		domainCommandsPath := filepath.Join(domainPath, commandsDir)
		var domainCommands []ContentFile
		if domainCommands, err = s.commands(domainCommandsPath, bundleExclude); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", domainCommandsPath).
				Wrapf(err, "scan domain commands")
		}
		domain.Commands = domainCommands
		s.logger().Debug("Scanned domain commands directory", "domain", domainName, "path", domainCommandsPath, "count", len(domainCommands))

		// Scan domain/checks/
		domainChecksPath := filepath.Join(domainPath, checksDir)
		var domainChecks []ContentFile
		if domainChecks, err = s.markdownFiles(domainChecksPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", domainChecksPath).
				Wrapf(err, "scan domain checks")
		}
		domain.Checks = domainChecks

		domains[domainName] = domain
	}

	return domains, nil
}

// scanAgents scans one agents directory without following symlinks.
func scanAgents(agentsPath string) ([]ContentFile, error) {
	return newIncludeScanner(context.Background(), osView(agentsPath)).agents(agentsPath)
}

// loadContentFile loads a content file and parses optional frontmatter
//
// A symlinked file is refused and its target is never read: an include (or a
// cloned repository) could otherwise point a content file at any local file and
// have it rendered into the outputs, and the content lock does not pin it.
func loadContentFile(v workspace.View, log logger.Logger, path string) (ContentFile, error) {
	if info, err := v.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		log.Warn("Skipping symlinked content file; symlinks are not followed", "path", path)
		return ContentFile{}, oops.
			With("path", path).
			Errorf("content file %s is a symlink; symlinks are not followed", path)
	}
	return readContentFile(v, log, path)
}

// readContentFile reads and parses a content file the caller has already
// cleared under the symlink policy.
func readContentFile(v workspace.View, log logger.Logger, path string) (ContentFile, error) {
	data, err := readCapped(v, path)
	if err != nil {
		return ContentFile{}, oops.
			With("path", path).
			Wrapf(err, "read content file")
	}

	content := string(data)
	filename := filepath.Base(path)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Parse frontmatter (if present) - inlined to avoid import cycle
	metadata, actualContent, malformed := parseFrontmatter(content)
	if !malformed && metadata == nil && hasUnclosedFrontmatter(content) {
		malformed = true // an opening '---' that never closes would leak into the prompt as text
	}
	if malformed {
		warnMalformedFrontmatter(log, path)
	}

	return ContentFile{
		Name:                 name,
		Path:                 path,
		Content:              actualContent,
		Metadata:             metadata,
		MalformedFrontmatter: malformed,
	}, nil
}
