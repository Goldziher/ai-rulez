package verifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

const ruleBody = "# Database\n\n## Migrations\n\nEvery migration needs a down section.\n"

// specProject writes files plus the given spec TOML under .ai-rulez/verifiers
// and returns a config with the rules "database" and "api" loaded as content.
func specProject(t *testing.T, files map[string]string, specTOML string) *config.Config {
	t.Helper()
	root := writeFiles(t, files)
	cfgDir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, VerifiersDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, VerifiersDirName, "t.toml"), []byte(specTOML), 0o644))
	rules := []config.ContentFile{}
	for _, name := range []string{"database", "api"} {
		p := filepath.Join(cfgDir, "rules", name+".md")
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(ruleBody), 0o644))
		rules = append(rules, config.ContentFile{Name: name, Path: p, Content: ruleBody})
	}
	return &config.Config{BaseDir: root, ConfigDir: cfgDir, Content: &config.ContentTree{Rules: rules}}
}

func TestLoadSpecs_Validation(t *testing.T) {
	const head = "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n"
	tests := []struct {
		name string
		toml string
		want string // substring of the problem; "" means the spec loads
	}{
		{"valid", head + "[verifiers.require.regex]\nregex = \"x\"\n", ""},
		{"unknown key", head + "bogus = 1\n[verifiers.require.regex]\nregex = \"x\"\n", "invalid TOML"},
		{"no target", "[[verifiers]]\nid = \"v\"\n[verifiers.require.file_exists]\npath = \"a\"\n", "exactly one of rule"},
		{"two targets", head + "skill = \"s\"\n[verifiers.require.regex]\nregex = \"x\"\n", "exactly one of rule"},
		{"dangling rule", "[[verifiers]]\nid = \"v\"\nrule = \"nope\"\n[verifiers.require.file_exists]\npath = \"a\"\n", "does not exist"},
		{"missing anchor", head + "anchor = \"## Nope\"\n[verifiers.require.regex]\nregex = \"x\"\n", "anchor"},
		{"present anchor", head + "anchor = \"## Migrations\"\n[verifiers.require.regex]\nregex = \"x\"\n", ""},
		{"no predicate", head, "needs a [verifiers.require]"},
		{"bad regex", head + "[verifiers.require.regex]\nregex = \"(\"\n", "invalid regex"},
		{"backreference is RE2-invalid", head + "[verifiers.require.regex]\nregex = \"(a)\\\\1\"\n", "invalid regex"},
		{"two predicates", head + "[verifiers.require.regex]\nregex = \"x\"\n[verifiers.require.forbid]\nregex = \"y\"\n", "exactly one of"},
		{"bad in", head + "[verifiers.require.regex]\nregex = \"x\"\nin = \"nowhere\"\n", "invalid in"},
		{"same-file needs when_changed", "[[verifiers]]\nid = \"v\"\nrule = \"database\"\n[verifiers.require.regex]\nregex = \"x\"\n", "needs when_changed"},
		{"unknown template var", head + "[verifiers.require.paired]\nfor_each = \"a/{rel}.go\"\nrequires_exists = \"{nope}\"\n", "unknown template variable"},
		{"paired needs one requirement", head + "[verifiers.require.paired]\nfor_each = \"a/{rel}.go\"\n", "exactly one of requires_changed"},
		{"bad id", "[[verifiers]]\nid = \"Bad Id\"\nrule = \"database\"\n[verifiers.require.file_exists]\npath = \"a\"\n", "invalid id"},
		{"bad severity", head + "severity = \"fatal\"\n[verifiers.require.regex]\nregex = \"x\"\n", "invalid severity"},
		{"bad example expect", head + "[verifiers.require.regex]\nregex = \"x\"\n[[verifiers.examples]]\nname = \"e\"\nexpect = \"maybe\"\n", "expect"},
		{"example path escape", head + "[verifiers.require.regex]\nregex = \"x\"\n[[verifiers.examples]]\nname = \"e\"\nexpect = \"pass\"\nfiles = { \"../x\" = \"y\" }\n", "outside the fixture"},
		{"glob_count needs bound", head + "[verifiers.require.glob_count]\nfiles = \"*\"\n", "min or max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := specProject(t, map[string]string{"a.sql": "x"}, tt.toml)

			// Act
			specs, problems := LoadSpecs(cfg)

			// Assert
			if tt.want == "" {
				assert.Empty(t, problems)
				assert.Len(t, specs, 1)
				return
			}
			require.Len(t, problems, 1, "specs=%v", specs)
			assert.Contains(t, problems[0].Message, tt.want)
			assert.Empty(t, specs)
		})
	}
}

func TestLoadSpecs_NestingDepthLimit(t *testing.T) {
	const head = "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n"
	leaf := "[verifiers.require.%s.regex]\nregex = \"x\"\n"
	deep := head + strings.Replace(leaf, "%s", "not.not.not.not", 1)
	ok := head + strings.Replace(leaf, "%s", "not.not.not", 1)

	_, okProblems := LoadSpecs(specProject(t, nil, ok))
	_, deepProblems := LoadSpecs(specProject(t, nil, deep))

	assert.Empty(t, okProblems)
	require.Len(t, deepProblems, 1)
	assert.Contains(t, deepProblems[0].Message, "deeper than 4")
}

func TestLoadSpecs_DuplicateIDAcrossConfigAndFile(t *testing.T) {
	cfg := specProject(t, nil, "[[verifiers]]\nid = \"dup\"\nrule = \"database\"\n[verifiers.require.file_exists]\npath = \"a\"\n")
	cfg.Verifiers = []config.VerifierConfig{{Name: "dup", Type: "file_exists", Path: "a"}}

	specs, problems := LoadSpecs(cfg)

	assert.Empty(t, specs)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "duplicate")
}

func TestLoadSpecs_RefusesSymlinkedFile(t *testing.T) {
	cfg := specProject(t, nil, "")
	outside := filepath.Join(t.TempDir(), "evil.toml")
	require.NoError(t, os.WriteFile(outside, []byte("[[verifiers]]\nid = \"x\"\n"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(cfg.ConfigDir, VerifiersDirName, "link.toml"))

	specs, problems := LoadSpecs(cfg)

	assert.Empty(t, specs)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "symlink")
}

func TestRun_InvalidDeclarationIsAR9H2NotSilentlyDropped(t *testing.T) {
	cfg := specProject(t, nil, "[[verifiers]]\nid = \"v\"\nrule = \"ghost\"\n[verifiers.require.file_exists]\npath = \"a\"\n")

	rep := Run(context.Background(), cfg, Options{})

	require.Len(t, rep.Results, 1)
	assert.Equal(t, StatusError, rep.Results[0].Status)
	assert.Equal(t, CodeVerifierInvalid, rep.Results[0].Code)
	assert.True(t, rep.CannotRun())
}

func runSpecs(t *testing.T, cfg *config.Config, opts Options) map[string]Result {
	t.Helper()
	rep := Run(context.Background(), cfg, opts)
	require.NoError(t, rep.Err)
	out := map[string]Result{}
	for _, r := range rep.Results {
		out[r.Name] = r
	}
	return out
}

func TestRun_SpecPredicates(t *testing.T) {
	files := map[string]string{
		"db/1.sql":            "create\n-- down\ndrop\n",
		"db/2.sql":            "create\r\n",
		"src/api/users.py":    "import app.db\r\nx = 1\n",
		"src/api/misc.py":     "x = 1\n",
		"tests/test_users.py": "t",
		"LICENSE":             "mit",
	}
	tests := []struct {
		name     string
		spec     string
		want     Status
		wantFile string
		wantLine int
	}{
		{"regex same-file fails on the file lacking it", `
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "(?m)^-- down$"`, StatusFail, "db/2.sql", 0},
		{"regex any-file passes when one matches", `
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "(?m)^-- down$"
in = "any-file"`, StatusPass, "", 0},
		{"forbid reports file and line, CRLF-normalized", `
when_changed = ["src/api/*.py"]
[verifiers.require.forbid]
regex = "(?m)^import app\\.db$"
in = "same-file"`, StatusFail, "src/api/users.py", 1},
		{"forbid passes when absent", `
when_changed = ["src/api/misc.py"]
[verifiers.require.forbid]
regex = "app\\.db"`, StatusPass, "", 0},
		{"file_exists pass", `
[verifiers.require.file_exists]
path = "LICENSE"`, StatusPass, "", 0},
		{"file_exists negated", `
[verifiers.require.file_exists]
path = "LICENSE"
exists = false`, StatusFail, "", 0},
		{"glob_count", `
[verifiers.require.glob_count]
files = "db/*.sql"
max = 1`, StatusFail, "", 0},
		{"paired requires_exists passes", `
when_changed = ["src/api/users.py"]
[verifiers.require.paired]
for_each = "src/api/{rel}.py"
requires_exists = "tests/test_{stem}.py"`, StatusPass, "", 0},
		{"paired requires_exists fails", `
when_changed = ["src/api/misc.py"]
[verifiers.require.paired]
for_each = "src/api/{rel}.py"
requires_exists = "tests/test_{stem}.py"`, StatusFail, "src/api/misc.py", 0},
		{"all fails when one child fails", `
when_changed = ["db/*.sql"]
[[verifiers.require.all]]
[verifiers.require.all.regex]
regex = "create"
[[verifiers.require.all]]
[verifiers.require.all.regex]
regex = "-- down"`, StatusFail, "db/2.sql", 0},
		{"any passes when one child holds", `
when_changed = ["db/2.sql"]
[[verifiers.require.any]]
[verifiers.require.any.regex]
regex = "nothing"
[[verifiers.require.any]]
[verifiers.require.any.regex]
regex = "create"`, StatusPass, "", 0},
		{"not inverts a holding predicate", `
[verifiers.require.not.file_exists]
path = "LICENSE"`, StatusFail, "", 0},
		{"not inverts a failing predicate", `
[verifiers.require.not.file_exists]
path = "NOPE"`, StatusPass, "", 0},
		{"not applicable when nothing matches", `
when_changed = ["*.rs"]
[verifiers.require.regex]
regex = "x"`, StatusNotApplicable, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := specProject(t, files, "[[verifiers]]\nid = \"v\"\nrule = \"database\"\n"+tt.spec+"\n")

			// Act
			got := runSpecs(t, cfg, Options{})["v"]

			// Assert
			assert.Equal(t, tt.want, got.Status, "%+v", got)
			if tt.wantFile != "" {
				require.NotEmpty(t, got.Findings, "%+v", got)
				assert.Equal(t, tt.wantFile, got.Findings[0].File)
				if tt.wantLine > 0 {
					assert.Equal(t, tt.wantLine, got.Findings[0].Line)
				}
			}
			if tt.want == StatusFail {
				assert.Equal(t, CodeVerifierFailed, got.Code)
				require.NotNil(t, got.Target)
				assert.Equal(t, "database", got.Target.ID)
				assert.Equal(t, ".ai-rulez/rules/database.md", got.Target.Path)
			}
		})
	}
}

func TestRun_TemplateTraversalIsRejected(t *testing.T) {
	cfg := specProject(t, map[string]string{"src/a.py": "x"}, `
[[verifiers]]
id = "v"
rule = "database"
when_changed = ["src/*.py"]
[verifiers.require.paired]
for_each = "src/{rel}.py"
requires_exists = "../../{stem}.txt"
`)

	got := runSpecs(t, cfg, Options{})["v"]

	assert.Equal(t, StatusError, got.Status)
	assert.Contains(t, got.Message, "outside the project")
}

func TestRun_StrictApplicabilityFlagsDeadScope(t *testing.T) {
	const spec = "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.rs\"]\n[verifiers.require.regex]\nregex = \"x\"\n"
	cfg := specProject(t, map[string]string{"a.go": "x"}, spec)

	off := runSpecs(t, cfg, Options{})["v"]
	on := runSpecs(t, cfg, Options{StrictApplicability: true})["v"]

	assert.Equal(t, StatusNotApplicable, off.Status)
	assert.Equal(t, StatusFail, on.Status)
	assert.Equal(t, CodeVerifierDeadScope, on.Code)
}

// gitProject makes a repository with a base commit on main and a topic branch,
// then calls change to modify it.
func gitProject(t *testing.T, files map[string]string, spec string, change func(root string)) *config.Config {
	t.Helper()
	cfg := specProject(t, files, spec)
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
		out, err := gitutil.CommandNoContext(cfg.BaseDir, full...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "topic")
	change(cfg.BaseDir)
	git("add", "-A")
	git("commit", "-q", "--allow-empty", "-m", "topic")
	return cfg
}

func TestRun_SinceScopesToChangedFilesAndResolvesAgainstFullTree(t *testing.T) {
	const spec = `
[[verifiers]]
id = "has-test"
rule = "api"
when_changed = ["src/**/*.py"]
[verifiers.require.paired]
for_each = "src/{rel}.py"
requires_exists = "tests/test_{stem}.py"

[[verifiers]]
id = "no-todo-added"
rule = "api"
when_changed = ["src/**/*.py"]
[verifiers.require.forbid]
regex = "TODO"
in = "diff-added"
`
	files := map[string]string{
		"src/legacy.py":       "# TODO old\nx = 1\n",
		"src/other.py":        "y = 1\n",
		"tests/test_other.py": "t",
	}
	cfg := gitProject(t, files, spec, func(root string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, "src/other.py"), []byte("y = 1\n# TODO new\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "src/legacy.py"), []byte("# TODO old\nx = 2\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "src/fresh.py"), []byte("z = 1\n"), 0o644))
	})

	got := runSpecs(t, cfg, Options{Since: "main"})

	// has-test: other.py has tests/test_other.py in the tree although the test did not change.
	assert.Equal(t, StatusFail, got["has-test"].Status)
	files3 := []string{}
	for _, f := range got["has-test"].Findings {
		files3 = append(files3, f.File)
	}
	assert.ElementsMatch(t, []string{"src/fresh.py", "src/legacy.py"}, files3)
	// no-todo-added: only the TODO on an added line fails; the old one on legacy.py does not.
	require.Equal(t, StatusFail, got["no-todo-added"].Status)
	require.Len(t, got["no-todo-added"].Findings, 1)
	assert.Equal(t, "src/other.py", got["no-todo-added"].Findings[0].File)
	assert.Equal(t, 2, got["no-todo-added"].Findings[0].Line)
}

func TestRun_SinceCountsUntrackedFilesAndStagedMode(t *testing.T) {
	const spec = "[[verifiers]]\nid = \"v\"\nrule = \"api\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.regex]\nregex = \"-- down\"\n"
	cfg := gitProject(t, map[string]string{"old.sql": "x"}, spec, func(root string) {})
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "untracked.sql"), []byte("x"), 0o644))

	since := runSpecs(t, cfg, Options{Since: "main"})["v"]
	staged := runSpecs(t, cfg, Options{Staged: true})["v"]

	require.Equal(t, StatusFail, since.Status)
	assert.Equal(t, "untracked.sql", since.Findings[0].File)
	assert.Equal(t, StatusNotApplicable, staged.Status, "nothing is staged")
}

func TestRun_MissingBaseIsAnErrorNotAPass(t *testing.T) {
	const spec = "[[verifiers]]\nid = \"v\"\nrule = \"api\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.regex]\nregex = \"x\"\n"
	cfg := gitProject(t, nil, spec, func(root string) {})

	rep := Run(context.Background(), cfg, Options{Since: "origin/missing"})

	require.Error(t, rep.Err)
	assert.Contains(t, rep.Err.Error(), "origin/missing")
	assert.Empty(t, rep.Results)
}

func TestRun_SinceOutsideARepositoryIsAnError(t *testing.T) {
	cfg := specProject(t, nil, "[[verifiers]]\nid = \"v\"\nrule = \"api\"\n[verifiers.require.file_exists]\npath = \"a\"\n")

	rep := Run(context.Background(), cfg, Options{Since: "main"})

	require.Error(t, rep.Err)
}

func TestRun_SinceAndStagedAreExclusive(t *testing.T) {
	rep := Run(context.Background(), specProject(t, nil, ""), Options{Since: "main", Staged: true})

	require.Error(t, rep.Err)
}

func TestRun_RuleFilterAndNames(t *testing.T) {
	const spec = `
[[verifiers]]
id = "a"
rule = "database"
[verifiers.require.file_exists]
path = "x"
[[verifiers]]
id = "b"
rule = "api"
[verifiers.require.file_exists]
path = "x"
`
	cfg := specProject(t, map[string]string{"x": "1"}, spec)

	byRule := runSpecs(t, cfg, Options{Rule: "api"})
	rep := Run(context.Background(), cfg, Options{Names: []string{"zzz"}})

	assert.Len(t, byRule, 1)
	assert.Contains(t, byRule, "b")
	require.Error(t, rep.Err)
}

func TestRun_HostileRegexFinishesInTime(t *testing.T) {
	cfg := specProject(t, map[string]string{"a.txt": strings.Repeat("a", 200000) + "!"}, `
[[verifiers]]
id = "v"
rule = "database"
when_changed = ["a.txt"]
[verifiers.require.regex]
regex = "(a+)+$"
`)

	got := runSpecs(t, cfg, Options{})["v"]

	assert.Equal(t, StatusFail, got.Status)
}

func TestRun_FindingExcerptMasksSecrets(t *testing.T) {
	cfg := specProject(t, map[string]string{"a.txt": "token = abcdefghijklmnop1234\n"}, `
[[verifiers]]
id = "v"
rule = "database"
when_changed = ["a.txt"]
[verifiers.require.forbid]
regex = "token = \\S+"
`)

	got := runSpecs(t, cfg, Options{})["v"]

	require.Len(t, got.Findings, 1)
	assert.NotContains(t, got.Findings[0].Match, "abcdefghijklmnop1234")
}

func TestRunExamples(t *testing.T) {
	cfg := specProject(t, nil, `
[[verifiers]]
id = "v"
rule = "database"
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "-- down"
[[verifiers.examples]]
name = "good"
files = { "db/1.sql" = "-- down\n" }
changed = ["db/1.sql"]
expect = "pass"
[[verifiers.examples]]
name = "bad"
files = { "db/2.sql" = "x\n" }
changed = ["db/2.sql"]
expect = "fail"
[[verifiers.examples]]
name = "wrong expectation"
files = { "db/3.sql" = "x\n" }
changed = ["db/3.sql"]
expect = "pass"
[[verifiers.examples]]
name = "out of scope"
files = { "other.txt" = "x\n" }
changed = ["other.txt"]
expect = "not_applicable"

[[verifiers]]
id = "untested"
rule = "api"
[verifiers.require.file_exists]
path = "a"
`)

	rep, err := RunExamples(context.Background(), cfg, nil)

	require.NoError(t, err)
	oks := map[string]bool{}
	for _, r := range rep.Results {
		oks[r.Example] = r.OK
	}
	assert.Equal(t, map[string]bool{"good": true, "bad": true, "wrong expectation": false, "out of scope": true}, oks)
	assert.Equal(t, []string{"untested"}, rep.Untested)
	assert.True(t, rep.Failed())
	_, err = RunExamples(context.Background(), cfg, []string{"nope"})
	require.Error(t, err)
}

func failingReport(t *testing.T) *Report {
	t.Helper()
	cfg := specProject(t, map[string]string{"db/1.sql": "x\n", "db/2.sql": "-- down\n"}, `
[[verifiers]]
id = "v"
rule = "database"
anchor = "## Migrations"
severity = "error"
message = "needs down"
fix = "add one"
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "-- down"
`)
	rep := Run(context.Background(), cfg, Options{})
	require.NoError(t, rep.Err)
	return rep
}

func TestWriteSARIF_MapsFailuresToTheDeclaringRule(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteSARIF(&buf, failingReport(t), "1.2.3"))

	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	assert.Equal(t, "2.1.0", doc["version"])
	run := doc["runs"].([]any)[0].(map[string]any)
	results := run["results"].([]any)
	require.Len(t, results, 1)
	r := results[0].(map[string]any)
	assert.Equal(t, "AR9H1/v", r["ruleId"])
	assert.Equal(t, "error", r["level"])
	props := r["properties"].(map[string]any)
	assert.Equal(t, "database", props["rule"])
	related := r["relatedLocations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	assert.Equal(t, ".ai-rulez/rules/database.md", related["artifactLocation"].(map[string]any)["uri"])
	assert.EqualValues(t, 3, related["region"].(map[string]any)["startLine"])
	assert.NotEmpty(t, r["partialFingerprints"].(map[string]any)[fingerprintKey])
	loc := r["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	assert.Equal(t, "db/1.sql", loc["artifactLocation"].(map[string]any)["uri"])
}

func TestWriteSARIF_FingerprintSurvivesLineShift(t *testing.T) {
	a := fingerprint("v", Finding{File: "a.go", Line: 3, Match: "TODO  x"})
	b := fingerprint("v", Finding{File: "a.go", Line: 40, Match: "TODO x"})
	c := fingerprint("v", Finding{File: "b.go", Line: 3, Match: "TODO x"})

	assert.Equal(t, a, b)
	assert.NotEqual(t, a, c)
}

func TestWriteJUnit_OneSuitePerRule(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJUnit(&buf, failingReport(t)))

	var doc junitSuites
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Suites, 1)
	assert.Equal(t, "rule:database", doc.Suites[0].Name)
	assert.Equal(t, 1, doc.Failed)
	require.Len(t, doc.Suites[0].Cases, 1)
	assert.Equal(t, "AR9H1", doc.Suites[0].Cases[0].Failure.Type)
}

func TestWriteText_ShowsRuleFindingsAndFix(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteText(&buf, failingReport(t)))

	out := buf.String()
	assert.Contains(t, out, `AR9H1 v (rule "database", .ai-rulez/rules/database.md:3)`)
	assert.Contains(t, out, "db/1.sql")
	assert.Contains(t, out, "fix: add one")
}

func TestExplainAndList(t *testing.T) {
	cfg := specProject(t, nil, "[[verifiers]]\nid = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\nfix = \"do it\"\n[verifiers.require.forbid]\nregex = \"DROP\"\n")
	var buf bytes.Buffer

	require.NoError(t, Explain(&buf, cfg, "v"))
	rows := List(cfg)

	assert.Contains(t, buf.String(), `enforces: rule "database"`)
	assert.Contains(t, buf.String(), "forbid `DROP`")
	assert.Contains(t, buf.String(), "fix: do it")
	require.Len(t, rows, 1)
	assert.Equal(t, "rule:database", rows[0].Target)
	require.Error(t, Explain(&buf, cfg, "nope"))
}

func TestReportFailedAt(t *testing.T) {
	rep := &Report{Results: []Result{{Status: StatusFail, Severity: "warning"}}}

	assert.False(t, rep.FailedAt("error"))
	assert.True(t, rep.FailedAt("warning"))
	assert.True(t, rep.FailedAt("info"))
	assert.False(t, rep.FailedAt("none"))
}

func TestRun_InlineSpecInConfigToml(t *testing.T) {
	// Arrange: a spec-form verifier and a flat one declared in config.toml, plus a file spec.
	files := map[string]string{"db/1.sql": "create\n", "db/2.sql": "create\n-- DROP\n"}
	cfg := specProject(t, files, "[[verifiers]]\nid = \"from-file\"\nrule = \"api\"\n[verifiers.require.file_exists]\npath = \"db/1.sql\"\n")
	cfg.Verifiers = []config.VerifierConfig{
		{Name: "flat", Type: "file_exists", Path: "db/1.sql"},
		{
			Name: "no-drop", Rule: "database", WhenChanged: []string{"db/*.sql"}, Fix: "remove DROP",
			Require: &vspec.Require{Forbid: &vspec.RegexPred{Regex: "DROP", In: "any-file", Files: "db/*.sql"}},
		},
		{Name: "bad", Rule: "ghost", Require: &vspec.Require{FileExists: &vspec.FileExistsPred{Path: "a"}}},
	}

	// Act
	rep := Run(context.Background(), cfg, Options{})
	got := map[string]Result{}
	for _, r := range rep.Results {
		got[r.Name] = r
	}
	rows := List(cfg)

	// Assert
	assert.Equal(t, StatusPass, got["flat"].Status)
	assert.Equal(t, StatusPass, got["from-file"].Status)
	assert.Equal(t, StatusFail, got["no-drop"].Status, "%+v", got["no-drop"])
	assert.Equal(t, "config.toml", got["no-drop"].Source)
	assert.Equal(t, StatusError, got["bad"].Status)
	assert.Equal(t, CodeVerifierInvalid, got["bad"].Code)
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"flat", "no-drop", "from-file", "bad"}, names)

	var buf bytes.Buffer
	require.NoError(t, Explain(&buf, cfg, "no-drop"))
	assert.Contains(t, buf.String(), "fix: remove DROP")
}
