package doctor

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// CheckSessionStart is the check of the [session_start] hook.
const CheckSessionStart = "session-start"

// npmRegistryURL is what the remote-session probe asks: the registry the npx launcher fetches ai-rulez from.
const npmRegistryURL = "https://registry.npmjs.org/ai-rulez"

// probeTimeout bounds the registry probe.
const probeTimeout = 5 * time.Second

// checkSessionStart: the SessionStart hook generate adds can work. It lists the
// launchers in the order the hook tries them with the version each pins, warns when the
// committed settings do not carry the hook (a fresh clone would not get it), and, inside a
// remote session, warns when none of ai-rulez, npx and uvx is on PATH or the npm registry
// is not reachable through the egress policy.
func checkSessionStart(ctx context.Context, s *state) []Finding {
	if s.cfg == nil || !s.cfg.HasSessionStart() {
		return nil
	}
	launchers := s.cfg.SessionStartLaunchers()
	steps := make([]string, len(launchers))
	for i, l := range launchers {
		steps[i] = l.Command
	}
	out := []Finding{{
		Check: CheckSessionStart, Severity: SeverityInfo,
		Message: "the SessionStart hook tries, in order: " + strings.Join(steps, "; then "),
	}}
	out = append(out, sessionStartCommitted(ctx, s)...)
	if remote := ambient.Getenv(nil, config.SessionStartRemoteEnv); remote == "true" || remote == "1" {
		out = append(out, sessionStartRemote(ctx, s)...)
	}
	return out
}

// sessionStartCommitted checks that .claude/settings.json holds the hook and that git does not ignore it.
func sessionStartCommitted(ctx context.Context, s *state) []Finding {
	const rel = ".claude/settings.json"
	path := filepath.Join(s.cfg.BaseDir, filepath.FromSlash(rel))
	data, err := os.ReadFile(path) //nolint:gosec // a fixed path inside the project
	if err != nil || !strings.Contains(string(data), "session-start") {
		return []Finding{{
			Check: CheckSessionStart, Severity: SeverityWarning, Path: rel,
			Message: "the SessionStart hook is not in the generated settings, so a fresh clone does not regenerate its outputs",
			Hint:    "run `ai-rulez generate` and commit " + rel,
		}}
	}
	run := s.opts.Runner
	if run == nil {
		run = runner.Exec{}
	}
	res := run.Run(ctx, runner.Spec{
		Argv: []string{"git", "check-ignore", "-q", rel}, Dir: s.cfg.BaseDir,
		Env: runner.ScrubEnv(runner.HostEnv(), nil, nil), Timeout: probeTimeout, ShortLived: true,
	})
	if res.Status == runner.StatusOK { // exit 0: the path is ignored
		return []Finding{{
			Check: CheckSessionStart, Severity: SeverityWarning, Path: rel,
			Message: rel + " is ignored by git, so a remote session's fresh clone has no SessionStart hook",
			Hint:    "commit the generated settings (do not ignore .claude/settings.json), or set `[session_start] generate = false`",
		}}
	}
	return nil
}

// sessionStartRemote checks what the hook needs inside a remote session.
func sessionStartRemote(ctx context.Context, s *state) []Finding {
	look := s.opts.LookPath
	if _, err := look("ai-rulez"); err == nil {
		return nil
	}
	_, npxErr := look("npx")
	_, uvxErr := look("uvx")
	if npxErr != nil && uvxErr != nil {
		return []Finding{{
			Check: CheckSessionStart, Severity: SeverityWarning,
			Message: "none of ai-rulez, npx and uvx is on PATH, so the SessionStart hook cannot run here",
			Hint:    "install ai-rulez in the environment's setup script, or add node or uv to the environment",
		}}
	}
	if npxErr != nil || s.opts.Probe == nil {
		return nil
	}
	status, err := s.opts.Probe(ctx, npmRegistryURL)
	host := npmRegistryURL
	if u, perr := url.Parse(npmRegistryURL); perr == nil {
		host = u.Hostname()
	}
	switch {
	case status == 403 || status == 407:
		return []Finding{{
			Check: CheckSessionStart, Severity: SeverityWarning,
			Message: fmt.Sprintf("the network policy denied %s (HTTP %d), so npx cannot fetch ai-rulez for the SessionStart hook", host, status),
			Hint:    "allow " + host + " in the environment's network policy, or install ai-rulez in its setup script",
		}}
	case err != nil:
		return []Finding{{
			Check: CheckSessionStart, Severity: SeverityWarning,
			Message: fmt.Sprintf("%s is not reachable (%s), so npx cannot fetch ai-rulez for the SessionStart hook", host, firstLine(err.Error())),
			Hint:    "allow " + host + " in the environment's network policy, or install ai-rulez in its setup script",
		}}
	}
	return nil
}
