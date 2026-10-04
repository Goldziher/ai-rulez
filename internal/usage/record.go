package usage

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/samber/oops"
)

// Event names written to the log.
const EventSkillInvoked = "skill_invoked"

// Hook event names and the Skill tool name, as a harness sends them on stdin.
const (
	hookPreToolUse         = "PreToolUse"
	hookUserPromptExpanded = "UserPromptExpansion"
	toolSkill              = "Skill"
)

// Entry is one usage log line. It holds identifiers only.
type Entry struct {
	Time  string `json:"ts"`
	Event string `json:"event"`
	// Skill is the name exactly as the harness reported it (a plugin skill may
	// carry a "plugin:" prefix); ID is the index id it resolves to.
	Skill string `json:"skill"`
	ID    string `json:"id"`
	// Hash is the skill's content hash from the index at the time of use, empty
	// when the skill is not in the index or its id is ambiguous.
	Hash    string `json:"hash,omitempty"`
	Session string `json:"session,omitempty"`
	// Invocation is "tool" when the model called the Skill tool and "slash" when
	// the user typed the skill's command.
	Invocation string `json:"invocation"`
	Harness    string `json:"harness"`
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

	entry := entryFor(&event)
	if entry == nil {
		return nil, nil
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	entry.Time = now().UTC().Format(time.RFC3339)
	entry.ID = skillID(entry.Skill)
	entry.Hash = lookupHash(options.IndexPath, event.CWD, entry.ID)

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

func entryFor(event *hookEvent) *Entry {
	entry := &Entry{Event: EventSkillInvoked, Session: event.SessionID, Harness: "claude"}
	switch {
	case event.Name == hookPreToolUse && event.Tool == toolSkill && event.ToolInput.Skill != "":
		entry.Skill = strings.TrimSpace(event.ToolInput.Skill)
		entry.Invocation = "tool"
	case event.Name == hookUserPromptExpanded && event.CommandName != "":
		entry.Skill = strings.TrimSpace(strings.TrimPrefix(event.CommandName, "/"))
		entry.Invocation = "slash"
	default:
		return nil
	}
	if entry.Skill == "" {
		return nil
	}
	return entry
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
	if output, err := cmd.CombinedOutput(); err != nil {
		return oops.With("output", strings.TrimSpace(string(output))).Wrapf(err, "run usage sink command")
	}
	return nil
}
