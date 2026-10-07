package usage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
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
// served or role) and stay readable, as do version 2 lines (no digest scheme or
// event id); readers ignore fields they do not know. Version 3 adds digest,
// digest_scheme and event_id.
const EntrySchemaVersion = 3

// Digest schemes a log line can name in digest_scheme. Only DigestSchemeSkill is
// the canonical skill digest the lock and the eval store share; a digest in any
// other scheme (or none) joins by skill id only.
const (
	DigestSchemeSkill  = "ai-rulez/skill/v1"
	DigestSchemeServed = "ai-rulez/served-skill/v1"
)

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
	// Digest is the skill's content digest at the time of use, named by
	// DigestScheme: the lock's canonical skill digest when the skills index has
	// one, the served skill's provenance digest for a served load without it.
	Digest string `json:"digest,omitempty"`
	// DigestScheme names how Digest was computed (DigestSchemeSkill or
	// DigestSchemeServed); empty on lines written before version 3.
	DigestScheme string `json:"digest_scheme,omitempty"`
	// EventID de-duplicates a line that appears twice (a replayed spool, two merged
	// copies of one log): the id is derived once, when the line is written, and
	// travels with the line. It is not a function of the event content: two real
	// loads of one skill in one second differ by a random nonce, so re-firing a hook
	// is not deduplicated. 16 hex digits of a salted hash, so it does not link
	// events across machines. Empty before version 3.
	EventID string `json:"event_id,omitempty"`
	// Resource marks the load of a supporting file, not the skill's SKILL.md;
	// reports do not count it as a further use of the skill. Only RecordServed
	// sets it: Record sees harness hooks and logs SKILL.md reads alone.
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
	// AsyncSink, when set, delivers the sink line in the background instead of
	// running SinkCommand inline (a long-running server must not wait for it).
	AsyncSink *AsyncSink
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
	// Nonce overrides the random part of the event id (tests).
	Nonce func() string
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
	now := ambient.Clock(nil).Now
	if options.Now != nil {
		now = options.Now
	}
	entry.Time = now().UTC().Format(time.RFC3339)
	entry.ID = skillID(entry.Skill)
	entry.Hash, entry.Digest = lookupIdentity(options.IndexPath, event.CWD, entry.ID)
	if entry.Digest != "" {
		entry.DigestScheme = DigestSchemeSkill
	}
	entry.EventID = newEventID(loadSalt(saltPathFor(options, event.CWD)), entry, options)
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
	switch {
	case options.AsyncSink != nil:
		options.AsyncSink.Send(line)
	case options.SinkCommand != "":
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

// splitShell splits a command line at ;, |, && and newlines that are outside
// quotes (a lone & is a redirect like 2>&1 or a background marker, not a separator), so text inside a quoted argument (a commit message) never
// starts a command of its own.
func splitShell(command string) []string {
	var (
		segments []string
		cur      strings.Builder
		quote    rune
		escaped  bool
	)
	flush := func() {
		segments = append(segments, cur.String())
		cur.Reset()
	}
	runes := []rune(command)
	for i, r := range runes {
		isAnd := r == '&' && ((i+1 < len(runes) && runes[i+1] == '&') || (i > 0 && runes[i-1] == '&'))
		switch {
		case escaped:
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ';' || r == '|' || r == '\n' || isAnd:
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return segments
}

func skillFromReadingCommand(command string) string {
	for _, segment := range splitShell(command) {
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

// skillPathPattern accepts / and \ as separators: Codex and Cursor on Windows pass backslash paths.
var skillPathPattern = regexp.MustCompile(`(?:^|[\s/\\'"=:])skills[/\\]([a-z0-9][a-z0-9._-]*)[/\\]SKILL\.md`)

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

// lookupIdentity returns the index's blake3 hash and canonical skill digest of
// a skill, empty when the skill is not in the index or its id is ambiguous. An
// index written before digests existed has the hash only.
func lookupIdentity(indexPath, cwd, id string) (hash, digest string) {
	path := indexPath
	if path == "" {
		if cwd == "" {
			return "", ""
		}
		path = DefaultIndexPath(cwd, ".ai-rulez")
	}
	index, err := LoadIndex(path)
	if err != nil {
		return "", ""
	}
	if matches := index.byID(id); len(matches) == 1 {
		return matches[0].Hash, matches[0].Digest
	}
	return "", ""
}

// randomNonce separates two loads of one skill in the same second.
func randomNonce() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(ambient.Clock(nil).Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw)
}

// newEventID derives the event id: the first 16 hex digits of
// sha256(salt, ts, id, invocation, session, nonce). The nonce is random, so the id
// is unique per recorded load; a copy of the written line keeps it. The salt is
// the machine's, so the id links nothing across machines.
func newEventID(salt string, entry *Entry, options RecordOptions) string {
	nonce := randomNonce
	if options.Nonce != nil {
		nonce = options.Nonce
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{salt, entry.Time, entry.ID, entry.Invocation, entry.Session, nonce()}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// appendLine appends one line to the log. It refuses to write through a symlink
// at the file or at any directory below the project root: a repository can
// commit .ai-rulez, .ai-rulez/local or usage.jsonl as a symlink, and a recorder
// it launches would otherwise append JSON to whatever the link points at.
//
// It holds the shared log lock while it writes, so a prune never replaces the file between the write and its
// own re-read. The lock is best effort: when it cannot be had in appendLockWait (a prune of a huge log, a
// refused lock path) the line is appended anyway, because a hook must never stall or drop its event.
func appendLine(path string, line []byte) error {
	return AppendLogLine(path, line)
}

// AppendLogLine appends one complete line to the usage log at path under the
// log's shared lock, the way every recorder of that log must (see appendLine):
// the telemetry JSONL emitter writes the same file.
func AppendLogLine(path string, line []byte) error {
	if release, err := lockLog(path, false, appendLockWait); err == nil {
		defer release()
	}
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
