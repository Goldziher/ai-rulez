package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/samber/oops"
)

type piPackage struct {
	Name           string             `json:"name"`
	Version        string             `json:"version"`
	Description    string             `json:"description,omitempty"`
	Author         *config.Author     `json:"author,omitempty"`
	Keywords       []string           `json:"keywords"`
	Homepage       string             `json:"homepage,omitempty"`
	Repository     openCodeRepository `json:"repository"`
	License        string             `json:"license,omitempty"`
	PackageManager string             `json:"packageManager,omitempty"`
	Type           string             `json:"type"`
	Files          []string           `json:"files"`
	Pi             piResources        `json:"pi"`
}

type piResources struct {
	Skills  []string `json:"skills"`
	Prompts []string `json:"prompts"`
}

func renderPi(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	openCodeName := openCodePackageName(m)
	name := openCodeName[:strings.LastIndex(openCodeName, "/")+1] + "pi-" + m.Name
	if slices.Contains(m.Runtimes, config.PluginRuntimeOpenCode) {
		name = openCodeName
	}
	if m.PiPackageName != "" {
		name = m.PiPackageName
	}
	keywords := append([]string{}, m.Keywords...)
	if !slices.Contains(keywords, "pi-package") {
		keywords = append(keywords, "pi-package")
	}
	skillsDir := m.piSkillsDir()
	content, err := bundleContent(m, baseDir, contentLayout{Skills: true, SkillsRoot: skillsDir})
	if err != nil {
		return nil, err
	}
	prompts, err := piPrompts(m, baseDir, skillsDir)
	if err != nil {
		return nil, err
	}
	payload := append(append([]config.OutputFile(nil), content...), prompts...)
	files, err := generatedPackagePaths(payload, baseDir)
	if err != nil {
		return nil, err
	}
	resources := piResourcePaths(files, skillsDir)
	if m.PiSkillsEntry == config.PiSkillsEntryDirectory {
		files, resources.Skills = piDirectoryEntries(files, skillsDir, resources.Skills)
	}
	pkg, err := piPackageOutput(m, baseDir, piPackage{
		Name: name, Version: m.Version, Description: m.Description, Author: m.Author,
		Keywords: keywords,
		Homepage: m.Homepage, Repository: openCodeRepository{Type: "git", URL: m.Repository},
		License: m.License, PackageManager: m.PackageManager, Type: "module",
		Files: append(files, piExistingTopLevel(baseDir, payload)...),
		Pi:    resources,
	})
	if err != nil {
		return nil, err
	}
	if len(m.MCP)+len(m.Hooks)+len(m.Agents) > 0 || m.Statusline != nil {
		m.log().Warn("Pi packages bundle skills and prompts only; MCP servers, hooks, agents and statuslines are skipped")
	}
	return append([]config.OutputFile{pkg}, payload...), nil
}

// piDirectoryEntries collapses the skills directory to one files entry and one
// pi.skills entry (skills_entry = "directory"). A directory with no skill writes
// neither.
func piDirectoryEntries(files []string, skillsDir string, skills []string) (entries, piSkills []string) {
	root := filepath.ToSlash(filepath.Clean(skillsDir))
	prefix := root + "/"
	kept := make([]string, 0, len(files))
	collapsed := false
	for _, file := range files {
		if strings.HasPrefix(file, prefix) {
			collapsed = true
			continue
		}
		kept = append(kept, file)
	}
	if !collapsed {
		return files, skills
	}
	kept = append(kept, prefix)
	slices.Sort(kept)
	return kept, []string{"./" + root}
}

// piExistingTopLevel returns the "assets/" and "README.md" files entries that
// exist in the bundle: written by the payload or already present in baseDir.
// A path that does not exist is left out so npm is not told to publish it.
func piExistingTopLevel(baseDir string, payload []config.OutputFile) []string {
	var entries []string
	for _, name := range []string{"assets", "README.md"} {
		exists := false
		if _, err := os.Stat(filepath.Join(baseDir, name)); err == nil {
			exists = true
		}
		for _, out := range payload {
			rel, err := filepath.Rel(baseDir, out.Path)
			if err == nil && (filepath.ToSlash(rel) == name || strings.HasPrefix(filepath.ToSlash(rel), name+"/")) {
				exists = true
			}
		}
		if !exists {
			continue
		}
		if name == "assets" {
			name = "assets/"
		}
		entries = append(entries, name)
	}
	return entries
}

// Root package.json keys the Pi runtime owns in package_json = "merge" mode.
// Every other key belongs to the author and is preserved.
const (
	piKeyName        = "name"
	piKeyVersion     = "version"
	piKeyDescription = "description"
	piKeyAuthor      = "author"
	piKeyRepository  = "repository"
	piKeyKeywords    = "keywords"
	piKeyPi          = "pi"
	piKeyFiles       = "files"
)

// piOwnedKeys returns the root package.json keys the Pi runtime owns in
// package_json = "merge" mode, in a stable order. Every other key belongs to
// the author and is preserved. It is a function, not a package variable, so no
// mutable slice is added to the package.
func piOwnedKeys() []string {
	return []string{
		piKeyName, piKeyVersion, piKeyDescription, piKeyAuthor,
		piKeyRepository, piKeyKeywords, piKeyPi, piKeyFiles,
	}
}

// piPackageOutput renders the root package.json. In "generate" mode it replaces
// the file from the plugin metadata; in "merge" mode it overlays only the keys
// ai-rulez owns (piOwnedKeys) onto the existing file, keeping every other key
// (devDependencies, scripts, packageManager, ...).
func piPackageOutput(m *Manifest, baseDir string, pkg piPackage) (config.OutputFile, error) {
	path := filepath.Join(baseDir, "package.json")
	if !m.mergesPiPackageJSON() {
		return jsonOutput(path, pkg)
	}
	merged := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &merged); err != nil {
			return config.OutputFile{}, oops.With("path", path).Wrapf(err, "parse the existing root package.json")
		}
	case !os.IsNotExist(err):
		return config.OutputFile{}, oops.With("path", path).Wrapf(err, "read the existing root package.json")
	}
	fields := piOwnedFields(pkg)
	if authored, ok := merged[piKeyName].(string); ok && authored != "" && m.PiPackageName == "" {
		delete(fields, piKeyName)
	}
	for key, value := range fields {
		merged[key] = value
	}
	return jsonOutput(path, merged)
}

// piOwnedFields returns the owned-key subset of the Pi package metadata. An
// empty description and an absent author are omitted, matching the generate-mode
// struct's omitempty tags.
func piOwnedFields(pkg piPackage) map[string]any {
	fields := map[string]any{
		piKeyName:       pkg.Name,
		piKeyVersion:    pkg.Version,
		piKeyRepository: pkg.Repository,
		piKeyKeywords:   pkg.Keywords,
		piKeyPi:         pkg.Pi,
		piKeyFiles:      pkg.Files,
	}
	if pkg.Description != "" {
		fields[piKeyDescription] = pkg.Description
	}
	if pkg.Author != nil {
		fields[piKeyAuthor] = pkg.Author
	}
	return fields
}

func piPrompts(m *Manifest, baseDir, skillsDir string) ([]config.OutputFile, error) {
	provider, err := providers.LoadBuiltin("pi")
	if err != nil {
		return nil, oops.Wrapf(err, "load Pi prompt renderer")
	}
	cfg := &config.Config{Name: m.Name, Version: "5.0", BaseDir: m.SourceDir}
	outputs, err := provider.Generate(&config.ContentTree{Commands: m.Commands}, baseDir, cfg)
	if err != nil {
		return nil, oops.Wrapf(err, "render Pi prompts")
	}
	var prompts []config.OutputFile
	root := filepath.Join(baseDir, ".pi", "prompts") + string(filepath.Separator)
	for _, out := range outputs {
		if strings.HasPrefix(out.Path, root) {
			out.RawContent = []byte(rebasePiPromptLinks(out.Content, skillsDir))
			out.Content = ""
			prompts = append(prompts, out)
		}
	}
	return prompts, nil
}

const (
	// commandsDir is where a command's relative links are written to resolve: the
	// bundle's commands/ directory, beside skills/ at the root.
	commandsDir  = "commands"
	piPromptsDir = ".pi/prompts"
	skillsRoot   = "skills"
)

// rebasePiPromptLinks rewrites the relative links in a command body for the Pi
// prompt it becomes. Links are authored against commands/, so ../skills/x/SKILL.md
// reaches the root skills/ tree. A Pi prompt lives two levels down in
// .pi/prompts/, and its skills sit in skillsDir, so the link is resolved from
// commands/ and re-expressed from .pi/prompts/ to wherever the target now is.
// Inline links and reference definitions are rewritten, bare or in angle
// brackets, outside fenced code and inline code spans; the path is escaped for a
// link destination and any query or fragment is kept. A link that leaves the
// bundle root is kept as written.
func rebasePiPromptLinks(body, skillsDir string) string {
	skills := filepath.ToSlash(filepath.Clean(skillsDir))
	rebase := func(dest string, _ int) (string, bool) {
		inner, wrapped := dest, false
		if len(dest) >= 2 && dest[0] == '<' && dest[len(dest)-1] == '>' {
			inner, wrapped = dest[1:len(dest)-1], true
		}
		p, suffix, internal := okf.SplitDest(inner)
		if !internal || !strings.HasPrefix(p, "../") {
			return "", false
		}
		resolved, ok, inside := okf.ResolveLink(commandsDir, inner)
		if !ok || !inside {
			return "", false
		}
		switch {
		case resolved == skillsRoot:
			resolved = skills
		case strings.HasPrefix(resolved, skillsRoot+"/"):
			resolved = skills + strings.TrimPrefix(resolved, skillsRoot)
		}
		rebased, err := filepath.Rel(piPromptsDir, filepath.FromSlash(resolved))
		if err != nil {
			return "", false
		}
		out := okf.JoinDest(filepath.ToSlash(rebased), suffix)
		if wrapped {
			out = "<" + out + ">"
		}
		return out, true
	}
	return okf.RewriteLinkDefs(okf.RewriteLinks(body, rebase), rebase)
}

func piResourcePaths(files []string, skillsDir string) piResources {
	resources := piResources{Skills: []string{}, Prompts: []string{}}
	root := filepath.ToSlash(filepath.Clean(skillsDir))
	for _, file := range files {
		switch {
		case filepath.Base(file) == skillManifestName && filepath.ToSlash(filepath.Dir(filepath.Dir(file))) == root:
			resources.Skills = append(resources.Skills, "./"+filepath.ToSlash(filepath.Dir(file)))
		case strings.HasPrefix(file, ".pi/prompts/") && strings.HasSuffix(file, ".md"):
			resources.Prompts = append(resources.Prompts, "./"+file)
		}
	}
	return resources
}

func generatedPackagePaths(outputs []config.OutputFile, baseDir string) ([]string, error) {
	files := make([]string, 0, len(outputs))
	for _, out := range outputs {
		relative, err := filepath.Rel(baseDir, out.Path)
		if err != nil {
			return nil, oops.With("path", out.Path).Wrapf(err, "resolve generated package path")
		}
		files = append(files, filepath.ToSlash(relative))
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}
