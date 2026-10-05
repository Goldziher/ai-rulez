package lint

import (
	"path/filepath"
	"regexp"
	"strings"
)

// CodeCredentialTaint reports a clear flow of credential data into a network command.
const CodeCredentialTaint = "AR028"

func init() {
	registerRules(RuleInfo{CodeCredentialTaint, "credential-taint-flow", SeverityWarning, "a shell block or script reads a credential (file or secret variable) and passes it to a network command through a variable, a pipe or a temporary file"})
	registerTextScan(scanTaint)
}

var (
	assignRe      = regexp.MustCompile(`^(?:export\s+|local\s+|declare\s+(?:-\w+\s+)?|readonly\s+)?([A-Za-z_]\w*)=(.*)$`)
	readVarRe     = regexp.MustCompile(`^read\s+(?:-\w+\s+)*([A-Za-z_]\w*)\s*<\s*(\S+)`)
	redirectOutRe = regexp.MustCompile(`>>?\s*(\S+)`)
	teeRe         = regexp.MustCompile(`\btee\s+(?:-\w+\s+)*(\S+)`)
	varRefRe      = regexp.MustCompile(`\$\{?([A-Za-z_]\w*)\}?`)
	fileArgRe     = regexp.MustCompile(`(?:@|<\s*|-T\s+|--upload-file\s+|--data-binary\s+@?)(\S+)`)
	taintSinks    = map[string]bool{
		"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true, "ssh": true, "scp": true, "rsync": true, "socat": true,
		"dig": true, "nslookup": true, "http": true, "xh": true, "ftp": true, "telnet": true, "sftp": true,
	}
	pipeSplitRe = regexp.MustCompile(`\|\|?`)
	stageSplit  = regexp.MustCompile(`&&|\|\||;`)
)

// taintState tracks which variables and files hold credential data.
type taintState struct {
	vars  map[string]taintOrigin
	files map[string]taintOrigin
}

type taintOrigin struct {
	line int
	env  bool // the data came from a secret environment variable, not a file
	what string
}

func isShellScript(t *scanText) bool {
	if t.md {
		return false
	}
	switch strings.ToLower(filepath.Ext(t.abs)) {
	case ".sh", ".bash", ".zsh":
		return true
	}
	first, _, _ := strings.Cut(t.raw, "\n")
	return strings.HasPrefix(first, "#!") && (strings.Contains(first, "sh") || strings.Contains(first, "bash") || strings.Contains(first, "zsh")) && !strings.Contains(first, "python")
}

func scanTaint(r *runner, t *scanText) {
	groups := map[int][]scanLine{}
	var order []int
	for _, l := range t.logicalLines() {
		if !t.shellLike(l) || (!t.md && !isShellScript(t)) {
			continue
		}
		if _, seen := groups[l.Block]; !seen {
			order = append(order, l.Block)
		}
		groups[l.Block] = append(groups[l.Block], l)
	}
	for _, b := range order {
		st := &taintState{vars: map[string]taintOrigin{}, files: map[string]taintOrigin{}}
		for _, l := range groups[b] {
			text := stripShellComment(l.Text)
			for _, seg := range stageSplit.Split(text, -1) {
				st.step(r, t, l, strings.TrimSpace(seg))
			}
		}
	}
}

// sourceIn reports credential data a text reads directly: a credential path
// read, or a secret environment variable.
func (st *taintState) sourceIn(text string, line int) (taintOrigin, bool) {
	if hit, ok := detectCredentialAccess(text); ok {
		return taintOrigin{line: line, what: hit.label}, true
	}
	if vars := secretVars(text); len(vars) > 0 {
		return taintOrigin{line: line, env: true, what: "$" + vars[0]}, true
	}
	return taintOrigin{}, false
}

// taintedRef returns the origin of a tainted variable the text references.
func (st *taintState) taintedRef(text string) (string, taintOrigin, bool) {
	for _, m := range varRefRe.FindAllStringSubmatch(text, -1) {
		if o, ok := st.vars[m[1]]; ok {
			return m[1], o, true
		}
	}
	return "", taintOrigin{}, false
}

// dataOf returns the credential data a text carries, if any.
func (st *taintState) dataOf(text string, line int) (taintOrigin, bool) {
	if _, o, ok := st.taintedRef(text); ok {
		return o, true
	}
	return st.sourceIn(text, line)
}

func (st *taintState) step(r *runner, t *scanText, l scanLine, seg string) {
	if seg == "" {
		return
	}
	if m := readVarRe.FindStringSubmatch(seg); m != nil {
		if o, ok := st.fileOrigin(m[2], l.No); ok {
			st.vars[m[1]] = o
		} else {
			delete(st.vars, m[1])
		}
		return
	}
	if m := assignRe.FindStringSubmatch(seg); m != nil && !strings.Contains(m[1], "-") {
		if o, ok := st.dataOf(m[2], l.No); ok {
			st.vars[m[1]] = o
		} else {
			delete(st.vars, m[1])
		}
		return
	}
	stages := pipeSplitRe.Split(seg, -1)
	var carried *taintOrigin // credential data flowing down the pipeline
	for i, stage := range stages {
		stage = strings.TrimSpace(stage)
		words := shellWords(stage)
		cmd := commandWord(words)
		if cmd != "" && taintSinks[cmd] {
			if o, via, ok := st.sinkTaint(stage, carried, i > 0); ok && !r.allHostsAllowed(seg) {
				if o.env && via == viaHeader {
					continue
				}
				if r.hasFinding(CodeExfilCommand, t.abs, l.No) {
					return
				}
				r.add(CodeCredentialTaint, t.abs, l.No, "%s read at line %d reaches %q %s; this sends a credential off the machine", o.what, o.line, cmd, via)
				return
			}
			continue
		}
		if o, ok := st.dataOf(stage, l.No); ok {
			oo := o
			carried = &oo
		}
		if carried != nil {
			if m := redirectOutRe.FindStringSubmatch(stage); m != nil {
				st.files[unquote(m[1])] = *carried
			} else if m := teeRe.FindStringSubmatch(stage); m != nil {
				st.files[unquote(m[1])] = *carried
			}
		}
	}
}

func (st *taintState) fileOrigin(path string, line int) (taintOrigin, bool) {
	path = unquote(path)
	if o, ok := st.files[path]; ok {
		return o, true
	}
	if hit, ok := detectCredentialAccess("cat " + path); ok {
		return taintOrigin{line: line, what: hit.label}, true
	}
	return taintOrigin{}, false
}

// sinkTaint decides whether a network command stage receives credential data.
func (st *taintState) sinkTaint(stage string, carried *taintOrigin, inPipe bool) (taintOrigin, string, bool) {
	if inPipe && carried != nil {
		return *carried, "through a pipe", true
	}
	for _, m := range fileArgRe.FindAllStringSubmatch(stage, -1) {
		if o, ok := st.fileOrigin(m[1], 0); ok {
			return o, "as an uploaded file", true
		}
	}
	if _, o, ok := st.taintedRef(stage); ok {
		if _, _, inBody := st.taintedRef(headerArgRe.ReplaceAllString(stage, " ")); !inBody {
			return o, viaHeader, true
		}
		return o, "in its URL or body", true
	}
	return taintOrigin{}, "", false
}

func unquote(s string) string { return strings.Trim(s, `"'`) }

// commandWord is the first word of a command that is not an assignment or a wrapper.
func commandWord(words []string) string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.Contains(w, "/"):
		case w == "sudo" || w == "time" || w == "nohup" || w == "env" || w == "command" || w == "exec" || w == "xargs":
		default:
			return filepath.Base(w)
		}
		words = words[1:]
	}
	return ""
}

// hasFinding reports whether a finding with code already exists on that line.
func (r *runner) hasFinding(code, abs string, line int) bool {
	file := r.display(abs)
	for _, f := range r.findings {
		if f.Code == code && f.File == file && f.Line == line {
			return true
		}
	}
	return false
}

const viaHeader = "in an authentication header"
