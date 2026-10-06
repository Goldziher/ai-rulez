package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const verifiersBase = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"

func resetVerifiersFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		verifiersStrict, verifiersJSON, verifiersNames, verifiersProfile, noLocal, configDir = false, false, nil, "", false, ""
		verifiersSince, verifiersStaged, verifiersAll, verifiersRule, verifiersFormat = "", false, false, "", ""
		verifiersFailOn, verifiersOut, verifiersDead, verifiersListJSON = "", "", false, false
	}
	reset()
	t.Cleanup(reset)
}

func TestRunVerifiers_ExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		config string
		strict bool
		json   bool
		names  []string
		want   int
	}{
		{"none configured", verifiersBase, false, false, nil, 0},
		{"all pass", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \".ai-rulez/config.toml\"\n", false, false, nil, 0},
		{"error failure", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\n", false, false, nil, exitVerifiersFindings},
		{"warning passes", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", false, true, nil, 0},
		{"warning fails with strict", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", true, false, nil, exitVerifiersFindings},
		{"unknown name cannot run", verifiersBase, false, false, []string{"nope"}, exitVerifiersCannotRun},
		{"invalid config cannot run", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"command\"\n", false, false, nil, exitVerifiersCannotRun},
		{"unloadable config cannot run", brokenRootConfig, false, true, nil, exitVerifiersCannotRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVerifiersFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), tt.config)
			chdir(t, root)
			verifiersStrict, verifiersJSON, verifiersNames = tt.strict, tt.json, tt.names
			var out bytes.Buffer

			got := runVerifiers(context.Background(), nil, &out)

			if got != tt.want {
				t.Errorf("exit code = %d, want %d\n%s", got, tt.want, out.String())
			}
			if tt.json && got != exitVerifiersCannotRun {
				var decoded map[string]any
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
					t.Errorf("--json output is not valid JSON: %v\n%s", err, out.String())
				}
			}
		})
	}
}

func TestListVerifiers(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+"[[verifiers]]\nname = \"readme\"\ndescription = \"needs a README\"\ntype = \"file_exists\"\npath = \"README.md\"\n")
	chdir(t, root)
	var out bytes.Buffer

	got := listVerifiers(context.Background(), nil, &out)

	if got != 0 {
		t.Fatalf("exit code = %d\n%s", got, out.String())
	}
	for _, want := range []string{"readme", "file_exists", "needs a README"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunVerifiers_FailureOutranksCannotRun(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+
		"[[verifiers]]\nname = \"a-missing\"\ntype = \"file_exists\"\npath = \"MISSING\"\n"+
		"[[verifiers]]\nname = \"b-binary\"\ntype = \"forbid\"\nglob = \"bin.dat\"\npattern = \"x\"\n")
	writeFile(t, filepath.Join(root, "bin.dat"), "a\x00b")
	chdir(t, root)
	var out bytes.Buffer

	got := runVerifiers(context.Background(), nil, &out)

	if got != exitVerifiersFindings {
		t.Errorf("exit code = %d, want %d (a failure must not be hidden by an error)\n%s", got, exitVerifiersFindings, out.String())
	}
	if !strings.Contains(out.String(), "1 failed, 1 could not run") {
		t.Errorf("report should show both outcomes:\n%s", out.String())
	}
}

func TestVerifiersListAndValidate_SurfaceConfigErrors(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+
		"[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"x\"\npattern = \"nope\"\n")
	chdir(t, root)
	var out bytes.Buffer

	if got := listVerifiers(context.Background(), nil, &out); got != exitVerifiersCannotRun {
		t.Errorf("list exit code = %d, want %d for a verifier with an inapplicable field", got, exitVerifiersCannotRun)
	}
}

// specProjectRoot writes a project with the rule "database" and a spec verifier
// that requires a "-- down" section in db/*.sql, with one failing migration.
func specProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "database.md"), "# Database\n\nNeeds down.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "verifiers", "db.toml"), `[[verifiers]]
id = "has-down"
rule = "database"
severity = "warning"
fix = "add a down section"
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
files = { "db/1.sql" = "x\n" }
changed = ["db/1.sql"]
expect = "fail"
`)
	writeFile(t, filepath.Join(root, "db", "1.sql"), "create\n")
	return root
}

func TestRunVerifiers_SpecFormatsAndFailOn(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		failOn   string
		strict   bool
		want     int
		contains string
	}{
		{"text names the rule", "", "", false, 0, `AR9H1 has-down (rule "database"`},
		{"warning fails with strict", "", "", true, exitVerifiersFindings, "fix: add a down section"},
		{"fail-on warning", "json", "warning", false, exitVerifiersFindings, `"code": "AR9H1"`},
		{"fail-on none", "", "none", true, 0, "has-down"},
		{"sarif", "sarif", "", false, 0, `"ruleId": "AR9H1/has-down"`},
		{"junit", "junit", "", false, 0, `name="rule:database"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVerifiersFlags(t)
			chdir(t, specProjectRoot(t))
			verifiersFormat, verifiersFailOn, verifiersStrict = tt.format, tt.failOn, tt.strict
			var out bytes.Buffer

			got := runVerifiers(context.Background(), nil, &out)

			if got != tt.want {
				t.Errorf("exit code = %d, want %d\n%s", got, tt.want, out.String())
			}
			if !strings.Contains(out.String(), tt.contains) {
				t.Errorf("output missing %q:\n%s", tt.contains, out.String())
			}
		})
	}
}

func TestRunVerifiers_RejectsBadFlags(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
	}{
		{"since and staged", func() { verifiersSince, verifiersStaged = "main", true }},
		{"unknown format", func() { verifiersFormat = "xml" }},
		{"unknown fail-on", func() { verifiersFailOn = "fatal" }},
		{"option-like since", func() { verifiersSince = "--output=x" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVerifiersFlags(t)
			chdir(t, specProjectRoot(t))
			tt.setup()

			got := runVerifiers(context.Background(), nil, &bytes.Buffer{})

			if got != exitVerifiersCannotRun {
				t.Errorf("exit code = %d, want %d", got, exitVerifiersCannotRun)
			}
		})
	}
}

func TestRunVerifiers_SinceMissingBaseExitsOne(t *testing.T) {
	resetVerifiersFlags(t)
	root := specProjectRoot(t)
	chdir(t, root)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@e.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "x"}} {
		if out, err := gitutil.CommandNoContext(root, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	verifiersSince = "no-such-base"
	var out bytes.Buffer

	got := runVerifiers(context.Background(), nil, &out)

	if got != exitVerifiersCannotRun {
		t.Errorf("exit code = %d, want %d (a missing base must not pass)\n%s", got, exitVerifiersCannotRun, out.String())
	}
}

func TestRunVerifiers_OutWritesTheReportToAFile(t *testing.T) {
	resetVerifiersFlags(t)
	chdir(t, specProjectRoot(t))
	target := filepath.Join(t.TempDir(), "report.sarif")
	verifiersFormat, verifiersOut = "sarif", target
	var out bytes.Buffer

	got := runVerifiers(context.Background(), nil, &out)

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 || out.Len() != 0 || !strings.Contains(string(data), `"version": "2.1.0"`) {
		t.Errorf("exit %d, stdout %q, file %q", got, out.String(), data)
	}
}

func TestTestAndExplainVerifiers(t *testing.T) {
	resetVerifiersFlags(t)
	chdir(t, specProjectRoot(t))
	var testOut, explainOut bytes.Buffer

	gotTest := testVerifiers(context.Background(), nil, &testOut)
	gotExplain := explainVerifier(context.Background(), "has-down", nil, &explainOut)
	gotUnknown := explainVerifier(context.Background(), "nope", nil, &bytes.Buffer{})

	if gotTest != 0 || !strings.Contains(testOut.String(), "2 of 2 examples passed") {
		t.Errorf("test exit %d:\n%s", gotTest, testOut.String())
	}
	if gotExplain != 0 || !strings.Contains(explainOut.String(), `enforces: rule "database"`) {
		t.Errorf("explain exit %d:\n%s", gotExplain, explainOut.String())
	}
	if gotUnknown != exitVerifiersCannotRun {
		t.Errorf("unknown explain exit = %d", gotUnknown)
	}
}

func TestTestVerifiers_FailingExampleExitsTwo(t *testing.T) {
	resetVerifiersFlags(t)
	root := specProjectRoot(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "verifiers", "db.toml"), `[[verifiers]]
id = "has-down"
rule = "database"
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "-- down"
[[verifiers.examples]]
name = "wrong"
files = { "db/1.sql" = "x\n" }
changed = ["db/1.sql"]
expect = "pass"
`)
	chdir(t, root)
	var out bytes.Buffer

	got := testVerifiers(context.Background(), nil, &out)

	if got != exitVerifiersFindings || !strings.Contains(out.String(), "FAIL has-down: wrong") {
		t.Errorf("exit %d:\n%s", got, out.String())
	}
}

func TestListVerifiers_ShowsSpecsAndJSON(t *testing.T) {
	resetVerifiersFlags(t)
	chdir(t, specProjectRoot(t))
	var text, js bytes.Buffer

	listVerifiers(context.Background(), nil, &text)
	verifiersListJSON = true
	listVerifiers(context.Background(), nil, &js)

	if !strings.Contains(text.String(), "rule:database") {
		t.Errorf("list should show the enforced rule:\n%s", text.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(js.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["target"] != "rule:database" {
		t.Errorf("list --json = %s (%v)", js.String(), err)
	}
}

func TestRunVerifiers_JSONFollowsTheSchema(t *testing.T) {
	resetVerifiersFlags(t)
	chdir(t, specProjectRoot(t))
	verifiersFormat = "json"
	var out bytes.Buffer

	runVerifiers(context.Background(), nil, &out)

	validateAgainst(t, "../../schema/verifiers-report.schema.json", out.Bytes())
}

func TestVerifierRunOptionsJSONFlagConflict(t *testing.T) {
	tests := []struct {
		name       string
		json       bool
		format     string
		wantFormat string
		wantErr    bool
	}{
		{"json alone", true, "", "json", false},
		{"json with json", true, "json", "json", false},
		{"json with text", true, "text", "", true},
		{"json with sarif", true, "sarif", "", true},
		{"format alone", false, "junit", "junit", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldJ, oldF := verifiersJSON, verifiersFormat
			t.Cleanup(func() { verifiersJSON, verifiersFormat = oldJ, oldF })
			verifiersJSON, verifiersFormat = tt.json, tt.format

			_, format, _, err := verifierRunOptions()

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if format != tt.wantFormat {
				t.Fatalf("format = %q, want %q", format, tt.wantFormat)
			}
		})
	}
}
