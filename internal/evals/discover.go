package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxCaseFileBytes bounds one case file, prompt file or fixture source.
const maxCaseFileBytes = 1 << 20

// ProjectEvalsDir is the project-level tree under the config directory.
const ProjectEvalsDir = "evals"

// Skill is a skill in the source tree together with where its cases live.
type Skill struct {
	// ID is the skill's directory name.
	ID string
	// Domain is empty for a root skill.
	Domain string
	// Dir is the absolute skill directory.
	Dir string
	// EvalDirs are the absolute directories searched for cases that exist.
	EvalDirs []string
	// Escaped lists eval directories left out because they resolve (through a
	// symlink) to somewhere outside the config directory.
	Escaped []string
}

// FindSkills lists the skills under a config directory (root skills, then domain
// skills), sorted by id. A skill id that appears twice keeps the first (root
// before domain, domains alphabetically).
func FindSkills(configDir string) ([]Skill, error) {
	found, err := skillsIn(configDir, filepath.Join(configDir, "skills"), "")
	if err != nil {
		return nil, err
	}
	domains, err := os.ReadDir(filepath.Join(configDir, "domains"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read domains: %w", err)
	}
	for _, domain := range domains {
		if !domain.IsDir() {
			continue
		}
		more, err := skillsIn(configDir, filepath.Join(configDir, "domains", domain.Name(), "skills"), domain.Name())
		if err != nil {
			return nil, err
		}
		found = append(found, more...)
	}
	seen := map[string]bool{}
	out := found[:0]
	for _, skill := range found {
		if !seen[skill.ID] {
			seen[skill.ID] = true
			out = append(out, skill)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, nil
}

// skillsIn lists the skill directories (those holding a SKILL.md) in skillsDir.
func skillsIn(configDir, skillsDir, domain string) ([]Skill, error) {
	entries, err := os.ReadDir(skillsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", skillsDir, err)
	}
	var out []Skill
	for _, entry := range entries {
		dir := filepath.Join(skillsDir, entry.Name())
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if info, statErr := os.Stat(filepath.Join(dir, "SKILL.md")); statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		skill := Skill{ID: entry.Name(), Domain: domain, Dir: dir}
		for _, candidate := range []string{filepath.Join(dir, "evals"), filepath.Join(configDir, ProjectEvalsDir, entry.Name())} {
			if info, statErr := os.Stat(candidate); statErr != nil || !info.IsDir() {
				continue
			}
			if !resolvedInside(configDir, candidate) {
				skill.Escaped = append(skill.Escaped, candidate)
				continue
			}
			skill.EvalDirs = append(skill.EvalDirs, candidate)
		}
		out = append(out, skill)
	}
	return out, nil
}

// LoadCases reads every case file of a skill, resolves prompt files and fixture
// sources, and returns the authored cases (near misses not yet expanded) with the
// problems found. Files are read in path order, so the result is deterministic.
func LoadCases(skill *Skill) ([]Case, []Problem) {
	var cases []Case
	var problems []Problem
	ids := map[string]string{}
	for _, dir := range skill.Escaped {
		problems = append(problems, Problem{File: dir, Line: 1, Message: "eval directory resolves outside the config directory through a symlink"})
	}
	for _, dir := range skill.EvalDirs {
		for _, file := range caseFiles(dir) {
			data, err := readBounded(file)
			if err != nil {
				problems = append(problems, Problem{File: file, Line: 1, Message: err.Error()})
				continue
			}
			parsed, probs := ParseFile(file, data)
			problems = append(problems, probs...)
			for i := range parsed {
				c := parsed[i]
				c.ResolvedDir = filepath.Dir(file)
				problems = append(problems, c.resolve(dir)...)
				if c.ID != "" {
					if other, dup := ids[c.ID]; dup && other != file {
						problems = append(problems, Problem{File: file, Line: c.Line, Message: fmt.Sprintf("case id %q is also used in %s", c.ID, other)})
					}
					ids[c.ID] = file
				}
				cases = append(cases, c)
			}
		}
	}
	return cases, append(problems, nearMissCollisions(cases)...)
}

// nearMissCollisions reports an authored case whose id equals one derived from a
// near_miss list: the runner's results for the two would overwrite each other.
func nearMissCollisions(cases []Case) []Problem {
	authored := map[string]bool{}
	for i := range cases {
		authored[cases[i].ID] = true
	}
	var problems []Problem
	for i := range cases {
		for n := range cases[i].NearMiss {
			if id := nearMissID(cases[i].ID, n); authored[id] {
				problems = append(problems, Problem{File: cases[i].File, Line: cases[i].Line, Message: fmt.Sprintf("case %q: near_miss %d derives the id %q, which another case already uses", cases[i].ID, n+1, id)})
			}
		}
	}
	return problems
}

// resolve replaces prompt_file and file sources with their contents so a case
// handed to a runner is self-contained. root bounds where they may point.
func (c *Case) resolve(root string) []Problem {
	var problems []Problem
	fail := func(format string, args ...any) {
		problems = append(problems, Problem{File: c.File, Line: c.Line, Message: fmt.Sprintf("case %q: ", c.ID) + fmt.Sprintf(format, args...)})
	}
	read := func(rel string) (string, bool) {
		if checkRelPath(rel) != "" {
			return "", false // already reported by validation
		}
		full := filepath.Join(c.ResolvedDir, filepath.FromSlash(rel))
		if !within(root, full) {
			fail("%q points outside %s", rel, root)
			return "", false
		}
		// Every path component may be a symlink, so judge the resolved path.
		real, err := filepath.EvalSymlinks(full)
		if err != nil {
			fail("%q is not a regular file", rel)
			return "", false
		}
		if !resolvedInside(root, real) {
			fail("%q points outside %s", rel, root)
			return "", false
		}
		info, err := os.Stat(real)
		if err != nil || !info.Mode().IsRegular() {
			fail("%q is not a regular file", rel)
			return "", false
		}
		data, err := readBounded(real)
		if err != nil {
			fail("%v", err)
			return "", false
		}
		return string(data), true
	}
	if c.PromptFile != "" {
		if text, ok := read(c.PromptFile); ok {
			c.Prompt, c.PromptFile = strings.TrimRight(text, "\n"), ""
		}
	}
	for i := range c.Files {
		if c.Files[i].Source != "" {
			if text, ok := read(c.Files[i].Source); ok {
				c.Files[i].Content, c.Files[i].Source = text, ""
			}
		}
	}
	return problems
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolvedInside reports whether target, after resolving every symlink in both
// paths, lies inside root. Either path that cannot be resolved is outside.
func resolvedInside(root, target string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	return within(realRoot, realTarget)
}

func readBounded(file string) ([]byte, error) {
	f, err := os.Open(file) //nolint:gosec // the path comes from walking the user's eval tree
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only handle
	data, err := io.ReadAll(io.LimitReader(f, maxCaseFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCaseFileBytes {
		return nil, fmt.Errorf("file is larger than %d bytes", maxCaseFileBytes)
	}
	return data, nil
}

// caseFiles lists the case files below dir in path order, skipping hidden entries
// and result directories.
func caseFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // an unreadable entry is skipped
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		name := d.Name()
		if p != dir && (strings.HasPrefix(name, ".") || (d.IsDir() && (name == dirResults || name == "node_modules"))) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && IsCaseFile(name) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// SkillDigest is the sha256 of a skill's authored content: every regular file
// below the skill directory except its top-level evals/ directory, in path order.
// Editing a case therefore never makes the skill itself look edited.
func SkillDigest(skillDir string) (string, error) {
	return treeDigest([]digestRoot{{dir: skillDir, skip: "evals"}})
}

// CasesDigest is the sha256 of a skill's eval material (every file below its
// eval directories, fixtures and graders included).
func CasesDigest(skill *Skill) (string, error) {
	roots := make([]digestRoot, 0, len(skill.EvalDirs))
	for _, dir := range skill.EvalDirs {
		roots = append(roots, digestRoot{dir: dir, authored: true})
	}
	return treeDigest(roots)
}

type digestRoot struct {
	dir string
	// skip is a top-level directory left out of the digest.
	skip string
	// authored leaves out the results and node_modules directories, which case
	// discovery skips too. Hidden fixtures stay in: a case can reference them.
	authored bool
}

// osJunk are files operating systems and editors drop into folders; they must not
// change a digest, or the same skill would hash differently on two machines.
func osJunk(name string) bool {
	return name == ".DS_Store" || name == "Thumbs.db" || name == "desktop.ini" || strings.HasPrefix(name, "._")
}

// digestSkips says whether the walk leaves an entry out.
func (r digestRoot) skips(rel, name string, isDir bool) bool {
	if rel == "." {
		return false
	}
	if isDir && r.skip != "" && rel == r.skip {
		return true
	}
	if osJunk(name) {
		return true
	}
	if r.authored {
		return isDir && (name == dirResults || name == "node_modules")
	}
	return false
}

func treeDigest(roots []digestRoot) (string, error) {
	hash := sha256.New()
	for i, root := range roots {
		var files []string
		err := filepath.WalkDir(root.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(root.dir, p)
			if relErr != nil {
				return relErr
			}
			if root.skips(rel, d.Name(), d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() && (d.Type().IsRegular() || d.Type()&fs.ModeSymlink != 0) {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("digest %s: %w", root.dir, err)
		}
		sort.Strings(files)
		hash.Write([]byte("root " + strconv.Itoa(i) + "\n"))
		for _, rel := range files {
			data, err := digestContent(filepath.Join(root.dir, rel))
			if err != nil {
				return "", fmt.Errorf("digest %s: %w", rel, err)
			}
			hash.Write([]byte(filepath.ToSlash(rel) + "\x00" + strconv.Itoa(len(data)) + "\x00"))
			hash.Write(data)
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// digestContent is a file's bytes, or for a symlink its target path, so
// retargeting a link changes the digest without the digest reading outside the tree.
func digestContent(file string) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(file)
		if err != nil {
			return nil, err
		}
		return []byte("-> " + target), nil
	}
	return os.ReadFile(file) //nolint:gosec // walking the user's own tree
}
