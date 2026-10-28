package areas

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
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
	clean := filepath.Clean(filepath.FromSlash(name))
	if safefs.RelEscapes(clean) {
		return "", "must stay inside the project root"
	}
	path = filepath.Join(root, clean)
	// A symlink may not lead the file out of the project either; a missing file is left to LoadFile.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if realRoot, err := filepath.EvalSymlinks(root); err == nil && !safefs.Within(realRoot, resolved) {
			return "", "must stay inside the project root"
		}
	}
	return path, ""
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
// with the inline [exposure] form: in the problem's area table, the line assigning the problem's key with the
// offending text, else the first line holding that text (the table header when there is none); or the
// [skill_areas] line whose key is the offending skill. It is 0 when the problem is not about an entry or the
// entry is not found. A line inside a multi-line string is never read as a table header.
func (p Problem) LineIn(lines []string) int {
	switch {
	case p.area > 0:
		return areaLine(lines, p.area, p.field, p.needle)
	case p.skill != "":
		return skillLine(lines, p.skill)
	}
	return 0
}

// areaLine finds needle in the n-th area table, preferring the line that assigns field, else that table's
// header line.
func areaLine(lines []string, n int, field, needle string) int {
	headers := tableHeaders(lines)
	start, seen := -1, 0
	for i, h := range headers {
		if !h.ok || !isAreaTable(h.name) {
			continue
		}
		seen++
		if seen == n {
			start = i
			break
		}
	}
	if start < 0 {
		return 0
	}
	if needle == "" {
		return start + 1
	}
	fallback := start + 1
	for i := start + 1; i < len(lines) && !headers[i].ok; i++ {
		if !strings.Contains(lines[i], needle) {
			continue
		}
		if key, ok := lineKey(lines[i]); ok && key == field {
			return i + 1
		}
		if fallback == start+1 {
			fallback = i + 1
		}
	}
	return fallback
}

// SkillLine is the 1-based line assigning skill in the [skill_areas] table of the lines of an areas file (or
// the [exposure.skill_areas] table of a configuration file), else that table's header line, else 0.
func SkillLine(lines []string, skill string) int { return skillLine(lines, skill) }

// skillLine finds the line assigning skill in the skill table, else that table's header line.
func skillLine(lines []string, skill string) int {
	headers := tableHeaders(lines)
	start := -1
	for i, h := range headers {
		if h.ok {
			if start >= 0 {
				break
			}
			if isSkillTable(h.name) {
				start = i
			}
			continue
		}
		if key, ok := lineKey(lines[i]); start >= 0 && ok && key == skill {
			return i + 1
		}
	}
	return start + 1
}

// isAreaTable and isSkillTable recognize the tables of an area entry and of the skill assignments, in an areas
// file and in the inline form.
func isAreaTable(name string) bool  { return name == "area" || name == "exposure.areas" }
func isSkillTable(name string) bool { return name == "skill_areas" || name == "exposure.skill_areas" }

// header is what tableHeaders makes of one line: whether it is a table header, and the table's name.
type header struct {
	name string
	ok   bool
}

// tableHeaders reads the table header of every line, skipping the lines a multi-line string spans.
func tableHeaders(lines []string) []header {
	out := make([]header, len(lines))
	open := ""
	for i, l := range lines {
		if open == "" {
			out[i].name, out[i].ok = tableHeader(l)
		}
		open = multilineOpen(l, open)
	}
	return out
}

// multilineOpen is the multi-line string delimiter (three double or three single quotes) still open
// after line, given the one open before it (empty for none).
func multilineOpen(line, open string) string {
	for {
		if open == "" {
			i, d := firstDelimiter(line)
			if i < 0 {
				return ""
			}
			open, line = d, line[i+len(d):]
		}
		j := strings.Index(line, open)
		if j < 0 {
			return open
		}
		line, open = line[j+len(open):], ""
	}
}

func firstDelimiter(line string) (index int, delimiter string) {
	basic, literal := strings.Index(line, `"""`), strings.Index(line, "'''")
	switch {
	case basic < 0 && literal < 0:
		return -1, ""
	case literal < 0 || (basic >= 0 && basic < literal):
		return basic, `"""`
	}
	return literal, "'''"
}

// LineKey is the key a TOML line assigns (see lineKey); ok is false for a line that assigns none.
func LineKey(line string) (key string, ok bool) { return lineKey(line) }

// lineKey is the key a TOML line assigns: a bare, "basic" or 'literal' key followed by "=".
func lineKey(line string) (string, bool) {
	t := strings.TrimSpace(line)
	var key, rest string
	switch {
	case strings.HasPrefix(t, `"`):
		end := closingQuote(t)
		if end < 0 {
			return "", false
		}
		k, err := strconv.Unquote(t[:end+1])
		if err != nil {
			k = t[1:end]
		}
		key, rest = k, t[end+1:]
	case strings.HasPrefix(t, "'"):
		end := strings.IndexByte(t[1:], '\'')
		if end < 0 {
			return "", false
		}
		key, rest = t[1:end+1], t[end+2:]
	default:
		end := strings.IndexAny(t, " \t=")
		if end <= 0 {
			return "", false
		}
		key, rest = t[:end], t[end:]
	}
	return key, strings.HasPrefix(strings.TrimSpace(rest), "=")
}

// closingQuote is the index of the quote that closes the basic string at the start of t, or -1.
func closingQuote(t string) int {
	for i := 1; i < len(t); i++ {
		switch t[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

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
