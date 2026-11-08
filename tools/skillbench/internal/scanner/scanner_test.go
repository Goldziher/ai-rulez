package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fake writes an executable that stands in for a scanner.
func fake(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	return path
}

func scan(t *testing.T, name, script string) Verdict {
	t.Helper()
	tool, err := New(name, fake(t, script), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Scan(t.Context(), t.TempDir(), t.TempDir())
}

const outArg = `while [ $# -gt 0 ]; do [ "$1" = "--output" ] && out="$2"; shift; done; `

func TestAIRulezVerdict(t *testing.T) {
	v := scan(t, "ai-rulez", `echo 'no ai-rulez config found; linting detected files' >&2
echo '{"findings":[{"code":"AR014","severity":"error"},{"code":"AR007","severity":"warning"},{"code":"AR014","severity":"error"}]}'
exit 2`)
	if !v.Flagged || v.MaxSeverity != "high" || v.Findings != 3 || strings.Join(v.Rules, ",") != "AR007,AR014" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestAIRulezCleanAndConfigFailure(t *testing.T) {
	if v := scan(t, "ai-rulez", `echo '{"findings":[]}'`); v.Flagged || v.Error != "" {
		t.Fatalf("clean = %+v", v)
	}
	if v := scan(t, "ai-rulez", `exit 1`); v.Error != "exit" {
		t.Fatalf("exit 1 is a failure, not a finding: %+v", v)
	}
}

// TestAIRulezRowsDifferOnlyInGate checks the two rows pass their own gate to the same binary.
func TestAIRulezRowsDifferOnlyInGate(t *testing.T) {
	const body = `case "$*" in *"--fail-on threat"*) echo '{"findings":[]}' ;; *"--fail-on warning"*) echo '{"findings":[{"code":"AR007","severity":"warning"}]}'; exit 2 ;; *) exit 9 ;; esac`
	if v := scan(t, "ai-rulez-threat", body); v.Flagged || v.Error != "" {
		t.Fatalf("threat row = %+v", v)
	}
	if v := scan(t, "ai-rulez", body); !v.Flagged {
		t.Fatalf("warning row = %+v", v)
	}
}

func TestThreatRowNeedsTheGate(t *testing.T) {
	ctx := t.Context()
	newer, err := New("ai-rulez-threat", fake(t, `echo 'What exits 2: a severity (error, the default, warning, info), threat, or none'`), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	older, err := New("ai-rulez-threat", fake(t, `echo 'What exits 2: a severity (error, the default, warning, info) or none'`), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	warning, err := New("ai-rulez", fake(t, `exit 1`), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !newer.Supported(ctx) || older.Supported(ctx) || !warning.Supported(ctx) {
		t.Fatal("only a release whose scan help names threat has a threat row; other rows need no probe")
	}
}

func TestCiscoUsesIsSafe(t *testing.T) {
	v := scan(t, "skill-scanner", outArg+`echo '{"is_safe":false,"findings":[{"rule_id":"X","severity":"HIGH"}]}' > "$out"; exit 1`)
	if !v.Flagged || v.MaxSeverity != "high" {
		t.Fatalf("verdict = %+v", v)
	}
	v = scan(t, "skill-scanner", outArg+`echo '{"is_safe":true,"findings":[{"rule_id":"X","severity":"LOW"}]}' > "$out"`)
	if v.Flagged || v.MaxSeverity != "low" {
		t.Fatalf("the tool's verdict wins over its findings: %+v", v)
	}
}

func TestSkillSpectorUsesRecommendation(t *testing.T) {
	v := scan(t, "skillspector", outArg+`echo '{"risk_assessment":{"recommendation":"CAUTION"},"issues":[{"pattern":"P1","severity":"MEDIUM"}]}' > "$out"`)
	if !v.Flagged || strings.Join(v.Rules, ",") != "P1" {
		t.Fatalf("verdict = %+v", v)
	}
	v = scan(t, "skillspector", outArg+`echo '{"risk_assessment":{"recommendation":"SAFE"},"issues":[]}' > "$out"`)
	if v.Flagged {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestMissingReportIsAnError(t *testing.T) {
	if v := scan(t, "skill-scanner", `exit 0`); v.Error != "no-report" {
		t.Fatalf("verdict = %+v", v)
	}
	if v := scan(t, "skillspector", outArg+`echo 'not json' > "$out"`); v.Error != "unparseable" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestTimeoutIsAnError(t *testing.T) {
	tool, err := New("ai-rulez", fake(t, `exec sleep 5`), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if v := tool.Scan(t.Context(), t.TempDir(), t.TempDir()); v.Error != "timeout" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestEnvironmentIsClean(t *testing.T) {
	t.Setenv("AI_RULEZ_CONFIG", "leak")
	t.Setenv("ANTHROPIC_API_KEY", "leak")
	v := scan(t, "ai-rulez", `env | grep -q leak && exit 7; echo '{"findings":[]}'`)
	if v.Error != "" {
		t.Fatalf("the environment leaked: %+v", v)
	}
}

func TestUnknownTool(t *testing.T) {
	if _, err := New("nope", "", time.Second); err == nil {
		t.Fatal("unknown tool must fail")
	}
}

func TestVersion(t *testing.T) {
	tool, err := New("ai-rulez", fake(t, `echo 'ai-rulez version 5.11.1'`), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := tool.Version(t.Context()); err != nil || v != "5.11.1" {
		t.Fatalf("version = %q, %v", v, err)
	}
}

// TestCommittedCacheMatchesAdapters fails when an edit changes how a competitor is invoked or parsed,
// so the committed verdicts would all miss. Rerun the benchmark for that tool and commit its new
// cache; an edit to registry.go never gets here.
func TestCommittedCacheMatchesAdapters(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "bench", "skills", "cache", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{toolCisco, toolSkillSpector} {
		tool, err := New(name, "", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		suffix := "-" + tool.AdapterDigest() + ".jsonl"
		found := false
		for _, f := range files {
			found = found || strings.HasPrefix(filepath.Base(f), name+"-") && strings.HasSuffix(f, suffix)
		}
		if !found {
			t.Errorf("%s: no committed cache for adapter %s", name, tool.AdapterDigest())
		}
	}
}
