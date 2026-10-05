package skillsource

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Limits on what one skill may contain; a skill is text an agent reads.
const (
	maxFileBytes  = 2 << 20
	maxSkillBytes = 8 << 20
	skillFile     = "SKILL.md"
)

// File is one file of a skill.
type File struct {
	// Path is relative to the skill directory, slash-separated.
	Path    string
	Content []byte
}

// Skill is one skill found in a source.
type Skill struct {
	// Name is the served name: the directory name with the source's name_prefix.
	Name string
	// Dir is the skill directory name inside the source.
	Dir string
	// Files lists SKILL.md first, then the rest in path order. When the source
	// has a name_prefix, SKILL.md's `name:` is rewritten to Name.
	Files []File
}

// Discover lists the skills below root: each immediate child directory that has
// a SKILL.md, or root itself when it has one. Include/exclude globs match the
// directory name; exclude wins. Symlinks are never followed (a link could point
// out of the source), and a skill over the size limits is skipped with a warning.
func Discover(spec Spec, root string) ([]Skill, error) {
	var dirs []string
	if fileExists(filepath.Join(root, skillFile)) {
		dirs = []string{root}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, oops.With("path", root).Wrapf(err, "read skill source %q", spec.Name)
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && fileExists(filepath.Join(root, e.Name(), skillFile)) {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
	}
	var skills []Skill
	for _, dir := range dirs {
		base := filepath.Base(dir)
		if !selected(spec, base) {
			continue
		}
		files, err := readSkill(dir)
		if err != nil {
			logger.Warn("Skipping a skill in a skill source", "source", spec.Name, "skill", base, "reason", err.Error())
			continue
		}
		name := spec.NamePrefix + base
		if spec.NamePrefix != "" {
			files[0].Content = rewriteName(files[0].Content, name)
		}
		skills = append(skills, Skill{Name: name, Dir: base, Files: files})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func selected(spec Spec, name string) bool {
	match := func(globs []string) bool {
		for _, g := range globs {
			if ok, err := path.Match(strings.TrimSpace(g), name); err == nil && ok {
				return true
			}
		}
		return false
	}
	if match(spec.Exclude) {
		return false
	}
	return len(spec.Include) == 0 || match(spec.Include)
}

func fileExists(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode().IsRegular()
}

func readSkill(dir string) ([]File, error) {
	var files []File
	total := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != dir {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		if !info.Mode().IsRegular() {
			return nil // symlinks and devices are not served
		}
		if info.Size() > maxFileBytes {
			return oops.Errorf("%s is larger than %d bytes", p, maxFileBytes)
		}
		total += int(info.Size())
		if total > maxSkillBytes {
			return oops.Errorf("the skill is larger than %d bytes", maxSkillBytes)
		}
		data, err := os.ReadFile(p) //nolint:gosec // p comes from WalkDir below the resolved source
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Content: data})
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "read skill directory")
	}
	sort.Slice(files, func(i, j int) bool {
		if (files[i].Path == skillFile) != (files[j].Path == skillFile) {
			return files[i].Path == skillFile
		}
		return files[i].Path < files[j].Path
	})
	if len(files) == 0 || files[0].Path != skillFile {
		return nil, oops.Errorf("no %s", skillFile)
	}
	return files, nil
}

// rewriteName sets the frontmatter `name:` of a SKILL.md, so the served name and
// the frontmatter agree. Content without frontmatter is returned unchanged.
func rewriteName(content []byte, name string) []byte {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return content
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return content
	}
	front, after := rest[:end], rest[end:]
	quoted, err := yaml.Marshal(name)
	if err != nil {
		return content
	}
	line := "name: " + strings.TrimSpace(string(quoted))
	lines := strings.Split(front, "\n")
	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(l, "name:") {
			lines[i], replaced = line, true
			break
		}
	}
	if !replaced {
		lines = append([]string{line}, lines...)
	}
	return []byte("---\n" + strings.Join(lines, "\n") + after)
}
