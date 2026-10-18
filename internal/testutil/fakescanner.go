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

// fakeScan is the parsed command line of the fake scanner.
type fakeScan struct {
	opts      map[string]string
	args      []string
	dirs      []string
	scripts   []string
	listFiles bool
	// alsoGlobal adds a result without a location to the per-directory ones.
	alsoGlobal bool
}

func parseFakeScan(args []string) fakeScan {
	s := fakeScan{opts: map[string]string{"--rule": "FAKE1", "--level": "warning"}, args: args}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--list-files":
			s.listFiles = true
		case a == "--also-global":
			s.alsoGlobal = true
		case strings.HasPrefix(a, "--") && i+1 < len(args):
			s.opts[a] = args[i+1]
			i++
		default:
			if info, err := os.Stat(a); err == nil && info.IsDir() {
				s.dirs = append(s.dirs, a)
			} else if err == nil {
				s.scripts = append(s.scripts, a)
			}
		}
	}
	return s
}

// log appends one line (kind, time, arguments) to the --log file, if any.
func (s fakeScan) log(kind string) {
	path := s.opts["--log"]
	if path == "" {
		return
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // a test log
		fmt.Fprintf(f, "%s %d %s\n", kind, time.Now().UnixNano(), strings.Join(s.args, " ")) //nolint:errcheck,gosec // a test log
		f.Close()                                                                            //nolint:errcheck,gosec // a test log
	}
}

// rendezvous waits until the number of runs --rendezvous-count asks for has started, each leaving a marker in the
// --rendezvous directory. It gives up after 10s so a scanner that is never joined cannot hang the test.
func (s fakeScan) rendezvous() {
	dir := s.opts["--rendezvous"]
	want, err := strconv.Atoi(s.opts["--rendezvous-count"])
	if dir == "" || err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(os.Getpid())), nil, 0o600); err != nil {
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if entries, rerr := os.ReadDir(dir); rerr == nil && len(entries) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// message is the text of every finding.
func (s fakeScan) message() string {
	if n, err := strconv.Atoi(s.opts["--msg-len"]); err == nil {
		return strings.Repeat("x", n)
	}
	if msg := s.opts["--msg"]; msg != "" {
		return msg
	}
	if len(s.scripts) > 0 {
		data, _ := os.ReadFile(s.scripts[0]) //nolint:gosec,errcheck // a test script; unreadable means no message
		if msg := strings.TrimSpace(string(data)); msg != "" {
			return msg
		}
	}
	return "found"
}

// results is the SARIF results: one per directory argument, else one without a location.
func (s fakeScan) results() []map[string]any {
	msg := s.message()
	var results []map[string]any
	add := func(file, text string) {
		r := map[string]any{"ruleId": s.opts["--rule"], "level": s.opts["--level"], "message": map[string]any{"text": text}}
		if file != "" {
			r["locations"] = []any{map[string]any{"physicalLocation": map[string]any{
				"artifactLocation": map[string]any{"uri": filepath.ToSlash(file)}, "region": map[string]any{"startLine": 1}}}}
		}
		results = append(results, r)
	}
	for _, d := range s.dirs {
		text := msg
		if s.listFiles {
			text = "files=" + strings.Join(filesUnder(d), ",")
		}
		add(filepath.Join(d, "SKILL.md"), text)
	}
	if len(s.dirs) == 0 || s.alsoGlobal {
		add("", msg)
	}
	return results
}

// RunFakeScanner is the scanner and returns its exit status. Flags (each takes
// the next argument as its value unless noted):
//
//	--msg TEXT | --msg-len N   message of every finding (default "found")
//	--rule ID, --level L       SARIF rule id and level (default FAKE1, warning)
//	--sleep D                  wait before answering
//	--rendezvous DIR           with --rendezvous-count N: leave a marker in DIR and wait (up to 10s) until N runs
//	                           have, so a test proves runs overlapped without depending on timing
//	--log FILE                 append one line at the start and one at the end, with the arguments
//	--out FILE                 write the report there instead of stdout
//	--print-version V          print "fake-scanner V" (after any --sleep) and exit
//	--list-files (no value)    the message lists the regular files under each directory argument
//	--also-global (no value)   one more result, without a location, besides the per-directory ones
//
// An argument that is an existing directory gets one finding at its SKILL.md; an
// existing file is a script whose trimmed content is the message unless --msg
// is given. With no directory there is one finding without a location.
func RunFakeScanner(args []string) int {
	s := parseFakeScan(args)
	if v, ok := s.opts["--print-version"]; ok {
		if d, err := time.ParseDuration(s.opts["--sleep"]); err == nil {
			time.Sleep(d)
		}
		fmt.Fprintln(os.Stdout, "fake-scanner "+v) //nolint:errcheck // test scanner output
		return 0
	}
	s.log("start")
	if d, err := time.ParseDuration(s.opts["--sleep"]); err == nil {
		time.Sleep(d)
	}
	s.rendezvous()
	doc, _ := json.Marshal(map[string]any{"version": "2.1.0", "runs": []any{map[string]any{"results": s.results()}}}) //nolint:errcheck // plain values
	var w io.Writer = os.Stdout
	if out := s.opts["--out"]; out != "" {
		if err := os.WriteFile(out, doc, 0o600); err != nil {
			return 1
		}
		w = io.Discard
	}
	fmt.Fprintln(w, string(doc)) //nolint:errcheck // test scanner output
	s.log("end")
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
