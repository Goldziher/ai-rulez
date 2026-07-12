package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

const hermesSourcePath = ".ai-rulez/hermes/index.py"

var nonPythonIdentifier = regexp.MustCompile(`[^a-zA-Z0-9_]`)
var semverPrerelease = regexp.MustCompile(`-(alpha|beta|rc)\.(\d+)$`)

func init() {
	register(config.PluginRuntimeHermes, renderHermes)
}

func renderHermes(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	pluginDir := filepath.Join(baseDir, ".hermes", "plugins", m.Name)
	module, err := hermesModule(m, filepath.Join(pluginDir, "hermes.py"))
	if err != nil {
		return nil, err
	}

	author := ""
	if m.Author != nil {
		author = m.Author.Name
	}
	manifestBytes := []byte(hermesManifestYAML(m, author))
	outputs := []config.OutputFile{
		{Path: filepath.Join(pluginDir, "__init__.py"), RawContent: []byte(hermesPackageInit(m.Version))},
		module,
		{Path: filepath.Join(pluginDir, "plugin.yaml"), RawContent: manifestBytes},
	}

	content, err := bundleContent(m, baseDir, contentLayout{
		Root: filepath.Join(".hermes", "plugins", m.Name), Skills: true, Commands: true, Agents: true,
	})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	packageRoot := filepath.Join(baseDir, ".hermes", "package")
	moduleName := pythonModuleName(m.Name) + "_hermes_plugin"
	moduleRoot := filepath.Join(packageRoot, "src", moduleName)
	packageModule, err := hermesModule(m, filepath.Join(moduleRoot, "hermes.py"))
	if err != nil {
		return nil, err
	}
	packageContent, err := bundleContent(m, baseDir, contentLayout{
		Root: filepath.Join(".hermes", "package", "src", moduleName), Skills: true, Commands: true, Agents: true,
	})
	if err != nil {
		return nil, err
	}
	pyproject, err := hermesPyproject(m, moduleName)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs,
		config.OutputFile{Path: filepath.Join(packageRoot, "pyproject.toml"), RawContent: pyproject},
		config.OutputFile{Path: filepath.Join(moduleRoot, "__init__.py"), RawContent: []byte(hermesPackageInit(m.Version))},
		packageModule,
		config.OutputFile{Path: filepath.Join(moduleRoot, "plugin.yaml"), RawContent: manifestBytes},
	)
	return append(outputs, packageContent...), nil
}

func hermesManifestYAML(m *Manifest, author string) string {
	var manifest strings.Builder
	for _, field := range []struct{ name, value string }{
		{"name", m.Name}, {"version", m.Version}, {"description", m.Description}, {"author", author}, {"kind", "standalone"},
	} {
		if field.value != "" {
			fmt.Fprintf(&manifest, "%s: %s\n", field.name, strconv.Quote(field.value))
		}
	}
	return manifest.String()
}

func pythonModuleName(name string) string {
	module := strings.ToLower(nonPythonIdentifier.ReplaceAllString(name, "_"))
	module = strings.Trim(module, "_")
	if module == "" || (module[0] >= '0' && module[0] <= '9') {
		return "plugin_" + module
	}
	return module
}

func hermesPyproject(m *Manifest, moduleName string) ([]byte, error) {
	requiresPython := ">=3.11"
	if m.Hermes != nil && m.Hermes.RequiresPython != "" {
		requiresPython = m.Hermes.RequiresPython
	}
	document := struct {
		BuildSystem struct {
			Requires     []string `toml:"requires"`
			BuildBackend string   `toml:"build-backend"`
		} `toml:"build-system"`
		Project struct {
			Name           string                       `toml:"name"`
			Version        string                       `toml:"version"`
			Description    string                       `toml:"description"`
			RequiresPython string                       `toml:"requires-python"`
			Keywords       []string                     `toml:"keywords,omitempty"`
			EntryPoints    map[string]map[string]string `toml:"entry-points"`
			URLs           map[string]string            `toml:"urls,omitempty"`
		} `toml:"project"`
		Tool struct {
			Hatch struct {
				Build struct {
					Targets struct {
						Wheel struct {
							Packages []string `toml:"packages"`
						} `toml:"wheel"`
					} `toml:"targets"`
				} `toml:"build"`
			} `toml:"hatch"`
		} `toml:"tool"`
	}{}
	document.BuildSystem.Requires = []string{"hatchling>=1.27"}
	document.BuildSystem.BuildBackend = "hatchling.build"
	document.Project.Name = m.Name + "-hermes-plugin"
	document.Project.Version = pythonPackageVersion(m.Version)
	document.Project.Description = m.Description
	document.Project.RequiresPython = requiresPython
	document.Project.Keywords = append([]string(nil), m.Keywords...)
	document.Project.EntryPoints = map[string]map[string]string{"hermes_agent.plugins": {m.Name: moduleName}}
	document.Project.URLs = map[string]string{"Homepage": m.Homepage, "Repository": m.Repository}
	document.Tool.Hatch.Build.Targets.Wheel.Packages = []string{"src/" + moduleName}
	content, err := toml.Marshal(document)
	if err != nil {
		return nil, oops.Wrapf(err, "marshal Hermes pyproject")
	}
	return content, nil
}

func hermesPackageInit(version string) string {
	return fmt.Sprintf("\"\"\"Hermes Agent plugin package.\"\"\"\n\nfrom . import hermes\nfrom .hermes import register\n\n__version__ = %s\n__all__ = [\"hermes\", \"register\"]\n", strconv.Quote(pythonPackageVersion(version)))
}

func pythonPackageVersion(version string) string {
	return semverPrerelease.ReplaceAllStringFunc(version, func(prerelease string) string {
		parts := semverPrerelease.FindStringSubmatch(prerelease)
		label := map[string]string{"alpha": "a", "beta": "b", "rc": "rc"}[parts[1]]
		return label + parts[2]
	})
}

func hermesModule(m *Manifest, outputPath string) (config.OutputFile, error) {
	source := hermesSourcePath
	if m.Hermes != nil && m.Hermes.Source != "" {
		source = m.Hermes.Source
	}
	sourcePath := filepath.Join(m.SourceDir, source)
	info, err := os.Stat(sourcePath)
	if err == nil {
		if info.IsDir() {
			return config.OutputFile{}, oops.With("path", sourcePath).Errorf("Hermes entrypoint must be a file")
		}
		return passthroughFile(m.SourceDir, sourcePath, outputPath)
	}
	if !os.IsNotExist(err) {
		return config.OutputFile{}, oops.With("path", sourcePath).Wrapf(err, "stat Hermes entrypoint")
	}
	if m.Hermes != nil && m.Hermes.Source != "" {
		return config.OutputFile{}, oops.With("path", sourcePath).Errorf("configured Hermes entrypoint does not exist")
	}
	return config.OutputFile{Path: outputPath, RawContent: []byte(hermesScaffold(m.Name))}, nil
}

func hermesScaffold(name string) string {
	return fmt.Sprintf(`"""Hermes adapter for %s.

This generated no-op keeps the plugin loadable without inventing runtime behavior.
To add Hermes tools, hooks, commands, or other registrations:

1. Create .ai-rulez/hermes/index.py.
2. Implement register(ctx) in that user-owned source file.
3. Run ai-rulez generate --plugin.

Project-local Hermes plugins are trusted code. Enable them explicitly with
HERMES_ENABLE_PROJECT_PLUGINS=true and validate all external input.
"""


def register(ctx):
    """Register this plugin with Hermes Agent."""
    del ctx
`, name)
}
