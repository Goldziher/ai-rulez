// Package agentssource completes the content tree of the "agents" source
// (`source = "agents"`): it adds the root AGENTS.md as context and drops
// generated files from what was scanned under .agents. config cannot import the
// importer, so this package joins them and internal/project wires it into the
// load as config.Resolvers.Agents.
package agentssource

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Read is a config.AgentsSourceReader: tree is the content scanned from .agents;
// the result adds the root AGENTS.md (as a context file) and leaves out every
// file an earlier generate run wrote, so output is never read back as source.
func Read(ctx context.Context, cfg *config.Config, tree *config.ContentTree) (*config.ContentTree, error) {
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
	md := importer.ReadAgentsMD(fsys, importer.Options{SplitHeadings: cfg.AgentsMDSplit})
	for _, note := range md.Notes {
		cfg.Warn(config.AgentsMDFilename+" "+note.Reason, "source", config.SourceAgents)
	}
	taken := map[string]bool{}
	for i := range tree.Context {
		taken[tree.Context[i].Name] = true
	}
	files := make([]config.ContentFile, 0, len(md.Items))
	for i, name := range itemNames(md.Items, taken) {
		files = append(files, contextFile(cfg, name, &md.Items[i], config.AgentsMDFilename, ""))
	}
	nested := readNested(ctx, cfg, fsys, taken)
	tree.Context = append(append(files, tree.Context...), nested...)
	return tree, nil
}

// itemNames names the AGENTS.md items: a heading used twice gets -2, -3, and a
// name already taken by a file under .agents/context gets -md. taken is updated.
func itemNames(items []importer.Item, taken map[string]bool) []string {
	names := make([]string, 0, len(items))
	own := map[string]int{}
	for i := range items {
		name := items[i].Name
		own[name]++
		if n := own[name]; n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
		for taken[name] {
			name += "-md"
		}
		taken[name] = true
		names = append(names, name)
	}
	return names
}

// contextFile turns an AGENTS.md item into a context file, parsing the
// frontmatter it may carry like any other content file. rel is the file the
// item came from, relative to the project; scope is its directory for a nested
// AGENTS.md ("" for the root), which scopes the item to `<scope>/**` unless its
// frontmatter says otherwise.
func contextFile(cfg *config.Config, name string, item *importer.Item, rel, scope string) config.ContentFile {
	meta, body, malformed := config.ParseFrontmatterChecked(string(item.Main))
	if readers := agentsMDReaders(cfg); len(readers) > 0 && (meta == nil || len(meta.Targets) == 0) {
		// A tool that loads AGENTS.md itself needs no copy of it in its own files.
		if meta == nil {
			meta = &config.Metadata{}
		}
		meta.Targets = readers
	}
	if scope != "" && (meta == nil || len(meta.PathScope()) == 0) {
		if meta == nil {
			meta = &config.Metadata{}
		}
		meta.Globs = []string{escapeGlob(scope) + "/**"}
	}
	return config.ContentFile{
		Name:                 name,
		Path:                 filepath.Join(cfg.BaseDir, filepath.FromSlash(rel)),
		Content:              body,
		Metadata:             meta,
		MalformedFrontmatter: malformed,
	}
}

// escapeGlob backslash-escapes the glob metacharacters (and the comma that
// splits a glob list) in a literal directory path, so the scope `<dir>/**`
// matches that directory and no other.
func escapeGlob(literal string) string {
	var b strings.Builder
	for i := 0; i < len(literal); i++ {
		if strings.IndexByte(`[]{}*?!\,`, literal[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(literal[i])
	}
	return b.String()
}

// agentsMDReaders lists, as negated targets, the configured presets that read
// the root AGENTS.md on their own. A preset whose root file is shared with
// another preset (copilot and copilot-cli write one .github/copilot-instructions.md)
// cannot be excluded by name: the shared file renders identically for all of
// them, so the name is ignored there. The file itself is negated instead, and
// only when every configured preset writing it reads AGENTS.md.
func agentsMDReaders(cfg *config.Config) []string {
	reads := map[string]bool{}
	for i := range cfg.Presets {
		if cfg.Presets[i].IsBuiltIn() && readsAgentsMD(cfg.Presets[i].BuiltIn) {
			reads[cfg.Presets[i].BuiltIn] = true
		}
	}
	var out []string
	for i := range cfg.Presets {
		name := cfg.Presets[i].BuiltIn
		if !reads[name] {
			continue
		}
		out = append(out, "!"+name)
		consumer, _ := config.SharedOutputConsumerFor(name)
		if root := consumer.OwnRootFile; root != "" && sharedRootFullyRead(cfg, root, reads) {
			out = append(out, "!"+root)
		}
	}
	return out
}

// readsAgentsMD reports whether the preset loads the root AGENTS.md itself in
// this source. Gemini does not: agents_md is off, so its settings list only
// GEMINI.md as a context file and AGENTS.md stays unread.
func readsAgentsMD(preset string) bool {
	if preset == string(config.PresetGemini) {
		return false
	}
	consumer, ok := config.SharedOutputConsumerFor(preset)
	return ok && consumer.Reads(config.SharedAgentsMD)
}

// sharedRootFullyRead reports whether every configured preset that writes the
// root file reads AGENTS.md, so leaving the text out of that file loses nothing.
func sharedRootFullyRead(cfg *config.Config, rootFile string, reads map[string]bool) bool {
	for _, owner := range rulefiles.RootOwners(rootFile) {
		for i := range cfg.Presets {
			if cfg.Presets[i].IsBuiltIn() && cfg.Presets[i].BuiltIn == owner && !reads[owner] {
				return false
			}
		}
	}
	return true
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
