package catalogsite

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A minimal Chrome DevTools Protocol client over --remote-debugging-pipe: no
// WebSocket and no third-party module. Chrome reads commands from file descriptor 3
// and writes replies and events to file descriptor 4, as NUL-terminated JSON.

const (
	browserEnv       = "AI_RULEZ_CHROME"
	browserOpTimeout = 30 * time.Second
	cdpMaxMessage    = 64 << 20
)

// chromeBinary finds a Chrome or Chromium to drive, or "" when there is none.
func chromeBinary() string {
	if env := os.Getenv(browserEnv); env != "" {
		return env
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

// lockedBuffer is a bytes.Buffer safe for the exec package's copying goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type cdpMessage struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	Error     *cdpError       `json:"error"`
	SessionID string          `json:"sessionId"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type browser struct {
	t       *testing.T
	cmd     *exec.Cmd
	toChrom *os.File
	mu      sync.Mutex
	nextID  int
	waiters map[int]chan cdpMessage
	events  []cdpMessage
	changed chan struct{}
	session string
}

// startBrowser launches headless Chrome with a throwaway profile and opens one
// page. The browser is closed when the test ends.
func startBrowser(t *testing.T, bin string) *browser {
	t.Helper()
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--disable-extensions",
		"--disable-background-networking", "--disable-component-update", "--disable-sync", "--mute-audio",
		"--user-data-dir=" + t.TempDir(), "--remote-debugging-pipe"}
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	return launchBrowser(t, bin, append(args, "about:blank"))
}

// launchBrowser runs bin with args, wired to the protocol pipes, and opens a page.
func launchBrowser(t *testing.T, bin string, args []string) *browser {
	t.Helper()
	// Chrome's end of the pipes: it reads commands from fd 3 and writes to fd 4.
	cmdR, cmdW, err := os.Pipe()
	require.NoError(t, err)
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	cmd := exec.Command(bin, args...) //nolint:gosec // a browser the test machine has, found by chromeBinary
	cmd.ExtraFiles = []*os.File{cmdR, outW}
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	_ = cmdR.Close() //nolint:errcheck // the child owns its ends now
	_ = outW.Close() //nolint:errcheck // the child owns its ends now

	b := &browser{t: t, cmd: cmd, toChrom: cmdW, waiters: map[int]chan cdpMessage{}, changed: make(chan struct{}, 1)}
	go b.read(outR)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("chrome stderr:\n%s", stderr.String())
		}
		_ = cmdW.Close() //nolint:errcheck // closing the pipe asks Chrome to exit
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }() //nolint:errcheck // exit status is irrelevant
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill() //nolint:errcheck // last resort
			<-done
		}
		_ = outR.Close() //nolint:errcheck // read side of a dead pipe
	})

	target := b.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	var created struct {
		TargetID string `json:"targetId"`
	}
	require.NoError(t, json.Unmarshal(target, &created))
	attached := b.call("", "Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true})
	var att struct {
		SessionID string `json:"sessionId"`
	}
	require.NoError(t, json.Unmarshal(attached, &att))
	b.session = att.SessionID
	for _, m := range []string{"Page.enable", "Runtime.enable", "Log.enable", "Network.enable"} {
		b.call(b.session, m, nil)
	}
	// Record CSP violations before any page script runs. The listener is injected
	// by the protocol, so the page's own policy does not apply to it.
	b.call(b.session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `
window.__csp = [];
document.addEventListener("securitypolicyviolation", function (e) {
  window.__csp.push(e.violatedDirective + " " + e.blockedURI);
});`})
	return b
}

func (b *browser) read(r *os.File) {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		raw, err := br.ReadBytes(0)
		if err != nil {
			return
		}
		raw = bytes.TrimSuffix(raw, []byte{0})
		if len(raw) > cdpMaxMessage {
			continue
		}
		var m cdpMessage
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		b.mu.Lock()
		if m.ID != 0 {
			if ch, ok := b.waiters[m.ID]; ok {
				delete(b.waiters, m.ID)
				ch <- m
			}
		} else {
			b.events = append(b.events, m)
		}
		b.mu.Unlock()
		select {
		case b.changed <- struct{}{}:
		default:
		}
	}
}

// call sends a command and returns its result.
func (b *browser) call(session, method string, params any) json.RawMessage {
	b.t.Helper()
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	ch := make(chan cdpMessage, 1)
	b.waiters[id] = ch
	b.mu.Unlock()

	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if session != "" {
		msg["sessionId"] = session
	}
	data, err := json.Marshal(msg)
	require.NoError(b.t, err)
	_, err = b.toChrom.Write(append(data, 0))
	require.NoError(b.t, err)
	select {
	case m := <-ch:
		require.Nilf(b.t, m.Error, "%s failed: %+v", method, m.Error)
		return m.Result
	case <-time.After(browserOpTimeout):
		b.t.Fatalf("%s: no reply from the browser within %s", method, browserOpTimeout)
		return nil
	}
}

// count returns how many events with the given method have arrived.
func (b *browser) count(method string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, e := range b.events {
		if e.Method == method {
			n++
		}
	}
	return n
}

// waitFor blocks until more than seen events of method have arrived.
func (b *browser) waitFor(method string, seen int) {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), browserOpTimeout)
	defer cancel()
	for b.count(method) <= seen {
		select {
		case <-b.changed:
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			b.t.Fatalf("timed out waiting for %s", method)
		}
	}
}

// navigate loads url and waits for its load event.
func (b *browser) navigate(url string) {
	b.t.Helper()
	seen := b.count("Page.loadEventFired")
	res := b.call(b.session, "Page.navigate", map[string]any{"url": url})
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	require.NoError(b.t, json.Unmarshal(res, &nav))
	require.Emptyf(b.t, nav.ErrorText, "navigating to %s", url)
	b.waitFor("Page.loadEventFired", seen)
}

// eval evaluates a JavaScript expression in the page and returns its JSON value.
func (b *browser) eval(expr string) json.RawMessage {
	b.t.Helper()
	res := b.call(b.session, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true})
	var out struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	require.NoError(b.t, json.Unmarshal(res, &out))
	if out.ExceptionDetails != nil {
		b.t.Fatalf("evaluating %q: %s %s", expr, out.ExceptionDetails.Text, out.ExceptionDetails.Exception.Description)
	}
	return out.Result.Value
}

func (b *browser) evalString(expr string) string {
	b.t.Helper()
	var s string
	require.NoError(b.t, json.Unmarshal(b.eval(expr), &s))
	return s
}

func (b *browser) evalInt(expr string) int {
	b.t.Helper()
	var n int
	require.NoError(b.t, json.Unmarshal(b.eval(expr), &n))
	return n
}

// click clicks the first element matching selector and waits for the navigation
// it causes, when navigates is set.
func (b *browser) click(selector string, navigates bool) {
	b.t.Helper()
	seen := b.count("Page.loadEventFired")
	require.Truef(b.t, b.evalBool(fmt.Sprintf(`(function(){var e=document.querySelector(%q);if(!e){return false}e.click();return true})()`, selector)),
		"no element matches %s", selector)
	if navigates {
		b.waitFor("Page.loadEventFired", seen)
	}
}

func (b *browser) evalBool(expr string) bool {
	b.t.Helper()
	var v bool
	require.NoError(b.t, json.Unmarshal(b.eval(expr), &v))
	return v
}

// problems lists what the browser reported since the start: console errors and
// warnings, uncaught exceptions, log entries of error level (CSP refusals and
// failed loads), JavaScript dialogs and requests that left the file system.
func (b *browser) problems() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, e := range b.events {
		switch e.Method {
		case "Runtime.consoleAPICalled":
			var p struct {
				Type string `json:"type"`
				Args []struct {
					Value       any    `json:"value"`
					Description string `json:"description"`
				} `json:"args"`
			}
			if json.Unmarshal(e.Params, &p) == nil && (p.Type == "error" || p.Type == "warning" || p.Type == "assert") {
				var parts []string
				for _, a := range p.Args {
					parts = append(parts, fmt.Sprint(a.Value)+a.Description)
				}
				out = append(out, "console."+p.Type+": "+strings.Join(parts, " "))
			}
		case "Runtime.exceptionThrown":
			out = append(out, "uncaught exception: "+string(e.Params))
		case "Log.entryAdded":
			var p struct {
				Entry struct {
					Level, Source, Text, URL string
				} `json:"entry"`
			}
			if json.Unmarshal(e.Params, &p) == nil && (p.Entry.Level == "error" || p.Entry.Level == "warning") {
				out = append(out, fmt.Sprintf("log %s/%s: %s %s", p.Entry.Source, p.Entry.Level, p.Entry.Text, p.Entry.URL))
			}
		case "Page.javascriptDialogOpening":
			out = append(out, "a JavaScript dialog opened: "+string(e.Params))
		case "Network.requestWillBeSent":
			var p struct {
				Request struct {
					URL string `json:"url"`
				} `json:"request"`
			}
			if json.Unmarshal(e.Params, &p) == nil {
				u := p.Request.URL
				if !strings.HasPrefix(u, "file://") && !strings.HasPrefix(u, "about:") && !strings.HasPrefix(u, "data:") {
					out = append(out, "network request: "+u)
				}
			}
		}
	}
	return out
}

// chromeLaunches reports whether bin starts and answers --version. A browser can
// be installed and still not run (killed by the OS, missing libraries).
func chromeLaunches(bin string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), browserOpTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput() //nolint:gosec // the test machine's own browser
	return strings.TrimSpace(string(out)), err == nil
}

// requireBrowser returns the Chrome to use or skips the test.
func requireBrowser(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("browser tests are skipped with -short")
	}
	bin := chromeBinary()
	if bin == "" {
		t.Skipf("no Chrome or Chromium found (set %s to the executable)", browserEnv)
	}
	if _, ok := chromeLaunches(bin); !ok {
		t.Skipf("%s is installed but does not start on this machine", bin)
	}
	return bin
}

func fileURL(dir, rel string) string {
	abs, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		panic(err)
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash // a Windows drive path
	}
	return "file://" + slash
}
