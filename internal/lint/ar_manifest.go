// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (supply-chain checks SC1, SC6 and SC10 in static_patterns_supply_chain.py).
// Modified by the ai-rulez authors: rewritten as offline RE2 matchers.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// The rules for dependency manifests a skill ships (requirements*.txt,
// package.json, pyproject.toml, pip.conf, .npmrc) and for the commands that
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
			Why:  "A dependency without an exact version resolves to whatever release is current when the skill's scripts run, so a compromised or broken release reaches the agent's machine unreviewed. Bounded ranges (`~=`, `^`, `>=1,<2`) report at info, open-ended ones at warning. `requirements.in` is the input of a lock compiler and is never reported.",
			Bad:  "`requests` or `requests>=2` in `scripts/requirements.txt`, or `\"left-pad\": \"latest\"` in `package.json`",
			Good: "`requests==2.32.3` (ideally with `--hash=`), `\"left-pad\": \"1.3.0\"`, or a git requirement that names a full commit",
		},
		CodeSourceRedirect: {
			Why:  "An index or registry other than the default decides which package a name resolves to. `--extra-index-url` lets a same-named package on another host win (dependency confusion), and `--trusted-host` turns TLS verification off for that host. List the project's own mirror in `lint.security.allowed_hosts`.",
			Bad:  "`pip install --extra-index-url https://pkgs.example.net/simple helper` or `registry=https://npm.example.net/` in `.npmrc`",
			Good: "Install from the default index, or name the project's mirror in `lint.security.allowed_hosts`",
		},
		CodeTyposquat: {
			Why:  "A package name one keystroke from a popular one is how typosquatting works. The check compares against a short embedded list of widely installed PyPI and npm packages and skips known real neighbors such as `preact` and names that differ only by trailing digits (`psycopg`/`psycopg2`, `httpx`/`httpx2`), so it reports at info: a legitimate package that happens to sit one edit away is a false positive, and a squat of a package outside the list is missed.",
			Bad:  "`reqeusts==2.0.0` in `requirements.txt` (a transposition of `requests`)",
			Good: "`requests==2.32.3`, or `# ai-rulez-lint-ignore: AR056` above a real package that is one edit from a popular one",
		},
	})
	s.addTextScan(scanManifest, AnalyzerSecurity)
}

type manifestKind int

const (
	manifestNone manifestKind = iota
	manifestRequirements
	manifestPackageJSON
	manifestPyproject
	manifestIni
	manifestNpmrc
)

var requirementsFileRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^(?:requirements|constraints)(?:[-_.][a-z0-9_.-]+)?\.txt$`)
})

func manifestOf(abs string) manifestKind {
	base := strings.ToLower(filepath.Base(abs))
	switch {
	case requirementsFileRe().MatchString(base):
		return manifestRequirements
	case base == filePackageJSON:
		return manifestPackageJSON
	case base == "pyproject.toml":
		return manifestPyproject
	case base == "pip.conf" || base == "pip.ini" || base == "uv.toml":
		return manifestIni
	case base == fileNpmrc || base == ".yarnrc":
		return manifestNpmrc
	}
	return manifestNone
}

func scanManifest(r *runner, t *scanText) {
	switch manifestOf(t.abs) {
	case manifestRequirements:
		r.checkRequirements(t)
	case manifestPackageJSON:
		r.checkPackageJSON(t)
	case manifestPyproject:
		r.checkPyproject(t)
	case manifestIni:
		r.checkIndexConfig(t, iniSourceRe())
	case manifestNpmrc:
		r.checkIndexConfig(t, npmrcSourceRe())
	}
	r.checkInstallCommands(t)
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
		return path[i+1:]
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
	if i := strings.Index(text, ";"); i >= 0 && !strings.Contains(text[:i], "://") {
		text = strings.TrimSpace(text[:i])
	}
	return text, hashed
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

// reportPin adds the AR054 finding the requirement deserves, if any.
func (r *runner) reportPin(abs string, line int, req requirement) {
	switch {
	case req.level == pinned:
	case req.vcs:
		r.addSev(SeverityWarning, CodeUnpinnedManifest, abs, line, "%s installs from a VCS URL that does not name a full commit; pin the 40-character commit so the code cannot change under the skill", req.name)
	case req.level == pinNone:
		r.addSev(SeverityWarning, CodeUnpinnedManifest, abs, line, "%s has no pinned version; pin it with == so a new release cannot change what the skill installs", req.name)
	case req.level == pinOpen:
		r.addSev(SeverityWarning, CodeUnpinnedManifest, abs, line, "%s sets only a lower bound (%s), so any later release is accepted; pin it with ==", req.name, req.spec)
	default:
		r.addSev(SeverityInfo, CodeUnpinnedManifest, abs, line, "%s allows a version range (%s); pin it with == or install from a lock file", req.name, req.spec)
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

func (r *runner) checkRequirements(t *scanText) {
	for _, l := range manifestLines(t) {
		if strings.HasPrefix(l.text, "-") && !strings.HasPrefix(l.text, "-e ") && !strings.HasPrefix(l.text, "--editable ") {
			r.reportSources(t.abs, l.no, l.text)
			continue
		}
		if req, ok := parseRequirement(l.text); ok {
			r.reportPin(t.abs, l.no, req)
			r.reportTyposquat(t.abs, l.no, req.name, pyPIUniverse)
		}
	}
}

type pyprojectDoc struct {
	Project struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	DependencyGroups map[string][]any `toml:"dependency-groups"`
	Tool             struct {
		UV struct {
			IndexURL      string           `toml:"index-url"`
			ExtraIndexURL []string         `toml:"extra-index-url"`
			Index         []map[string]any `toml:"index"`
			DevDeps       []string         `toml:"dev-dependencies"`
		} `toml:"uv"`
		Poetry struct {
			Source []map[string]any `toml:"source"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func (r *runner) checkPyproject(t *scanText) {
	var doc pyprojectDoc
	if toml.Unmarshal([]byte(t.raw), &doc) != nil {
		return
	}
	var specs []string
	specs = append(specs, doc.Project.Dependencies...)
	for _, name := range sortedKeys(doc.Project.OptionalDependencies) {
		specs = append(specs, doc.Project.OptionalDependencies[name]...)
	}
	for _, name := range sortedKeys(doc.DependencyGroups) {
		for _, e := range doc.DependencyGroups[name] {
			if s, ok := e.(string); ok {
				specs = append(specs, s)
			}
		}
	}
	specs = append(specs, doc.Tool.UV.DevDeps...)
	for _, spec := range specs {
		req, ok := parseRequirement(spec)
		if !ok {
			continue
		}
		line := lineContaining(t.rawLines(), spec)
		r.reportPin(t.abs, line, req)
		r.reportTyposquat(t.abs, line, req.name, pyPIUniverse)
	}
	var urls []string
	urls = append(urls, doc.Tool.UV.IndexURL)
	urls = append(urls, doc.Tool.UV.ExtraIndexURL...)
	for _, src := range slices.Concat(doc.Tool.UV.Index, doc.Tool.Poetry.Source) {
		if u, ok := src[keyURL].(string); ok {
			urls = append(urls, u)
		}
	}
	for _, u := range urls {
		if u != "" {
			r.reportRedirect(t.abs, lineContaining(t.rawLines(), u), "package index", u, false)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var packageJSONSections = []string{"dependencies", "devDependencies", "optionalDependencies"}

// npmSpec is the reading of one package.json dependency value; skip marks a
// value that names no registry release (a local path, a tarball URL).
type npmSpec struct {
	level pinLevel
	vcs   bool
	skip  bool
}

var localSpecPrefixes = []string{"file:", "link:", "workspace:", "portal:", "patch:", "catalog:", "./", "../", "/", "~/"}

func classifyNPMSpec(v string) npmSpec {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == "*" || strings.EqualFold(v, "x") || strings.EqualFold(v, "latest"):
		return npmSpec{level: pinNone}
	case hasAnyPrefix(v, localSpecPrefixes):
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
		return
	}
	lines := t.rawLines()
	for _, section := range packageJSONSections {
		var deps map[string]string
		if raw, ok := pkg[section]; !ok || json.Unmarshal(raw, &deps) != nil {
			continue
		}
		from := lineContaining(lines, `"`+section+`"`)
		for _, name := range sortedKeys(deps) {
			line := lineFrom(lines, from, `"`+name+`"`)
			if spec := classifyNPMSpec(deps[name]); !spec.skip {
				r.reportPin(t.abs, line, requirement{name: name, level: spec.level, vcs: spec.vcs, spec: deps[name]})
			}
			r.reportTyposquat(t.abs, line, name, npmUniverse)
		}
	}
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

var defaultSourceHosts = nameSet("pypi.org files.pythonhosted.org registry.npmjs.org registry.yarnpkg.com")

var (
	pipOptionRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)(--index-url|--extra-index-url|--find-links|--trusted-host|-i|-f)(?:=|\s+)(\S+)`)
	})
	npmOptionRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)--(?:@[A-Za-z0-9_.-]+:)?registry(?:=|\s+)(\S+)`)
	})
	configSetRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:(?:npm|yarn|pnpm)\s+config\s+set\s+(?:@[A-Za-z0-9_.-]+:)?registry|pip3?\s+config\s+set\s+\S*index-url)\s+(\S+)`)
	})
	sourceEnvRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:PIP_INDEX_URL|PIP_EXTRA_INDEX_URL|UV_INDEX_URL|UV_EXTRA_INDEX_URL|UV_DEFAULT_INDEX|(?i:npm_config_registry))=(\S+)`)
	})
	iniSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(index-url|extra-index-url|find-links|trusted-host)\s*[=:]\s*(\S+)`)
	})
	npmrcSourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:@[A-Za-z0-9_.-]+:)?(registry)\s*=\s*(\S+)`)
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

// reportSources reports the index options of one requirements line.
func (r *runner) reportSources(abs string, line int, text string) {
	for _, m := range pipOptionRe().FindAllStringSubmatch(text, -1) {
		r.reportOption(abs, line, m[1], m[2])
	}
}

func (r *runner) reportOption(abs string, line int, option, value string) {
	switch option {
	case "--trusted-host":
		r.reportRedirect(abs, line, "trusted host (TLS verification off)", value, true)
	case "--find-links", "-f", "find-links":
		r.reportRedirect(abs, line, "find-links location", value, false)
	case "registry":
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
	if loopback, _ := hostClass(host); loopback || defaultSourceHosts[host] || hostAllowed(host, r.security().AllowedHosts) {
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

func mayRedirectSource(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "index") || strings.Contains(text, "registry") || strings.Contains(text, "trusted-host") || strings.Contains(text, "find-links") || strings.Contains(text, " -i ") || strings.Contains(text, " -f ")
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

// reportTyposquat adds AR056 when name is one edit from a popular package.
func (r *runner) reportTyposquat(abs string, line int, name string, u nameUniverse) {
	if near := u.typosquatOf(name); near != "" {
		r.add(CodeTyposquat, abs, line, "%q is one edit from the popular package %q; check that it is the package you mean (typosquatting)", name, near)
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
	if len(n) < minSquatLen || strings.HasPrefix(n, "@") || u.popular[n] || u.neighbors[n] {
		return ""
	}
	for _, p := range u.sorted {
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
// substitution or swap of two adjacent characters.
func oneEdit(a, b string) bool {
	if a == b {
		return false
	}
	if editDistance(a, b) == 1 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	i := 0
	for a[i] == b[i] {
		i++
	}
	return i+1 < len(a) && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:]
}
