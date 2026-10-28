package areas

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// MaxFileBytes bounds the areas file read; a reader that shows its lines reads it with the same bound.
const MaxFileBytes = 4 << 20

// fileDoc is the layout of an areas file.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type fileDoc struct {
	Area       []config.ExposureArea `toml:"area"`
	SkillAreas map[string]string     `toml:"skill_areas"`
}

// Load returns the area map an [exposure] table configures: the inline [[exposure.areas]] and
// [exposure.skill_areas], or the areas_file it names, relative to the project root. It returns nil and no
// problems when the table is absent or configures no areas.
func Load(cfg *config.ExposureConfig, root string) (*Map, []Problem) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.AreasFile == "" {
		if len(cfg.Areas) == 0 && len(cfg.SkillAreas) == 0 {
			return nil, nil
		}
		return New(cfg.Areas, cfg.SkillAreas)
	}
	if len(cfg.Areas) > 0 || len(cfg.SkillAreas) > 0 {
		return nil, []Problem{{Message: "exposure sets both areas_file and inline areas or skill_areas; keep the area map in one place", Key: "areas_file"}}
	}
	path, why := resolveAreasFile(root, cfg.AreasFile)
	if why != "" {
		return nil, []Problem{{Message: fmt.Sprintf("exposure.areas_file %q %s", cfg.AreasFile, why), Key: "areas_file"}}
	}
	return LoadFile(path)
}

// resolveAreasFile joins a configured areas_file to the project root, or says why it cannot be used.
func resolveAreasFile(root, name string) (path, why string) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return "", "must be a forward-slash path relative to the project root"
	}
	if safefs.RelEscapes(filepath.FromSlash(name)) {
		return "", "must stay inside the project root"
	}
	return filepath.Join(root, filepath.FromSlash(name)), ""
}

// LoadFile reads and validates an areas file: [[area]] tables and a [skill_areas] table. Every problem names
// the file; an unknown key is one, so a misspelled key is never silently ignored.
func LoadFile(path string) (*Map, []Problem) {
	data, err := safefs.ReadFileLimited(path, MaxFileBytes)
	if err != nil {
		return nil, []Problem{{File: path, Message: fmt.Sprintf("areas file cannot be read: %v", err)}}
	}
	var doc fileDoc
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, decodeProblems(path, err)
	}
	m, problems := New(doc.Area, doc.SkillAreas)
	lines := strings.Split(string(data), "\n")
	for i := range problems {
		problems[i].File = path
		problems[i].Line = max(problems[i].LineIn(lines), 1)
	}
	return m, problems
}

func decodeProblems(path string, err error) []Problem {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		out := make([]Problem, 0, len(strict.Errors))
		for i := range strict.Errors {
			row, _ := strict.Errors[i].Position()
			out = append(out, Problem{File: path, Line: max(row, 1), Message: fmt.Sprintf("areas file has unknown key %q", strings.Join(strict.Errors[i].Key(), "."))})
		}
		return out
	}
	var decodeErr *toml.DecodeError
	if errors.As(err, &decodeErr) {
		row, _ := decodeErr.Position()
		return []Problem{{File: path, Line: max(row, 1), Message: fmt.Sprintf("areas file is not valid TOML: %s", decodeErr.Error())}}
	}
	return []Problem{{File: path, Line: 1, Message: fmt.Sprintf("areas file is not valid: %v", err)}}
}

// LineIn is the 1-based line of the problem's entry in the lines of an areas file, or of a configuration file
// with the inline [exposure] form: the line in the problem's area table holding the offending text (the table
// header when there is none), or the [skill_areas] line of the offending key. It is 0 when the problem is not
// about an entry or the entry is not found.
func (p Problem) LineIn(lines []string) int {
	switch {
	case p.area > 0:
		return areaLine(lines, p.area, p.needle)
	case p.skill != "":
		return skillLine(lines, p.skill)
	}
	return 0
}

// areaLine finds needle in the n-th area table, else that table's header line.
func areaLine(lines []string, n int, needle string) int {
	seen, start := 0, 0
	for i, l := range lines {
		name, isHeader := tableHeader(l)
		switch {
		case isHeader && start > 0:
			return start
		case isHeader && isAreaTable(name):
			seen++
			if seen == n {
				start = i + 1
				if needle == "" {
					return start
				}
			}
		case start > 0 && strings.Contains(l, needle):
			return i + 1
		}
	}
	return start
}

// skillLine finds the line assigning skill in the skill table, else that table's header line.
func skillLine(lines []string, skill string) int {
	start := 0
	for i, l := range lines {
		if name, isHeader := tableHeader(l); isHeader {
			if start > 0 {
				break
			}
			if isSkillTable(name) {
				start = i + 1
			}
			continue
		}
		if key, _, found := strings.Cut(l, "="); start > 0 && found && strings.Trim(strings.TrimSpace(key), `"'`) == skill {
			return i + 1
		}
	}
	return start
}

// isAreaTable and isSkillTable recognize the tables of an area entry and of the skill assignments, in an areas
// file and in the inline form.
func isAreaTable(name string) bool  { return name == "area" || name == "exposure.areas" }
func isSkillTable(name string) bool { return name == "skill_areas" || name == "exposure.skill_areas" }

// tableHeader reports whether a TOML line is a table header and its name.
func tableHeader(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "[") {
		return "", false
	}
	if end := strings.Index(t, "]"); end > 0 {
		t = t[:end]
	}
	return strings.TrimSpace(strings.Trim(t, "[")), true
}
