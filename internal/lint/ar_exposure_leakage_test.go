package lint

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLeakage is an ExposureSource with a fixed leakage, inputs and root.
type fakeLeakage struct {
	noExposure
	leakage ExposureLeakage
	ok      bool
	inputs  []string
	root    string
}

func (f fakeLeakage) Leakage() (ExposureLeakage, bool) { return f.leakage, f.ok }
func (f fakeLeakage) Inputs() []string                 { return f.inputs }
func (f fakeLeakage) Root() string                     { return f.root }

func leakMessage(kind, name, term, area string) string {
	return kind + ` "` + name + `" at session start names term "` + term + `" of area "` + area + `"`
}

// The leaks of a repository whose root CLAUDE.md, MCP server and listed skill name the billing area, in the
// order the session exposure finds them (walk order: the root files, then .claude).
var (
	serverLeak = ExposureLeak{Area: "billing", Term: "invoice", Kind: "mcp_server", Name: "invoice-db", Path: ".mcp.json", Scope: ".", Line: 1, Excerpt: "invoice-db", Message: leakMessage("mcp_server", "invoice-db", "invoice", "billing")}
	claudeLeak = ExposureLeak{Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 3, Excerpt: "The invoice flow.", Message: leakMessage("claude_md", "CLAUDE.md", "invoice", "billing")}
	skillLeak  = ExposureLeak{Ambiguous: true, Area: "billing", Term: "account", Kind: "skill", Name: "lookup", Path: ".claude/skills/lookup/SKILL.md", Scope: ".", Line: 1, Excerpt: "lookup: Finds an account.", Message: leakMessage("skill", "lookup", "account", "billing")}
)

// leakProject writes a repository holding the files the leaks name; table is appended to the configuration.
func leakProject(t *testing.T, table string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":          baseConfig + "\n" + table,
		"CLAUDE.md":                      "# Guide\nRun the tests.\nThe invoice flow.\n",
		".claude/skills/lookup/SKILL.md": "---\nname: lookup\ndescription: Finds an account.\n---\nbody\n",
		".claude/settings.json":          "{}\n",
		".mcp.json":                      `{"mcpServers": {"invoice-db": {"type": "http", "url": "https://example.com/mcp"}}}`,
		"README.md":                      "unrelated\n",
		"tools/areas.toml":               "[[area]]\nname = \"billing\"\nterms = [\"invoice\"]\n[skill_areas]\nlookup = \"billing\"\nghost = \"billing\"\n",
	})
	gitAdd(t, root)
	return root
}

// leakSource is the session exposure of leakProject at root.
func leakSource(root string, leaks ...ExposureLeak) fakeLeakage {
	return fakeLeakage{
		ok:      true,
		leakage: ExposureLeakage{Checked: 4, Leaks: leaks},
		inputs: []string{
			filepath.Join(root, ".ai-rulez", "config.toml"),
			filepath.Join(root, ".claude", "settings.json"),
		},
	}
}

func leakRun(t *testing.T, root string, opts ...Option) *Report {
	t.Helper()
	cfg := loadNoRemote(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	rep, err := RunWith(cfg, tree, Options{Cwd: root}, opts...)
	require.NoError(t, err)
	return rep
}

func codeFindings(fs []Finding, codes ...string) []Finding {
	var out []Finding
	for _, f := range fs {
		for _, c := range codes {
			if f.Code == c {
				out = append(out, f)
			}
		}
	}
	return out
}

func TestExposureLeakageRules(t *testing.T) {
	type want struct {
		code, file string
		line       int
		severity   Severity
		message    string
	}
	server := want{CodeExposureAreaTerm, ".mcp.json", 1, SeverityError, serverLeak.Message}
	claude := want{CodeExposureAreaTerm, "CLAUDE.md", 3, SeverityError, claudeLeak.Message}
	skill := want{CodeExposureAreaAmbiguous, ".claude/skills/lookup/SKILL.md", 1, SeverityWarning, skillLeak.Message}
	tests := []struct {
		name  string
		table string
		src   func(root string) ExposureSource
		want  []want
	}{
		{name: "no source is inert", src: func(string) ExposureSource { return nil }},
		{name: "a source without an area map is inert", src: func(string) ExposureSource { return fakeLeakage{} }},
		{
			name: "a term is an error and an ambiguous listing a warning, on the artifact's line",
			src:  func(root string) ExposureSource { return leakSource(root, serverLeak, claudeLeak, skillLeak) },
			want: []want{skill, server, claude},
		},
		{
			name:  "lint.severity moves a rule",
			table: "[lint.severity]\nAR9W0 = \"warning\"\nAR9W1 = \"off\"\n",
			src:   func(root string) ExposureSource { return leakSource(root, serverLeak, claudeLeak, skillLeak) },
			want:  []want{{server.code, server.file, server.line, SeverityWarning, server.message}, {claude.code, claude.file, claude.line, SeverityWarning, claude.message}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.table)
			var opts []Option
			if src := tt.src(root); src != nil {
				opts = append(opts, WithExposure(src))
			}

			// Act
			rep := leakRun(t, root, opts...)

			// Assert
			var got []want
			for _, f := range codeFindings(rep.Findings, CodeExposureAreaTerm, CodeExposureAreaAmbiguous) {
				got = append(got, want{f.Code, filepath.ToSlash(f.File), f.Line, f.Severity, f.Message})
				assert.Equal(t, AnalyzerInfo{AnalyzerExposure, ScopeBundle}, AnalyzerFor(f.Code))
			}
			assert.Equal(t, tt.want, got, dump(rep.Findings))
		})
	}
}

func TestExposureLeakageAcceptance(t *testing.T) {
	accept := func(rule, path, match string) string {
		return "\n[[lint.accept]]\nrule = \"" + rule + "\"\npath = \"" + path + "\"\nmatch = '" + match + "'\nreason = \"kept on purpose\"\n"
	}
	tests := []struct {
		name         string
		accepts      string
		wantAccepted []string // prefixes of the messages of the accepted leak findings
		wantStale    int
	}{
		{name: "no acceptance"},
		{name: "a match on the artifact accepts it", accepts: accept("AR9W0", ".mcp.json", `mcp_server "invoice-`), wantAccepted: []string{`mcp_server "invoice-db"`}},
		{name: "a match on the term accepts it", accepts: accept("exposure-area-term", "CLAUDE.md", `names term "invoice"`), wantAccepted: []string{`claude_md "CLAUDE.md"`}},
		{name: "a warning is accepted by its own rule", accepts: accept("AR9W1", ".claude/skills/*/SKILL.md", `term "account"`), wantAccepted: []string{`skill "lookup"`}},
		{name: "another rule's entry accepts nothing and is stale", accepts: accept("AR9W1", ".mcp.json", `mcp_server "invoice-`), wantStale: 1},
		{name: "an entry that matches nothing is stale", accepts: accept("AR9W0", "CLAUDE.md", `names term "ledger"`), wantStale: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.accepts)

			// Act
			rep := leakRun(t, root, WithExposure(leakSource(root, serverLeak, claudeLeak, skillLeak)))

			// Assert
			var accepted []string
			for _, f := range codeFindings(rep.Findings, CodeExposureAreaTerm, CodeExposureAreaAmbiguous) {
				if f.Meta != nil && f.Meta.Accepted {
					accepted = append(accepted, f.Message)
					assert.Equal(t, "kept on purpose", f.Meta.AcceptReason)
				}
			}
			require.Len(t, accepted, len(tt.wantAccepted), dump(rep.Findings))
			for i, w := range tt.wantAccepted {
				assert.True(t, strings.HasPrefix(accepted[i], w), "%q does not start with %q", accepted[i], w)
			}
			assert.Equal(t, tt.wantStale, countCode(rep.Findings, CodeAcceptUnused), dump(rep.Findings))
		})
	}
}

func TestReport_Leakage(t *testing.T) {
	// Arrange
	root := leakProject(t, "[lint]\nignore_paths = [\".claude/rules/**\"]\n[lint.severity]\nAR9W1 = \"error\"\n"+
		"\n[[lint.accept]]\nrule = \"AR9W0\"\npath = \".mcp.json\"\nmatch = 'mcp_server \"invoice-'\nreason = \"server names stay\"\n"+
		"\n[[lint.accept]]\nrule = \"AR9W0\"\npath = \"CLAUDE.md\"\nmatch = 'names term \"ledger\"'\nreason = \"gone\"\n"+
		"\n[[lint.accept]]\nrule = \"AR401\"\npath = \"CLAUDE.md\"\nmatch = 'nothing'\nreason = \"another rule\"\n")
	writeFiles(t, root, map[string]string{".claude/rules/billing.md": "The invoice rule.\n"})
	ignored := ExposureLeak{Area: "billing", Term: "invoice", Kind: "rule", Name: "billing", Path: ".claude/rules/billing.md", Scope: ".", Line: 1, Excerpt: "The invoice rule.", Message: leakMessage("rule", "billing", "invoice", "billing")}

	src := leakSource(root, serverLeak, ignored, claudeLeak, skillLeak)
	unread := ExposureUnchecked{Path: ".claude/skills/big/SKILL.md", Kind: "skill", Name: "big", Reason: "over the read limit"}
	src.leakage.Unchecked = []ExposureUnchecked{unread}

	// Act
	got := leakRun(t, root, WithExposure(src)).Leakage()

	// Assert
	require.NotNil(t, got)
	assert.Equal(t, &LeakageResult{
		Checked:   4,
		Unchecked: []ExposureUnchecked{unread},
		Leaks: []LeakFinding{
			{ExposureLeak: serverLeak, Code: CodeExposureAreaTerm, Severity: SeverityError, Accepted: true, Reason: "server names stay"},
			{ExposureLeak: claudeLeak, Code: CodeExposureAreaTerm, Severity: SeverityError},
			{ExposureLeak: skillLeak, Code: CodeExposureAreaAmbiguous, Severity: SeverityError},
		},
		Stale: []string{"lint.accept[1] (AR9W0 in CLAUDE.md) accepted no finding; remove it or fix its path or match"},
	}, got)
}

func TestReport_LeakageIsNilWhenTheRulesReadNone(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
	}{
		{name: "no source"},
		{name: "no area map", opts: []Option{WithExposure(fakeLeakage{})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := leakProject(t, "")
			assert.Nil(t, leakRun(t, root, tt.opts...).Leakage())
		})
	}
}

func TestExposureSkillAreaUnknown(t *testing.T) {
	tests := []struct {
		name      string
		table     string
		areasFile string
		unknown   []ExposureSkillArea
		wantFile  string
		wantLine  int
	}{
		{name: "known keys are clean"},
		{
			name:     "a key of the configuration is reported on its line",
			table:    "[exposure]\n[[exposure.areas]]\nname = \"billing\"\n[exposure.skill_areas]\nlookup = \"billing\"\nghost = \"billing\"\n",
			unknown:  []ExposureSkillArea{{Skill: "ghost", Area: "billing"}},
			wantFile: ".ai-rulez/config.toml",
			wantLine: 10,
		},
		{
			name:      "a key of the areas file is reported there",
			table:     "[exposure]\nareas_file = \"tools/areas.toml\"\n",
			areasFile: "tools/areas.toml",
			unknown:   []ExposureSkillArea{{Skill: "ghost", Area: "billing"}},
			wantFile:  "tools/areas.toml",
			wantLine:  6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.table)
			src := leakSource(root)
			src.leakage.UnknownSkills = tt.unknown
			if tt.areasFile != "" {
				src.leakage.AreasFile = filepath.Join(root, filepath.FromSlash(tt.areasFile))
			}

			// Act
			fs := codeFindings(leakRun(t, root, WithExposure(src)).Findings, CodeExposureSkillUnknown)

			// Assert
			require.Len(t, fs, len(tt.unknown), dump(fs))
			for i, u := range tt.unknown {
				assert.Equal(t, `skill_areas assigns "`+u.Skill+`" to "`+u.Area+`", but no skill of the session inventory is named "`+u.Skill+`"; rename or remove the entry`, fs[i].Message)
				assert.Equal(t, SeverityWarning, fs[i].Severity)
				assert.Equal(t, AnalyzerInfo{AnalyzerExposure, ScopeBundle}, AnalyzerFor(fs[i].Code))
				assert.Equal(t, tt.wantFile, filepath.ToSlash(fs[i].File))
				assert.Equal(t, tt.wantLine, fs[i].Line)
			}
		})
	}
}

func TestExposureLeakageUnderChanged(t *testing.T) {
	leaks := []string{".claude/skills/lookup/SKILL.md", ".mcp.json", "CLAUDE.md"}
	tests := []struct {
		name    string
		changed []string
		want    []string
	}{
		{name: "a changed artifact keeps its own finding", changed: []string{"CLAUDE.md"}, want: []string{"CLAUDE.md"}},
		{name: "a changed configuration keeps every leak", changed: []string{".ai-rulez/config.toml"}, want: leaks},
		{name: "changed settings keep every leak", changed: []string{".claude/settings.json"}, want: leaks},
		{name: "an unrelated change keeps none", changed: []string{"README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, "")
			rep := leakRun(t, root, WithExposure(leakSource(root, serverLeak, claudeLeak, skillLeak)))

			// Act
			NarrowToChanged(rep, tt.changed, "HEAD")

			// Assert
			var got []string
			for _, f := range codeFindings(rep.Findings, CodeExposureAreaTerm, CodeExposureAreaAmbiguous) {
				got = append(got, f.RepoPath())
			}
			assert.Equal(t, tt.want, got, dump(rep.Findings))
		})
	}
}

// A source rooted below the project (exposure leakage --root) names its artifacts relative to that root, so a
// leak is reported in the file below it.
func TestExposureLeakageUnderTheSourceRoot(t *testing.T) {
	// Arrange
	root := leakProject(t, "")
	writeFiles(t, root, map[string]string{"services/billing/CLAUDE.md": "# Billing\nEvery invoice.\n"})
	leak := ExposureLeak{Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 2, Excerpt: "Every invoice.", Message: leakMessage("claude_md", "CLAUDE.md", "invoice", "billing")}
	src := leakSource(root, leak)
	src.root = filepath.Join(root, "services", "billing")

	// Act
	rep := leakRun(t, root, WithExposure(src))

	// Assert
	fs := codeFindings(rep.Findings, CodeExposureAreaTerm)
	require.Len(t, fs, 1, dump(rep.Findings))
	assert.Equal(t, "services/billing/CLAUDE.md", filepath.ToSlash(fs[0].File))
	assert.Equal(t, 2, fs[0].Line)
	got := rep.Leakage()
	require.NotNil(t, got)
	require.Len(t, got.Leaks, 1, "the view finds the finding in the file below the source root")
	assert.Equal(t, "CLAUDE.md", got.Leaks[0].Path)
}

// Under a source rooted below the project, an entry can only be judged when its path could match a file of that
// root: an entry for a file elsewhere accepts nothing there without being stale.
func TestExposureLeakageAcceptanceUnderTheSourceRoot(t *testing.T) {
	accept := func(rule, path, match string) string {
		return "\n[[lint.accept]]\nrule = \"" + rule + "\"\npath = \"" + path + "\"\nmatch = '" + match + "'\nreason = \"kept\"\n"
	}
	tests := []struct {
		name      string
		accepts   string
		wantStale int
	}{
		{name: "an entry for a file outside the root is not judged", accepts: accept("AR9W0", ".mcp.json", `mcp_server "invoice-`)},
		{name: "an entry for a file under the root that accepts nothing is stale", accepts: accept("AR9W0", "services/billing/CLAUDE.md", `names term "ledger"`), wantStale: 1},
		{name: "an entry for a file under the root that accepts the leak is used", accepts: accept("AR9W0", "services/billing/CLAUDE.md", `names term "invoice"`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.accepts)
			writeFiles(t, root, map[string]string{"services/billing/CLAUDE.md": "# Billing\nEvery invoice.\n"})
			gitAdd(t, root)
			leak := ExposureLeak{Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 2, Excerpt: "Every invoice.", Message: leakMessage("claude_md", "CLAUDE.md", "invoice", "billing")}
			src := leakSource(root, leak)
			src.root = filepath.Join(root, "services", "billing")

			// Act
			rep := leakRun(t, root, WithExposure(src))

			// Assert
			assert.Equal(t, tt.wantStale, countCode(rep.Findings, CodeAcceptUnused), dump(rep.Findings))
		})
	}
}

// A stale acceptance is part of the leakage view even when AR9I0 itself is off or ignored.
func TestExposureLeakageStaleAcceptanceWithoutAR9I0(t *testing.T) {
	tests := []struct {
		name  string
		extra string
	}{
		{name: "AR9I0 reported"},
		{name: "AR9I0 off", extra: "[lint.severity]\nAR9I0 = \"off\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.extra+"\n[[lint.accept]]\nrule = \"AR9W0\"\npath = \"CLAUDE.md\"\nmatch = 'names term \"ledger\"'\nreason = \"gone\"\n")

			// Act
			got := leakRun(t, root, WithExposure(leakSource(root, claudeLeak))).Leakage()

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, []string{"lint.accept[0] (AR9W0 in CLAUDE.md) accepted no finding; remove it or fix its path or match"}, got.Stale)
		})
	}
}

// An AR9W6 finding depends on the files that shape the session, so a renamed or deleted skill (a change of the
// settings or the marketplace) keeps it under --changed.
func TestExposureSkillAreaUnknownUnderChanged(t *testing.T) {
	tests := []struct {
		name    string
		changed []string
		want    int
	}{
		{name: "the areas file keeps it", changed: []string{"tools/areas.toml"}, want: 1},
		{name: "the settings keep it", changed: []string{".claude/settings.json"}, want: 1},
		{name: "an unrelated change keeps none", changed: []string{"README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, "[exposure]\nareas_file = \"tools/areas.toml\"\n")
			src := leakSource(root)
			src.leakage.UnknownSkills = []ExposureSkillArea{{Skill: "ghost", Area: "billing"}}
			src.leakage.AreasFile = filepath.Join(root, "tools", "areas.toml")
			rep := leakRun(t, root, WithExposure(src))

			// Act
			NarrowToChanged(rep, tt.changed, "HEAD")

			// Assert
			assert.Len(t, codeFindings(rep.Findings, CodeExposureSkillUnknown), tt.want, dump(rep.Findings))
		})
	}
}
