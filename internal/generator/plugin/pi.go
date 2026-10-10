package plugin

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/samber/oops"
)

type piPackage struct {
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Description string             `json:"description,omitempty"`
	Keywords    []string           `json:"keywords"`
	Homepage    string             `json:"homepage,omitempty"`
	Repository  openCodeRepository `json:"repository"`
	License     string             `json:"license,omitempty"`
	Type        string             `json:"type"`
	Files       []string           `json:"files"`
	Pi          piResources        `json:"pi"`
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
	content, err := bundleContent(m, baseDir, contentLayout{Root: ".pi", Skills: true})
	if err != nil {
		return nil, err
	}
	prompts, err := piPrompts(m, baseDir)
	if err != nil {
		return nil, err
	}
	payload := append(content, prompts...)
	files, err := generatedPackagePaths(payload, baseDir)
	if err != nil {
		return nil, err
	}
	pkg, err := jsonOutput(filepath.Join(baseDir, "package.json"), piPackage{
		Name: name, Version: m.Version, Description: m.Description, Keywords: keywords,
		Homepage: m.Homepage, Repository: openCodeRepository{Type: "git", URL: m.Repository},
		License: m.License, Type: "module", Files: append(files, "assets/", "README.md"),
		Pi: piResourcePaths(files),
	})
	if err != nil {
		return nil, err
	}
	if len(m.MCP)+len(m.Hooks)+len(m.Agents) > 0 || m.Statusline != nil {
		m.log().Warn("Pi packages bundle skills and prompts only; MCP servers, hooks, agents and statuslines are skipped")
	}
	return append([]config.OutputFile{pkg}, payload...), nil
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

func piResourcePaths(files []string) piResources {
	resources := piResources{Skills: []string{}, Prompts: []string{}}
	for _, file := range files {
		switch {
		case filepath.ToSlash(filepath.Dir(filepath.Dir(file))) == ".pi/skills" && filepath.Base(file) == "SKILL.md":
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
