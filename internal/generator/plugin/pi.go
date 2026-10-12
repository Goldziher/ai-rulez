package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
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
	keywords := append([]string{}, m.Keywords...)
	if !slices.Contains(keywords, "pi-package") {
		keywords = append(keywords, "pi-package")
	}
	skillsDir := m.piSkillsDir()
	content, err := bundleContent(m, baseDir, contentLayout{Skills: true, SkillsRoot: skillsDir})
	if err != nil {
		return nil, err
	}
	prompts, err := piPrompts(m, baseDir)
	if err != nil {
		return nil, err
	}
	payload := append(append([]config.OutputFile(nil), content...), prompts...)
	files, err := generatedPackagePaths(payload, baseDir)
	if err != nil {
		return nil, err
	}
	pkg, err := piPackageOutput(m, baseDir, piPackage{
		Name: name, Version: m.Version, Description: m.Description, Author: m.Author,
		Keywords: keywords,
		Homepage: m.Homepage, Repository: openCodeRepository{Type: "git", URL: m.Repository},
		License: m.License, PackageManager: m.PackageManager, Type: "module",
		Files: append(files, "assets/", "README.md"),
		Pi:    piResourcePaths(files, skillsDir),
	})
	if err != nil {
		return nil, err
	}
	if len(m.MCP)+len(m.Hooks)+len(m.Agents) > 0 || m.Statusline != nil {
		m.log().Warn("Pi packages bundle skills and prompts only; MCP servers, hooks, agents and statuslines are skipped")
	}
	return append([]config.OutputFile{pkg}, payload...), nil
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
	for key, value := range piOwnedFields(pkg) {
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

func piPrompts(m *Manifest, baseDir string) ([]config.OutputFile, error) {
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
			out.RawContent = []byte(out.Content)
			out.Content = ""
			prompts = append(prompts, out)
		}
	}
	return prompts, nil
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
