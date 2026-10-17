package config

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/samber/oops"
)

// SessionStartConfig configures the built-in SessionStart hook: in a remote
// (cloud) agent session it runs `ai-rulez session-start`, which regenerates the
// outputs of the fresh clone and tells the agent how the configuration is managed.
// It is a table of its own because `[hooks]` cannot coexist with the `[[hooks]]`
// array of tables in TOML.
type SessionStartConfig struct {
	// Generate, when false, removes the hook (and with it the prompt). Nil means on.
	Generate *bool `yaml:"generate,omitempty" json:"generate,omitempty" toml:"generate,omitempty"`
	// When is "remote" (default: act only when CLAUDE_CODE_REMOTE is set) or "always".
	When string `yaml:"when,omitempty" json:"when,omitempty" toml:"when,omitempty"`
	// Mode is "auto" (default: --locked when ai-rulez.lock exists, else plain),
	// "locked", "frozen" or "plain".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" toml:"mode,omitempty"`
	// Timeout bounds the whole run, in seconds. Zero means 120.
	Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty" toml:"timeout,omitempty"`
	// Version pins the ai-rulez version the hook runs through npx and uvx. Empty
	// means the latest published release.
	Version string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	// Command replaces the whole launcher (executable followed by its arguments,
	// without the trailing "session-start"), for example ["ai-rulez"]. It runs
	// without a shell. Version is ignored when it is set.
	Command []string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	// Prompt, when false, leaves the context injection out. Nil means on.
	Prompt *bool `yaml:"prompt,omitempty" json:"prompt,omitempty" toml:"prompt,omitempty"`
	// PromptText replaces the default context. {{server}} is the ai-rulez MCP server's name.
	PromptText string `yaml:"prompt_text,omitempty" json:"prompt_text,omitempty" toml:"prompt_text,omitempty"`
}

// Values of [session_start] when and mode.
const (
	SessionStartWhenRemote = "remote"
	SessionStartWhenAlways = "always"
	SessionStartModeAuto   = "auto"
	SessionStartModeLocked = "locked"
	SessionStartModeFrozen = "frozen"
	SessionStartModePlain  = "plain"
)

// HookBuiltinSessionStart marks the HookGroup that generation synthesizes for [session_start].
const HookBuiltinSessionStart = "session-start"

const (
	hookEventSessionStart = "SessionStart"
	// SessionStartRemoteEnv is the variable Claude Code sets to "true" in a remote (cloud) session.
	SessionStartRemoteEnv = "CLAUDE_CODE_REMOTE"
	// SessionStartDefaultTimeoutSeconds bounds a session-start run unless [session_start] timeout says otherwise.
	SessionStartDefaultTimeoutSeconds = 120
	// sessionStartMaxTimeoutSeconds is the longest accepted timeout.
	sessionStartMaxTimeoutSeconds = 900
	// sessionStartHookSlackSeconds is added to the run timeout for the harness-level hook timeout, so the
	// command's own bound fires first.
	sessionStartHookSlackSeconds = 30
	// SessionStartContextLimit caps the context injected into the agent, in bytes.
	SessionStartContextLimit = 2048
	sessionStartSubcommand   = "session-start"
	// sessionStartLauncherPath names the launcher that runs an installed ai-rulez.
	sessionStartLauncherPath = "path"
	sessionStartFailureLimit = 200
)

// SessionStartHarnesses lists the harnesses that get the hook. CLAUDE_CODE_REMOTE is
// set by Claude Code only, and only Claude Code documents SessionStart output
// (hookSpecificOutput.additionalContext) as session context; another harness would
// run a command that never does anything, so it is skipped, silently.
func SessionStartHarnesses() []string { return []string{HarnessClaude} }

// validLauncherVersion accepts a bare version or dist-tag: letters, digits, '.', '_' and '-',
// starting with a letter or digit, so it cannot carry shell syntax.
func validLauncherVersion(v string) bool {
	if v == "" || !unicode.IsLetter(rune(v[0])) && !unicode.IsDigit(rune(v[0])) {
		return false
	}
	for _, r := range v {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' && r != '-') {
			return false
		}
	}
	return true
}

// GenerateOn reports whether the hook is on; it is unless generate = false.
func (s *SessionStartConfig) GenerateOn() bool { return s == nil || s.Generate == nil || *s.Generate }

// PromptOn reports whether the context injection is on; it is unless prompt = false.
func (s *SessionStartConfig) PromptOn() bool { return s == nil || s.Prompt == nil || *s.Prompt }

// WhenOrDefault is the effective [session_start] when.
func (s *SessionStartConfig) WhenOrDefault() string {
	if s == nil || s.When == "" {
		return SessionStartWhenRemote
	}
	return s.When
}

// ModeOrDefault is the effective [session_start] mode.
func (s *SessionStartConfig) ModeOrDefault() string {
	if s == nil || s.Mode == "" {
		return SessionStartModeAuto
	}
	return s.Mode
}

// TimeoutSecondsOrDefault is the effective run timeout in seconds.
func (s *SessionStartConfig) TimeoutSecondsOrDefault() int {
	if s == nil || s.Timeout <= 0 {
		return SessionStartDefaultTimeoutSeconds
	}
	return s.Timeout
}

// resolveLauncherVersion is the version an npx or uvx launch pins: the configured
// one, else the running binary's, and "latest" for a dev build. The self MCP entry
// and the guard share it; the session-start hook does not (see SessionStartLaunchers).
func resolveLauncherVersion(configured, binary string) string {
	for _, v := range []string{configured, binary} {
		if v != "" && v != "dev" {
			return v
		}
	}
	return "latest"
}

// SessionStartLauncher is one way the hook starts ai-rulez, in the order it tries them.
type SessionStartLauncher struct {
	Name    string // path, npx or uvx
	Command string // the command line, with the version it pins
}

// SessionStartLaunchers lists the launchers the hook tries, in order: an installed
// ai-rulez, then npx, then uvx, the last two on the latest published release unless
// [session_start] version pins one. A configured command is the only launcher.
func (c *Config) SessionStartLaunchers() []SessionStartLauncher {
	s := c.SessionStart
	if s != nil && len(s.Command) > 0 {
		return []SessionStartLauncher{{Name: ItemKindCommand, Command: shellJoinArgs(append(slices.Clone(s.Command), sessionStartSubcommand))}}
	}
	npxSpec, uvxSpec := "ai-rulez@latest", "ai-rulez@latest"
	if s != nil && s.Version != "" {
		npxSpec, uvxSpec = "ai-rulez@"+s.Version, "ai-rulez=="+s.Version
	}
	return []SessionStartLauncher{
		{Name: sessionStartLauncherPath, Command: "ai-rulez " + sessionStartSubcommand},
		{Name: SelfLauncherNPX, Command: "npx -y " + npxSpec + " " + sessionStartSubcommand},
		{Name: "uvx", Command: "uvx --from " + uvxSpec + " ai-rulez " + sessionStartSubcommand},
	}
}

// remoteGate exits the hook unless the harness says the session is remote.
const remoteGate = `case "${CLAUDE_CODE_REMOTE:-}" in 1|[Tt][Rr][Uu][Ee]) ;; *) exit 0;; esac; `

// neutralRun is the shell function that runs the npx and uvx launchers from an empty,
// freshly created private directory (created under TMPDIR, with an explicit template because BSD mktemp -d ignores TMPDIR), so a project's .npmrc,
// node_modules/.bin or uv configuration cannot substitute the package that runs. The
// installed-binary attempt stays in the project directory. CLAUDE_PROJECT_DIR is
// exported first, so `ai-rulez session-start` still finds the project after the cd.
const neutralRun = `export CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$PWD}"; ` +
	`n() { d=$(mktemp -d "${TMPDIR:-/tmp}/ai-rulez.XXXXXX" 2>/dev/null) || d=/; (cd "$d" 2>/dev/null || exit 1; "$@"); r=$?; ` +
	`if [ "$d" != / ]; then rmdir "$d" 2>/dev/null; fi; return $r; }; `

// SessionStartCommand is the shell command of the session-start hook. The script
// (no single quotes in it) tries an installed ai-rulez, then npx, then uvx (the
// latter two on the latest release unless pinned, and from a neutral directory);
// an attempt that fails to run falls through to the next, and when none runs it
// prints one line and exits 0, so it never fails a session. With when = "remote"
// it returns at once unless CLAUDE_CODE_REMOTE is set, before npx has a chance to
// download anything on a developer's machine.
func (c *Config) SessionStartCommand() string {
	launchers := c.SessionStartLaunchers()
	if len(launchers) == 1 {
		return launchers[0].Command
	}
	var b strings.Builder
	if c.SessionStart.WhenOrDefault() == SessionStartWhenRemote {
		b.WriteString(remoteGate)
	}
	b.WriteString(neutralRun)
	for _, l := range launchers {
		bin, _, _ := strings.Cut(l.Command, " ")
		run := l.Command
		if l.Name != sessionStartLauncherPath {
			run = "n " + l.Command
		}
		fmt.Fprintf(&b, "if command -v %s >/dev/null 2>&1 && %s; then exit 0; fi; ", bin, run)
	}
	b.WriteString(`echo "ai-rulez session-start: no ai-rulez, npx or uvx could run; skipped" >&2; exit 0`)
	return "sh -c '" + b.String() + "'"
}

// HasSessionStart reports whether generation should add the session-start hook:
// it is on by default for a project that generates for Claude Code, unless a
// [[hooks]] group of the user already runs `ai-rulez session-start` (the user's wins).
func (c *Config) HasSessionStart() bool {
	if c == nil || c.UserScope || !c.SessionStart.GenerateOn() {
		return false
	}
	if !c.generatesFor(SessionStartHarnesses()) {
		return false
	}
	return !c.userRunsSessionStart()
}

func (c *Config) generatesFor(harnesses []string) bool {
	for i := range c.Presets {
		if slices.Contains(harnesses, c.Presets[i].GetName()) {
			return true
		}
	}
	return false
}

// userRunsSessionStart reports whether a hand-written SessionStart hook already runs
// `ai-rulez session-start`, however it launches it.
func (c *Config) userRunsSessionStart() bool {
	for _, g := range c.UserHooks() {
		if g.Event != hookEventSessionStart {
			continue
		}
		for _, a := range g.Hooks {
			text := strings.Join(append([]string{a.Command}, a.Args...), " ")
			if strings.Contains(text, "ai-rulez") && strings.Contains(text, sessionStartSubcommand) {
				return true
			}
		}
	}
	return false
}

// EnableSessionStartHooks adds the synthesized session-start group to Hooks. It is
// idempotent, so every generator built from the same Config shares one group, and the
// group is never written back to config.toml.
func (c *Config) EnableSessionStartHooks() {
	if !c.HasSessionStart() {
		return
	}
	guardMu.Lock()
	defer guardMu.Unlock()
	for i := range c.Hooks {
		if c.Hooks[i].Builtin == HookBuiltinSessionStart {
			return
		}
	}
	c.Hooks = append(c.Hooks, HookGroup{
		Event:   hookEventSessionStart,
		Targets: SessionStartHarnesses(),
		Builtin: HookBuiltinSessionStart,
		Hooks: []HookAction{{
			Type:    HookTypeCommand,
			Command: c.SessionStartCommand(),
			Timeout: c.SessionStart.TimeoutSecondsOrDefault() + sessionStartHookSlackSeconds,
		}},
	})
}

// DropSessionStartHooks removes the synthesized session-start group; user scope calls
// it once the scope is known, since the hook belongs to a project's clone.
func (c *Config) DropSessionStartHooks() {
	if c == nil {
		return
	}
	guardMu.Lock()
	defer guardMu.Unlock()
	c.Hooks = slices.DeleteFunc(c.Hooks, func(g HookGroup) bool { return g.Builtin == HookBuiltinSessionStart })
}

func shellJoinArgs(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		parts[i] = shellQuoteArg(arg)
	}
	return strings.Join(parts, " ")
}

// SessionStartContext is the text the hook injects into the agent's context. Every
// part is derived from the configuration: the generated files and where their sources
// are, the MCP server (only when it is enabled) and the served-skills section
// (the one [mcp] skills_instructions writes into the instruction files). failure,
// when not empty, is the one-line reason the generate step failed. The result is
// deterministic, free of control characters and at most SessionStartContextLimit bytes.
func (c *Config) SessionStartContext(failure string) string {
	var parts []string
	server := c.skillsServerName()
	dir := c.skillsInstructionsDirName()
	if text := strings.TrimSpace(c.sessionStartPromptText()); text != "" {
		parts = append(parts, expandTemplateVars(text, map[string]string{varServer: server}))
	} else {
		parts = append(parts, fmt.Sprintf("ai-rulez manages this repository's agent configuration. The rules, skills, agents and commands "+
			"in the harness folders, AGENTS.md and CLAUDE.md are generated from the sources in `%s/`; a change belongs in the "+
			"source, then `ai-rulez generate` renders it, and an edit to a generated file is overwritten.", dir))
		if c.HasSelfServer() {
			parts = append(parts, fmt.Sprintf("The %#q MCP server offers tools to create, update, validate and generate this configuration.", server))
		}
		if text, show := c.SkillsInstructions(); show {
			parts = append(parts, text)
		}
	}
	if failure = sanitizeContext(strings.Join(strings.Fields(failure), " "), sessionStartFailureLimit); failure != "" {
		parts = append(parts, "The `ai-rulez generate` run at session start failed ("+failure+"), so generated files may be out of date.")
	}
	return sanitizeContext(strings.Join(parts, "\n\n"), SessionStartContextLimit)
}

func (c *Config) sessionStartPromptText() string {
	if c.SessionStart == nil || !c.SessionStart.PromptOn() {
		return ""
	}
	return c.SessionStart.PromptText
}

// sanitizeContext replaces control characters other than newlines with a space and
// cuts s to at most limit bytes on a rune boundary.
func sanitizeContext(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return ' '
	}, s)
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// validateSessionStart rejects values that cannot form a hook, and tuning keys
// that generate = false would leave silently inert.
func (c *Config) validateSessionStart() error {
	s := c.SessionStart
	if s == nil {
		return nil
	}
	switch s.WhenOrDefault() {
	case SessionStartWhenRemote, SessionStartWhenAlways:
	default:
		return oops.Hint(`Use "remote" or "always"`).Errorf("invalid session_start.when %q", s.When)
	}
	switch s.ModeOrDefault() {
	case SessionStartModeAuto, SessionStartModeLocked, SessionStartModeFrozen, SessionStartModePlain:
	default:
		return oops.Hint(`Use "auto", "locked", "frozen" or "plain"`).Errorf("invalid session_start.mode %q", s.Mode)
	}
	if s.Timeout < 0 || s.Timeout > sessionStartMaxTimeoutSeconds {
		return oops.Hint(fmt.Sprintf("Use 1 to %d seconds, or omit it for %d", sessionStartMaxTimeoutSeconds, SessionStartDefaultTimeoutSeconds)).
			Errorf("session_start.timeout %d is out of range", s.Timeout)
	}
	if !s.GenerateOn() && (s.When != "" || s.Mode != "" || s.Timeout != 0 || s.Version != "" || len(s.Command) > 0 || s.Prompt != nil || s.PromptText != "") {
		return oops.Hint("Set `generate = true` under [session_start], or remove the other keys").
			Errorf("session_start keys other than generate have no effect while session_start.generate = false")
	}
	return s.validateLaunch()
}

func (s *SessionStartConfig) validateLaunch() error {
	if len(s.Command) > 0 && strings.TrimSpace(s.Command[0]) == "" {
		return oops.Hint(`The first element of session_start.command is the executable, e.g. ["ai-rulez"]`).
			Errorf("session_start.command executable must not be empty")
	}
	if len(s.Command) > 0 && s.Version != "" {
		return oops.Hint("session_start.command replaces the whole launcher, so a pinned version has no effect; remove one of them").
			Errorf("session_start.version and session_start.command are mutually exclusive")
	}
	if s.Version != "" && !validLauncherVersion(s.Version) {
		return oops.Hint(`Use a bare version or npm dist-tag such as "5.4.1" or "latest"`).
			Errorf("session_start.version %q is not a valid version", s.Version)
	}
	if s.PromptText == "" {
		return nil
	}
	if !s.PromptOn() {
		return oops.Hint("Remove `prompt = false`, or remove prompt_text").Errorf("session_start.prompt_text requires session_start.prompt to be on")
	}
	if strings.TrimSpace(s.PromptText) == "" {
		return oops.Hint("Write the text, or remove the key to use the default").Errorf("session_start.prompt_text must not be blank")
	}
	if unknown := unknownTemplateVars(s.PromptText, varServer); len(unknown) > 0 {
		return oops.Hint("Available variable: {{server}}").Errorf("session_start.prompt_text uses unknown variable {{%s}}", unknown[0])
	}
	return nil
}
