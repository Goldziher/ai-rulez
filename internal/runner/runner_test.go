package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not portable to windows")
	}
	p := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		timeout    time.Duration
		maxOut     int64
		wantStatus Status
		wantCode   int
		wantOut    string
		wantTrunc  bool
	}{
		{name: "success", body: "printf hi", wantStatus: StatusOK, wantOut: "hi"},
		{name: "non-zero exit keeps output", body: "printf partial; exit 3", wantStatus: StatusExit, wantCode: 3, wantOut: "partial"},
		{name: "timeout kills a sleeper", body: "sleep 30", timeout: 200 * time.Millisecond, wantStatus: StatusTimeout, wantCode: -1},
		{name: "output cap truncates without blocking", body: "yes x | head -c 100000", maxOut: 10, wantStatus: StatusOK, wantOut: "x\nx\nx\nx\nx\n", wantTrunc: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			spec := Spec{Argv: []string{script(t, tt.body)}, Env: ScrubEnv(os.Environ(), nil, nil), Timeout: tt.timeout, MaxOutput: tt.maxOut}
			// Act
			res := Run(context.Background(), spec)
			// Assert
			if res.Status != tt.wantStatus {
				t.Fatalf("status = %s (%v), want %s", res.Status, res.Err, tt.wantStatus)
			}
			if tt.wantStatus != StatusTimeout && res.ExitCode != tt.wantCode {
				t.Errorf("exit = %d, want %d", res.ExitCode, tt.wantCode)
			}
			if tt.wantOut != "" && string(res.Stdout) != tt.wantOut {
				t.Errorf("stdout = %q, want %q", res.Stdout, tt.wantOut)
			}
			if res.StdoutTruncated != tt.wantTrunc {
				t.Errorf("truncated = %v, want %v", res.StdoutTruncated, tt.wantTrunc)
			}
		})
	}
}

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix only")
	}
	// The child spawns a grandchild that holds stdout open and would outlive it.
	marker := filepath.Join(t.TempDir(), "alive")
	body := "(sleep 2; echo late > " + marker + ") &\nwait\n"
	start := time.Now()
	res := Run(context.Background(), Spec{Argv: []string{script(t, body)}, Env: ScrubEnv(os.Environ(), nil, nil), Timeout: 200 * time.Millisecond})
	if res.Status != StatusTimeout {
		t.Fatalf("status = %s, want timeout", res.Status)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Errorf("Run took %s; the group was not killed", time.Since(start))
	}
	time.Sleep(2300 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("grandchild survived the timeout")
	}
}

func TestRunUnavailable(t *testing.T) {
	tests := []struct{ name, argv0 string }{
		{"not on PATH", "ai-rulez-no-such-binary-xyz"},
		{"missing path", filepath.Join(t.TempDir(), "nope")},
		{"directory", t.TempDir()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Run(context.Background(), Spec{Argv: []string{tt.argv0}})
			if res.Status != StatusUnavailable || res.Err == nil {
				t.Fatalf("status = %s err = %v, want unavailable", res.Status, res.Err)
			}
		})
	}
	if res := Run(context.Background(), Spec{}); res.Status != StatusError {
		t.Errorf("empty argv status = %s, want error", res.Status)
	}
}

func TestRunRelativeCommandResolvesAgainstDir(t *testing.T) {
	p := script(t, "printf rel")
	res := Run(context.Background(), Spec{Argv: []string{"./" + filepath.Base(p)}, Dir: filepath.Dir(p), Env: ScrubEnv(os.Environ(), nil, nil)})
	if res.Status != StatusOK || string(res.Stdout) != "rel" {
		t.Fatalf("got %s %q (%v)", res.Status, res.Stdout, res.Err)
	}
}

func TestScrubEnv(t *testing.T) {
	parent := []string{
		"PATH=/bin", "HOME=/h", "LC_ALL=C", "ANTHROPIC_API_KEY=sk", "HTTPS_PROXY=http://p", "GITHUB_TOKEN=t",
		"CUSTOM=1", "KEEP_ME=yes", "BAD",
	}
	got := ScrubEnv(parent, []string{"KEEP_ME"}, []string{"TERM=xterm", "HOME=/scratch"})
	want := []string{"HOME=/scratch", "KEEP_ME=yes", "LC_ALL=C", "NO_COLOR=1", "PATH=/bin", "TERM=xterm"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("env = %v, want %v", got, want)
	}
	if again := ScrubEnv(parent, []string{"KEEP_ME"}, []string{"TERM=xterm", "HOME=/scratch"}); strings.Join(again, ",") != strings.Join(got, ",") {
		t.Error("ScrubEnv is not deterministic")
	}
}

func TestChildSeesOnlyScrubbedEnv(t *testing.T) {
	t.Setenv("AR_PLANTED_SECRET_TOKEN", "planted")
	res := Run(context.Background(), Spec{Argv: []string{script(t, "env")}, Env: ScrubEnv(os.Environ(), nil, nil)})
	if res.Status != StatusOK {
		t.Fatalf("status %s: %v", res.Status, res.Err)
	}
	if strings.Contains(string(res.Stdout), "AR_PLANTED") {
		t.Errorf("planted variable leaked:\n%s", res.Stdout)
	}
}

func TestSensitive(t *testing.T) {
	tests := map[string]bool{
		"HTTP_PROXY": true, "https_proxy": true, "ANTHROPIC_API_KEY": true, "OPENAI_API_KEY": true, "GITHUB_TOKEN": true,
		"AWS_ACCESS_KEY_ID": true, "DB_PASSWORD": true, "SSH_AUTH_SOCK": true, "MY_SECRET": true,
		"PATH": false, "LANG": false, "SNYK_REGION": false, "KEYBOARD": false, "TOKENIZERS_PARALLELISM": false,
	}
	for name, want := range tests {
		if got := Sensitive(name); got != want {
			t.Errorf("Sensitive(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestEffectiveTimeout(t *testing.T) {
	tests := []struct{ in, want time.Duration }{{0, DefaultTimeout}, {-1, DefaultTimeout}, {time.Second, time.Second}, {time.Hour, MaxTimeout}}
	for _, tt := range tests {
		if got := EffectiveTimeout(tt.in); got != tt.want {
			t.Errorf("EffectiveTimeout(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestRunExitZeroWhileGrandchildHoldsStdout(t *testing.T) {
	// Arrange: the child exits 0 but a backgrounded grandchild keeps stdout open.
	old := killGrace
	killGrace = 300 * time.Millisecond
	t.Cleanup(func() { killGrace = old })
	marker := filepath.Join(t.TempDir(), "alive")
	body := "(sleep 1.5; echo late > " + marker + ") &\nprintf hi\nexit 0\n"
	// Act
	res := Run(context.Background(), Spec{Argv: []string{script(t, body)}, Env: ScrubEnv(os.Environ(), nil, nil)})
	// Assert
	if res.Status != StatusOK || res.ExitCode != 0 || string(res.Stdout) != "hi" {
		t.Fatalf("got %s code %d out %q (%v), want ok/0/hi", res.Status, res.ExitCode, res.Stdout, res.Err)
	}
	time.Sleep(1800 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("daemonised grandchild survived Run")
	}
}

func TestRunNonZeroWhileGrandchildHoldsStdout(t *testing.T) {
	old := killGrace
	killGrace = 300 * time.Millisecond
	t.Cleanup(func() { killGrace = old })
	res := Run(context.Background(), Spec{Argv: []string{script(t, "(sleep 5) &\nprintf partial\nexit 4\n")}, Env: ScrubEnv(os.Environ(), nil, nil)})
	if res.Status != StatusExit || res.ExitCode != 4 || string(res.Stdout) != "partial" {
		t.Fatalf("got %s code %d out %q (%v), want exit/4/partial", res.Status, res.ExitCode, res.Stdout, res.Err)
	}
}

func TestRunRejectsRelativePathEntries(t *testing.T) {
	// Arrange: a tool that is only reachable through a relative PATH entry.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ar-dot-tool"), []byte("#!/bin/sh\nprintf pwned"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, pathEntry := range []string{".", "./"} {
		t.Run(pathEntry, func(t *testing.T) {
			t.Setenv("PATH", pathEntry)
			// Act
			res := Run(context.Background(), Spec{Argv: []string{"ar-dot-tool"}, Env: ScrubEnv(os.Environ(), nil, nil)})
			// Assert
			if res.Status != StatusUnavailable || res.Err == nil {
				t.Fatalf("status = %s (%v), want unavailable", res.Status, res.Err)
			}
		})
	}
}

func TestRunRelativeDirAndCommand(t *testing.T) {
	// Arrange: Dir is itself relative to the parent's working directory.
	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "tool.sh"), []byte("#!/bin/sh\npwd"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	// Act
	res := Run(context.Background(), Spec{Argv: []string{"./tool.sh"}, Dir: "sub", Env: ScrubEnv(os.Environ(), nil, nil)})
	// Assert
	if res.Status != StatusOK || !strings.HasSuffix(strings.TrimSpace(string(res.Stdout)), "sub") {
		t.Fatalf("got %s %q (%v)", res.Status, res.Stdout, res.Err)
	}
}

func TestSensitiveExtended(t *testing.T) {
	for _, name := range []string{"GITHUB_PAT", "NPM_PAT", "SENTRY_DSN", "DATABASE_URL", "SLACK_WEBHOOK_URL", "COOKIE", "SESSION", "MY_SESSION_ID", "HTTP_COOKIE"} {
		if !Sensitive(name) {
			t.Errorf("Sensitive(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"PATH", "PATIENT", "COMPATIBLE"} {
		if Sensitive(name) {
			t.Errorf("Sensitive(%q) = true, want false", name)
		}
	}
}
