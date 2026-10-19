// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (supply-chain checks SC1 and SC6 in src/skillspector/nodes/analyzers/static_patterns_supply_chain.py, SC10 in src/skillspector/dependency_sources.py, and the MCP registry data in src/skillspector/mcp_registry.py).
// Modified by the ai-rulez authors: rewritten as offline RE2 matchers.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

// The rules for dependency manifests a skill ships (requirements*.txt,
// package.json, pyproject.toml, pip.conf, .npmrc, .yarnrc) and for the commands that
// install from them.

// Codes for the manifest rules.
const (
	CodeUnpinnedManifest = "AR054"
	CodeSourceRedirect   = "AR055"
	CodeTyposquat        = "AR056"
)

func registerArManifest(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeUnpinnedManifest, "unpinned-manifest-dependency", SeverityWarning, "a requirements*.txt, package.json or pyproject.toml bundled with a skill leaves a dependency without a pinned version (no specifier, latest, a lower bound only, or a git URL without a commit)"},
		RuleInfo{CodeSourceRedirect, "package-source-redirect", SeverityWarning, "a manifest, package-manager config or install command points pip, uv or npm at an index or registry other than the default (--index-url, --extra-index-url, --trusted-host, registry=)"},
		RuleInfo{CodeTyposquat, "typosquat-candidate", SeverityInfo, "a bundled dependency name is one edit from a widely installed PyPI or npm package and is not a known package of its own"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeUnpinnedManifest: {
			Why:  "A dependency without an exact version resolves to whatever release is current when the skill's scripts run, so a compromised or broken release reaches the agent's machine unreviewed. Bounded ranges (`~=`, `^`, `>=1,<2`) report at info, open-ended ones at warning. The rule reads the manifests anywhere in a skill directory: `requirements*.txt` (also `*-requirements.txt`, `requirements.lock` and files in a `requirements/` directory), `package.json`, and `pyproject.toml` with `[project]`, `[build-system]`, `[dependency-groups]`, `[tool.poetry]`, `[tool.pdm]` and `[tool.uv]` dependencies. `requirements.in` is the input of a lock compiler and is never reported. A manifest next to a lock file (`package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `uv.lock`, `poetry.lock`, `pdm.lock` and the like) is not reported, and `[build-system]` requirements report at info at most. A manifest the rule cannot read (over 512 KiB, binary, or not parseable) is reported at info as not analyzable; a UTF-16 file is decoded, a byte order mark is ignored, and a file is listed at most 100 times with a count of the rest.",
			Bad:  "`requests` or `requests>=2` in `scripts/requirements.txt`, or `\"left-pad\": \"latest\"` in `package.json`",
			Good: "`requests==2.32.3` (ideally with `--hash=`), `\"left-pad\": \"1.3.0\"`, or a git requirement that names a full commit",
		},
		CodeSourceRedirect: {
			Why:  "An index or registry other than the default decides which package a name resolves to. `--extra-index-url` lets a same-named package on another host win (dependency confusion), and `--trusted-host` turns TLS verification off for that host. The rule reads `--index-url`, `--extra-index-url`, `--default-index`, `--index`, `--find-links` and `--trusted-host`, `pip.conf`/`pip.ini`/`uv.toml`, `pyproject.toml` (`[tool.uv]`, `[tool.poetry.source]`, `[tool.pdm.source]`), `.npmrc`, `.yarnrc`, `.yarnrc.yml` and `registry` options of install commands. A scoped registry (`@myorg:registry=`, `npmScopes`) is the usual defense against dependency confusion and is not reported. `pypi.org`, `pypi.python.org`, `files.pythonhosted.org`, the npm and Yarn registries and `download.pytorch.org` are default hosts. List the project's own mirror in `lint.security.allowed_hosts`.",
			Bad:  "`pip install --extra-index-url https://pkgs.example.net/simple helper` or `registry=https://npm.example.net/` in `.npmrc`",
			Good: "Install from the default index, or name the project's mirror in `lint.security.allowed_hosts`",
		},
		CodeTyposquat: {
			Why:  "A package name one keystroke from a popular one is how typosquatting works. The check compares each dependency with the 300 most downloaded PyPI and npm packages and about 200 hand-picked popular ones (it embeds the public download rankings, about 300 KB, and uses no network) and skips every name that is itself in the top 10,000 of its ecosystem, a known real neighbor, or differs only by trailing digits (`psycopg`/`psycopg2`). It reports at info: a real package outside the rankings that happens to sit one edit away is a false positive, and a squat of a package outside the top 300 is missed.",
			Bad:  "`reqeusts==2.0.0` in `requirements.txt` (a transposition of `requests`)",
			Good: "`requests==2.32.3`, or `# ai-rulez-lint-ignore: AR056` above a real package that is one edit from a popular one",
		},
	})
	s.addTextScan(scanManifest, AnalyzerSecurity)
	s.addRunCheck(checkSkillManifests, AnalyzerSecurity)
}

type manifestKind int

const (
	manifestNone manifestKind = iota
	manifestRequirements
	manifestPackageJSON
	manifestPyproject
	manifestIni
	manifestUvToml
	manifestNpmrc
	manifestYarnrc
	manifestYarnrcYml
)

const (
	dirRequirements = "requirements"
	bom             = "\ufeff"
	// maxManifestBytes is the largest manifest the rules read, like the cap on skill resources.
	maxManifestBytes = 512 * 1024
	// maxManifestFindings is how many AR054 and how many AR056 findings one file lists.
	maxManifestFindings = 100
)

var requirementsFileRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^(?:.+[-_.])?(?:requirements|constraints)(?:[-_.][a-z0-9_.-]+)?\.(?:txt|lock)$`)
})

func manifestOf(abs string) manifestKind {
	base := strings.ToLower(filepath.Base(abs))
	switch {
	case requirementsFileRe().MatchString(base),
		strings.HasSuffix(base, ".txt") && strings.EqualFold(filepath.Base(filepath.Dir(abs)), dirRequirements):
		return manifestRequirements
	case base == filePackageJSON:
		return manifestPackageJSON
	case base == "pyproject.toml":
		return manifestPyproject
	case base == "pip.conf" || base == "pip.ini":
		return manifestIni
	case base == "uv.toml":
		return manifestUvToml
	case base == fileNpmrc:
		return manifestNpmrc
	case base == ".yarnrc":
		return manifestYarnrc
	case base == ".yarnrc.yml":
		return manifestYarnrcYml
	}
	return manifestNone
}

// withoutBOM returns t without a UTF-8 byte order mark at its start, which would
// otherwise make the first requirement or the JSON document unreadable.
func withoutBOM(t *scanText) *scanText {
	if !strings.HasPrefix(t.raw, bom) {
		return t
	}
	c := *t
	c.raw = strings.TrimPrefix(t.raw, bom)
	c.lines = slices.Clone(t.lines)
	if len(c.lines) > 0 {
		c.lines[0].Text = strings.TrimPrefix(c.lines[0].Text, bom)
	}
	return &c
}

func scanManifest(r *runner, t *scanText) {
	t = withoutBOM(t)
	switch manifestOf(t.abs) {
	case manifestRequirements:
		r.checkRequirements(t)
	case manifestPackageJSON:
		r.checkPackageJSON(t)
	case manifestPyproject:
		r.checkPyproject(t)
	case manifestIni:
		r.checkIndexConfig(t, iniSourceRe())
	case manifestUvToml:
		r.checkIndexConfig(t, uvTomlSourceRe())
	case manifestNpmrc:
		r.checkIndexConfig(t, npmrcSourceRe())
	case manifestYarnrc:
		r.checkIndexConfig(t, yarnrcSourceRe())
	case manifestYarnrcYml:
		r.checkYarnrcYml(t)
	}
	r.checkInstallCommands(t)
}

// checkSkillManifests reads the manifests of a skill that the resource scan does not:
// those outside scripts/, references/ and assets/ (the skill root, lib/, ...), and those
// the resource scan skips as too large, binary or not UTF-8. A file it can decode
// goes through the same scan as a resource; one it cannot read is reported as not
// analyzable, so silence does not read as a clean manifest.
func checkSkillManifests(r *runner) {
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || it.kind != kindSkill || it.itemDir == "" {
			continue
		}
		scanned := map[string]bool{}
		for _, res := range it.cf.Resources {
			if scannableResource(res) {
				scanned[filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath))] = true
			}
		}
		for _, f := range r.skillTree(it.itemDir) {
			if scanned[f.path] || manifestOf(f.path) == manifestNone || inPycache(f.path) {
				continue
			}
			r.scanUnreadManifest(it.itemDir, f)
		}
	}
}

// notAnalyzable reports a manifest the rules could not read, so silence does not
// read as a clean manifest.
func (r *runner) notAnalyzable(abs, rel, why string) {
	r.addSev(SeverityInfo, CodeUnpinnedManifest, abs, 1, "%s is not analyzable (%s): AR054, AR055 and AR056 did not read it", rel, why)
}

func (r *runner) scanUnreadManifest(dir string, f *dirFile) {
	rel, _ := filepath.Rel(dir, f.path) //nolint:errcheck // display only
	rel = filepath.ToSlash(rel)
	notAnalyzable := func(why string) { r.notAnalyzable(f.path, rel, why) }
	if f.size > maxManifestBytes {
		notAnalyzable("larger than 512 KiB")
		return
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		return
	}
	text, why := decodeManifest(data)
	if why != "" {
		notAnalyzable(why)
		return
	}
	r.securityScan(f.path, text)
}

// decodeManifest turns the bytes of a manifest into UTF-8 text: a UTF-16 file with a
// byte order mark (what PowerShell redirection writes) is decoded, and a stray
// invalid byte is replaced. why is set for content that is not text.
func decodeManifest(data []byte) (text, why string) {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data), ""
	case bytes.IndexByte(data, 0) >= 0:
		return "", "binary content"
	case !utf8.Valid(data):
		return strings.ToValidUTF8(string(data), "\ufffd"), ""
	}
	return string(data), ""
}

func decodeUTF16(data []byte) string {
	order := func(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
	if data[0] == 0xFE {
		order = func(b []byte) uint16 { return uint16(b[1]) | uint16(b[0])<<8 }
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 2; i+1 < len(data); i += 2 {
		units = append(units, order(data[i:]))
	}
	return string(utf16.Decode(units))
}

// pinLevel is how firmly a dependency specifier fixes the release.
type pinLevel int

const (
	pinned pinLevel = iota
	pinBounded
	pinOpen
	pinNone
)

var (
	reqNameRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)\s*(?:\[[^\]]*\])?\s*(.*)$`)
	})
	specClauseRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(===|==|~=|!=|<=|>=|<|>)\s*(\S+)$`) })
	fullCommitRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`) })
	exactSemverRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^[=v]*\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	})
	distTagRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Za-z][A-Za-z-]*$`) })
	repoSpecRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:#.*)?$`) })
)

func classifyPySpec(spec string) pinLevel {
	spec = strings.Trim(strings.TrimSpace(spec), "()")
	if spec == "" {
		return pinNone
	}
	upper, lower := false, false
	for _, clause := range strings.Split(spec, ",") {
		m := specClauseRe().FindStringSubmatch(strings.TrimSpace(clause))
		if m == nil {
			continue
		}
		switch m[1] {
		case "===":
			return pinned
		case "==":
			if !strings.Contains(m[2], "*") {
				return pinned
			}
			upper = true
		case "~=", "<", "<=":
			upper = true
		case ">=", ">":
			lower = true
		}
	}
	switch {
	case upper:
		return pinBounded
	case lower:
		return pinOpen
	}
	return pinNone
}

// vcsRef returns the ref a VCS URL names after the @ of its path, "" when it names none.
func vcsRef(u string) string {
	u, _, _ = strings.Cut(u, "#")
	_, rest, ok := strings.Cut(u, "://")
	if !ok {
		rest = u
	}
	_, path, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(path, "@"); i >= 0 {
		ref, _, _ := strings.Cut(strings.TrimSpace(path[i+1:]), " ")
		return ref
	}
	return ""
}

func isVCSURL(s string) bool {
	return strings.Contains(s, "git+") || strings.Contains(s, "hg+") || strings.Contains(s, "svn+") || strings.Contains(s, "bzr+")
}

// requirement is one dependency read from a requirements line or a PEP 508 string.
type requirement struct {
	name  string
	level pinLevel
	vcs   bool
	spec  string
}

// requirementText strips the per-requirement options, the editable flag and the
// environment marker from a requirements line, and reports whether the line
// carries hashes.
func requirementText(text string) (clean string, hashed bool) {
	text = strings.TrimSpace(text)
	hashed = strings.Contains(text, "--hash=")
	if i := strings.Index(text, " --"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	for _, flag := range []string{"-e ", "--editable "} {
		if rest, found := strings.CutPrefix(text, flag); found {
			text = strings.TrimSpace(rest)
		}
	}
	if i := markerIndex(text); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	return text, hashed
}

// markerIndex is where the environment marker of a requirement starts, -1 when it has
// none. After a URL the marker must follow whitespace (PEP 508), because a bare ; can
// belong to the URL.
func markerIndex(text string) int {
	i := strings.Index(text, ";")
	if i < 0 || !strings.Contains(text[:i], "://") {
		return i
	}
	for j := i; j >= 0; {
		if j > 0 && (text[j-1] == ' ' || text[j-1] == '\t') {
			return j
		}
		k := strings.Index(text[j+1:], ";")
		if k < 0 {
			return -1
		}
		j += 1 + k
	}
	return -1
}

// parseRequirement reads one requirement. ok is false for lines that name no
// pinnable dependency: options, local paths and archive URLs.
func parseRequirement(text string) (req requirement, ok bool) {
	text, hashed := requirementText(text)
	named := strings.Contains(text, " @ ")
	switch {
	case text == "" || strings.HasPrefix(text, "-"):
		return requirement{}, false
	case isVCSURL(text) && !named:
		return vcsRequirement(text, text), true
	case strings.Contains(text, "://") && !named:
		return requirement{}, false
	}
	m := reqNameRe().FindStringSubmatch(text)
	if m == nil {
		return requirement{}, false
	}
	name, rest := m[1], strings.TrimSpace(m[2])
	if after, found := strings.CutPrefix(rest, "@"); found {
		if u := strings.TrimSpace(after); isVCSURL(u) {
			return vcsRequirement(name, u), true
		}
		return requirement{}, false
	}
	level := classifyPySpec(rest)
	if hashed && level != pinNone {
		level = pinned
	}
	return requirement{name: name, level: level, spec: rest}, true
}

func vcsRequirement(name, u string) requirement {
	level := pinNone
	if fullCommitRe().MatchString(vcsRef(u)) {
		level = pinned
	}
	return requirement{name: name, level: level, vcs: true}
}

// pinFinding is the AR054 finding the requirement deserves, if any. npm names the
// ecosystem so the advice fits it: == is Python syntax.
func pinFinding(req requirement, npm bool) (sev Severity, msg string, ok bool) {
	pin := "pin it with =="
	if npm {
		pin = "pin an exact version (1.2.3)"
	}
	switch {
	case req.level == pinned:
		return "", "", false
	case req.vcs:
		return SeverityWarning, fmt.Sprintf("%s installs from a VCS URL that does not name a full commit; pin the 40-character commit so the code cannot change under the skill", req.name), true
	case req.level == pinNone:
		return SeverityWarning, fmt.Sprintf("%s has no pinned version; %s so a new release cannot change what the skill installs", req.name, pin), true
	case req.level == pinOpen:
		return SeverityWarning, fmt.Sprintf("%s sets only a lower bound (%s), so any later release is accepted; %s", req.name, req.spec, pin), true
	}
	return SeverityInfo, fmt.Sprintf("%s allows a version range (%s); %s or install from a lock file", req.name, req.spec, pin), true
}

// depReporter reports the dependencies of one manifest, at most maxManifestFindings
// AR054 and as many AR056 findings, and counts the rest into one summary each.
type depReporter struct {
	r          *runner
	abs        string
	locked     bool // a lock file beside the manifest fixes the versions
	infoOnly   bool // the dependencies are build tooling: no finding above info
	pins       int
	squats     int
	morePins   int
	moreSquats int
}

func (d *depReporter) dep(line int, req requirement, u nameUniverse) {
	if sev, msg, ok := pinFinding(req, !u.pep503); ok && !d.locked {
		if d.infoOnly {
			sev = SeverityInfo
		}
		if d.pins < maxManifestFindings {
			d.pins++
			d.r.addSev(sev, CodeUnpinnedManifest, d.abs, line, "%s", msg)
		} else {
			d.morePins++
		}
	}
	if near := u.typosquatOf(req.name); near != "" {
		if d.squats < maxManifestFindings {
			d.squats++
			d.r.add(CodeTyposquat, d.abs, line, "%q is one edit from the popular package %q; check that it is the package you mean (typosquatting)", req.name, near)
		} else {
			d.moreSquats++
		}
	}
}

// finish adds the summaries of the findings the cap left out.
func (d *depReporter) finish() {
	if d.morePins > 0 {
		d.r.addSev(SeverityInfo, CodeUnpinnedManifest, d.abs, 1, "%d more dependencies in this file are not pinned; only the first %d are listed", d.morePins, maxManifestFindings)
	}
	if d.moreSquats > 0 {
		d.r.add(CodeTyposquat, d.abs, 1, "%d more dependency names in this file are one edit from a popular package; only the first %d are listed", d.moreSquats, maxManifestFindings)
	}
}

type manifestLine struct {
	no   int
	text string
}

// manifestLines joins continuation lines and drops comments and blanks.
func manifestLines(t *scanText) []manifestLine {
	var out []manifestLine
	for i := 0; i < len(t.lines); i++ {
		l := t.lines[i]
		text := l.Text
		for strings.HasSuffix(strings.TrimRight(text, " \t"), `\`) && i+1 < len(t.lines) {
			i++
			text = strings.TrimSuffix(strings.TrimRight(text, " \t"), `\`) + " " + strings.TrimSpace(t.lines[i].Text)
		}
		text = strings.TrimSpace(text)
		if j := strings.Index(text, " #"); j >= 0 {
			text = strings.TrimSpace(text[:j])
		}
		if text != "" && !strings.HasPrefix(text, "#") {
			out = append(out, manifestLine{l.No, text})
		}
	}
	return out
}

// lockedBeside reports whether the directory of a manifest holds one of the lock files.
func lockedBeside(abs string, locks ...string) bool {
	dir := filepath.Dir(abs)
	for _, name := range locks {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

var (
	npmLocks = sync.OnceValue(func() []string {
		return []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"}
	})
	pythonLocks = sync.OnceValue(func() []string { return []string{"uv.lock", "poetry.lock", "pdm.lock"} })
)

func (r *runner) checkRequirements(t *scanText) {
	rep := &depReporter{r: r, abs: t.abs}
	for _, l := range manifestLines(t) {
		if strings.HasPrefix(l.text, "-") && !strings.HasPrefix(l.text, "-e ") && !strings.HasPrefix(l.text, "--editable ") {
			r.reportSources(t.abs, l.no, l.text)
			continue
		}
		if req, ok := parseRequirement(l.text); ok {
			rep.dep(l.no, req, pyPIUniverse())
		}
	}
	rep.finish()
}

// tomlMap, tomlList and tomlString read a decoded TOML or JSON value, giving the zero
// value for any other type, so one key of the wrong type cannot hide the rest.
func tomlMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func tomlList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func tomlString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func tomlDig(v any, path ...string) any {
	for _, key := range path {
		v = tomlMap(v)[key]
	}
	return v
}

// tomlStrings returns the string elements of a list value.
func tomlStrings(v any) []string {
	var out []string
	for _, e := range tomlList(v) {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// pyprojectSpecs lists the PEP 508 strings of a pyproject.toml: project,
// dependency-group, PDM and uv dependencies.
func pyprojectSpecs(doc map[string]any) []string {
	specs := tomlStrings(tomlDig(doc, "project", "dependencies"))
	for _, groups := range []map[string]any{tomlMap(tomlDig(doc, "project", "optional-dependencies")), tomlMap(tomlDig(doc, "dependency-groups")), tomlMap(tomlDig(doc, "tool", "pdm", "dev-dependencies"))} {
		for _, name := range sortedKeys(groups) {
			specs = append(specs, tomlStrings(groups[name])...)
		}
	}
	return append(specs, tomlStrings(tomlDig(doc, "tool", "uv", "dev-dependencies"))...)
}

// poetryDep is one entry of a [tool.poetry] dependency table.
type poetryDep struct {
	name string
	req  requirement
}

// poetryDeps reads the dependency tables of [tool.poetry]: dependencies,
// dev-dependencies and every group. Local paths and URLs name no release.
func poetryDeps(doc map[string]any) []poetryDep {
	poetry := tomlMap(tomlDig(doc, "tool", "poetry"))
	tables := []map[string]any{tomlMap(poetry["dependencies"]), tomlMap(poetry["dev-dependencies"])}
	groups := tomlMap(poetry["group"])
	for _, g := range sortedKeys(groups) {
		tables = append(tables, tomlMap(tomlMap(groups[g])["dependencies"]))
	}
	var out []poetryDep
	for _, table := range tables {
		for _, name := range sortedKeys(table) {
			if req, ok := poetryRequirement(name, table[name]); ok {
				out = append(out, poetryDep{name, req})
			}
		}
	}
	return out
}

func poetryRequirement(name string, v any) (requirement, bool) {
	if strings.EqualFold(name, "python") {
		return requirement{}, false
	}
	switch t := v.(type) {
	case string:
		return requirement{name: name, level: poetryLevel(t), spec: t}, true
	case map[string]any:
		return poetryTableRequirement(name, t)
	case []any:
		req, found := requirement{name: name, level: pinned}, false
		for _, e := range t {
			if r, ok := poetryRequirement(name, e); ok {
				found = true
				if r.level > req.level || r.vcs {
					req = r
				}
			}
		}
		return req, found
	}
	return requirement{}, false
}

func poetryTableRequirement(name string, t map[string]any) (requirement, bool) {
	if t["path"] != nil || t[keyURL] != nil {
		return requirement{}, false
	}
	if git := tomlString(t["git"]); git != "" {
		req := requirement{name: name, level: pinNone, vcs: true}
		if fullCommitRe().MatchString(tomlString(t["rev"])) {
			req.level = pinned
		}
		return req, true
	}
	version := tomlString(t["version"])
	return requirement{name: name, level: poetryLevel(version), spec: version}, true
}

// poetryLevel classifies a Poetry version constraint: a bare version is an exact
// pin, ^ and ~ and wildcards are bounded, comparison operators follow PEP 440.
func poetryLevel(spec string) pinLevel {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "" || spec == "*":
		return pinNone
	case strings.HasPrefix(spec, "^") || strings.HasPrefix(spec, "~") && !strings.HasPrefix(spec, "~="):
		return pinBounded
	case strings.ContainsAny(spec, "<>=!,"):
		return classifyPySpec(spec)
	case strings.Contains(spec, "*"):
		return pinBounded
	}
	return pinned
}

func (r *runner) checkPyproject(t *scanText) {
	var doc map[string]any
	if toml.Unmarshal([]byte(t.raw), &doc) != nil {
		r.notAnalyzable(t.abs, filepath.Base(t.abs), "does not parse")
		return
	}
	idx := newLineIndex(t.rawLines())
	rep := &depReporter{r: r, abs: t.abs, locked: lockedBeside(t.abs, pythonLocks()...)}
	for _, spec := range pyprojectSpecs(doc) {
		if req, ok := parseRequirement(spec); ok {
			rep.dep(idx.literal(spec), req, pyPIUniverse())
		}
	}
	rep.infoOnly = true // the build backend is tooling, not what the skill's scripts import
	for _, spec := range tomlStrings(tomlDig(doc, "build-system", "requires")) {
		if req, ok := parseRequirement(spec); ok {
			rep.dep(idx.literal(spec), req, pyPIUniverse())
		}
	}
	rep.infoOnly = false
	for _, d := range poetryDeps(doc) {
		rep.dep(idx.key(d.name), d.req, pyPIUniverse())
	}
	rep.finish()
	urls := append(tomlStrings(tomlDig(doc, "tool", "uv", "extra-index-url")), tomlString(tomlDig(doc, "tool", "uv", "index-url")))
	for _, tab := range [][]any{tomlList(tomlDig(doc, "tool", "uv", "index")), tomlList(tomlDig(doc, "tool", "poetry", "source")), tomlList(tomlDig(doc, "tool", "pdm", "source"))} {
		for _, src := range tab {
			urls = append(urls, tomlString(tomlMap(src)[keyURL]))
		}
	}
	for _, u := range urls {
		if u != "" {
			r.reportRedirect(t.abs, idx.literal(u), "package index", u, false)
		}
	}
}

// lineIndex finds the line of a string literal or a key in one pass over a file, so
// that looking up many dependencies costs one scan, not one scan each. A value that
// occurs on several lines is handed out in file order.
type lineIndex struct {
	lits, keys map[string][]int
	used       map[string]int
}

var (
	literalRe    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'([^']*)'`) })
	tomlKeyRe    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^\s*(?:"([^"]+)"|([A-Za-z0-9_.-]+))\s*=`) })
	jsonKeyRe    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:`) })
	nameAssignRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^\s*name\s*=`) })
)

func newLineIndex(lines []string) *lineIndex {
	x := &lineIndex{lits: map[string][]int{}, keys: map[string][]int{}, used: map[string]int{}}
	for i, line := range lines {
		no := i + 1
		if m := tomlKeyRe().FindStringSubmatch(line); m != nil {
			x.keys[m[1]+m[2]] = append(x.keys[m[1]+m[2]], no)
		}
		if nameAssignRe().MatchString(line) {
			continue // the project's own name is not one of its dependencies
		}
		for _, m := range literalRe().FindAllStringSubmatch(line, -1) {
			lit := m[2]
			if m[0][0] == '"' {
				lit = unquoteBasic(m[1])
			}
			x.lits[lit] = append(x.lits[lit], no)
		}
	}
	return x
}

func unquoteBasic(s string) string {
	if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return u
	}
	return s
}

// literal is the next line holding s as a whole string, 1 when there is none.
func (x *lineIndex) literal(s string) int { return x.take("l"+s, x.lits[s]) }

// key is the next line that assigns the TOML key name, 1 when there is none.
func (x *lineIndex) key(name string) int { return x.take("k"+name, x.keys[name]) }

func (x *lineIndex) take(id string, lines []int) int {
	n := x.used[id]
	if n >= len(lines) {
		return 1
	}
	x.used[id] = n + 1
	return lines[n]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var packageJSONSections = sync.OnceValue(func() []string {
	return []string{"dependencies", "devDependencies", "optionalDependencies"}
})

// npmSpec is the reading of one package.json dependency value; skip marks a
// value that names no registry release (a local path, a tarball URL).
type npmSpec struct {
	level pinLevel
	vcs   bool
	skip  bool
}

var localSpecPrefixes = sync.OnceValue(func() []string {
	return []string{"file:", "link:", "workspace:", "portal:", "patch:", "catalog:", "./", "../", "/", "~/"}
})

func classifyNPMSpec(v string) npmSpec {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == "*" || strings.EqualFold(v, "x") || strings.EqualFold(v, "latest"):
		return npmSpec{level: pinNone}
	case hasAnyPrefix(v, localSpecPrefixes()):
		return npmSpec{skip: true}
	case strings.HasPrefix(v, "npm:"):
		return classifyNPMAlias(v)
	case isGitSpec(v):
		return classifyGitSpec(v)
	case strings.Contains(v, "://"):
		return npmSpec{skip: true}
	}
	return npmSpec{level: classifyNPMVersion(v)}
}

// classifyNPMAlias reads npm:<name>@<range> by the range it names.
func classifyNPMAlias(v string) npmSpec {
	if i := strings.LastIndex(v, "@"); i > len("npm:") {
		return classifyNPMSpec(v[i+1:])
	}
	return npmSpec{level: pinNone}
}

func classifyGitSpec(v string) npmSpec {
	_, ref, _ := strings.Cut(v, "#")
	switch {
	case fullCommitRe().MatchString(ref):
		return npmSpec{level: pinned, vcs: true}
	case strings.HasPrefix(ref, "semver:"):
		return npmSpec{level: pinBounded, vcs: true}
	}
	return npmSpec{level: pinNone, vcs: true}
}

func classifyNPMVersion(v string) pinLevel {
	switch {
	case exactSemverRe().MatchString(v):
		return pinned
	case distTagRe().MatchString(v):
		return pinNone
	case strings.HasPrefix(v, ">") && !strings.Contains(v, "<") && !strings.Contains(v, "||"):
		return pinOpen
	}
	return pinBounded
}

func isGitSpec(v string) bool {
	return hasAnyPrefix(v, []string{"git+", "git://", "github:", "gitlab:", "bitbucket:", "gist:"}) || repoSpecRe().MatchString(v)
}

func (r *runner) checkPackageJSON(t *scanText) {
	var pkg map[string]json.RawMessage
	if json.Unmarshal([]byte(t.raw), &pkg) != nil {
		r.notAnalyzable(t.abs, filepath.Base(t.abs), "does not parse")
		return
	}
	keys := jsonKeyLines(t.rawLines())
	rep := &depReporter{r: r, abs: t.abs, locked: lockedBeside(t.abs, npmLocks()...)}
	for _, section := range packageJSONSections() {
		var deps map[string]json.RawMessage
		if raw, ok := pkg[section]; !ok || json.Unmarshal(raw, &deps) != nil {
			continue
		}
		from := keys.after(section, 1)
		for _, name := range sortedKeys(deps) {
			var value string
			if json.Unmarshal(deps[name], &value) != nil {
				continue // a value that is not a string names no release
			}
			line := keys.after(name, from)
			req := requirement{name: name, level: pinned}
			if spec := classifyNPMSpec(value); !spec.skip {
				req = requirement{name: name, level: spec.level, vcs: spec.vcs, spec: value}
			}
			rep.dep(line, req, npmUniverse())
		}
	}
	rep.finish()
}

// jsonKeys maps each JSON object key of a file to the sorted lines it appears on.
type jsonKeys map[string][]int

func jsonKeyLines(lines []string) jsonKeys {
	keys := jsonKeys{}
	for i, line := range lines {
		for _, m := range jsonKeyRe().FindAllStringSubmatch(line, -1) {
			name := unquoteBasic(m[1])
			keys[name] = append(keys[name], i+1)
		}
	}
	return keys
}

// after is the first line at or after from that holds the key name, or from.
func (k jsonKeys) after(name string, from int) int {
	lines := k[name]
	if i := sort.SearchInts(lines, from); i < len(lines) {
		return lines[i]
	}
	return from
}

// lineFrom is the 1-based line at or after from that holds needle, or from.
func lineFrom(lines []string, from int, needle string) int {
	for i := max(from-1, 0); i < len(lines); i++ {
		if strings.Contains(lines[i], needle) {
			return i + 1
		}
	}
	return from
}

func (t *scanText) rawLines() []string { return strings.Split(t.raw, "\n") }

var defaultSourceHosts = sync.OnceValue(func() map[string]bool {
	return nameSet("pypi.org pypi.python.org files.pythonhosted.org registry.npmjs.org registry.yarnpkg.com download.pytorch.org")
})

var (
	pipOptionRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)(--index-url|--extra-index-url|--default-index|--index|--find-links|--trusted-host|-i|-f)(?:=|\s+)(\S+)`)
	})
	npmOptionRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)--registry(?:=|\s+)(\S+)`)
	})
	configSetRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:(?:npm|yarn|pnpm)\s+config\s+set\s+registry|pip3?\s+config\s+set\s+\S*index-url)\s+(\S+)`)
	})
	sourceEnvRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:PIP_INDEX_URL|PIP_EXTRA_INDEX_URL|UV_INDEX_URL|UV_EXTRA_INDEX_URL|UV_DEFAULT_INDEX|(?i:npm_config_registry))=(\S+)`)
	})
	iniSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^\s*(index-url|extra-index-url|find-links|trusted-host)\s*[=:]\s*(\S+)`)
	})
	uvTomlSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)(?:^|[\s{,\[])(index-url|extra-index-url|find-links|default-index|url)\s*=\s*["']?([^"'\s,}]+)`)
	})
	yarnrcSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*"?(registry)"?[\s=]+"?([^"\s]+)`)
	})
	yarnrcYmlSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(npmRegistryServer)\s*:\s*["']?([^"'\s]+)`)
	})
	npmrcSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(registry)\s*=\s*(\S+)`)
	})
	pipContextRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\b(?:pip3?|pipx|uv|poetry)\b`) })
	npmContextRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\b(?:npm|npx|yarn|pnpm|bun)\b`) })
)

func (r *runner) checkIndexConfig(t *scanText, re *regexp.Regexp) {
	for _, l := range t.lines {
		if m := re.FindStringSubmatch(l.Text); m != nil {
			r.reportOption(t.abs, l.No, m[1], m[2])
		}
	}
}

// checkYarnrcYml reads npmRegistryServer, except inside npmScopes: a scoped registry is
// the usual defense against dependency confusion, not a redirect.
func (r *runner) checkYarnrcYml(t *scanText) {
	inScopes := false
	for _, l := range t.lines {
		if l.Text != "" && l.Text[0] != ' ' && l.Text[0] != '\t' && l.Text[0] != '#' {
			inScopes = strings.HasPrefix(l.Text, "npmScopes:")
		}
		if inScopes {
			continue
		}
		if m := yarnrcYmlSourceRe().FindStringSubmatch(l.Text); m != nil {
			r.reportOption(t.abs, l.No, m[1], m[2])
		}
	}
}

// reportSources reports the index options of one requirements line.
func (r *runner) reportSources(abs string, line int, text string) {
	for _, m := range pipOptionRe().FindAllStringSubmatch(text, -1) {
		r.reportOption(abs, line, m[1], m[2])
	}
}

func (r *runner) reportOption(abs string, line int, option, value string) {
	switch strings.ToLower(option) {
	case "--trusted-host":
		r.reportRedirect(abs, line, "trusted host (TLS verification off)", value, true)
	case "--find-links", "-f", "find-links":
		r.reportRedirect(abs, line, "find-links location", value, false)
	case "registry", "npmregistryserver":
		r.reportRedirect(abs, line, "npm registry", value, false)
	default:
		r.reportRedirect(abs, line, "package index", value, false)
	}
}

// reportRedirect adds AR055 when value names a host that is neither a default
// package host, loopback nor listed in lint.security.allowed_hosts.
func (r *runner) reportRedirect(abs string, line int, what, value string, bareHost bool) {
	host, ok := r.redirectHost(value, bareHost)
	if !ok {
		return
	}
	r.add(CodeSourceRedirect, abs, line, "%s %s replaces the default; packages resolved there can differ from the ones on the public registry (dependency confusion); list the host in lint.security.allowed_hosts if it is the project's own", what, host)
}

func (r *runner) redirectHost(value string, bareHost bool) (string, bool) {
	value = strings.Trim(value, "\"'`,;")
	if i := strings.Index(value, "://"); i > 0 { // uv: --index name=https://host/simple
		if eq := strings.LastIndex(value[:i], "="); eq >= 0 {
			value = value[eq+1:]
		}
	}
	if value == "" || strings.ContainsAny(value, "${}") {
		return "", false
	}
	if !strings.Contains(value, "://") {
		if !bareHost {
			return "", false
		}
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if loopback, _ := hostClass(host); loopback || defaultSourceHosts()[host] || hostAllowed(host, r.security().AllowedHosts) {
		return "", false
	}
	return host, true
}

func (r *runner) checkInstallCommands(t *scanText) {
	if manifestOf(t.abs) != manifestNone {
		return
	}
	for _, l := range t.lines {
		if l.Front || l.Neg || !mayRedirectSource(l.Text) {
			continue
		}
		for _, seg := range t.commandSegments(l, unpinnedStartRe()) {
			r.checkSourceCommand(t.abs, l.No, seg)
		}
	}
}

var redirectSourceStems = sync.OnceValue(func() *stemIndex {
	return newStemIndex([]string{"index", "registry", "trusted-host", "find-links", " -i ", " -f "})
})

func mayRedirectSource(text string) bool {
	return redirectSourceStems().containsAny(foldText(text))
}

func (r *runner) checkSourceCommand(abs string, line int, seg string) {
	if pipContextRe().MatchString(seg) {
		r.reportSources(abs, line, seg)
	}
	if npmContextRe().MatchString(seg) {
		for _, m := range npmOptionRe().FindAllStringSubmatch(seg, -1) {
			r.reportRedirect(abs, line, "npm registry", m[1], false)
		}
	}
	for _, re := range []*regexp.Regexp{configSetRe(), sourceEnvRe()} {
		for _, m := range re.FindAllStringSubmatch(seg, -1) {
			r.reportRedirect(abs, line, "package source", m[1], false)
		}
	}
}

var pep503SepRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[-_.]+`) })

// minSquatLen is the shortest name AR056 compares: below it, one edit reaches
// too many unrelated real names.
const minSquatLen = 5

func (u nameUniverse) typosquatOf(name string) string {
	n := strings.ToLower(name)
	if u.pep503 {
		n = pep503SepRe().ReplaceAllString(n, "-")
	}
	if len(n) < minSquatLen || strings.HasPrefix(n, "@") || u.real[n] {
		return ""
	}
	for _, p := range u.targets {
		if len(p) >= minSquatLen && oneEdit(n, p) && !versionedFork(n, p) {
			return p
		}
	}
	return ""
}

// versionedFork reports whether the names differ only by trailing digits:
// psycopg and psycopg2, httpx and httpx2. Projects name a rewrite or a major
// release that way, so the pair is far more often two real packages than a squat.
func versionedFork(a, b string) bool {
	if len(a) < len(b) {
		a, b = b, a
	}
	rest, ok := strings.CutPrefix(a, b)
	return ok && strings.Trim(rest, "0123456789") == ""
}

// oneEdit reports whether a and b differ by exactly one insertion, deletion,
// substitution or swap of two adjacent characters. It compares bytes in one pass
// and allocates nothing, so it can run against every popular name for every
// dependency of a large manifest.
func oneEdit(a, b string) bool {
	la, lb := len(a), len(b)
	if la-lb > 1 || lb-la > 1 || a == b {
		return false
	}
	i := 0
	for i < la && i < lb && a[i] == b[i] {
		i++
	}
	switch {
	case la == lb:
		return a[i+1:] == b[i+1:] || (i+1 < la && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:])
	case la > lb:
		return a[i+1:] == b[i:]
	}
	return b[i+1:] == a[i:]
}
