package usage

import (
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// ServedLoad describes one load through the skills server. Like every usage
// entry it holds identifiers only: no arguments, no file contents.
type ServedLoad struct {
	Skill  string
	Digest string
	// Session is the MCP session id; it is logged as a salted hash, like a hook's.
	Session string
	Harness string
	// Role is the role the server resolved for the session, when there is one.
	Role     string
	Resource bool
}

// InvocationMCP is the Invocation of a served load.
const InvocationMCP = "mcp"

// RecordServed logs one `load_skill` through the same sinks and entry format as
// the hook recorder, with served=true. The content hash and canonical digest
// come from the skills index when the skill is in it; without a digest there the
// served load's own provenance digest is logged under its own scheme. With neither a log path nor a sink command in
// options nothing is written (the recorder stays off until opted into).
func RecordServed(load ServedLoad, options RecordOptions) (*Entry, error) {
	now := ambient.Clock(nil).Now
	if options.Now != nil {
		now = options.Now
	}
	harness := strings.TrimSpace(load.Harness)
	if harness == "" {
		harness = "mcp"
	}
	entry := &Entry{
		Time:       now().UTC().Format(time.RFC3339),
		Event:      EventSkillInvoked,
		Skill:      load.Skill,
		ID:         skillID(load.Skill),
		Version:    EntrySchemaVersion,
		Outcome:    OutcomeLoaded,
		Role:       strings.TrimSpace(load.Role),
		Invocation: InvocationMCP,
		Harness:    harness,
		Served:     true,
		Resource:   load.Resource,
	}
	entry.Session = hashedSession(load.Session, options, "")
	entry.Hash, entry.Digest = lookupIdentity(options.IndexPath, "", entry.ID)
	switch {
	case entry.Digest != "":
		entry.DigestScheme = DigestSchemeSkill // the index's canonical digest joins with eval results
	case load.Digest != "":
		entry.Digest, entry.DigestScheme = load.Digest, DigestSchemeServed
	}
	entry.EventID = newEventID(loadSalt(saltPathFor(options, "")), entry, options)
	if options.LogPath == "" && options.SinkCommand == "" {
		return entry, nil
	}
	return emit(entry, options)
}
