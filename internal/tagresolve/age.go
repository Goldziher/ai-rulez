package tagresolve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Rule code of a tag held back by min_release_age (docs/strict-validation.md).
const (
	CodeHeldBack   = "AR733"
	CodeOutdated   = "AR734"
	maxGateLookups = 50
)

// Sources of a tag's release time, in order of trust (docs/lockfile.md).
const (
	SourceForge     = "forge"
	SourceFirstSeen = "first-seen"
	SourceCommit    = "commit"
)

// ReleaseTime is when a tag was released and where that time came from.
type ReleaseTime struct {
	At   time.Time
	From string
}

// ReleaseTimer finds the release time of a tag. It is the network- and
// store-facing part of the age gate (internal/releasetime implements it).
type ReleaseTimer interface {
	ReleaseTime(ctx context.Context, tag RawTag) (ReleaseTime, error)
}

// AgeGate holds back tags younger than Min. The zero value and Min <= 0 hold
// nothing back.
type AgeGate struct {
	Min   time.Duration
	Now   time.Time
	Timer ReleaseTimer
}

// Active reports whether the gate can hold a tag back.
func (g *AgeGate) Active() bool { return g != nil && g.Min > 0 && g.Timer != nil }

// observer is implemented by a ReleaseTimer that records the tags it sees (the
// first-seen record), so a scheduled run builds history even for tags it does not choose.
type observer interface{ Observe(tags []RawTag) }

func (g *AgeGate) observe(tags []RawTag) {
	if !g.Active() {
		return
	}
	if o, ok := g.Timer.(observer); ok {
		o.Observe(tags)
	}
}

// Held is a tag the gate held back.
type Held struct {
	Tag      string
	Commit   string
	Released time.Time
	From     string
	// AgeDays is the whole days since release (0 when the time is unknown).
	AgeDays int
	// EligibleAt is when the tag passes the gate; zero when the time is unknown.
	EligibleAt time.Time
	// Reason is set when the release time could not be found (the tag is held back, fail closed).
	Reason string
}

// MarshalJSON writes the times as RFC 3339 strings and leaves out the unknown ones.
func (h Held) MarshalJSON() ([]byte, error) {
	out := struct {
		Tag        string `json:"tag"`
		Commit     string `json:"commit,omitempty"`
		Released   string `json:"released,omitempty"`
		From       string `json:"released_from,omitempty"`
		AgeDays    int    `json:"age_days"`
		EligibleAt string `json:"eligible_at,omitempty"`
		Reason     string `json:"reason,omitempty"`
	}{Tag: h.Tag, Commit: h.Commit, From: h.From, AgeDays: h.AgeDays, Reason: h.Reason}
	if !h.Released.IsZero() {
		out.Released = h.Released.UTC().Format(time.RFC3339)
	}
	if !h.EligibleAt.IsZero() {
		out.EligibleAt = h.EligibleAt.UTC().Format(time.RFC3339)
	}
	return json.Marshal(out)
}

// String is the AR733 line of a held tag.
func (h Held) String() string {
	if h.Reason != "" {
		return fmt.Sprintf("%s %s held back: %s", CodeHeldBack, h.Tag, h.Reason)
	}
	return fmt.Sprintf("%s %s held back: released %s ago (%s)", CodeHeldBack, h.Tag, humanAge(h.AgeDays), h.From)
}

func humanAge(days int) string {
	if days <= 0 {
		return "less than a day"
	}
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// gate decides one candidate. ok=false holds it back (with the reason in held).
func (g *AgeGate) check(ctx context.Context, c Candidate) (rt ReleaseTime, held *Held) {
	rt, err := g.Timer.ReleaseTime(ctx, c.Tag)
	if err != nil {
		return rt, &Held{Tag: c.Tag.Name, Commit: c.Tag.Commit, Reason: oneLine(err.Error())}
	}
	age := g.Now.Sub(rt.At)
	if age >= g.Min {
		return rt, nil
	}
	if age < 0 {
		age = 0
	}
	return rt, &Held{Tag: c.Tag.Name, Commit: c.Tag.Commit, Released: rt.At, From: rt.From,
		AgeDays: int(age / (24 * time.Hour)), EligibleAt: rt.At.Add(g.Min)}
}
