package lint

import (
	"encoding/json"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// CodeCommandMissing reports a backticked command that names a target the
// repository's own build files do not define.
const CodeCommandMissing = "AR403"

func registerArCommands(s *ruleSet) {
	s.addRules(RuleInfo{CodeCommandMissing, "command-missing", SeverityWarning, "a backticked `npm run X`, `make X`, `task X`, `just X` or `pytest -m X` names a script, target, task, recipe or marker the repository does not define"})
	s.addRunCheck(checkDeadCommands, AnalyzerReferences)
}

var (
	npmRunRe      = regexp.MustCompile(`^(?:npm|pnpm|yarn|bun)\s+run(?:-script)?\s+(?:--silent\s+|-s\s+|--if-present\s+)*([A-Za-z0-9][A-Za-z0-9:_.@/-]*)`)
	makeRe        = regexp.MustCompile(`^make\s+(.+)$`)
	taskRe        = regexp.MustCompile(`^(?:task|go-task)\s+([a-z][A-Za-z0-9:_.-]*)`)
	justRe        = regexp.MustCompile(`^just\s+([A-Za-z_][A-Za-z0-9_:-]*)`)
	pytestMarkRe  = regexp.MustCompile(`^(?:python3?\s+-m\s+)?pytest\b.*?\s-m\s*(?:"([^"]*)"|'([^']*)'|(\S+))`)
	scopedFlagRe  = regexp.MustCompile(`(?:^|\s)(?:--prefix|--workspaces?|--filter|-w|-C|--directory|-f|--file|--justfile|--taskfile|-t|-d|--dir|--cwd)(?:[=\s]|$)|\bcd\s`)
	placeholderRe = regexp.MustCompile(`[<>${}*|\\]|\.{3}|\bXXX\b|\bNAME\b`)
	makeTargetRe  = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.%/ -]*?)\s*:(?:[^=]|$)`)
	justRecipeRe  = regexp.MustCompile(`^@?([A-Za-z_][A-Za-z0-9_-]*)\b[^:=\n]*:(?:[^=]|$)`)
	markerLineRe  = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?::.*)?$`)
	addMarkerRe   = regexp.MustCompile(`addinivalue_line\(\s*["']markers["']\s*,\s*["']\s*([A-Za-z_][A-Za-z0-9_]*)`)
	usedMarkerRe  = regexp.MustCompile(`\bpytest\.mark\.([A-Za-z_][A-Za-z0-9_]*)`)
	markerWordRe  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	targetTokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

var pytestBuiltinMarks = map[string]bool{
	"skip": true, "skipif": true, "xfail": true, "parametrize": true, "usefixtures": true, "filterwarnings": true,
	wordAnd: true, "or": true, "not": true, boolTrue: true, boolFalse: true,
}

// manifests indexes the build files of the repository.
type manifests struct {
	scripts  map[string]bool
	hasNPM   bool
	targets  map[string]bool
	hasMake  bool
	dynMake  bool
	tasks    map[string]bool
	hasTask  bool
	taskNS   bool // the Taskfile includes others, so namespaced tasks are unknown
	recipes  map[string]bool
	hasJust  bool
	justMods bool
	markers  map[string]bool
	hasPyt   bool
}

func (r *runner) loadManifests() *manifests { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	m := &manifests{scripts: map[string]bool{}, targets: map[string]bool{}, tasks: map[string]bool{}, recipes: map[string]bool{}, markers: map[string]bool{}}
	paths := r.tree.Paths()
	sort.Strings(paths)
	var pyFiles []string
	for _, p := range paths {
		base := path.Base(p)
		lower := strings.ToLower(base)
		switch {
		case strings.Contains(p, "node_modules/"):
			continue
		case base == "package.json":
			m.readPackageJSON(r.tree.Top, p)
		case lower == "makefile" || lower == "gnumakefile" || strings.HasSuffix(lower, ".mk"):
			m.readMakefile(r.tree.Top, p)
		case strings.HasPrefix(lower, "taskfile.") && (strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")):
			m.readTaskfile(r.tree.Top, p)
		case lower == "justfile" || lower == ".justfile":
			m.readJustfile(r.tree.Top, p)
		case lower == "pyproject.toml":
			m.readPyproject(r.tree.Top, p)
		case lower == "pytest.ini" || lower == "tox.ini" || lower == "setup.cfg":
			m.readIni(r.tree.Top, p)
		case strings.HasSuffix(lower, ".py"):
			pyFiles = append(pyFiles, p)
		}
	}
	if m.hasPyt || len(pyFiles) > 0 {
		m.hasPyt = true
		m.readPythonMarkers(r.tree.Top, pyFiles)
	}
	return m
}

func readTracked(top, rel string) string {
	data, err := readSmallFile(filepath.Join(top, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return string(data)
}

func (m *manifests) readPackageJSON(top, rel string) {
	var pkg struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	data := readTracked(top, rel)
	if data == "" || json.Unmarshal([]byte(data), &pkg) != nil {
		return
	}
	m.hasNPM = true
	for name := range pkg.Scripts {
		m.scripts[name] = true
	}
}

func (m *manifests) readMakefile(top, rel string) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	m.hasMake = true
	for _, line := range strings.Split(readTracked(top, rel), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "\t") || trim == "" || strings.HasPrefix(trim, "#"):
			continue
		case strings.HasPrefix(trim, "include ") || strings.HasPrefix(trim, "-include ") || strings.HasPrefix(trim, "sinclude "):
			m.dynMake = true
			continue
		}
		head := line
		if strings.HasPrefix(trim, ".PHONY") {
			if _, after, ok := strings.Cut(trim, ":"); ok {
				for _, t := range strings.Fields(after) {
					m.targets[t] = true
				}
			}
			continue
		}
		if mm := makeTargetRe.FindStringSubmatch(head); mm != nil {
			for _, t := range strings.Fields(mm[1]) {
				if strings.ContainsAny(t, "$%(") {
					m.dynMake = true
				}
				m.targets[t] = true
			}
		} else if strings.Contains(head, "$(") && strings.Contains(head, ":") {
			m.dynMake = true
		}
	}
}

func (m *manifests) readTaskfile(top, rel string) {
	var tf struct {
		Tasks    map[string]any `yaml:"tasks"`
		Includes map[string]any `yaml:"includes"`
	}
	if yaml.Unmarshal([]byte(readTracked(top, rel)), &tf) != nil {
		return
	}
	m.hasTask = true
	for name := range tf.Tasks {
		m.tasks[name] = true
	}
	if len(tf.Includes) > 0 {
		m.taskNS = true
	}
}

func (m *manifests) readJustfile(top, rel string) {
	m.hasJust = true
	for _, line := range strings.Split(readTracked(top, rel), "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "mod ") || strings.HasPrefix(line, "import "):
			m.justMods = true
		case strings.HasPrefix(line, "alias "):
			if f := strings.Fields(line); len(f) > 1 {
				m.recipes[f[1]] = true
			}
		default:
			if mm := justRecipeRe.FindStringSubmatch(line); mm != nil {
				m.recipes[mm[1]] = true
			}
		}
	}
}

func (m *manifests) readPyproject(top, rel string) {
	var doc struct {
		Tool struct {
			Pytest struct {
				Ini struct {
					Markers []string `toml:"markers"`
				} `toml:"ini_options"`
			} `toml:"pytest"`
		} `toml:"tool"`
	}
	if toml.Unmarshal([]byte(readTracked(top, rel)), &doc) != nil {
		return
	}
	if len(doc.Tool.Pytest.Ini.Markers) > 0 || strings.Contains(readTracked(top, rel), "[tool.pytest") {
		m.hasPyt = true
	}
	for _, mk := range doc.Tool.Pytest.Ini.Markers {
		if w := markerWordRe.FindString(mk); w != "" {
			m.markers[w] = true
		}
	}
}

func (m *manifests) readIni(top, rel string) {
	in := false
	inMarkers := false
	for _, line := range strings.Split(readTracked(top, rel), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			in = trim == "[pytest]" || trim == "[tool:pytest]"
			inMarkers = false
			if in {
				m.hasPyt = true
			}
			continue
		}
		if !in {
			continue
		}
		if rest, ok := strings.CutPrefix(trim, "markers"); ok {
			inMarkers = true
			if _, val, has := strings.Cut(rest, "="); has && strings.TrimSpace(val) != "" {
				if w := markerWordRe.FindString(val); w != "" {
					m.markers[w] = true
				}
			}
			continue
		}
		if inMarkers && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			if mm := markerLineRe.FindStringSubmatch(line); mm != nil {
				m.markers[mm[1]] = true
			}
			continue
		}
		inMarkers = false
	}
}

// maxPythonScan bounds how many Python files are read for marker usage.
const maxPythonScan = 3000

func (m *manifests) readPythonMarkers(top string, pyFiles []string) {
	sort.SliceStable(pyFiles, func(i, j int) bool {
		rank := func(p string) int {
			b := path.Base(p)
			switch {
			case b == "conftest.py":
				return 0
			case strings.HasPrefix(b, "test_") || strings.HasSuffix(b, "_test.py"):
				return 1
			}
			return 2
		}
		return rank(pyFiles[i]) < rank(pyFiles[j])
	})
	for i, p := range pyFiles {
		if i >= maxPythonScan {
			break
		}
		text := readTracked(top, p)
		if !strings.Contains(text, "pytest") {
			continue
		}
		for _, mm := range addMarkerRe.FindAllStringSubmatch(text, -1) {
			m.markers[mm[1]] = true
		}
		for _, mm := range usedMarkerRe.FindAllStringSubmatch(text, -1) {
			m.markers[mm[1]] = true
		}
	}
}

func checkDeadCommands(r *runner) {
	var m *manifests
	manifest := func() *manifests {
		if m == nil {
			m = r.loadManifests()
		}
		return m
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.owned {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		for _, l := range d.body() {
			for _, mm := range backtickRe.FindAllStringSubmatch(l.Text, -1) {
				span := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(mm[1]), "$ "))
				if span == "" || placeholderRe.MatchString(span) {
					continue
				}
				r.checkCommandSpan(it, l.No, span, manifest)
			}
		}
	}
}

func (r *runner) checkCommandSpan(it *item, line int, span string, manifest func() *manifests) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	if mm := pytestMarkRe.FindStringSubmatch(span); mm != nil {
		expr := mm[1] + mm[2] + mm[3]
		m := manifest()
		if !m.hasPyt {
			return
		}
		for _, w := range markerWordRe.FindAllString(expr, -1) {
			if !pytestBuiltinMarks[w] && !m.markers[w] {
				r.add(CodeCommandMissing, it.abs, line, "`pytest -m` names the marker %q, which no pytest configuration or test registers", w)
			}
		}
		return
	}
	if scopedFlagRe.MatchString(span) {
		return
	}
	switch {
	case npmRunRe.MatchString(span):
		name := npmRunRe.FindStringSubmatch(span)[1]
		if m := manifest(); m.hasNPM && !m.scripts[name] {
			r.add(CodeCommandMissing, it.abs, line, "`%s`: no package.json defines a script named %q", span, name)
		}
	case makeRe.MatchString(span):
		m := manifest()
		if !m.hasMake || m.dynMake {
			return
		}
		for _, tok := range strings.Fields(makeRe.FindStringSubmatch(span)[1]) {
			if strings.HasPrefix(tok, "-") || strings.Contains(tok, "=") || !targetTokenRe.MatchString(tok) {
				continue
			}
			if !m.targets[tok] {
				r.add(CodeCommandMissing, it.abs, line, "`%s`: no Makefile defines the target %q", span, tok)
			}
		}
	case taskRe.MatchString(span):
		name := taskRe.FindStringSubmatch(span)[1]
		if m := manifest(); m.hasTask && !m.tasks[name] && (!m.taskNS || !strings.Contains(name, ":")) {
			r.add(CodeCommandMissing, it.abs, line, "`%s`: no Taskfile defines the task %q", span, name)
		}
	case justRe.MatchString(span):
		name := justRe.FindStringSubmatch(span)[1]
		if m := manifest(); m.hasJust && !m.recipes[name] && (!m.justMods || !strings.Contains(name, "::")) {
			r.add(CodeCommandMissing, it.abs, line, "`%s`: no justfile defines the recipe %q", span, name)
		}
	}
}
