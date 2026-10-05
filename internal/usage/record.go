package usage

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
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

	if options.LogPath != "" {
		if err := appendLine(options.LogPath, line); err != nil {
			return nil, err
		}
	}
	if options.SinkCommand != "" {
		if err := runSink(options.SinkCommand, line); err != nil {
			return nil, err
		}
	}
	return entry, nil
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
			entry.Skill = skillFromPath(event.ToolInput.Command, event.ToolInput.FilePath, event.ToolInput.Path)
			entry.Invocation = "read"
		}
	}
	if entry.Skill == "" {
		return nil
	}
	return entry
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

func appendLine(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return oops.With("path", path).Wrapf(err, "create usage log directory")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // user-chosen log path
	if err != nil {
		return oops.With("path", path).Wrapf(err, "open usage log")
	}
	// One write of a whole line keeps concurrent sessions from interleaving.
	if _, err := file.Write(line); err != nil {
		_ = file.Close() //nolint:errcheck // the write error is the one to report
		return oops.With("path", path).Wrapf(err, "write usage log")
	}
	return oops.Wrapf(file.Close(), "close usage log")
}

func runSink(command string, line []byte) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command) //nolint:gosec // the user configured this command
	} else {
		cmd = exec.Command("sh", "-c", command) //nolint:gosec // the user configured this command
	}
	cmd.Stdin = bytes.NewReader(line)
	// A sink that runs git must not inherit a hook's repository selection.
	cmd.Env = gitutil.Env(nil)
	if output, err := cmd.CombinedOutput(); err != nil {
		return oops.With("output", strings.TrimSpace(string(output))).Wrapf(err, "run usage sink command")
	}
	return nil
}
