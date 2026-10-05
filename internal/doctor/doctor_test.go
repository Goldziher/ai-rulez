package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const baseConfig = "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\ngitignore = false\n"

// project writes files under a fresh temp dir and returns it.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// loader loads the project in dir; remote includes are never fetched.
func loader(dir string) Loader {
	return func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
		return config.LoadConfig(ctx, dir, append(opts, config.WithoutRemote())...)
	}
}

func generate(t *testing.T, dir string) {
	t.Helper()
	cfg, err := loader(dir)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := generator.NewGenerator(cfg).Generate(""); err != nil {
		t.Fatalf("generate: %v", err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// run executes the whole doctor with every binary reported present, so the
// tools check stays quiet unless a test says otherwise.
func run(t *testing.T, dir string) *Report {
	t.Helper()
	return Run(context.Background(), Options{
		Load:     loader(dir),
		LookPath: func(name string) (string, error) { return "/bin/" + name, nil },
	})
}

func byCheck(r *Report, check string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

func TestRun_CleanProject(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/r.md": "# R\n\nbody\n"})
	generate(t, dir)

	report := run(t, dir)

	if len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", report.Findings)
	}
	if report.Failed(true) {
		t.Error("a clean report must not fail, even with strict")
	}
}

func TestCheckConfig(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "unparseable config", files: map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\npresets = [\n"}, want: "config"},
		{name: "invalid config", files: map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"t\"\npresets = []\n"}, want: "config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := project(t, tt.files)

			report := run(t, dir)

			got := byCheck(report, CheckConfig)
			if len(got) == 0 || got[0].Severity != SeverityError {
				t.Fatalf("config findings = %+v, want an error", got)
			}
			if !report.Failed(false) {
				t.Error("an invalid config must fail the run")
			}
		})
	}
}

func TestCheckPresets(t *testing.T) {
	tests := []struct {
		preset   string
		wantMsg  string
		wantHint string
	}{
		{preset: "windsurf", wantMsg: `"windsurf" was removed`, wantHint: `"devin"`},
		{preset: "continue-dev", wantMsg: `"continue-dev" was removed`, wantHint: "drop it"},
		{preset: "claud", wantMsg: `unknown preset "claud"`, wantHint: `did you mean "claude"?`},
		{preset: "cursr", wantMsg: `unknown preset "cursr"`, wantHint: `did you mean "cursor"?`},
		{preset: "zzzzzzzzzzzz", wantMsg: `unknown preset`, wantHint: "available presets:"},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"" + tt.preset + "\"]\n"
			dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})

			got := byCheck(run(t, dir), CheckPresets)

			if len(got) != 1 {
				t.Fatalf("preset findings = %+v, want one", got)
			}
			if got[0].Severity != SeverityError || !strings.Contains(got[0].Message, tt.wantMsg) || !strings.Contains(got[0].Hint, tt.wantHint) {
				t.Errorf("finding = %+v, want message %q and hint %q", got[0], tt.wantMsg, tt.wantHint)
			}
		})
	}
}

func TestCheckPresets_UnknownPresetIsNotReportedTwice(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"t\"\npresets = [\"windsurf\"]\n"})

	report := run(t, dir)

	if got := byCheck(report, CheckConfig); len(got) != 0 {
		t.Errorf("config findings = %+v, want the presets check to own this", got)
	}
}

func TestLevenshteinAndNearest(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"abc", "abc", 0}, {"abc", "", 3}, {"kitten", "sitting", 3}, {"claud", "claude", 1}} {
		if got := levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	names := []string{"claude", "cursor", "codex"}
	if got := nearest("claud", names); got != "claude" {
		t.Errorf("nearest = %q", got)
	}
	if got := nearest("something-else-entirely", names); got != "" {
		t.Errorf("nearest of a far name = %q, want none", got)
	}
}

func TestCheckDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   string // substring of the message; "" means no finding
	}{
		{name: "in sync", mutate: func(*testing.T, string) {}},
		{name: "deleted output", mutate: func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "CLAUDE.md")); err != nil {
				t.Fatal(err)
			}
		}, want: "missing"},
		{name: "source changed", mutate: func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", "r.md"), []byte("# R\n\nchanged\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, want: "stale"},
		{name: "hand edit", mutate: func(t *testing.T, dir string) {
			f, err := os.OpenFile(filepath.Join(dir, "CLAUDE.md"), os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if _, err := f.WriteString("hand edit\n"); err != nil {
				t.Fatal(err)
			}
		}, want: "edited"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/r.md": "# R\n\nbody\n"})
			generate(t, dir)
			tt.mutate(t, dir)

			got := byCheck(run(t, dir), CheckDrift)

			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) == 0 || got[0].Severity != SeverityWarning || !strings.Contains(got[0].Message, tt.want) {
				t.Fatalf("findings = %+v, want a warning containing %q", got, tt.want)
			}
		})
	}
}

func TestCheckGitignore(t *testing.T) {
	cfg := strings.Replace(baseConfig, "gitignore = false", "gitignore = true", 1)
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	gitInit(t, dir)

	// Before generate the managed block does not exist yet.
	before := byCheck(run(t, dir), CheckGitignore)
	if len(before) == 0 || before[0].Severity != SeverityWarning {
		t.Fatalf("before generate: findings = %+v, want warnings", before)
	}

	generate(t, dir)
	if after := byCheck(run(t, dir), CheckGitignore); len(after) != 0 {
		t.Fatalf("after generate: findings = %+v, want none", after)
	}
}

func TestCheckGitignore_OffAndOutsideRepo(t *testing.T) {
	// gitignore = false: committed outputs are meant to be tracked.
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitInit(t, dir)
	if got := byCheck(run(t, dir), CheckGitignore); len(got) != 0 {
		t.Errorf("gitignore off: findings = %+v, want none", got)
	}

	// Outside a repository git cannot answer, so nothing is reported.
	cfg := strings.Replace(baseConfig, "gitignore = false", "gitignore = true", 1)
	plain := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(plain))
	if got := byCheck(run(t, plain), CheckGitignore); len(got) != 0 {
		t.Errorf("not a repo: findings = %+v, want none", got)
	}
}

func TestDocumentFinding(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		wantBad bool
	}{
		{name: "valid json", file: "s.json", content: `{"a":1}`},
		{name: "jsonc with comments", file: "s.json", content: "{\n// c\n\"a\":1,\n}"},
		{name: "broken json", file: "s.json", content: `{"a":`, wantBad: true},
		{name: "valid toml", file: "c.toml", content: "a = 1\n"},
		{name: "broken toml", file: "c.toml", content: "a = [\n", wantBad: true},
		{name: "valid yaml", file: "c.yaml", content: "a: 1\n"},
		{name: "broken yaml", file: "c.yaml", content: "a: [1\nb: {\n", wantBad: true},
		{name: "unmergeable extension", file: "c.txt", content: "{{{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := project(t, map[string]string{tt.file: tt.content})

			f, bad := documentFinding(dir, filepath.Join(dir, tt.file))

			if bad != tt.wantBad {
				t.Fatalf("bad = %v (%+v), want %v", bad, f, tt.wantBad)
			}
			if bad && (f.Severity != SeverityError || f.Check != CheckDocuments || f.Path != tt.file) {
				t.Errorf("finding = %+v", f)
			}
		})
	}
	if _, bad := documentFinding(t.TempDir(), "/no/such/file.json"); bad {
		t.Error("a missing document is not a parse failure")
	}
}

func TestCheckDocuments_ReadsMergedDocumentsFromTheManifest(t *testing.T) {
	cfg := baseConfig + "\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"srv\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	generate(t, dir)
	cfgLoaded, err := loader(dir)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	merged := generator.NewGenerator(cfgLoaded).MergedDocumentPaths()
	if len(merged) == 0 {
		t.Skip("no merged document recorded for this preset set")
	}
	if got := byCheck(run(t, dir), CheckDocuments); len(got) != 0 {
		t.Fatalf("valid documents: findings = %+v", got)
	}
	var target string
	for _, p := range merged {
		if strings.HasSuffix(p, ".json") {
			target = p
		}
	}
	if target == "" {
		t.Skip("no merged JSON document")
	}
	if err := os.WriteFile(target, []byte(`{"broken":`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := byCheck(run(t, dir), CheckDocuments)

	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("findings = %+v, want one error", got)
	}
}

func TestCheckMCPEnv(t *testing.T) {
	cfg := baseConfig + `
[[mcp_servers]]
name = "srv"
command = "srv"
[mcp_servers.env]
TOKEN = "${DOCTOR_TEST_TOKEN}"
ROOT = "${PROJECT_ROOT}/x"
`
	tests := []struct {
		name    string
		env     map[string]string
		dotenv  string
		wantVar string
	}{
		{name: "unset", wantVar: "DOCTOR_TEST_TOKEN"},
		{name: "set in the environment", env: map[string]string{"DOCTOR_TEST_TOKEN": "x"}},
		{name: "set in .env", dotenv: "DOCTOR_TEST_TOKEN=x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".ai-rulez/config.toml": cfg}
			if tt.dotenv != "" {
				files[".env"] = tt.dotenv
			}
			dir := project(t, files)
			t.Setenv("DOCTOR_TEST_TOKEN", "")
			_ = os.Unsetenv("DOCTOR_TEST_TOKEN")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := byCheck(run(t, dir), CheckMCPEnv)

			if tt.wantVar == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Severity != SeverityWarning || !strings.Contains(got[0].Message, "${"+tt.wantVar+"}") {
				t.Fatalf("findings = %+v, want one warning naming %s", got, tt.wantVar)
			}
			// ${PROJECT_ROOT} always resolves, so it is never reported.
			if strings.Contains(got[0].Message, "PROJECT_ROOT") {
				t.Errorf("PROJECT_ROOT must not be reported: %+v", got[0])
			}
		})
	}
}

func TestCheckHooks(t *testing.T) {
	cfg := baseConfig + `
[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
script = "tools/missing.sh"

[[hooks]]
event = "PreToolUse"
[[hooks.hooks]]
script = "tools/plain.sh"
[[hooks.hooks]]
script = "tools/ready.sh"
`
	dir := project(t, map[string]string{
		".ai-rulez/config.toml": cfg,
		"tools/plain.sh":        "#!/bin/sh\n",
		"tools/ready.sh":        "#!/bin/sh\n",
	})
	if err := os.Chmod(filepath.Join(dir, "tools", "ready.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := byCheck(run(t, dir), CheckHooks)

	var missing, notExec bool
	for _, f := range got {
		missing = missing || strings.Contains(f.Message, "tools/missing.sh") && strings.Contains(f.Message, "does not exist")
		notExec = notExec || strings.Contains(f.Message, "tools/plain.sh") && strings.Contains(f.Message, "not executable")
		if strings.Contains(f.Message, "ready.sh") {
			t.Errorf("an executable script must not be reported: %+v", f)
		}
	}
	if len(got) != 2 || !missing || !notExec {
		t.Fatalf("findings = %+v, want the missing and the non-executable script", got)
	}
}

func TestCheckLock(t *testing.T) {
	cfg := baseConfig + "\n[[includes]]\nname = \"shared\"\nsource = \"https://github.com/acme/rules.git\"\nref = \"main\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})

	got := byCheck(run(t, dir), CheckLock)

	if len(got) != 1 || got[0].Severity != SeverityWarning || !strings.Contains(got[0].Hint, "ai-rulez lock") {
		t.Fatalf("findings = %+v, want one lock warning", got)
	}
	if clean := byCheck(run(t, project(t, map[string]string{".ai-rulez/config.toml": baseConfig})), CheckLock); len(clean) != 0 {
		t.Errorf("no remote sources: findings = %+v, want none", clean)
	}
}

func TestCheckTools(t *testing.T) {
	cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\", \"codex\", \"mcp\", \"claude\"]\ngitignore = false\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	have := map[string]bool{"claude": true}

	report := Run(context.Background(), Options{
		Load: loader(dir),
		LookPath: func(name string) (string, error) {
			if have[name] {
				return "/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	})

	got := byCheck(report, CheckTools)
	if len(got) != 1 || got[0].Severity != SeverityInfo || !strings.Contains(got[0].Message, `"codex"`) {
		t.Fatalf("findings = %+v, want one info for codex only (claude present, mcp unmapped)", got)
	}
	if c := (&Report{Findings: got}).Counts(); c[SeverityError]+c[SeverityWarning] != 0 {
		t.Error("a missing tool binary must never be more than info")
	}
}

func TestReport_FailedAndCounts(t *testing.T) {
	tests := []struct {
		name       string
		findings   []Finding
		strict     bool
		wantFailed bool
	}{
		{name: "empty", wantFailed: false},
		{name: "info only", findings: []Finding{{Severity: SeverityInfo}}, strict: true, wantFailed: false},
		{name: "warning", findings: []Finding{{Severity: SeverityWarning}}, wantFailed: false},
		{name: "warning strict", findings: []Finding{{Severity: SeverityWarning}}, strict: true, wantFailed: true},
		{name: "error", findings: []Finding{{Severity: SeverityError}}, wantFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Findings: tt.findings}
			if got := r.Failed(tt.strict); got != tt.wantFailed {
				t.Errorf("Failed(%v) = %v, want %v", tt.strict, got, tt.wantFailed)
			}
		})
	}
}

func TestRun_OrdersBySeverity(t *testing.T) {
	cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"windsurf\"]\n\n[[mcp_servers]]\nname = \"s\"\ncommand = \"s\"\n[mcp_servers.env]\nK = \"${DOCTOR_ORDER_VAR}\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})

	report := Run(context.Background(), Options{Load: loader(dir), LookPath: func(string) (string, error) { return "", errors.New("none") }})

	rank := -1
	for _, f := range report.Findings {
		if r := f.Severity.rank(); r < rank {
			t.Fatalf("findings not sorted by severity: %+v", report.Findings)
		} else {
			rank = r
		}
	}
}

func TestWriteJSONAndText(t *testing.T) {
	r := &Report{Root: "/p", Findings: []Finding{
		{Check: CheckPresets, Severity: SeverityError, Message: "boom", Hint: "fix it"},
		{Check: CheckDrift, Severity: SeverityWarning, Message: "stale", Path: "CLAUDE.md"},
	}}

	var js bytes.Buffer
	if err := WriteJSON(&js, r); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Summary  map[string]int `json:"summary"`
		Findings []Finding      `json:"findings"`
	}
	if err := json.Unmarshal(js.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, js.String())
	}
	if decoded.Summary["error"] != 1 || decoded.Summary["warning"] != 1 || decoded.Summary["info"] != 0 || len(decoded.Findings) != 2 {
		t.Errorf("decoded = %+v", decoded)
	}

	var txt bytes.Buffer
	if err := WriteText(&txt, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SEVERITY", "boom", "fix: fix it", "CLAUDE.md: stale", "1 error, 1 warning, 0 info"} {
		if !strings.Contains(txt.String(), want) {
			t.Errorf("text output missing %q:\n%s", want, txt.String())
		}
	}

	var empty bytes.Buffer
	if err := WriteText(&empty, &Report{Findings: []Finding{}}); err != nil || !strings.Contains(empty.String(), "No problems found.") {
		t.Errorf("empty report text = %q, err %v", empty.String(), err)
	}
}

func TestCheckHooks_IgnoresLintOverridesAndGit(t *testing.T) {
	// Arrange: the lint rules are switched off and the project is no git repo, so
	// only a direct look at the script paths can find the problem.
	cfg := baseConfig + `
[lint.severity]
AR504 = "off"
AR505 = "off"

[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
script = "tools/missing.sh"
[[hooks.hooks]]
script = "tools/plain.sh"
`
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg, "tools/plain.sh": "#!/bin/sh\n"})

	// Act
	got := byCheck(run(t, dir), CheckHooks)

	// Assert
	var missing, notExec bool
	for _, f := range got {
		missing = missing || strings.Contains(f.Message, "tools/missing.sh") && strings.Contains(f.Message, "does not exist")
		notExec = notExec || strings.Contains(f.Message, "tools/plain.sh") && strings.Contains(f.Message, "not executable")
	}
	if !missing || !notExec {
		t.Fatalf("findings = %+v, want the missing and the non-executable script", got)
	}
}

func TestRun_LoadsConfigWithoutRemote(t *testing.T) {
	// Arrange: a declared include that cannot be resolved; loading it would fail
	// (and, for a remote one, hit the network and write the cache).
	cfg := baseConfig + "\n[[includes]]\nname = \"gone\"\nsource = \"./does-not-exist\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	var tried [][]config.LoadOption
	load := func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
		tried = append(tried, opts)
		return config.LoadConfig(ctx, dir, opts...)
	}

	// Act
	report := Run(context.Background(), Options{Load: load, LookPath: func(n string) (string, error) { return n, nil }})

	// Assert
	if report.Unloadable {
		t.Fatalf("the config must load without resolving includes: %+v", report.Findings)
	}
	if got := byCheck(report, CheckConfig); len(got) != 0 {
		t.Fatalf("config findings = %+v", got)
	}
	if len(tried) == 0 {
		t.Fatal("Load was never called")
	}
	drift := byCheck(report, CheckDrift)
	if len(drift) != 1 || drift[0].Severity != SeverityInfo || !strings.Contains(drift[0].Message, "not checked") {
		t.Fatalf("drift findings = %+v, want one info that the check was skipped", drift)
	}
}

func TestRun_UnloadableConfigIsMarked(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\npresets = [\n"})

	report := run(t, dir)

	if !report.Unloadable {
		t.Fatal("a config that does not load must mark the report Unloadable")
	}
	if ok := run(t, project(t, map[string]string{".ai-rulez/config.toml": baseConfig})); ok.Unloadable {
		t.Error("a loadable config must not")
	}
}

func TestCheckMCPEnv_OnlyServersActiveInTheProfile(t *testing.T) {
	cfg := baseConfig + `
[profiles]
backend = []
frontend = []

[[mcp_servers]]
name = "api"
command = "api"
profiles = ["backend"]
[mcp_servers.env]
TOKEN = "${DOCTOR_PROFILE_TOKEN}"
`
	dir := project(t, map[string]string{".ai-rulez/config.toml": cfg})
	_ = os.Unsetenv("DOCTOR_PROFILE_TOKEN")
	for profile, want := range map[string]int{"frontend": 0, "backend": 1} {
		report := Run(context.Background(), Options{
			Load: loader(dir), Profile: profile,
			LookPath: func(n string) (string, error) { return n, nil },
		})
		if got := byCheck(report, CheckMCPEnv); len(got) != want {
			t.Errorf("profile %s: findings = %+v, want %d", profile, got, want)
		}
	}
}
