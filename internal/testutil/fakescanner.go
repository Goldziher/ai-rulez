package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FakeScannerArg makes a test binary act as an external scanner: a package's
// TestMain calls RunFakeScannerIfAsked first, and the scanner tests point
// [[lint.external]] command at os.Executable() followed by this argument. It
// starts a real process on every platform, which a shell-script scanner cannot.
const FakeScannerArg = "__ai_rulez_fake_scanner__"

// RunFakeScannerIfAsked runs the fake scanner and exits when the process was
// started as one (its first argument is FakeScannerArg); otherwise it returns.
func RunFakeScannerIfAsked() {
	if len(os.Args) > 1 && os.Args[1] == FakeScannerArg {
		os.Exit(RunFakeScanner(os.Args[2:]))
	}
}

// RunFakeScanner is the scanner and returns its exit status. Flags (each takes
// the next argument as its value unless noted):
//
//	--msg TEXT | --msg-len N   message of every finding (default "found")
//	--rule ID, --level L       SARIF rule id and level (default FAKE1, warning)
//	--sleep D                  wait before answering
//	--log FILE                 append one line at the start and one at the end, with the arguments
//	--out FILE                 write the report there instead of stdout
//	--print-version V          print "fake-scanner V" and exit
//	--list-files (no value)    the message lists the regular files under each directory argument
//
// An argument that is an existing directory gets one finding at its SKILL.md; an
// existing file is a script whose trimmed content is the message unless --msg
// is given. With no directory there is one finding without a location.
func RunFakeScanner(args []string) int {
	opts := map[string]string{"--rule": "FAKE1", "--level": "warning"}
	var dirs, scripts []string
	listFiles := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--list-files":
			listFiles = true
		case strings.HasPrefix(a, "--") && i+1 < len(args):
			opts[a] = args[i+1]
			i++
		default:
			if info, err := os.Stat(a); err == nil && info.IsDir() {
				dirs = append(dirs, a)
			} else if err == nil {
				scripts = append(scripts, a)
			}
		}
	}
	if v, ok := opts["--print-version"]; ok {
		fmt.Println("fake-scanner " + v)
		return 0
	}
	logLine := func(kind string) {
		path := opts["--log"]
		if path == "" {
			return
		}
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // a test log
			fmt.Fprintf(f, "%s %d %s\n", kind, time.Now().UnixNano(), strings.Join(args, " "))
			_ = f.Close()
		}
	}
	logLine("start")
	if d, err := time.ParseDuration(opts["--sleep"]); err == nil {
		time.Sleep(d)
	}
	msg := opts["--msg"]
	if n, err := strconv.Atoi(opts["--msg-len"]); err == nil {
		msg = strings.Repeat("x", n)
	}
	if msg == "" && len(scripts) > 0 {
		data, _ := os.ReadFile(scripts[0]) //nolint:gosec // a test script
		msg = strings.TrimSpace(string(data))
	}
	if msg == "" {
		msg = "found"
	}
	var results []map[string]any
	add := func(file, text string) {
		r := map[string]any{"ruleId": opts["--rule"], "level": opts["--level"], "message": map[string]any{"text": text}}
		if file != "" {
			r["locations"] = []any{map[string]any{"physicalLocation": map[string]any{
				"artifactLocation": map[string]any{"uri": filepath.ToSlash(file)}, "region": map[string]any{"startLine": 1}}}}
		}
		results = append(results, r)
	}
	for _, d := range dirs {
		text := msg
		if listFiles {
			text = "files=" + strings.Join(filesUnder(d), ",")
		}
		add(filepath.Join(d, "SKILL.md"), text)
	}
	if len(dirs) == 0 {
		add("", msg)
	}
	doc, _ := json.Marshal(map[string]any{"version": "2.1.0", "runs": []any{map[string]any{"results": results}}})
	var w io.Writer = os.Stdout
	if out := opts["--out"]; out != "" {
		if err := os.WriteFile(out, doc, 0o600); err != nil {
			return 1
		}
		w = io.Discard
	}
	fmt.Fprintln(w, string(doc))
	logLine("end")
	return 0
}

func filesUnder(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error { //nolint:errcheck // best effort listing
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p) //nolint:errcheck // p is below dir
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}
