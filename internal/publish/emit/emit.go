// Package emit renders the files a managed channel needs (a team marketplace
// index, catalog entities, registry records, steering files) from a published
// plugin. Emitters are pure: they read an Input and return files, never touching
// the network, the clock or the file system, so their output is reproducible
// and can be reviewed before an operator uploads or commits it.
package emit

import (
	"bytes"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Emitter statuses. An experimental emitter writes a format that is documented
// by its vendor but not backed by a published schema ai-rulez can test against;
// it refuses to run without --experimental and reports AR9N9.
const (
	StatusVerified     = "verified"
	StatusExperimental = "experimental"
)

// Activation modes of a Doc, the values of config.ActivationMode.
const (
	ModeAlways = "always"
	ModeGlob   = "glob"
	ModeAuto   = "auto"
	ModeManual = "manual"
)

// File is one emitted file: a slash path relative to the emitter's output
// directory and its bytes.
type File struct {
	Path string
	Data []byte
}

// Finding is a note about an emitted file: a field the target cannot express, a
// value that was normalised. It never fails the run.
type Finding struct {
	Message string
}

// Doc is a skill, rule or context file as an emitter sees it.
type Doc struct {
	Name        string
	Description string
	// Body is the full source text, front matter included for a skill.
	Body string
	// Mode is the activation mode of a rule (one of the Mode constants).
	Mode  string
	Globs []string
}

// Plugin is one published plugin.
type Plugin struct {
	Name, Description, Version, Category string
	Keywords                             []string
	Runtimes                             []string
	// BundleFile and BundleDigest identify the release archive of the plugin.
	BundleFile, BundleDigest string
	// Files are the plugin's files, paths relative to the plugin directory.
	Files []File
}

// Market is the marketplace identity (the [marketplace] and [plugin] tables).
type Market struct {
	Name, Description, OwnerName, OwnerEmail string
}

// Input is everything an emitter may read.
type Input struct {
	Version  string
	Repo     string
	Commit   string
	Tag      string
	LockTree string
	Market   Market
	Plugins  []Plugin
	// Rules are the project's root rules and context files (the plugin bundle
	// carries none), sorted by name.
	Rules []Doc
	// ARD is the [ard] table and the resources of the ard emitter; nil for the others.
	ARD *ARDInput
	// Options are the emitter's own settings ([[publish.emitters]] options).
	Options map[string]string
}

// Emitter renders one channel format.
type Emitter interface {
	// Name is the stable identifier used in config and on the command line.
	Name() string
	// Status is StatusVerified or StatusExperimental.
	Status() string
	// Emit returns the files, sorted by path.
	Emit(in Input) ([]File, []Finding, error)
}

var registry = map[string]Emitter{}

// Keys of front matter and of the emitted catalogs.
const (
	keyName        = "name"
	keyVersion     = "version"
	keyDescription = "description"
)

func register(e Emitter) { registry[e.Name()] = e }

// Lookup returns the emitter registered under name.
func Lookup(name string) (Emitter, bool) {
	e, ok := registry[name]
	return e, ok
}

// Names lists the registered emitters in sorted order.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func finish(files []File) ([]File, error) {
	seen := map[string]bool{}
	for _, f := range files {
		if !validRel(f.Path) {
			return nil, oops.Errorf("emitter produced the unsafe path %q", f.Path)
		}
		if seen[f.Path] {
			return nil, oops.Errorf("emitter produced %s twice", f.Path)
		}
		seen[f.Path] = true
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func validRel(p string) bool {
	if p == "" || strings.Contains(p, "\\") || path.IsAbs(p) || path.Clean(p) != p {
		return false
	}
	return p != ".." && !strings.HasPrefix(p, "../")
}

// jsonBytes encodes v with two-space indentation, a trailing newline and no HTML escaping.
func jsonBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, oops.Wrapf(err, "encode json")
	}
	return buf.Bytes(), nil
}

// Skills returns the skills of a plugin's files (skills/<name>/SKILL.md), sorted
// by name. The name and description come from the front matter; a skill without
// a name takes its directory name.
func Skills(files []File) []Doc {
	var out []Doc
	for _, f := range files {
		dir, ok := strings.CutSuffix(f.Path, "/SKILL.md")
		if !ok || !strings.HasPrefix(dir, "skills/") || strings.Count(dir, "/") != 1 {
			continue
		}
		fm := frontMatter(f.Data)
		name := fm[keyName]
		if name == "" {
			name = strings.TrimPrefix(dir, "skills/")
		}
		out = append(out, Doc{Name: name, Description: fm[keyDescription], Body: string(f.Data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// frontMatter reads the top-level string values of a leading YAML block.
func frontMatter(data []byte) map[string]string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil
	}
	block, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = strings.TrimSpace(s)
		}
	}
	return out
}
