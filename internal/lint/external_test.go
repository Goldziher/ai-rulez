package lint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const sarifOneResult = `{"version":"2.1.0","runs":[{"results":[{"ruleId":"X1","level":"error","message":{"text":"bad thing"},` +
	`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":3}}}]}]}]}`

// externalRun lints a fixture project with one scanner script and returns the report findings.
func externalRun(t *testing.T, scriptBody, extraKeys string, opts Options) []Finding {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/rules/r.md": "# Rule\n\nbody\n"})
	script := filepath.Join(root, "scan.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig +
		"\n[[lint.external]]\nname = \"fake\"\ncommand = [\"" + filepath.ToSlash(script) + "\"]\n" + extraKeys})
	gitAdd(t, root)
	cfg := loadNoRemote(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	opts.External = true
	rep, err := RunWith(cfg, tree, opts)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func TestExternalRunnerHardening(t *testing.T) {
	big := "head -c 200000 /dev/zero | tr '\\\\0' 'x'\\n"
	tests := []struct {
		name     string
		script   string
		keys     string
		opts     Options
		wantCode string
		wantSev  Severity
		wantMsg  string
		notCode  string
	}{
		{name: "undeclared egress warns and still runs", script: "echo '" + sarifOneResult + "'\n", wantCode: CodeScannerEgressUndeclared, wantSev: SeverityWarning},
		{name: "declared false is silent", script: "echo '" + sarifOneResult + "'\n", keys: "egress = false\n", wantCode: CodeExternalFinding, wantSev: SeverityError, wantMsg: "bad thing", notCode: CodeScannerEgressUndeclared},
		{name: "egress true is blocked without the flag", script: "echo '" + sarifOneResult + "'\n", keys: "egress = true\n", wantCode: CodeScannerEgressBlocked, wantSev: SeverityError, wantMsg: "--allow-egress=fake", notCode: CodeExternalFinding},
		{name: "egress true runs with the flag", script: "echo '" + sarifOneResult + "'\n", keys: "egress = true\n", opts: Options{AllowEgress: []string{"fake"}}, wantCode: CodeExternalFinding, wantSev: SeverityError, notCode: CodeScannerEgressBlocked},
		{name: "unreadable output is AR9E3", script: "echo boom >&2\nexit 3\n", wantCode: CodeScannerRunFailed, wantSev: SeverityError, wantMsg: "unreadable output", notCode: CodeExternalFinding},
		{name: "timeout is AR9E3", script: "sleep 30\n", keys: "timeout = \"300ms\"\n", wantCode: CodeScannerRunFailed, wantSev: SeverityError, wantMsg: "timed out"},
		{name: "huge output is not ingested", script: big, wantCode: CodeScannerRunFailed, wantSev: SeverityError, notCode: CodeExternalFinding},
		{name: "bad timeout is AR9E0 and the scanner does not run", script: "echo '" + sarifOneResult + "'\n", keys: "timeout = \"soon\"\n", wantCode: CodeScannerConfigInvalid, wantSev: SeverityError, notCode: CodeExternalFinding},
		{name: "credential env_pass on egress false is AR9E0", script: "echo '" + sarifOneResult + "'\n", keys: "egress = false\nenv_pass = [\"ANTHROPIC_API_KEY\"]\n", wantCode: CodeScannerConfigInvalid, wantSev: SeverityError, notCode: CodeExternalFinding},
		{name: "failed invocation is not a clean scan", script: `echo '{"version":"2.1.0","runs":[{"invocations":[{"executionSuccessful":false}],"results":[]}]}'` + "\n", wantCode: CodeScannerRunFailed, wantSev: SeverityError, wantMsg: "executionSuccessful"},
		{name: "empty runs with a failing exit is not clean", script: `echo '{"version":"2.1.0","runs":[]}'` + "\nexit 2\n", wantCode: CodeScannerRunFailed, wantSev: SeverityError},
		{name: "wrong SARIF version", script: `echo '{"version":"1.0.0","runs":[]}'` + "\n", wantCode: CodeScannerRunFailed, wantSev: SeverityError, wantMsg: "unsupported SARIF version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			findings := externalRun(t, tt.script, tt.keys, tt.opts)
			// Assert
			found := false
			for _, f := range findings {
				if f.Code == tt.wantCode && f.Severity == tt.wantSev && strings.Contains(f.Message, tt.wantMsg) {
					found = true
				}
				if tt.notCode != "" && f.Code == tt.notCode {
					t.Errorf("unexpected %s: %s", f.Code, f.Message)
				}
			}
			if !found {
				t.Fatalf("want %s %s containing %q:\n%s", tt.wantSev, tt.wantCode, tt.wantMsg, dump(findings))
			}
		})
	}
}

func TestExternalMissingBinaryIsANotice(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/rules/r.md":  "# Rule\n\nbody\n",
		".ai-rulez/config.toml": baseConfig + "\n[[lint.external]]\nname = \"ghost\"\ncommand = [\"ai-rulez-no-such-scanner\"]\negress = false\n",
	})
	gitAdd(t, root)
	cfg := loadNoRemote(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunWith(cfg, tree, Options{External: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Code == CodeScannerUnavailable {
			if f.Severity != SeverityWarning || !strings.Contains(f.Message, "[ghost]") {
				t.Fatalf("got %+v", f)
			}
			if Failed(rep.Findings, "error") {
				t.Fatalf("a missing scanner must not fail the default gate:\n%s", dump(rep.Findings))
			}
			return
		}
	}
	t.Fatalf("no AR9E2:\n%s", dump(rep.Findings))
}

func TestExternalScrubbedEnvironment(t *testing.T) {
	t.Setenv("AR_PLANTED_API_KEY", "planted-value")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid")
	// The scanner prints its environment as a SARIF message.
	script := `printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"ENV","level":"note","message":{"text":"planted=%s proxy=%s"}}]}]}' "$(env | grep -c AR_PLANTED)" "$(env | grep -c HTTPS_PROXY)"` + "\n"
	for _, tt := range []struct {
		name, keys string
		leaks      bool
	}{
		{"declared false scrubs", "egress = false\n", false},
		{"legacy inherits", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := externalRun(t, script, tt.keys, Options{})
			joined := dump(findings)
			want := "planted=0 proxy=0"
			if tt.leaks {
				want = "planted=1 proxy=1"
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("want %q:\n%s", want, joined)
			}
		})
	}
}

func TestEgressFlagViolation(t *testing.T) {
	tests := []struct {
		argv []string
		want string
	}{
		{[]string{"scan", "--format", "sarif"}, ""},
		{[]string{"skill-scanner", "scan", "--use-llm"}, "--use-llm"},
		{[]string{"scan", "--USE-VirusTotal"}, "--USE-VirusTotal"},
		{[]string{"scan", "--llm-provider=openai"}, "--llm-provider=openai"},
		{[]string{"scan", "--dangerously-run-mcp-servers"}, "--dangerously-run-mcp-servers"},
		{[]string{"scan", "--api-url", "https://example.com/v1"}, "--api-url"},
		{[]string{"scan", "--api-url=http://localhost:8080"}, ""},
		{[]string{"scan", "--endpoint", "127.0.0.1:9000"}, ""},
		{[]string{"scan", "--endpoint=https://scanner.example.org"}, "--endpoint=https://scanner.example.org"},
		{[]string{"scan", "-use-llm"}, "-use-llm"},
		{[]string{"scan", "-endpoint", "https://scanner.example.org"}, "-endpoint"},
		{[]string{"scan", "-api-url=http://localhost:8080"}, ""},
		{[]string{"scan", "--use-llm=false"}, ""},
		{[]string{"scan", "--use-llm=0"}, ""},
		{[]string{"scan", "--use-llm=true"}, "--use-llm=true"},
		{[]string{"scan", "--llm-provider=off"}, ""},
		{[]string{"scan", "--endpoint", "[::1]:8080"}, ""},
		{[]string{"scan", "--endpoint=http://[::1]:8080/x"}, ""},
		{[]string{"scan", "--endpoint=[::1]"}, ""},
		{[]string{"scan", "--endpoint=localhost:9000"}, ""},
		{[]string{"scan", "--endpoint=127.0.0.2:9000"}, ""},
		{[]string{"scan", "--endpoint=[2001:db8::1]:8080"}, "--endpoint=[2001:db8::1]:8080"},
		{[]string{"scan", "--endpoint=localhost.evil.com"}, "--endpoint=localhost.evil.com"},
		{[]string{"scan", "-v", "-o", "out"}, ""},
	}
	for _, tt := range tests {
		if got := egressFlagViolation(tt.argv); got != tt.want {
			t.Errorf("egressFlagViolation(%v) = %q, want %q", tt.argv, got, tt.want)
		}
	}
}

func TestParseExternalSARIF(t *testing.T) {
	res := func(extra string) string {
		return `{"version":"2.1.0","runs":[{"results":[{"ruleId":"R","message":{"text":"m"}` + extra + `}]}]}`
	}
	tests := []struct {
		name    string
		in      string
		exit    int
		want    int
		wantErr string
	}{
		{name: "clean run", in: `{"version":"2.1.0","runs":[{"results":[]}]}`, want: 0},
		{name: "no version is tolerated", in: `{"runs":[{"results":[]}]}`, want: 0},
		{name: "suppressed result dropped", in: res(`,"suppressions":[{"kind":"inSource"}]`), want: 0},
		{name: "markdown fallback", in: `{"runs":[{"results":[{"message":{"markdown":"md"}}]}]}`, want: 1},
		{name: "notification error", in: `{"runs":[{"invocations":[{"toolExecutionNotifications":[{"level":"error","message":{"text":"crash"}}]}],"results":[]}]}`, wantErr: "crash"},
		{name: "uriBaseId resolves against the declared base", in: `{"runs":[{"originalUriBaseIds":{"SRC":{"uri":"file:///work/proj/"}},"results":[{"message":{"text":"m"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a/b.md","uriBaseId":"SRC"}}}]}]}]}`, want: 1},
		{name: "not json", in: `nope`, wantErr: "invalid SARIF"},
		{name: "empty", in: ``, wantErr: "no output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExternal("sarif", []byte(tt.in), tt.exit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || len(got) != tt.want {
				t.Fatalf("got %d findings, err %v; want %d", len(got), err, tt.want)
			}
		})
	}
}

func TestParseExternalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		exit    int
		want    int
		wantErr string
	}{
		{name: "findings", in: `[{"file":"a.md","line":1,"message":"m"}]`, exit: 1, want: 1},
		{name: "empty list clean exit", in: `[]`, want: 0},
		{name: "empty list failing exit", in: `[]`, exit: 2, wantErr: "exited with status 2"},
		{name: "null failing exit", in: `null`, exit: 1, wantErr: "exited with status 1"},
		{name: "null clean exit", in: `null`, want: 0},
		{name: "no output failing exit", in: ``, exit: 1, wantErr: "no output"},
		{name: "not json", in: `x`, wantErr: "invalid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExternal("json", []byte(tt.in), tt.exit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || len(got) != tt.want {
				t.Fatalf("got %d, err %v; want %d", len(got), err, tt.want)
			}
		})
	}
}

func TestSARIFBaseIDResolution(t *testing.T) {
	root := t.TempDir()
	rootURI := "file://" + filepath.ToSlash(root) + "/"
	doc := func(base, uri string) string {
		return `{"runs":[{"originalUriBaseIds":{"SRC":{"uri":"` + base + `"}},"results":[{"message":{"text":"m"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"` + uri + `","uriBaseId":"SRC"}}}]}]}]}`
	}
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"base is the project", doc(rootURI, "x/y.md"), filepath.Join(root, "x", "y.md"), true},
		{"base is a subdirectory", doc(rootURI+"sub/", "y.md"), filepath.Join(root, "sub", "y.md"), true},
		{"relative base", doc("sub/", "y.md"), filepath.Join(root, "sub", "y.md"), true},
		{"base outside the project", doc("file:///etc/", "passwd"), "", false},
		{"traversal out of the base", doc(rootURI+"sub/", "../../x.md"), "", false},
		{"unknown base id falls back to the root", strings.Replace(doc(rootURI, "x.md"), `"SRC":{`, `"OTHER":{`, 1), filepath.Join(root, "x.md"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := parseExternal("sarif", []byte(tt.in), 0)
			if err != nil || len(found) != 1 {
				t.Fatalf("parse: %v, %d findings", err, len(found))
			}
			got, ok := resolveScannerPath(found[0].File, root)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("resolved %q -> %q, %v; want %q, %v", found[0].File, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestResolveScannerPath(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		raw    string
		want   string
		wantOK bool
	}{
		{"", "", true},
		{".ai-rulez/rules/r.md", filepath.Join(root, ".ai-rulez", "rules", "r.md"), true},
		{"file://" + filepath.ToSlash(root) + "/a%20b.md", filepath.Join(root, "a b.md"), true},
		{"file://localhost/" + strings.TrimPrefix(filepath.ToSlash(root), "/") + "/l.md", filepath.Join(root, "l.md"), true},
		{"file://evil.example.com" + filepath.ToSlash(root) + "/l.md", "", false},
		{"file://server/share/x.md", "", false},
		{"../outside.md", "", false},
		{"a/../../outside.md", "", false},
		{"/etc/passwd", "", false},
		{"file:///etc/passwd", "", false},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			raw    string
			want   string
			wantOK bool
		}{`sub\dir\x.md`, filepath.Join(root, "sub", "dir", "x.md"), true})
	} else {
		// A backslash is an ordinary file name character on unix: not a separator.
		tests = append(tests, struct {
			raw    string
			want   string
			wantOK bool
		}{`..\..\x.md`, filepath.Join(root, `..\..\x.md`), true})
	}
	for _, tt := range tests {
		got, ok := resolveScannerPath(tt.raw, root)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("resolveScannerPath(%q) = %q, %v; want %q, %v", tt.raw, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestSanitizeScannerText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"ansi and control", "\x1b[31mred\x1b[0m\x07 text", "red text"},
		{"bidi and zero width", "a\u202Eb\u200Bc\U000E0041d", "abcd"},
		{"newlines collapse", "one\n\ntwo\tthree", "one two three"},
		{"secret masked", "found AKIAIOSFODNN7EXAMPLE here", "found AKIA**** (20 characters) here"},
		{"truncated", strings.Repeat("é", 600), strings.Repeat("é", 500) + "..."},
	}
	for _, tt := range tests {
		if got := sanitizeScannerText(tt.in); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBatchArgs(t *testing.T) {
	base := []string{"scan", "--x"}
	files := []string{"/a/1.md", "/a/2.md", "/a/3.md", "/a/4.md"}
	tests := []struct {
		name   string
		files  []string
		budget int
		want   [][]string
	}{
		{"all fit", files, 1000, [][]string{append(append([]string{}, base...), files...)}},
		{"split", files, len("scan") + len("--x") + 2 + 2*(len("/a/1.md")+1), [][]string{
			{"scan", "--x", "/a/1.md", "/a/2.md"}, {"scan", "--x", "/a/3.md", "/a/4.md"}}},
		{"no files still runs once", nil, 1000, [][]string{base}},
		{"one oversized file is its own batch", []string{"/a/very-long-name.md"}, 5, [][]string{{"scan", "--x", "/a/very-long-name.md"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batchArgs(base, tt.files, tt.budget)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if strings.Join(got[i], "|") != strings.Join(tt.want[i], "|") {
					t.Errorf("batch %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExternalBatchesLargeFileLists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	old := argvBudget
	argvBudget = 400
	t.Cleanup(func() { argvBudget = old })
	root := t.TempDir()
	files := map[string]string{}
	for i := 0; i < 12; i++ {
		files[".ai-rulez/rules/rule-"+string(rune('a'+i))+".md"] = "# Rule\n\nbody\n"
	}
	writeFiles(t, root, files)
	script := filepath.Join(root, "scan.sh")
	counter := filepath.Join(root, "runs")
	body := "#!/bin/sh\necho x >> " + counter + "\nfor f in \"$@\"; do echo \"$f\" >> " + counter + ".files; done\necho '{\"runs\":[{\"results\":[]}]}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig +
		"\n[[lint.external]]\nname = \"fake\"\negress = false\ncommand = [\"" + filepath.ToSlash(script) + "\"]\n"})
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunWith(loadNoRemote(t, root), tree, Options{External: true}); err != nil {
		t.Fatal(err)
	}
	runs, _ := os.ReadFile(counter)
	seen, _ := os.ReadFile(counter + ".files")
	if n := strings.Count(string(runs), "x"); n < 2 {
		t.Errorf("scanner ran %d times, want the file list split across runs", n)
	}
	for i := 0; i < 12; i++ {
		if !strings.Contains(string(seen), "rule-"+string(rune('a'+i))+".md") {
			t.Errorf("rule %c was never passed to the scanner", 'a'+i)
		}
	}
}

func TestExternalLocationlessAndHugeLineResults(t *testing.T) {
	noLoc := `{"version":"2.1.0","runs":[{"results":[{"ruleId":"X1","level":"error","message":{"text":"no place"}}]}]}`
	hugeLine := `{"version":"2.1.0","runs":[{"results":[{"ruleId":"X2","level":"error","message":{"text":"far away"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":999999999999}}}]}]}]}`
	tests := []struct {
		name, sarif, msg, wantFile string
		maxLine                    int
	}{
		{name: "location-less result is attributed to the config file", sarif: noLoc, msg: "no place", wantFile: "config.toml", maxLine: 1},
		{name: "start line is clamped", sarif: hugeLine, msg: "far away", wantFile: "r.md", maxLine: 10_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			findings := externalRun(t, "echo '"+tt.sarif+"'\n", "egress = false\n", Options{})

			// Assert
			for _, f := range findings {
				if f.Code == CodeExternalFinding && strings.Contains(f.Message, tt.msg) {
					if filepath.Base(f.File) != tt.wantFile || f.Line < 1 || f.Line > tt.maxLine {
						t.Fatalf("got %s:%d, want file %s line 1..%d", f.File, f.Line, tt.wantFile, tt.maxLine)
					}
					return
				}
			}
			t.Fatalf("no AR011 finding for %q:\n%s", tt.msg, dump(findings))
		})
	}
}

func TestWriteMarkdownEscapesMessages(t *testing.T) {
	// Arrange
	c := Combined{Findings: []Finding{{Code: "AR303", Name: "n", Severity: SeverityWarning, File: "a.md", Line: 1,
		Message: "<img src=x onerror=1> [click](http://evil.example) `code` a|b"}}, Summary: Summary{Total: 1, Warnings: 1}}
	var sb strings.Builder

	// Act
	err := WriteMarkdown(&sb, c)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, bad := range []string{"<img", "[click](", "`code`"} {
		if strings.Contains(out, bad) {
			t.Errorf("markdown output contains unescaped %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, `\[click\]`) || !strings.Contains(out, `a\|b`) {
		t.Errorf("escapes missing:\n%s", out)
	}
}
