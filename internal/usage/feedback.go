package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
)

// EventFeedback is the event name of a feedback line.
const EventFeedback = "skill_feedback"

// FeedbackSchemaVersion is the version written to the "v" field of feedback lines.
const FeedbackSchemaVersion = 1

// Feedback kinds.
const (
	FeedbackMisled = "misled"
	FeedbackStale  = "stale"
	FeedbackWrong  = "wrong"
	FeedbackGreat  = "great"
)

// FeedbackKinds lists the accepted kinds in display order.
var FeedbackKinds = []string{FeedbackMisled, FeedbackStale, FeedbackWrong, FeedbackGreat}

// FeedbackFileName is the default feedback log name, next to the usage log.
const FeedbackFileName = "feedback.jsonl"

// notesDirName is where notes are kept, inside the directory of the feedback log.
const notesDirName = "feedback-notes"

var feedbackIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// FeedbackEntry is one feedback line. It holds identifiers only: the free text of
// a note is copied to a local file and only that file's name is recorded.
type FeedbackEntry struct {
	Version int    `json:"v"`
	Time    string `json:"ts"`
	Event   string `json:"event"`
	Skill   string `json:"skill"`
	ID      string `json:"id"`
	Hash    string `json:"hash,omitempty"`
	Kind    string `json:"kind"`
	Harness string `json:"harness,omitempty"`
	Role    string `json:"role,omitempty"`
	// Note is the name of the local note file (in feedback-notes/ beside the
	// feedback log), empty when no note was given.
	Note string `json:"note,omitempty"`
}

// FeedbackOptions configures RecordFeedback.
type FeedbackOptions struct {
	// LogPath is the feedback log. Required.
	LogPath string
	// IndexPath resolves the skill's current hash; a missing index is not an error.
	IndexPath string
	// NoteFile is a file whose text is kept as a local note.
	NoteFile string
	Harness  string
	Role     string
	// Now overrides the clock (tests).
	Now func() time.Time
}

// ValidFeedbackKind reports whether kind is accepted.
func ValidFeedbackKind(kind string) bool {
	for _, k := range FeedbackKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// RecordFeedback appends a feedback line for a skill and, when a note file is
// given, copies it to feedback-notes/ next to the log (mode 0600). The note text
// never enters the log line, the skills index, the eval results or any hash; it
// stays on this machine unless the user copies it.
func RecordFeedback(skill, kind string, options FeedbackOptions) (*FeedbackEntry, error) {
	id := skillID(strings.TrimSpace(skill))
	if !feedbackIDPattern.MatchString(id) {
		return nil, oops.Errorf("invalid skill name %q", skill)
	}
	if !ValidFeedbackKind(kind) {
		return nil, oops.Errorf("unknown feedback kind %q (use %s)", kind, strings.Join(FeedbackKinds, ", "))
	}
	if options.LogPath == "" {
		return nil, oops.Errorf("no feedback log path")
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	stamp := now().UTC()
	entry := &FeedbackEntry{
		Version: FeedbackSchemaVersion, Time: stamp.Format(time.RFC3339), Event: EventFeedback,
		Skill: strings.TrimSpace(skill), ID: id, Kind: kind, Harness: options.Harness, Role: strings.TrimSpace(options.Role),
		Hash: lookupHash(options.IndexPath, "", id),
	}
	if options.NoteFile != "" {
		note, err := keepNote(options.NoteFile, filepath.Join(filepath.Dir(options.LogPath), notesDirName), stamp, id, kind)
		if err != nil {
			return nil, err
		}
		entry.Note = note
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return nil, oops.Wrapf(err, "encode feedback entry")
	}
	if err := appendLine(options.LogPath, append(line, '\n')); err != nil {
		return nil, err
	}
	return entry, nil
}

// maxNoteBytes bounds a kept note.
const maxNoteBytes = 64 << 10

func keepNote(source, dir string, stamp time.Time, id, kind string) (string, error) {
	data, err := os.ReadFile(source) //nolint:gosec // user-chosen note file
	if err != nil {
		return "", oops.With("path", source).Wrapf(err, "read note file")
	}
	if len(data) > maxNoteBytes {
		return "", oops.With("path", source).Errorf("note is larger than %d bytes", maxNoteBytes)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", oops.Wrapf(err, "create notes directory")
	}
	name := stamp.Format("20060102T150405Z") + "-" + id + "-" + kind + ".txt"
	// O_EXCL keeps a second note in the same second from overwriting the first.
	for n := 1; ; n++ {
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // generated name inside the local notes directory
		if err == nil {
			_, writeErr := file.Write(data)
			if closeErr := file.Close(); writeErr != nil || closeErr != nil {
				return "", oops.Errorf("write note")
			}
			return name, nil
		}
		if !os.IsExist(err) || n > 100 {
			return "", oops.Wrapf(err, "write note")
		}
		name = stamp.Format("20060102T150405Z") + "-" + id + "-" + kind + "-" + string(rune('a'+n%26)) + ".txt"
	}
}

// ReadFeedback reads a feedback log; unreadable lines are counted, not fatal. A
// missing file is an error so callers can tell "no log" from "empty log".
func ReadFeedback(path string) (entries []FeedbackEntry, skipped int, err error) {
	file, err := os.Open(path) //nolint:gosec // user-chosen log path
	if err != nil {
		return nil, 0, oops.With("path", path).Wrapf(err, "open feedback log")
	}
	defer file.Close() //nolint:errcheck // read-only

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry FeedbackEntry
		if json.Unmarshal(line, &entry) != nil || entry.Event != EventFeedback || entry.ID == "" || !ValidFeedbackKind(entry.Kind) {
			skipped++
			continue
		}
		entries = append(entries, entry)
	}
	return entries, skipped, oops.Wrapf(scanner.Err(), "read feedback log")
}

// FeedbackCounts tallies feedback kinds per skill id.
func FeedbackCounts(entries []FeedbackEntry) map[string]map[string]int {
	out := map[string]map[string]int{}
	for i := range entries {
		e := &entries[i]
		if out[e.ID] == nil {
			out[e.ID] = map[string]int{}
		}
		out[e.ID][e.Kind]++
	}
	return out
}

// FeedbackText renders counts as "great 2, misled 1" in kind order.
func FeedbackText(counts map[string]int) string {
	var parts []string
	for _, kind := range FeedbackKinds {
		if n := counts[kind]; n > 0 {
			parts = append(parts, kind+" "+strconv.Itoa(n))
		}
	}
	return strings.Join(parts, ", ")
}
