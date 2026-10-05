package presets

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"gopkg.in/yaml.v3"
)

// commandFilesSpec describes a folder of one-file-per-command markdown files that
// a tool reads natively as custom slash commands (workflows, prompts).
type commandFilesSpec struct {
	// preset is matched against a command's `targets`.
	preset string
	// dir is the base-relative, slash-separated folder.
	dir string
	// ext is the file extension, with the dot (".md").
	ext string
	// frontmatter adds tool-specific frontmatter fields to the description.
	frontmatter func(command config.ContentFile) map[string]any
	// noFrontmatter writes the bare command body (tools whose command files are
	// plain markdown).
	noFrontmatter bool
}

// commandTargets reports whether the command is meant for the preset: it names
// it in `targets`, or names no target at all.
func commandTargets(command config.ContentFile, preset string) bool {
	if command.Metadata == nil || len(command.Metadata.Targets) == 0 {
		return true
	}
	return slices.Contains(command.Metadata.Targets, preset)
}

// commandDescription is the command's description from its frontmatter.
func commandDescription(command config.ContentFile) string {
	if command.Metadata == nil || command.Metadata.Extra == nil {
		return ""
	}
	return command.Metadata.Extra[keyDescription]
}

// commandFileOutputs renders every command targeting the preset into spec.dir,
// preceded by the folder marker. A project without commands gets neither, so no
// empty folder is created (a removed command is dropped by the manifest).
func commandFileOutputs(content *config.ContentTree, baseDir string, spec commandFilesSpec) ([]config.OutputFile, error) {
	root := filepath.Join(baseDir, filepath.FromSlash(spec.dir))
	var outputs []config.OutputFile
	for _, command := range allCommands(content) {
		if !commandTargets(command, spec.preset) {
			continue
		}
		text, err := renderCommandMarkdown(command, spec)
		if err != nil {
			return nil, fmt.Errorf("render command %s: %w", command.Name, err)
		}
		if len(outputs) == 0 {
			outputs = append(outputs, config.OutputFile{Path: root, IsDir: true})
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(root, sanitizeName(command.Name)+spec.ext),
			Content: text,
		})
	}
	return outputs, nil
}

// renderCommandMarkdown renders one command file: a description (plus the
// tool-specific fields) as YAML frontmatter, then the command body.
func renderCommandMarkdown(command config.ContentFile, spec commandFilesSpec) (string, error) {
	body := markdown.ProcessEmbeddedContent(command.Content)
	if spec.noFrontmatter {
		return body, nil
	}
	fm := map[string]any{}
	if desc := commandDescription(command); desc != "" {
		fm[keyDescription] = desc
	}
	if spec.frontmatter != nil {
		for k, v := range spec.frontmatter(command) {
			fm[k] = v
		}
	}
	if len(fm) == 0 {
		return body, nil
	}
	data, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal command frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(data)
	b.WriteString("---\n\n")
	b.WriteString(body)
	return b.String(), nil
}
