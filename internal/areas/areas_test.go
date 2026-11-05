package areas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// testAreas is a synthetic map shaped like a real one: overlapping claims with excludes and a name-prefix glob,
// a term shared by two areas, literal terms, and a shared tooling area.
const testAreas = `
[[area]]
name = "billing"
owner = "team-billing"
paths = ["services/billing_*", "services/invoices"]
terms = ["billing", "ledger", "invoice_id"]
ambiguous = ["account", "customer"]

[[area]]
name = "core-services"
owner = "backend"
paths = ["services", "!services/billing_*", "!services/invoices"]
terms = ["dao"]
ambiguous = ["account", "query"]

[[area]]
name = "search"
description = "the search service"
owner = "search-team"
paths = ["products/search"]
terms = ["ranker", "indexer", "@acme/ui"]
ambiguous = ["query"]

[[area]]
name = "tooling"
owner = "devx"
paths = ["tools"]
shared = true
terms = ["buildtool"]
`

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func testMap(t *testing.T) *Map {
	t.Helper()
	m, problems := LoadFile(writeFile(t, t.TempDir(), "areas.toml", testAreas))
	require.Empty(t, problems)
	require.NotNil(t, m)
	return m
}

func TestIsUnder(t *testing.T) {
	tests := []struct {
		scope, prefix string
		want          bool
	}{
		{"services/a", "services", true},
		{"services", "services", true},
		{"services2", "services", false},
		{"anything", "", true},
		{"", "services", false},
		{"anything", ".", true},
		{".", "", true},
		{"", ".", true},
		{".", "services", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, IsUnder(tt.scope, tt.prefix), "%q under %q", tt.scope, tt.prefix)
	}
}

// The repository root is "" here and "." in the repository index and the inventory; both name it everywhere.
func TestRootScopeMayBeEmptyOrDot(t *testing.T) {
	m, problems := New([]config.ExposureArea{
		{Name: "everything", Paths: []string{"*"}, Terms: []string{"ranker"}},
		{Name: "dotted", Paths: []string{"x*"}, Terms: []string{"ledger"}},
	}, nil)
	require.Empty(t, problems)
	for _, text := range []string{"", "the ranker", "the ledger"} {
		area, source := m.ClassifyWithSource("", text)
		dotArea, dotSource := m.ClassifyWithSource(".", text)
		assert.Equal(t, area, dotArea, "classify %q", text)
		assert.Equal(t, source, dotSource, "source of %q", text)
	}
	for _, a := range m.Areas() {
		assert.Equal(t, a.MatchLength(""), a.MatchLength("."), a.Name)
	}
	assert.Equal(t, m.Owner(Skill{Name: "s", Text: "the ranker"}), m.Owner(Skill{Name: "s", Scope: ".", Text: "the ranker"}))
	assert.Equal(t, m.Claim(""), m.Claim("."))
}

func TestFindTerms(t *testing.T) {
	m := testMap(t)
	tests := []struct {
		text string
		want []string
	}{
		{"Use BILLING via accounts_billing, not billings or rankery", []string{"billing"}},
		{"the invoice_id predicate", []string{"invoice_id"}},
		{"see services/billing_service/README for details", nil},
		{"edit `products/search/readme.md` and `services/ranker_core`", nil},
		{"install @acme/ui", []string{"@acme/ui"}},
		{"BILLING_ENV is set", []string{"billing"}},
		{"ledger and indexer, then the ledger again", []string{"indexer", "ledger"}},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			var got []string
			for _, match := range m.Find(tt.text) {
				if !match.Ambiguous {
					got = append(got, match.Term)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindReportsEveryAreaListingAnAmbiguousWord(t *testing.T) {
	m := testMap(t)
	assert.Equal(t, []TermMatch{
		{Area: "billing", Term: "account", Ambiguous: true},
		{Area: "core-services", Term: "account", Ambiguous: true},
	}, m.Find("open an account"))
}

func TestClassify(t *testing.T) {
	m := testMap(t)
	tests := []struct {
		scope, text, want string
		source            Source
	}{
		{"services/billing_service", "", "billing", SourcePath},
		{"services/billing_service/deep", "", "billing", SourcePath},
		{"services/invoices", "", "billing", SourcePath},
		{"services/other_lib", "", "core-services", SourcePath},
		{"products/search/backend", "", "search", SourcePath},
		{"services/billing_service", "ranker indexer", "billing", SourcePath},
		{"", "ledger exports", "billing", SourceTerms},
		{"", "customer lookup", Unclassified, SourceNone},
		{"", "billing and ranker", Unclassified, SourceNone},
		{"", "ranker, indexer and billing", "search", SourceTerms},
		{"", "", Unclassified, SourceNone},
	}
	for _, tt := range tests {
		area, source := m.ClassifyWithSource(tt.scope, tt.text)
		assert.Equal(t, tt.want, area, "%q %q", tt.scope, tt.text)
		assert.Equal(t, tt.source, source, "%q %q", tt.scope, tt.text)
		assert.Equal(t, tt.want, m.Classify(tt.scope, tt.text))
	}
}

func TestExcludedAndGlobPaths(t *testing.T) {
	m := testMap(t)
	core, ok := m.Area("core-services")
	require.True(t, ok)
	assert.True(t, core.Owns("services/other_lib"))
	assert.False(t, core.Owns("services/billing_service"))
	assert.False(t, core.Owns("services/invoices/x"))
	billing, _ := m.Area("billing")
	assert.True(t, billing.Owns("services/billing_lib"))
	assert.False(t, billing.Owns("services/other"))
	assert.Equal(t, len("services/invoices"), billing.MatchLength("services/invoices/x"))
}

func TestPathTieGoesToTheFirstArea(t *testing.T) {
	m, problems := New([]config.ExposureArea{
		{Name: "first", Paths: []string{"lib"}},
		{Name: "second", Paths: []string{"lib"}},
	}, nil)
	require.Empty(t, problems)
	assert.Equal(t, "first", m.Classify("lib/x", ""))
}

func TestTermsAreNormalizedAndWinOverAmbiguous(t *testing.T) {
	m, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"Foo", "foo ", "evidence", " "}, Ambiguous: []string{"evidence", "bar"}}}, nil)
	require.Empty(t, problems)
	a, _ := m.Area("a")
	assert.Equal(t, []string{"evidence", "foo"}, a.Terms)
	assert.Equal(t, []string{"bar"}, a.Ambiguous)
}

func TestATermMayBelongToSeveralAreas(t *testing.T) {
	m, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"cache"}}, {Name: "b", Terms: []string{"cache"}}}, nil)
	require.Empty(t, problems)
	assert.Equal(t, []TermMatch{{Area: "a", Term: "cache"}, {Area: "b", Term: "cache"}}, m.Find("cache role"))
}

func TestLongestAlternativeWins(t *testing.T) {
	m, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"ledger"}}, {Name: "b", Terms: []string{"ledger_sync"}}}, nil)
	require.Empty(t, problems)
	assert.Equal(t, []TermMatch{{Area: "b", Term: "ledger_sync"}}, m.Find("run ledger_sync now"))
}

func TestStripPathLiterals(t *testing.T) {
	m := testMap(t)
	assert.Equal(t, "see   and x", m.StripPathLiterals("see services/billing_x/a.md and x"))
	assert.Equal(t, "keep my/services/x and a/services/b", m.StripPathLiterals("keep my/services/x and a/services/b"))
	assert.Equal(t, "( )", m.StripPathLiterals("(tools/run.sh)"))
}

func TestOwner(t *testing.T) {
	m, problems := New([]config.ExposureArea{
		{Name: "billing", Paths: []string{"services/billing"}, Terms: []string{"invoice"}},
		{Name: "search", Terms: []string{"ranker"}},
		{Name: "tooling", Paths: []string{"tools"}, Shared: true},
	}, map[string]string{"deploy": "tooling", "kit:solo": "search", "lookup": "billing"})
	require.Empty(t, problems)
	tests := []struct {
		name  string
		skill Skill
		want  Owner
	}{
		{"skill_areas wins over the path claim", Skill{Name: "deploy", Scope: "services/billing"}, Owner{"tooling", SourceSkillAreas}},
		{"path claim", Skill{Name: "x", Scope: "services/billing"}, Owner{"billing", SourcePath}},
		{"terms", Skill{Name: "x", Text: "tune the ranker"}, Owner{"search", SourceTerms}},
		{"nothing", Skill{Name: "x", Text: "format files"}, Owner{Unclassified, SourceNone}},
		{"plugin skill by qualified name", Skill{Name: "solo", Plugin: "kit"}, Owner{"search", SourceSkillAreas}},
		{"plugin skill by bare name", Skill{Name: "lookup", Plugin: "kit"}, Owner{"billing", SourceSkillAreas}},
		{"plugin skill in a claimed directory", Skill{Name: "inner", Plugin: "p", Scope: "tools/.claude/plugins/p", Text: "p:inner"}, Owner{"tooling", SourcePath}},
		{"a repository skill never matches a qualified entry", Skill{Name: "solo"}, Owner{Unclassified, SourceNone}},
		{"a colon name of a repository skill is not split", Skill{Name: "ops:lookup"}, Owner{Unclassified, SourceNone}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, m.Owner(tt.skill), tt.name)
	}
	assert.Equal(t, "kit:solo", Skill{Name: "solo", Plugin: "kit"}.QualifiedName())
}

func TestNilMapIsEmpty(t *testing.T) {
	var m *Map
	assert.Nil(t, m.Areas())
	assert.Nil(t, m.Find("anything"))
	assert.Equal(t, Unclassified, m.Classify("x", "y"))
	assert.Equal(t, Owner{Unclassified, SourceNone}, m.Owner(Skill{Name: "x"}))
	assert.Equal(t, "a/b", m.StripPathLiterals("a/b"))
	assert.False(t, m.IsShared("x"))
}

func TestAccessorsReturnCopies(t *testing.T) {
	m := testMap(t)
	areas := m.Areas()
	areas[0].Terms[0] = "changed"
	skillAreas := m.SkillAreas()
	skillAreas["x"] = "y"
	again, _ := m.Area(areas[0].Name)
	assert.NotEqual(t, "changed", again.Terms[0])
	assert.Empty(t, m.SkillAreas())
	assert.True(t, m.IsShared("tooling"))
	assert.False(t, m.IsShared("billing"))
}

func TestInvalidMapsAreRejected(t *testing.T) {
	tests := []struct {
		name       string
		defs       []config.ExposureArea
		skillAreas map[string]string
		want       string
	}{
		{"absolute path", []config.ExposureArea{{Name: "a", Paths: []string{"/abs"}}}, nil, `path "/abs" must be a repository-relative prefix`},
		{"excluded absolute path", []config.ExposureArea{{Name: "a", Paths: []string{"!/abs"}}}, nil, `path "!/abs" must be a repository-relative prefix`},
		{"trailing slash", []config.ExposureArea{{Name: "a", Paths: []string{"dir/"}}}, nil, `path "dir/" must be a repository-relative prefix`},
		{"empty path", []config.ExposureArea{{Name: "a", Paths: []string{"!"}}}, nil, `path "!" is empty`},
		{"dot segment", []config.ExposureArea{{Name: "a", Paths: []string{"a/../b"}}}, nil, `".." segment`},
		{"empty segment", []config.ExposureArea{{Name: "a", Paths: []string{"a//b"}}}, nil, "empty"},
		{"inner star", []config.ExposureArea{{Name: "a", Paths: []string{"a*/b"}}}, nil, `"*" only at the end`},
		{"backslash", []config.ExposureArea{{Name: "a", Paths: []string{`a\b`}}}, nil, "forward slashes"},
		{"duplicate names", []config.ExposureArea{{Name: "a"}, {Name: "a"}}, nil, `duplicate area name "a"`},
		{"no name", []config.ExposureArea{{}}, nil, "area 1 has no name"},
		{"not kebab", []config.ExposureArea{{Name: "Billing_Svc"}}, nil, "lower-kebab"},
		{"reserved", []config.ExposureArea{{Name: Unclassified}}, nil, "reserved"},
		{"unknown skill area", []config.ExposureArea{{Name: "a"}}, map[string]string{"s": "b"}, `"s" to "b", which names no area`},
		{"empty skill name", []config.ExposureArea{{Name: "a"}}, map[string]string{" ": "a"}, "empty skill name"},
		{"declare escapes", []config.ExposureArea{{Name: "a", Declare: "../x"}}, nil, `declare "../x"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, problems := New(tt.defs, tt.skillAreas)
			assert.Nil(t, m)
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0].Message, tt.want)
		})
	}
}

func TestValidPathForms(t *testing.T) {
	for _, p := range []string{"services", "services/billing_*", "!services/legacy", "a.b/c-d", "*", "vendor/*", "!vendor/*", "a/b/*"} {
		assert.Empty(t, badPath(p), p)
	}
}

// "vendor/*" is a name prefix like any other: everything below vendor, not vendor itself.
func TestDirectoryStarClaimsWhatIsBelow(t *testing.T) {
	m, problems := New([]config.ExposureArea{
		{Name: "vendored", Paths: []string{"vendor/*", "!vendor/keep*"}},
		{Name: "rest", Paths: []string{"vendor"}},
	}, nil)
	require.Empty(t, problems)
	vendored, _ := m.Area("vendored")
	tests := []struct {
		scope  string
		length int
		claim  string
	}{
		{"vendor/lib", len("vendor/"), "vendored"},
		{"vendor/lib/deep", len("vendor/"), "vendored"},
		{"vendor", 0, "rest"},
		{"vendor/keeper", 0, "rest"},
		{"vendors/lib", 0, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.length, vendored.MatchLength(tt.scope), tt.scope)
		assert.Equal(t, tt.claim, m.Claim(tt.scope), tt.scope)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "tools/areas.toml", testAreas+"\n[skill_areas]\ndeploy = \"tooling\"\n")
	tests := []struct {
		name    string
		cfg     *config.ExposureConfig
		areas   int
		problem string
	}{
		{"no table", nil, 0, ""},
		{"no areas", &config.ExposureConfig{ContextWindow: 1000}, 0, ""},
		{"inline", &config.ExposureConfig{Areas: []config.ExposureArea{{Name: "a"}}}, 1, ""},
		{"file", &config.ExposureConfig{AreasFile: "tools/areas.toml"}, 4, ""},
		{"both", &config.ExposureConfig{AreasFile: "tools/areas.toml", SkillAreas: map[string]string{"x": "a"}}, 0, "keep the area map in one place"},
		{"absolute", &config.ExposureConfig{AreasFile: filepath.Join(root, "tools", "areas.toml")}, 0, "relative to the project root"},
		{"escapes", &config.ExposureConfig{AreasFile: "../areas.toml"}, 0, "inside the project root"},
		{"escapes after a directory", &config.ExposureConfig{AreasFile: "tools/../../areas.toml"}, 0, "inside the project root"},
		{"dot dot that stays inside", &config.ExposureConfig{AreasFile: "tools/../tools/areas.toml"}, 4, ""},
		{"missing", &config.ExposureConfig{AreasFile: "nope.toml"}, 0, "cannot be read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, problems := Load(tt.cfg, root)
			if tt.problem != "" {
				require.Len(t, problems, 1)
				assert.Contains(t, problems[0].Message, tt.problem)
				assert.Nil(t, m)
				return
			}
			require.Empty(t, problems)
			assert.Len(t, m.Areas(), tt.areas)
		})
	}
}

func TestLoadRefusesAnAreasFileSymlinkedOutOfTheProject(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeFile(t, outside, "areas.toml", testAreas)
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "areas.toml"), filepath.Join(root, "areas.toml"))
	m, problems := Load(&config.ExposureConfig{AreasFile: "areas.toml"}, root)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "inside the project root")
	assert.Nil(t, m)
}

func TestLoadFileProblemsCarryTheFileAndLine(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name, body, want string
		lines            []int
	}{
		{"bad claim", "[[area]]\nname = \"a\"\n\npaths = [\"x/\"]\n", `path "x/"`, []int{4}},
		{"duplicate", "[[area]]\nname = \"a\"\n[[area]]\nname = \"a\"\n", "duplicate", []int{4}},
		{"duplicate after the name as a term", "[[area]]\nname = \"b\"\nterms = [\"a\"]\n[[area]]\nname = \"a\"\n[[area]]\n\nname = \"a\"\n", "duplicate", []int{8}},
		{"the same bad claim in two areas", "[[area]]\nname = \"a\"\npaths = [\"x/\"]\n[[area]]\nname = \"b\"\npaths = [\"x/\"]\n", `path "x/"`, []int{3, 6}},
		{"an area without a name", "[[area]]\nname = \"a\"\n\n[[area]]\nterms = [\"x\"]\n", "area 2 has no name", []int{4}},
		{"unknown key", "[[area]]\nname = \"a\"\nowners = \"x\"\n", `unknown key "area.owners"`, []int{3}},
		{"unknown table", "[skills]\nx = \"a\"\n", `unknown key "skills"`, []int{1}},
		{"syntax", "[[area]\nname = 1\n", "not valid TOML", []int{1}},
		{"skill area", "[[area]]\nname = \"a\"\n[skill_areas]\nx = \"b\"\n", "names no area", []int{4}},
		{"duplicate after the same word as a term in its own table", "[[area]]\nname = \"ops\"\n[[area]]\nterms = [\"ops\"]\n\nname = \"ops\"\n", "duplicate", []int{6}},
		{"reserved name after the same word as a term", "[[area]]\nterms = [\"unclassified\"]\nname = \"unclassified\"\n", "is reserved", []int{3}},
		{"a quoted skill key holding =", "[[area]]\nname = \"a\"\n[skill_areas]\n\"x=y\" = \"nope\"\n", "names no area", []int{4}},
		{"a multi-line string line that looks like a table", "[[area]]\nname = \"a\"\ndescription = \"\"\"\n[not a table]\n\"\"\"\npaths = [\"x/\"]\n", `path "x/"`, []int{6}},
		{"skill area named earlier as a term", "[[area]]\nname = \"a\"\nterms = [\"ops\"]\n\n[skill_areas]\nfirst = \"a\"\n\"x-y\" = \"ops\"\nz = \"ops\"\n", "names no area", []int{7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, dir, tt.name+".toml", tt.body)
			m, problems := LoadFile(path)
			assert.Nil(t, m)
			require.Len(t, problems, len(tt.lines))
			for i, p := range problems {
				assert.Equal(t, path, p.File)
				assert.Equal(t, tt.lines[i], p.Line, "line of %q", p.Message)
				assert.Contains(t, p.Message, tt.want)
			}
		})
	}
}

// LineIn finds an entry of the inline [exposure] form the same way it finds one in an areas file.
func TestProblemLineInTheInlineForm(t *testing.T) {
	lines := strings.Split("[exposure]\n[[exposure.areas]]\nname = \"a\"\nterms = [\"ops\"]\n[[exposure.areas]]\nname = \"a\"\n[exposure.skill_areas]\ndeploy = \"ops\"\n", "\n")
	_, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"ops"}}, {Name: "a"}}, map[string]string{"deploy": "ops"})
	require.Len(t, problems, 2)
	assert.Equal(t, 6, problems[0].LineIn(lines), problems[0].Message)
	assert.Equal(t, 8, problems[1].LineIn(lines), problems[1].Message)
	assert.Equal(t, 0, Problem{Message: "x"}.LineIn(lines))
}

// The inline table and the areas file decode into the same type, so a map moves between them unchanged.
func TestInlineAndFileFormsAreTheSame(t *testing.T) {
	var doc fileDoc
	require.NoError(t, toml.Unmarshal([]byte(testAreas), &doc))
	inline, problems := Load(&config.ExposureConfig{Areas: doc.Area}, t.TempDir())
	require.Empty(t, problems)
	assert.Equal(t, testMap(t).Areas(), inline.Areas())
}

func TestSkillLine(t *testing.T) {
	file := strings.Split("[[area]]\nname = \"a\"\n[skill_areas]\ndeploy = \"a\"\n\"ops:probe\" = \"a\"\n", "\n")
	inline := strings.Split("[exposure]\n[exposure.skill_areas]\ndeploy = \"a\"\n[other]\nghost = \"a\"\n", "\n")
	tests := []struct {
		name  string
		lines []string
		skill string
		want  int
	}{
		{name: "a key of the areas file", lines: file, skill: "deploy", want: 4},
		{name: "a quoted key", lines: file, skill: "ops:probe", want: 5},
		{name: "a missing key is the table header", lines: file, skill: "ghost", want: 3},
		{name: "a key of the inline table", lines: inline, skill: "deploy", want: 3},
		{name: "a key after the table is not in it", lines: inline, skill: "ghost", want: 2},
		{name: "no table", lines: []string{"[[area]]", "name = \"a\""}, skill: "deploy", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SkillLine(tt.lines, tt.skill))
		})
	}
}

func TestExcludedCompoundsDoNotMatchTheirTerm(t *testing.T) {
	// Arrange: `xberg` names an area, but the org name `xberg-io` contains it.
	m, problems := New([]config.ExposureArea{
		{Name: "xberg", Terms: []string{"xberg"}, Exclude: []string{"XBerg-IO"}},
		{Name: "other", Terms: []string{"xberg-io"}},
	}, nil)
	require.Empty(t, problems)

	tests := []struct {
		name, text string
		want       []string
	}{
		{"the compound alone", "see github.com/xberg-io/repo", []string{"other/xberg-io"}},
		{"the compound in other case", "the Xberg-IO org", []string{"other/xberg-io"}},
		{"the compound and the bare term", "xberg-io hosts xberg", []string{"xberg/xberg", "other/xberg-io"}},
		{"the bare term", "run xberg now", []string{"xberg/xberg"}},
		{"no match at all", "nothing here", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, f := range m.Find(tt.text) {
				got = append(got, f.Area+"/"+f.Term)
			}
			assert.Equal(t, tt.want, got)
		})
	}
	area, ok := m.Area("xberg")
	require.True(t, ok)
	assert.Equal(t, []string{"xberg-io"}, area.Exclude)
}

func TestExcludeThatNamesNoTermIsRejected(t *testing.T) {
	m, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"foo"}, Exclude: []string{"bar-io"}}}, nil)
	assert.Nil(t, m)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, `exclude "bar-io" contains none of the terms or ambiguous words`)
}

const acceptFile = `
[[area]]
name = "xberg"
terms = ["xberg"]

[[accept]]
term = "XBerg"
path = "docs/*.md"
reason = "the product name in docs"

[[accept]]
term = "xberg"
area = "xberg"
path = "CLAUDE.md"
reason = "root note"
`

func TestLoadFileReadsAcceptances(t *testing.T) {
	path := writeFile(t, t.TempDir(), "areas.toml", acceptFile)

	m, problems := LoadFile(path)

	require.Empty(t, problems)
	got := m.Accepts()
	require.Len(t, got, 2)
	assert.Equal(t, Accept{Term: "xberg", Path: "docs/*.md", Reason: "the product name in docs", Index: 0, Line: 6}, got[0])
	assert.Equal(t, 11, got[1].Line)
	assert.Equal(t, 1, got[1].Index)
}

func TestAcceptingMatchesTermAreaAndPath(t *testing.T) {
	m, problems := LoadFile(writeFile(t, t.TempDir(), "areas.toml", acceptFile))
	require.Empty(t, problems)

	tests := []struct {
		name, area, term, path string
		want                   int
	}{
		{"glob path, term case-insensitive", "xberg", "XBERG", "docs/a.md", 0},
		{"exact path with area", "xberg", "xberg", "CLAUDE.md", 1},
		{"another path", "xberg", "xberg", "README.md", -1},
		{"another term", "xberg", "other", "CLAUDE.md", -1},
		{"another area", "billing", "xberg", "CLAUDE.md", -1},
		{"a joined warning area names it", "billing/xberg", "xberg", "CLAUDE.md", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ok := m.Accepting(tt.area, tt.term, tt.path)
			assert.Equal(t, tt.want >= 0, ok)
			if ok {
				assert.Equal(t, tt.want, a.Index)
			}
		})
	}
}

func TestInvalidAcceptancesAreRejected(t *testing.T) {
	tests := []struct {
		name, entry, want string
	}{
		{"no reason", "term = \"x\"\npath = \"a.md\"\n", "reason is required"},
		{"blank reason", "term = \"x\"\npath = \"a.md\"\nreason = \"  \"\n", "reason is required"},
		{"no term", "path = \"a.md\"\nreason = \"r\"\n", "term is empty"},
		{"no path", "term = \"x\"\nreason = \"r\"\n", "path is empty"},
		{"outside the root", "term = \"x\"\npath = \"../a.md\"\nreason = \"r\"\n", "repository-relative"},
		{"dot-dot after a leading slash", "term = \"x\"\npath = \"/../a.md\"\nreason = \"r\"\n", "repository-relative"},
		{"dot-dot inside the path", "term = \"x\"\npath = \"a/../b.md\"\nreason = \"r\"\n", "repository-relative"},
		{"malformed glob", "term = \"x\"\npath = \"[a.md\"\nreason = \"r\"\n", "malformed"},
		{"unknown area", "term = \"x\"\narea = \"nope\"\npath = \"a.md\"\nreason = \"r\"\n", `names no area`},
		{"unknown key", "term = \"x\"\npath = \"a.md\"\nreason = \"r\"\nwhy = \"r\"\n", `unknown key "accept.why"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "[[area]]\nname = \"a\"\nterms = [\"x\"]\n\n\n[[accept]]\n" + tt.entry
			m, problems := LoadFile(writeFile(t, t.TempDir(), "areas.toml", body))
			assert.Nil(t, m)
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0].Message, tt.want)
			assert.Positive(t, problems[0].Line)
		})
	}
}

func TestExcludedCompoundsNeedNameBoundaries(t *testing.T) {
	m, problems := New([]config.ExposureArea{{Name: "xberg", Terms: []string{"xberg"}, Exclude: []string{"xberg-io", "xberg-io-labs"}}}, nil)
	require.Empty(t, problems)
	tests := []struct {
		name, text string
		leaks      bool
	}{
		{"the compound", "the xberg-io org", false},
		{"the longer compound", "the xberg-io-labs org", false},
		{"both compounds", "xberg-io-labs and xberg-io", false},
		{"a longer name that starts with it", "the xberg-iox org", true},
		{"a longer name that ends with it", "the a-xberg-io org", true},
		{"a prefixed name", "the my-xberg-io org", true},
		{"a suffixed name", "the xberg-io-tools org", true},
		{"glued letters before", "the myxberg-io org", false},
		{"the compound and a leaked longer name", "xberg-io and xberg-iox", true},
		{"the compound at the ends of the text", "xberg-io", false},
		{"punctuation is a boundary", "(xberg-io), xberg-io.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.leaks, len(m.Find(tt.text)) > 0)
		})
	}
}

func TestExcludeEqualToATermIsRejected(t *testing.T) {
	tests := []struct {
		name string
		def  config.ExposureArea
	}{
		{"a term", config.ExposureArea{Name: "a", Terms: []string{"foo"}, Exclude: []string{"FOO"}}},
		{"an ambiguous word", config.ExposureArea{Name: "a", Terms: []string{"bar"}, Ambiguous: []string{"foo"}, Exclude: []string{"foo"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, problems := New([]config.ExposureArea{tt.def}, nil)
			assert.Nil(t, m)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, "would disable it")
		})
	}
}

func TestAcceptPathsFollowTheLintAcceptGlobs(t *testing.T) {
	tests := []struct {
		pattern, file string
		want          bool
	}{
		{"**/AGENTS.md", "AGENTS.md", true},
		{"**/AGENTS.md", "a/b/AGENTS.md", true},
		{"AGENTS.md", "AGENTS.md", true},
		{"AGENTS.md", "sub/AGENTS.md", true},
		{"docs/**/a.md", "docs/a.md", true},
		{"docs/**/a.md", "docs/x/y/a.md", true},
		{"docs/**/a.md", "other/a.md", false},
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/x/a.md", false},
		{"docs/**", "docs/x/y.md", true},
		{"docs", "docs/x/y.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" vs "+tt.file, func(t *testing.T) {
			body := "[[area]]\nname = \"a\"\nterms = [\"x\"]\n\n[[accept]]\nterm = \"x\"\npath = \"" + tt.pattern + "\"\nreason = \"r\"\n"
			m, problems := LoadFile(writeFile(t, t.TempDir(), "areas.toml", body))
			require.Empty(t, problems)

			_, ok := m.Accepting("a", "x", tt.file)

			assert.Equal(t, tt.want, ok)
		})
	}
}

func TestAcceptPathsNormalizeLikeLintAccept(t *testing.T) {
	tests := []struct {
		pattern, file string
		want          bool
	}{
		{"/CLAUDE.md", "CLAUDE.md", true},
		{"./docs/a.md", "docs/a.md", true},
		{"./docs/", "docs/x/a.md", true},
		{`docs/\*.md`, "docs/a.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" vs "+tt.file, func(t *testing.T) {
			body := "[[area]]\nname = \"a\"\nterms = [\"x\"]\n\n[[accept]]\nterm = \"x\"\npath = '" + tt.pattern + "'\nreason = \"r\"\n"
			m, problems := LoadFile(writeFile(t, t.TempDir(), "areas.toml", body))
			require.Empty(t, problems)
			_, ok := m.Accepting("a", "x", tt.file)
			assert.Equal(t, tt.want, ok)
		})
	}
}

func TestExcludeWithTheTermOnlyInsideAWordIsRejected(t *testing.T) {
	tests := []struct {
		name    string
		exclude string
		ok      bool
	}{
		{"inside a longer word", "researchers", false},
		{"glued letters", "mysearch", false},
		{"as a whole word of a compound", "search-engine", true},
		{"after punctuation", "web.search", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, problems := New([]config.ExposureArea{{Name: "a", Terms: []string{"search"}, Exclude: []string{tt.exclude}}}, nil)
			if tt.ok {
				require.Empty(t, problems)
				assert.NotNil(t, m)
				return
			}
			assert.Nil(t, m)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, "as a whole word")
		})
	}
}
