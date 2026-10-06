package usage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// Event names written to the log.
const EventSkillInvoked = "skill_invoked"

// EventItem is the event name internal/telemetry gives rule, agent, context and
// command events written to the same log. Skill-only readers ignore it.
const EventItem = "item_event"

// EntrySchemaVersion is the version written to the "v" field of new log lines.
// Lines without it predate the field (version 1: raw session id, no outcome,
// served or role) and stay readable; readers ignore fields they do not know.
const EntrySchemaVersion = 2

// Outcomes of a skill load. The recorder knows OutcomeLoaded at the moment of
// the hook; the others are for hooks a team wires to later events (a Stop hook
// can run `usage record --outcome used`). An unknown outcome is omitted.
const (
	OutcomeLoaded    = "loaded"
	OutcomeUsed      = "used"
	OutcomeAbandoned = "abandoned"
)

// Harness names the recorder understands.
const (
	HarnessClaude = "claude"
	HarnessCodex  = "codex"
	HarnessCursor = "cursor"
)

// SaltEnv overrides the salt that session ids are hashed with.
const SaltEnv = "AI_RULEZ_USAGE_SALT"

// Hook event names and the Skill tool name, as a harness sends them on stdin.
const (
	hookPreToolUse         = "PreToolUse"
	hookUserPromptExpanded = "UserPromptExpansion"
	toolSkill              = "Skill"
)

// Entry is one usage log line. It holds identifiers only.
type Entry struct {
	// Version is EntrySchemaVersion on lines written by this release, absent on
	// older ones.
	Version int    `json:"v,omitempty"`
	Time    string `json:"ts"`
	Event   string `json:"event"`
	// Skill is the name exactly as the harness reported it (a plugin skill may
	// carry a "plugin:" prefix); ID is the index id it resolves to.
	Skill string `json:"skill"`
	ID    string `json:"id"`
	// Hash is the skill's content hash from the index at the time of use, empty
	// when the skill is not in the index or its id is ambiguous.
	Hash string `json:"hash,omitempty"`
	// Session is a salted hash of the harness session id (16 hex digits), never the
	// raw id, so sessions can be counted but not looked up. Lines written before
	// version 2 may hold a raw id.
	Session string `json:"session,omitempty"`
	// Invocation is "tool" when the model called the Skill tool and "slash" when
	// the user typed the skill's command.
	Invocation string `json:"invocation"`
	Harness    string `json:"harness"`
	// Outcome is loaded, used or abandoned when the hook knows it, else omitted.
	Outcome string `json:"outcome,omitempty"`
	// Served marks a load through the skills server (`load_skill`) rather than a
	// harness-native skill invocation; see RecordServed.
	Served bool `json:"served,omitempty"`
	// Role is the role active when the skill loaded, when the caller says.
	Role string `json:"role,omitempty"`
	// Digest is the served skill's provenance digest.
	Digest string `json:"digest,omitempty"`
	// Resource marks the load of a supporting file, not the skill's SKILL.md;
	// reports do not count it as a further use of the skill.
	Resource bool `json:"resource,omitempty"`
}

// hookEvent is the subset of a Claude Code hook event the recorder reads. Fields
// such as prompt and tool_input other than skill are deliberately not declared,
// so they can never reach the log.
type hookEvent struct {
	Name      string `json:"hook_event_name"`
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Tool      string `json:"tool_name"`
	ToolInput struct {
		Skill string `json:"skill"`
		// Command and FilePath are read only to find a skills/<id>/SKILL.md path
		// (Codex and Cursor load skills by reading the file). The text is matched and
		// discarded: only the id reaches the log.
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	} `json:"tool_input"`
	CommandName string `json:"command_name"`
}

// RecordOptions configures Record.
type RecordOptions struct {
	// LogPath is the JSON Lines file appended to. Empty disables the file sink.
	LogPath string
	// SinkCommand, when set, is run through the shell with the log line on its
	// standard input. ai-rulez itself makes no network call; what the command
	// does is the user's choice.
	SinkCommand string
	// IndexPath locates the skills index used to resolve hashes. Empty means
	// <event cwd>/.ai-rulez/skills-index.json; a missing index is not an error.
	IndexPath string
	// Harness is claude (default), codex or cursor: it decides which hook payloads
	// count as a skill load.
	Harness string
	// Outcome overrides the outcome ("loaded" by default). Unknown values are dropped.
	Outcome string
	// Served and Role are copied to the entry.
	Served bool
	Role   string
	// SaltPath is the file holding the session salt, created on first use. Empty
	// means usage.salt next to the log, or <event cwd>/.ai-rulez/local/usage.salt.
	// AI_RULEZ_USAGE_SALT overrides the file.
	SaltPath string
	// Now overrides the clock (tests).
	Now func() time.Time
}

// Record reads one hook event from in and, when it is a skill invocation,
// appends one log line to the configured sinks. It returns the entry written, or
// nil when the event was not a skill invocation.
func Record(in io.Reader, options RecordOptions) (*Entry, error) {
	data, err := io.ReadAll(io.LimitReader(in, 4<<20))
	if err != nil {
		return nil, oops.Wrapf(err, "read hook event")
	}
	var event hookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, oops.Wrapf(err, "parse hook event")
	}

	harness := options.Harness
	if harness == "" {
		harness = HarnessClaude
	}
	entry := entryFor(&event, harness)
	if entry == nil {
		return nil, nil
	}
	entry.Version = EntrySchemaVersion
	entry.Outcome = OutcomeLoaded
	if options.Outcome != "" {
		entry.Outcome = normalizeOutcome(options.Outcome)
	}
	entry.Served, entry.Role = options.Served, strings.TrimSpace(options.Role)
	entry.Session = hashedSession(event.SessionID, options, event.CWD)
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	entry.Time = now().UTC().Format(time.RFC3339)
	entry.ID = skillID(entry.Skill)
	entry.Hash = lookupHash(options.IndexPath, event.CWD, entry.ID)
	return emit(entry, options)
}

// emit encodes entry and writes it to the configured sinks.
func emit(entry *Entry, options RecordOptions) (*Entry, error) {
	line, err := json.Marshal(entry)
	if err != nil {
		return nil, oops.Wrapf(err, "encode usage entry")
	}
	line = append(line, '\n')

	// A failing sink never drops the entry: the caller still has the event (for
	// telemetry, for example) and decides what the error means.
	var errs []error
	if options.LogPath != "" {
		if err := appendLine(options.LogPath, line); err != nil {
			errs = append(errs, err)
		}
	}
	if options.SinkCommand != "" {
		if err := runSink(options.SinkCommand, line); err != nil {
			errs = append(errs, err)
		}
	}
	return entry, errors.Join(errs...)
}

func entryFor(event *hookEvent, harness string) *Entry {
	entry := &Entry{Event: EventSkillInvoked, Harness: harness}
	switch harness {
	case HarnessClaude:
		switch {
		case event.Name == hookPreToolUse && event.Tool == toolSkill && event.ToolInput.Skill != "":
			entry.Skill = strings.TrimSpace(event.ToolInput.Skill)
			entry.Invocation = "tool"
		case event.Name == hookUserPromptExpanded && event.CommandName != "":
			entry.Skill = strings.TrimSpace(strings.TrimPrefix(event.CommandName, "/"))
			entry.Invocation = "slash"
		}
	case HarnessCodex, HarnessCursor:
		// Both harnesses name the event PreToolUse (Codex) or preToolUse (Cursor) and
		// load a skill by reading its SKILL.md.
		if strings.EqualFold(event.Name, hookPreToolUse) {
			entry.Skill = skillRead(event)
			entry.Invocation = "read"
		}
	}
	if entry.Skill == "" {
		return nil
	}
	return entry
}

// readTools are the tool names of file readers; a path handed to any other tool
// (Write, Edit, ...) is not a skill load.
var readTools = map[string]bool{"read": true, "readfile": true, "read_file": true, "view": true, "open": true, "openfile": true, "open_file": true, "cat": true}

// shellReaders are the commands that only read the file they are given.
var shellReaders = map[string]bool{"cat": true, "head": true, "tail": true, "less": true, "more": true, "bat": true, "nl": true, "sed": true, "get-content": true, "type": true}

// skillRead returns the id of the skill a Codex or Cursor pre-tool-use event
// loads: a read tool opening skills/<id>/SKILL.md, or a shell command whose
// reader (cat, head, sed -n, ...) is handed that path. Writes, edits, and other
// commands that merely mention the path (git add, rm, a linter) are not loads.
func skillRead(event *hookEvent) string {
	if readTools[strings.ToLower(event.Tool)] {
		if id := skillFromPath(event.ToolInput.FilePath, event.ToolInput.Path); id != "" {
			return id
		}
	}
	if shellTools[strings.ToLower(event.Tool)] {
		return skillFromReadingCommand(event.ToolInput.Command)
	}
	return ""
}

// shellTools are the tool names whose command field is a shell command line; the
// command of any other tool (apply_patch carries a patch) is not parsed.
var shellTools = map[string]bool{"bash": true, "shell": true, "local_shell": true, "exec_command": true, "run_terminal_cmd": true}

// redirection matches an output redirect and its target ("> f", ">>f", "2>f"), so
// a skill path that is only written to is not read. Input redirects and heredocs
// ("<<EOF") are left alone.
var redirection = regexp.MustCompile(`(?:\d*|&)>>?\s*\S+`)

var shellSeparators = regexp.MustCompile(`&&|\|\||[;|\n]`)

func skillFromReadingCommand(command string) string {
	for _, segment := range shellSeparators.Split(command, -1) {
		fields := strings.Fields(segment)
		for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.ContainsAny(fields[0], "/") {
			fields = fields[1:] // leading VAR=value assignments
		}
		if len(fields) == 0 || !shellReaders[strings.ToLower(filepath.Base(fields[0]))] {
			continue
		}
		if strings.EqualFold(filepath.Base(fields[0]), "sed") && sedEditsInPlace(fields[1:]) {
			continue
		}
		if id := skillFromPath(redirection.ReplaceAllString(strings.Join(fields[1:], " "), " ")); id != "" {
			return id
		}
	}
	return ""
}

func sedEditsInPlace(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--in-place") || (strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg, "i")) {
			return true
		}
	}
	return false
}

var skillPathPattern = regexp.MustCompile(`(?:^|[\s/'"=:])skills/([a-z0-9][a-z0-9._-]*)/SKILL\.md`)

// skillFromPath returns the skill id of the first skills/<id>/SKILL.md found in
// the candidates, or "".
func skillFromPath(candidates ...string) string {
	for _, text := range candidates {
		if match := skillPathPattern.FindStringSubmatch(text); match != nil {
			return match[1]
		}
	}
	return ""
}

func normalizeOutcome(value string) string {
	switch value {
	case OutcomeLoaded, OutcomeUsed, OutcomeAbandoned:
		return value
	}
	return ""
}

// skillID strips a "plugin:" prefix: the index lists skills by their own name.
func skillID(name string) string {
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func lookupHash(indexPath, cwd, id string) string {
	path := indexPath
	if path == "" {
		if cwd == "" {
			return ""
		}
		path = DefaultIndexPath(cwd, ".ai-rulez")
	}
	index, err := LoadIndex(path)
	if err != nil {
		return ""
	}
	if matches := index.byID(id); len(matches) == 1 {
		return matches[0].Hash
	}
	return ""
}

// appendLine appends one line to the log. It refuses to write through a symlink
// at the file or at any directory below the project root: a repository can
// commit .ai-rulez, .ai-rulez/local or usage.jsonl as a symlink, and a recorder
// it launches would otherwise append JSON to whatever the link points at.
func appendLine(path string, line []byte) error {
	return safefs.AppendLine(path, line) //nolint:wrapcheck // safefs errors carry the path
}

// Limits of the sink command: it is a convenience hook that must not stall the
// harness that launched the recorder, nor flood memory.
const (
	// SinkTimeout bounds one sink command; the process group is killed after it.
	SinkTimeout = 3 * time.Second
	// maxSinkOutput caps the captured stdout and stderr of a sink command.
	maxSinkOutput = 64 << 10
)

// sinkTimeout is SinkTimeout; tests shorten it.
var sinkTimeout = SinkTimeout

func runSink(command string, line []byte) error {
	argv := []string{"sh", "-c", command}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", command}
	}
	// A sink that runs git must not inherit a hook's repository selection.
	res := runner.Run(context.Background(), runner.Spec{
		Argv: argv, Env: gitutil.Env(nil), Stdin: line, Timeout: sinkTimeout, MaxOutput: maxSinkOutput,
	})
	if res.Status == runner.StatusOK {
		return nil
	}
	output := strings.TrimSpace(string(res.Stderr) + string(res.Stdout))
	switch res.Status {
	case runner.StatusTimeout:
		return oops.With("output", output).Errorf("usage sink command timed out after %s and was killed", sinkTimeout)
	case runner.StatusExit:
		return oops.With("output", output, "exit", res.ExitCode).Errorf("usage sink command failed")
	default:
		return oops.With("output", output).Wrapf(res.Err, "run usage sink command")
	}
}
