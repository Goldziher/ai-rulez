package lint

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// A stage is the read-only copy of ai-rulez-owned content a scanner is allowed
// to see. It is written from the content already loaded in memory, never by
// copying a path from disk, so a symlink in the repository cannot smuggle a
// file in, and it lives in a scratch directory outside the project. The
// scanner gets no other file: not .env, not unrelated code, not the config.

// Input kinds a [[lint.external]] entry may stage.
const (
	inputRules    = "rules"
	inputContext  = "context"
	inputSkills   = "skills"
	inputAgents   = "agents"
	inputCommands = "commands"
	inputChecks   = "checks"
	inputHooks    = "hooks"
	inputMCP      = "mcp"
	inputImports  = "imports"
)

var inputKinds = []string{inputRules, inputContext, inputSkills, inputAgents, inputCommands, inputChecks, inputHooks, inputMCP, inputImports}

// itemInputs maps an item kind to the input that selects it.
var itemInputs = map[string]string{
	kindRule: inputRules, kindContext: inputContext, kindSkill: inputSkills,
	kindAgent: inputAgents, kindCommand: inputCommands, kindCheck: inputChecks,
}

// Placeholders expanded in a staged scanner's command.
const (
	phStage     = "{stage}"
	phRoot      = "{root}"
	phFiles     = "{files}"
	phSkillDirs = "{skill_dirs}"
	phOut       = "{out}"
	phTmp       = "{tmp}"
)

var (
	stagePlaceholderRe = regexp.MustCompile(`\{[a-z_]+\}`)
	listHolders        = map[string]bool{phFiles: true, phSkillDirs: true}
	allHolders         = map[string]bool{phStage: true, phRoot: true, phFiles: true, phSkillDirs: true, phOut: true, phTmp: true}
)

// inputProblems lists what is wrong with an entry's inputs and its command's placeholders.
func inputProblems(ex config.LintExternal) []string {
	var problems []string
	for _, in := range ex.Inputs {
		if !containsString(inputKinds, in) {
			problems = append(problems, fmt.Sprintf("inputs %q is not one of %s", in, strings.Join(inputKinds, ", ")))
		}
	}
	for _, arg := range ex.Command {
		for _, ph := range stagePlaceholderRe.FindAllString(arg, -1) {
			switch {
			case !allHolders[ph]:
				problems = append(problems, fmt.Sprintf("command has the unknown placeholder %s (use %s)", ph, "{stage}, {root}, {files}, {skill_dirs}, {out} or {tmp}"))
			case len(ex.Inputs) == 0:
				problems = append(problems, fmt.Sprintf("command uses %s, which needs inputs (the staged content)", ph))
			case listHolders[ph] && arg != ph:
				problems = append(problems, fmt.Sprintf("%s must be an argument on its own (it expands to one argument per path)", ph))
			}
		}
	}
	return problems
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// scannerStage is one built stage.
type scannerStage struct {
	scratch string // the scratch directory that holds everything below
	root    string // scratch/stage
	// files are the absolute stage paths of every staged file, sorted.
	files []string
	// skillDirs are the absolute stage paths of the staged skill directories.
	skillDirs []string
	// source maps a stage-relative slash path to the file it was made from.
	source map[string]string
}

// stagedFile is one file about to be written.
type stagedFile struct {
	rel     string
	data    []byte
	source  string
	skillOf string // the stage-relative directory when the file belongs to a skill directory
}

// buildStage writes the stage for inputs. It returns an error only for a
// failure to create or write the scratch area.
func (r *runner) buildStage(inputs []string) (*scannerStage, error) {
	want := map[string]bool{}
	for _, in := range inputs {
		want[in] = true
	}
	files := r.stageFiles(want)
	base, err := os.MkdirTemp("", "ai-rulez-scan-")
	if err != nil {
		return nil, fmt.Errorf("create the scratch directory: %w", err)
	}
	if real, rerr := filepath.EvalSymlinks(base); rerr == nil {
		base = real // a scanner prints paths it resolved; match them
	}
	st := &scannerStage{scratch: base, root: filepath.Join(base, "stage"), source: map[string]string{}}
	if err := st.write(files); err != nil {
		st.cleanup()
		return nil, err
	}
	return st, nil
}

// stageFiles lists the files for the wanted inputs, sorted by stage path.
func (r *runner) stageFiles(want map[string]bool) []stagedFile {
	var out []stagedFile
	used := map[string]bool{}
	claim := func(rel string) string {
		rel = path.Clean(strings.ReplaceAll(rel, `\`, "/"))
		cand, n := rel, 1
		for used[cand] {
			n++
			cand = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(rel, path.Ext(rel)), n, path.Ext(rel))
		}
		used[cand] = true
		return cand
	}
	cfgFile := r.configFilePath()
	for i := range r.items {
		it := &r.items[i]
		if it.isDoc || (it.owned && !want[itemInputs[it.kind]]) || (!it.owned && !want[inputImports]) {
			continue
		}
		dir, rel := r.stagePath(it)
		if rel == "" {
			continue
		}
		file := stagedFile{rel: claim(rel), data: []byte(it.cf.Content), source: it.abs}
		if it.owned {
			// The loaded content has its frontmatter removed; the scanner must see the file
			// as it is on disk, so reported line numbers match the real file.
			if data, ok := r.readStageSource(it.abs); ok {
				file.data = data
			}
		} else {
			file.source = cfgFile
		}
		out = append(out, file)
		if dir == "" {
			continue
		}
		out[len(out)-1].skillOf = path.Dir(file.rel)
		for _, res := range it.cf.Resources {
			rr := path.Clean(strings.ReplaceAll(res.RelPath, `\`, "/"))
			if rr == "." || path.IsAbs(rr) || rr == ".." || strings.HasPrefix(rr, "../") {
				continue
			}
			src := cfgFile
			if it.owned {
				src = filepath.Join(it.itemDir, filepath.FromSlash(rr))
			}
			out = append(out, stagedFile{rel: claim(path.Join(path.Dir(file.rel), rr)), data: res.Content, source: src, skillOf: path.Dir(file.rel)})
		}
	}
	if want[inputHooks] && len(r.cfg.Hooks) > 0 {
		if data, err := json.MarshalIndent(r.cfg.Hooks, "", "  "); err == nil {
			out = append(out, stagedFile{rel: claim(r.stageBase() + "/hooks.json"), data: append(data, '\n'), source: cfgFile})
		}
	}
	if want[inputMCP] {
		if data := r.mcpStageJSON(); data != nil {
			out = append(out, stagedFile{rel: claim(r.stageBase() + "/mcp-servers.json"), data: data, source: cfgFile})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out
}

// maxStagedFile bounds a file copied into a stage.
const maxStagedFile = 4 << 20

// readStageSource reads a content file for staging. It refuses anything that is
// not a regular file whose resolved location is inside the repository, so a
// symlink cannot make the stage carry a file from elsewhere.
func (r *runner) readStageSource(abs string) ([]byte, bool) {
	if r.tree.Rel(abs) == "" {
		return nil, false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxStagedFile {
		return nil, false
	}
	data, err := os.ReadFile(abs) //nolint:gosec // resolved inside the repository above
	if err != nil {
		return nil, false
	}
	return data, true
}

// stageBase is the stage directory that stands for the configuration directory.
func (r *runner) stageBase() string {
	if abs, err := filepath.Abs(r.cfg.ConfigDir); err == nil {
		if rel := r.tree.Rel(abs); rel != "" && rel != "." {
			return rel
		}
	}
	return ".ai-rulez"
}

// stagePath returns the stage-relative path of an item (and its skill directory
// when it is a skill's SKILL.md). Owned items keep their repository-relative
// path; imported ones go under imports/ because their source is a cache or a
// remote, not a path in this project.
func (r *runner) stagePath(it *item) (skillDir, rel string) {
	if it.owned {
		rel = r.tree.Rel(it.abs)
		if rel == "" || strings.HasPrefix(rel, "../") {
			return "", ""
		}
	} else {
		id := sanitizeStageName(itemID(it.kind, it.cf))
		if it.kind == kindSkill {
			rel = path.Join(r.stageBase(), "imports", it.kind, id, "SKILL.md")
		} else {
			rel = path.Join(r.stageBase(), "imports", it.kind, id+".md")
		}
	}
	if it.kind == kindSkill && (it.itemDir != "" || !it.owned) {
		skillDir = path.Dir(rel)
	}
	return skillDir, rel
}

var stageNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeStageName(s string) string {
	s = strings.Trim(stageNameRe.ReplaceAllString(s, "-"), "-.")
	if s == "" {
		return "item"
	}
	return s
}

// mcpStageJSON renders the MCP servers without their secrets: environment
// variables and headers are reduced to their names.
func (r *runner) mcpStageJSON() []byte {
	servers := r.cfg.EffectiveMCPServers()
	if len(servers) == 0 {
		return nil
	}
	type entry struct {
		Name      string   `json:"name"`
		Transport string   `json:"transport,omitempty"`
		Command   string   `json:"command,omitempty"`
		Args      []string `json:"args,omitempty"`
		URL       string   `json:"url,omitempty"`
		EnvNames  []string `json:"env_names,omitempty"`
		Headers   []string `json:"header_names,omitempty"`
	}
	list := make([]entry, 0, len(servers))
	for i := range servers {
		s := &servers[i]
		e := entry{Name: s.Name, Transport: s.Transport, Command: s.Command, Args: s.Args, URL: s.URL}
		for k := range s.Env {
			e.EnvNames = append(e.EnvNames, k)
		}
		for k := range s.Headers {
			e.Headers = append(e.Headers, k)
		}
		sort.Strings(e.EnvNames)
		sort.Strings(e.Headers)
		list = append(list, e)
	}
	data, err := json.MarshalIndent(map[string]any{"mcp_servers": list}, "", "  ")
	if err != nil {
		return nil
	}
	return append(data, '\n')
}

// write creates the stage tree, then makes it read-only (directories 0500,
// files 0400). Writing goes through os.Root, so no stage path can leave the root.
func (st *scannerStage) write(files []stagedFile) error {
	for _, d := range []string{st.root, filepath.Join(st.scratch, "home"), filepath.Join(st.scratch, "tmp")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create the scratch directory: %w", err)
		}
	}
	root, err := os.OpenRoot(st.root)
	if err != nil {
		return fmt.Errorf("open the stage: %w", err)
	}
	defer root.Close() //nolint:errcheck // the root was only used for writing
	skills := map[string]bool{}
	for _, f := range files {
		if dir := path.Dir(f.rel); dir != "." {
			if err := root.MkdirAll(filepath.FromSlash(dir), 0o700); err != nil {
				return fmt.Errorf("stage %s: %w", f.rel, err)
			}
		}
		if err := root.WriteFile(filepath.FromSlash(f.rel), f.data, 0o600); err != nil {
			return fmt.Errorf("stage %s: %w", f.rel, err)
		}
		st.files = append(st.files, filepath.Join(st.root, filepath.FromSlash(f.rel)))
		st.source[f.rel] = f.source
		if f.skillOf != "" {
			skills[f.skillOf] = true
		}
	}
	for d := range skills {
		st.skillDirs = append(st.skillDirs, filepath.Join(st.root, filepath.FromSlash(d)))
	}
	sort.Strings(st.skillDirs)
	return st.seal()
}

// seal makes the staged tree read-only. On Windows the mode bits do not
// prevent writes by the owner; the stage is still a private copy.
func (st *scannerStage) seal() error {
	var dirs []string
	err := filepath.WalkDir(st.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		return os.Chmod(p, 0o400)
	})
	if err != nil {
		return fmt.Errorf("seal the stage: %w", err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], 0o500); err != nil {
			return fmt.Errorf("seal the stage: %w", err)
		}
	}
	return nil
}

// cleanup removes the scratch directory, restoring write permission first.
func (st *scannerStage) cleanup() {
	if st == nil || st.scratch == "" {
		return
	}
	_ = filepath.WalkDir(st.scratch, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // best effort before removal
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700) //nolint:errcheck // best effort
		}
		return nil
	})
	_ = os.RemoveAll(st.scratch) //nolint:errcheck // a temp directory the OS will reap
}

// outFile is the path of the {out} file.
func (st *scannerStage) outFile() string { return filepath.Join(st.scratch, "out.sarif") }

// expand turns a command template into argv. list placeholders become one
// argument per path; the others are replaced inside the argument. No shell is involved.
func (st *scannerStage) expand(cmd []string) []string {
	repl := strings.NewReplacer(phStage, st.root, phRoot, st.root, phOut, st.outFile(), phTmp, filepath.Join(st.scratch, "tmp"))
	argv := make([]string, 0, len(cmd)+len(st.files))
	for _, arg := range cmd {
		switch arg {
		case phFiles:
			argv = append(argv, st.files...)
		case phSkillDirs:
			argv = append(argv, st.skillDirs...)
		default:
			argv = append(argv, repl.Replace(arg))
		}
	}
	return argv
}

// env is the scrubbed environment of a staged run: the scratch home and temp
// directories replace the real ones, so a scanner finds no config or cache of the user.
func (st *scannerStage) env(pass, parent []string) []string {
	home, tmp := filepath.Join(st.scratch, "home"), filepath.Join(st.scratch, "tmp")
	extra := []string{"HOME=" + home, "TMPDIR=" + tmp}
	if runtime.GOOS == "windows" {
		extra = append(extra, "USERPROFILE="+home, "TMP="+tmp, "TEMP="+tmp)
	}
	return cmdrun.ScrubEnv(parent, pass, extra)
}
