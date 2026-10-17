// Package sessionstart implements `ai-rulez session-start`, the command the
// SessionStart hook of [session_start] runs. In a remote (cloud) agent session it
// regenerates the outputs of the fresh clone and injects a short context for the
// agent. It never blocks or fails a session: every path returns exit code 0, says
// at most one line on stderr when something went wrong, and is bounded by a timeout.
//
// stdout carries nothing but the hook's JSON document
// ({"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":...}});
// everything meant for a person goes to stderr, because Claude Code parses stdout.
package sessionstart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const (
	// maxPayloadBytes bounds the hook payload read from stdin.
	maxPayloadBytes = 1 << 20
	// maxLogBytes bounds the child output kept to explain a failure.
	maxLogBytes = 64 << 10
	// failureLimit bounds the failure reason printed and injected.
	failureLimit = 120
	prefix       = "ai-rulez session-start: "
)

// Sources of a SessionStart payload on which generate runs. clear and compact keep the
// working tree of a running session, so they only get the context.
const (
	sourceStartup = "startup"
	sourceResume  = "resume"
)

// Deps are the ambient things a run touches, so a test can fake each of them.
type Deps struct {
	Getenv  func(string) string
	Environ func() []string
	Getwd   func() (string, error)
	Runner  runner.Runner
	// Exe is the ai-rulez binary re-run as `generate` (a child process, so the
	// timeout kills its whole process tree and its output cannot reach stdout).
	Exe     string
	TempDir string
	// UID names the per-user lock directory inside TempDir and must be its owner (os.Getuid).
	UID int
	Now func() time.Time
	// LoadConfig loads the project rooted at dir without fetching remote includes.
	LoadConfig func(ctx context.Context, dir string) (*config.Config, error)
}

// Overrides are the command-line values that win over [session_start].
type Overrides struct {
	When    string
	Mode    string
	Timeout time.Duration
}

// Payload is the part of the hook's stdin the command reads.
type Payload struct {
	Source string `json:"source"`
	Cwd    string `json:"cwd"`
}

// ReadPayload reads the hook payload; empty, invalid or oversized input is an empty payload.
func ReadPayload(in io.Reader) Payload {
	var p Payload
	if in == nil {
		return p
	}
	data, err := io.ReadAll(io.LimitReader(in, maxPayloadBytes))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return Payload{}
	}
	return p
}

// IsRemote reports whether CLAUDE_CODE_REMOTE says the session runs in a remote environment.
func IsRemote(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(config.SessionStartRemoteEnv))) {
	case "1", "true":
		return true
	}
	return false
}

// Run executes one hook invocation and returns the exit code, which is always 0.
func Run(ctx context.Context, in io.Reader, out, errw io.Writer, d Deps, ov Overrides) int {
	payload := ReadPayload(in)
	dir := projectDir(d, payload)
	if dir == "" {
		return 0
	}
	root, hasGit, found := findRoot(dir)
	if !found {
		return 0 // no ai-rulez configuration here: nothing to do, nothing to say
	}
	cfg, err := d.LoadConfig(ctx, root)
	if err != nil || cfg == nil {
		say(errw, "cannot load the configuration; skipped")
		return 0
	}
	settings := cfg.SessionStart
	when := firstNonEmpty(ov.When, settings.WhenOrDefault())
	if when != config.SessionStartWhenAlways && !IsRemote(d.Getenv) {
		return 0
	}
	if !settings.GenerateOn() {
		return 0
	}
	if !hasGit {
		say(errw, dir+" is not in a git work tree; skipped")
		return 0
	}
	timeout := time.Duration(settings.TimeoutSecondsOrDefault()) * time.Second
	if ov.Timeout > 0 {
		timeout = ov.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	failure := ""
	if payload.Source == "" || payload.Source == sourceStartup || payload.Source == sourceResume {
		failure = generate(ctx, errw, d, cfg, root, firstNonEmpty(ov.Mode, settings.ModeOrDefault()), timeout)
	}
	if settings.PromptOn() {
		writeContext(out, errw, cfg.SessionStartContext(failure))
	}
	return 0
}

func projectDir(d Deps, p Payload) string {
	for _, dir := range []string{d.Getenv("CLAUDE_PROJECT_DIR"), p.Cwd} {
		if dir != "" && filepath.IsAbs(dir) {
			return filepath.Clean(dir)
		}
	}
	wd, err := d.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// findRoot walks up from dir to the first directory that holds an ai-rulez
// configuration, stopping at the top of the git work tree. hasGit reports whether
// dir lies in one.
func findRoot(dir string) (root string, hasGit, found bool) {
	for cur := dir; ; cur = filepath.Dir(cur) {
		if !found && (exists(filepath.Join(cur, ".ai-rulez", "config.toml")) ||
			exists(filepath.Join(cur, ".config", "ai-rulez", "config.toml"))) {
			root, found = cur, true
		}
		if exists(filepath.Join(cur, ".git")) {
			return root, true, found
		}
		if filepath.Dir(cur) == cur {
			return root, false, found
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// generate runs `ai-rulez generate` for the configured mode and returns the one-line
// reason it failed, or "" when it succeeded. It holds a lock file for the run, so
// two hooks starting together (a resume right after a start) do not race.
func generate(ctx context.Context, errw io.Writer, d Deps, cfg *config.Config, root, mode string, timeout time.Duration) string {
	release, ok := acquire(ctx, lockPath(d, root), timeout+lockSlack, d.Now)
	if !ok {
		reason := "another session-start run did not finish in time"
		say(errw, reason)
		return reason
	}
	defer release()

	var reason string
	for _, flags := range attempts(cfg, mode) {
		res := runGenerate(ctx, d, root, flags)
		if res.Status == runner.StatusOK {
			return ""
		}
		reason = explain(res)
		if deniedHost(res) != "" || res.Status == runner.StatusTimeout {
			break // a denied host stays denied, and the budget is spent
		}
	}
	say(errw, "generate failed: "+reason+"; generated files may be out of date")
	return reason
}

// lockSlack is how long past its own timeout a lock file is still believed.
const lockSlack = 30 * time.Second

// attempts lists the flag sets to try, in order. A locked run first tries offline: with
// a warm cache the lock covers everything and no network is needed; only when that
// fails does it go online, once.
func attempts(cfg *config.Config, mode string) [][]string {
	if mode == config.SessionStartModeAuto {
		mode = config.SessionStartModePlain
		if exists(filepath.Join(cfg.ConfigDir, lockfile.FileName)) {
			mode = config.SessionStartModeLocked
		}
	}
	switch mode {
	case config.SessionStartModeLocked:
		return [][]string{{"--locked", "--offline"}, {"--locked"}}
	case config.SessionStartModeFrozen:
		return [][]string{{"--frozen"}}
	}
	return [][]string{nil}
}

func runGenerate(ctx context.Context, d Deps, root string, flags []string) runner.Result {
	argv := append([]string{d.Exe, "generate", "--yes", "--quiet"}, flags...)
	remaining := time.Minute
	if dl, ok := ctx.Deadline(); ok {
		remaining = dl.Sub(d.Now())
	}
	if remaining <= 0 {
		return runner.Result{Status: runner.StatusTimeout, ExitCode: -1}
	}
	return d.Runner.Run(ctx, runner.Spec{
		Argv:      argv,
		Dir:       root,
		Env:       childEnv(d),
		Timeout:   remaining,
		MaxOutput: maxLogBytes,
	})
}

// childEnv is the scrubbed environment of the child: the base allowlist plus the proxy and
// certificate variables a fetch needs and AI_RULEZ_* settings. Tokens and other
// credentials of the session are not passed on.
func childEnv(d Deps) []string {
	var pass []string
	for _, kv := range d.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(name)
		switch {
		case strings.HasPrefix(up, "AI_RULEZ_") && !runner.Sensitive(name), isNetworkVar(up):
			pass = append(pass, name)
		}
	}
	return runner.ScrubEnv(d.Environ(), pass, []string{"AI_RULEZ_ACK_COMMANDS=1"})
}

func isNetworkVar(up string) bool {
	switch up {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
		"GIT_SSL_CAINFO", "CURL_CA_BUNDLE", "REQUESTS_CA_BUNDLE":
		return true
	}
	return false
}

// explain is the one-line reason a generate attempt failed.
func explain(res runner.Result) string {
	switch res.Status {
	case runner.StatusTimeout:
		return fmt.Sprintf("timed out after %s", res.Timeout.Round(time.Second))
	case runner.StatusUnavailable:
		return "the ai-rulez binary could not be started"
	}
	if host := deniedHost(res); host != "" {
		return "the network policy denied " + conservative(host)
	}
	if denied(res) {
		return "the network policy denied the request"
	}
	text := string(res.Stderr)
	if strings.TrimSpace(text) == "" {
		text = string(res.Stdout)
	}
	if line := truncate(conservative(lastLine(text))); line != "" {
		return line
	}
	return fmt.Sprintf("exit code %d", res.ExitCode)
}

// denied reports whether the output looks like an egress-policy refusal: HTTP 403
// (Forbidden) or 407 (proxy authentication) on a fetch.
func denied(res runner.Result) bool {
	text := string(res.Stderr) + string(res.Stdout)
	for _, marker := range []string{" 403", "403 Forbidden", "Forbidden", " 407", "Proxy Authentication Required"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// deniedHost is the host named in the output of a refused fetch, or "".
func deniedHost(res runner.Result) string {
	if !denied(res) {
		return ""
	}
	for _, field := range strings.Fields(string(res.Stderr) + " " + string(res.Stdout)) {
		field = strings.Trim(field, `"'()<>,;:`)
		if !strings.Contains(field, "://") {
			continue
		}
		if u, err := url.Parse(field); err == nil && u.Hostname() != "" {
			return sanitizeLine(u.Hostname())
		}
	}
	return ""
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := sanitizeLine(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

func truncate(s string) string {
	if len(s) <= failureLimit {
		return s
	}
	return strings.ToValidUTF8(s[:failureLimit], "") + "..."
}

// conservative keeps the child's output line to ASCII letters, digits, spaces and
// ".,:;_/@=+()-", replacing every other character with a space and collapsing runs, so
// text a launcher or the network chose cannot inject markup, quotes or instructions in
// code form into the agent's context.
func conservative(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune(".,:;_/@=+()-", r):
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func sanitizeLine(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
}

func say(errw io.Writer, msg string) {
	fmt.Fprintln(errw, prefix+sanitizeLine(msg)) //nolint:errcheck // a diagnostic line; a closed stderr changes nothing
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func writeContext(out, errw io.Writer, text string) {
	if text == "" {
		return
	}
	doc := hookOutput{hookSpecificOutput{HookEventName: "SessionStart", AdditionalContext: text}}
	data, err := json.Marshal(doc)
	if err != nil {
		say(errw, "cannot encode the context; skipped")
		return
	}
	fmt.Fprintf(out, "%s\n", data) //nolint:errcheck // stdout is the hook channel; nothing to do when it is closed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
