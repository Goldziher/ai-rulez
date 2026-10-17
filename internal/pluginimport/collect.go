package pluginimport

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

const (
	skillMarker = "SKILL.md"
	extMD       = ".md"
	extMDC      = ".mdc"
	extTOML     = ".toml"
)

// finding builds one line of the report with the code its status carries.
func finding(status importer.Status, source, field, reason string) importer.Finding {
	f := importer.Finding{Status: status, Source: source, Field: field, Reason: reason}
	switch status {
	case importer.StatusApproximated:
		f.Code = importer.CodeApproximated
	case importer.StatusDropped:
		f.Code = importer.CodeDropped
	case importer.StatusNeedsAction:
		f.Code = importer.CodeNeedsAction
	case importer.StatusUnsupported:
		f.Code = importer.CodeUnsupported
	case importer.StatusMapped:
	}
	return f
}

func (c *loader) note(status importer.Status, source, field, reason string) {
	c.res.Findings = append(c.res.Findings, finding(status, source, field, reason))
}

// componentPaths lists the directories or files of one component kind: the
// layout's defaults, then what the manifest and the marketplace entry name.
// Every path is confined to the plugin directory.
func (c *loader) componentPaths(defaults []string, field string) []string {
	var rels []string
	rels = append(rels, defaults...)
	for _, raw := range []json.RawMessage{c.manifest.Fields[field], c.opt.Inline[field]} {
		if len(raw) == 0 {
			continue
		}
		list, ok := stringList(raw)
		if !ok {
			continue // an inline object (hooks, mcpServers) is read elsewhere
		}
		rels = append(rels, list...)
	}
	var out []string
	seen := map[string]bool{}
	for _, rel := range rels {
		abs, err := SafeJoin(c.dir, rel)
		if err != nil {
			c.note(importer.StatusDropped, c.layout.manifest, field, err.Error())
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if _, err := os.Lstat(abs); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func (c *loader) rel(abs string) string {
	r, err := filepath.Rel(c.dir, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

func (c *loader) collectSkills() {
	if c.opt.Skills == nil {
		return
	}
	taken := map[string]bool{}
	for _, p := range c.componentPaths(c.layout.skills, "skills") {
		for _, dir := range c.skillDirs(p) {
			name := filepath.Base(dir)
			if !ValidName(name) {
				c.note(importer.StatusDropped, c.rel(dir), "", fmt.Sprintf("%q is not a valid skill name", name))
				continue
			}
			if taken[name] {
				continue
			}
			if len(taken) >= MaxSkills {
				c.note(importer.StatusDropped, c.rel(dir), "", fmt.Sprintf("a plugin delivers at most %d skills", MaxSkills))
				return
			}
			taken[name] = true
			cf, err := c.opt.Skills(c.ctx, dir, c.opt.NamePrefix+name)
			if err != nil {
				c.note(importer.StatusDropped, c.rel(dir), "", "the skill cannot be read: "+err.Error())
				continue
			}
			if c.opt.NamePrefix != "" {
				cf.ID = c.opt.NamePrefix + name // the directory carries the prefix, like the name
			}
			c.res.Skills = append(c.res.Skills, cf)
			c.note(importer.StatusMapped, c.rel(dir), "", "skills."+name)
		}
	}
}

// skillDirs lists the skill directories below p: p itself when it holds a
// SKILL.md, else its immediate subdirectories that do.
func (c *loader) skillDirs(p string) []string {
	info, err := os.Lstat(p)
	if err != nil || !info.IsDir() {
		return nil
	}
	if isRegular(filepath.Join(p, skillMarker)) {
		return []string{p}
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.IsDir() {
			if e.Type()&os.ModeSymlink != 0 {
				c.note(importer.StatusDropped, c.rel(filepath.Join(p, e.Name())), "", "a symlink is not followed")
			}
			continue
		}
		if sub := filepath.Join(p, e.Name()); isRegular(filepath.Join(sub, skillMarker)) {
			dirs = append(dirs, sub)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func isRegular(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode().IsRegular()
}

// markdownFiles lists the regular files with one of the extensions at p (a file)
// or below p (a directory), sorted, up to maxDepth levels deep. A symlink is
// reported and skipped.
func (c *loader) markdownFiles(p string, recursive bool, exts ...string) []string {
	info, err := os.Lstat(p)
	if err != nil {
		return nil
	}
	if info.Mode().IsRegular() {
		if hasExt(p, exts) {
			return []string{p}
		}
		return nil
	}
	if !info.IsDir() {
		return nil
	}
	var files []string
	depth := 1
	if recursive {
		depth = maxWalkDepth
	}
	c.walk(p, depth, func(file string) {
		if hasExt(file, exts) {
			files = append(files, file)
		}
	})
	sort.Strings(files)
	return files
}

func (c *loader) walk(dir string, depth int, visit func(file string)) {
	if depth <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		switch {
		case e.Type()&os.ModeSymlink != 0:
			c.note(importer.StatusDropped, c.rel(full), "", "a symlink is not followed")
		case strings.HasPrefix(e.Name(), "."):
		case e.IsDir():
			c.walk(full, depth-1, visit)
		case e.Type().IsRegular():
			visit(full)
		}
	}
}

func hasExt(file string, exts []string) bool {
	ext := strings.ToLower(filepath.Ext(file))
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// nameOf is the item name a file gets: its path below base without the
// extension, nested directories joined with '-'.
func nameOf(base, file string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		rel = filepath.Base(file)
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel))
	return strings.ReplaceAll(rel, "/", "-")
}

// markdownFile reads one markdown file into a content file named prefix+name.
func (c *loader) markdownFile(file, name string) (config.ContentFile, bool) {
	if !ValidName(name) {
		c.note(importer.StatusDropped, c.rel(file), "", fmt.Sprintf("%q is not a valid name", name))
		return config.ContentFile{}, false
	}
	data, err := readRegular(file, MaxContentBytes)
	if err != nil {
		c.note(importer.StatusDropped, c.rel(file), "", err.Error())
		return config.ContentFile{}, false
	}
	meta, body, malformed := config.ParseFrontmatterChecked(string(data))
	return config.ContentFile{Name: c.opt.NamePrefix + name, Path: file, Content: body, Metadata: meta, MalformedFrontmatter: malformed}, true
}

// collectMarkdown imports the markdown files of one kind into dst.
func (c *loader) collectMarkdown(dst *[]config.ContentFile, kind string, defaults []string, field string, recursive bool, exts ...string) {
	taken := map[string]bool{}
	for _, p := range c.componentPaths(defaults, field) {
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		base := p
		if info.Mode().IsRegular() {
			base = filepath.Dir(p)
		}
		for _, file := range c.markdownFiles(p, recursive, exts...) {
			name := nameOf(base, file)
			if taken[name] {
				continue
			}
			if len(taken) >= MaxFilesPerKind {
				c.note(importer.StatusDropped, c.rel(file), "", fmt.Sprintf("a plugin delivers at most %d %s", MaxFilesPerKind, kind))
				return
			}
			cf, ok := c.markdownFile(file, name)
			if !ok {
				continue
			}
			taken[name] = true
			*dst = append(*dst, cf)
			c.note(importer.StatusMapped, c.rel(file), "", kind+"."+name)
		}
	}
}

// collectTOMLCommands imports Gemini's TOML commands as command content.
func (c *loader) collectTOMLCommands() {
	taken := map[string]bool{}
	for _, p := range c.componentPaths(c.layout.commands, "commands") {
		for _, file := range c.markdownFiles(p, true, extTOML) {
			name := nameOf(p, file)
			if !ValidName(name) || taken[name] {
				c.note(importer.StatusDropped, c.rel(file), "", fmt.Sprintf("%q is not a valid command name", name))
				continue
			}
			if len(taken) >= MaxFilesPerKind {
				c.note(importer.StatusDropped, c.rel(file), "", fmt.Sprintf("a plugin delivers at most %d commands", MaxFilesPerKind))
				return
			}
			data, err := readRegular(file, MaxContentBytes)
			if err != nil {
				c.note(importer.StatusDropped, c.rel(file), "", err.Error())
				continue
			}
			var doc struct {
				Prompt      string `toml:"prompt"`
				Description string `toml:"description"`
			}
			if err := toml.Unmarshal(data, &doc); err != nil || strings.TrimSpace(doc.Prompt) == "" {
				c.note(importer.StatusUnsupported, c.rel(file), "", "the command is not valid TOML with a prompt")
				continue
			}
			taken[name] = true
			cf := config.ContentFile{Name: c.opt.NamePrefix + name, Path: file, Content: doc.Prompt}
			if doc.Description != "" {
				desc, _ := json.Marshal(doc.Description) //nolint:errcheck // a string always marshals
				cf.Metadata, _, _ = config.ParseFrontmatterChecked("---\ndescription: " + string(desc) + "\n---\n")
			}
			c.res.Commands = append(c.res.Commands, cf)
			c.note(importer.StatusApproximated, c.rel(file), "", "commands."+name+" (a Gemini TOML command is imported as a prompt; {{args}} and !{...} placeholders are kept verbatim)")
		}
	}
}

// collectContext imports the instruction files of a Gemini extension as rules.
func (c *loader) collectContext() {
	if c.layout.context == "" {
		return
	}
	names := []string{"GEMINI.md"}
	if list, ok := stringList(c.manifest.Fields[c.layout.context]); ok {
		names = list
	}
	prefix := safeStem(c.manifest.Name)
	if prefix == "" {
		prefix = safeStem(c.opt.Name)
	}
	for _, rel := range names {
		abs, err := SafeJoin(c.dir, rel)
		if err != nil {
			c.note(importer.StatusDropped, c.layout.manifest, c.layout.context, err.Error())
			continue
		}
		if !isRegular(abs) {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(path.Base(filepath.ToSlash(abs)), filepath.Ext(abs)))
		if cf, ok := c.markdownFile(abs, prefix+"-"+stem); ok {
			c.res.Rules = append(c.res.Rules, cf)
			c.note(importer.StatusMapped, c.rel(abs), "", "rules."+prefix+"-"+stem)
		}
	}
}

func safeStem(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
