package usage

import (
	"strings"
	"time"
)

// ServedLoad describes one load through the skills server. Like every usage
// entry it holds identifiers only: no arguments, no file contents.
type ServedLoad struct {
	Skill    string
	Digest   string
	Session  string
	Harness  string
	Resource bool
}

// InvocationMCP is the Invocation of a served load.
const InvocationMCP = "mcp"

// RecordServed logs one `load_skill` through the same sinks and entry format as
// the hook recorder, with served=true. The content hash comes from the skills
// index when the skill is in it. With neither a log path nor a sink command in
// options nothing is written (the recorder stays off until opted into).
func RecordServed(load ServedLoad, options RecordOptions) (*Entry, error) {
	now := time.Now
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
		Session:    load.Session,
		Invocation: InvocationMCP,
		Harness:    harness,
		Served:     true,
		Digest:     load.Digest,
		Resource:   load.Resource,
	}
	entry.Hash = lookupHash(options.IndexPath, "", entry.ID)
	if options.LogPath == "" && options.SinkCommand == "" {
		return entry, nil
	}
	return emit(entry, options)
}
