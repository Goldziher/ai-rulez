// Package agentssource completes the content tree of the "agents" source
// (`source = "agents"`): it adds the root AGENTS.md as context and drops
// generated files from what was scanned under .agents. config cannot import the
// importer, so this package joins them and internal/project wires it into the
// load as config.Resolvers.Agents.
package agentssource

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Read is a config.AgentsSourceReader: tree is the content scanned from .agents;
// the result adds the root AGENTS.md (as a context file) and leaves out every
// file an earlier generate run wrote, so output is never read back as source.
func Read(_ context.Context, cfg *config.Config, tree *config.ContentTree) (*config.ContentTree, error) {
	if tree == nil {
		tree = &config.ContentTree{Domains: map[string]*config.Domain{}}
	}
	view := cfg.View()
	var generated []string
	screen := func(files []config.ContentFile) []config.ContentFile {
		kept := files[:0:0]
		for i := range files {
			if isGenerated(view, files[i].Path) {
				generated = append(generated, displayPath(cfg.BaseDir, files[i].Path))
				continue
			}
			kept = append(kept, files[i])
		}
		return kept
	}
	tree.Rules = screen(tree.Rules)
	tree.Context = screen(tree.Context)
	tree.Skills = screen(tree.Skills)
	tree.Agents = screen(tree.Agents)
	tree.Commands = screen(tree.Commands)
	tree.Checks = screen(tree.Checks)
	for _, d := range tree.Domains {
		d.Rules = screen(d.Rules)
		d.Context = screen(d.Context)
		d.Skills = screen(d.Skills)
		d.Agents = screen(d.Agents)
		d.Commands = screen(d.Commands)
		d.Checks = screen(d.Checks)
	}
	if len(generated) > 0 {
		cfg.Warn("ignored generated files under "+config.AgentsDirName+" (output of an earlier generate run is never source): "+
			strings.Join(generated, ", "), "source", config.SourceAgents)
	}

	fsys, err := baseFS(cfg)
	if err != nil {
		return nil, err
	}
	md := importer.ReadAgentsMD(fsys, importer.Options{})
	for _, note := range md.Notes {
		cfg.Warn(config.AgentsMDFilename+" "+note.Reason, "source", config.SourceAgents)
	}
	taken := map[string]bool{}
	for i := range tree.Context {
		taken[tree.Context[i].Name] = true
	}
	for i := range md.Items {
		name := md.Items[i].Name
		for taken[name] {
			name += "-md"
		}
		taken[name] = true
		tree.Context = append([]config.ContentFile{contextFile(cfg, name, &md.Items[i])}, tree.Context...)
	}
	return tree, nil
}

// contextFile turns an AGENTS.md item into a context file, parsing the
// frontmatter it may carry like any other content file.
func contextFile(cfg *config.Config, name string, item *importer.Item) config.ContentFile {
	meta, body, malformed := config.ParseFrontmatterChecked(string(item.Main))
	return config.ContentFile{
		Name:                 name,
		Path:                 filepath.Join(cfg.BaseDir, config.AgentsMDFilename),
		Content:              body,
		Metadata:             meta,
		MalformedFrontmatter: malformed,
	}
}

// isGenerated reports whether the file at path carries the generated header.
func isGenerated(view workspace.View, path string) bool {
	if path == "" {
		return false
	}
	data, err := view.ReadFile(path)
	if err != nil {
		return false
	}
	return importer.IsGenerated(path, data)
}

// displayPath names path relative to the project, with forward slashes.
func displayPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// baseFS is the project directory as a file system: the workspace the config was
// loaded from, narrowed to the base directory.
func baseFS(cfg *config.Config) (fs.FS, error) {
	if cfg.Workspace == nil {
		return os.DirFS(cfg.BaseDir), nil
	}
	rel, err := workspace.Rel(cfg.Workspace, cfg.BaseDir)
	if err != nil {
		return nil, oops.With("base_dir", cfg.BaseDir).Wrapf(err, "locate the project in its workspace")
	}
	if rel == "." {
		return cfg.Workspace, nil
	}
	sub, err := fs.Sub(cfg.Workspace, rel)
	if err != nil {
		return nil, oops.With("base_dir", cfg.BaseDir).Wrapf(err, "narrow the workspace to the project")
	}
	return sub, nil
}
