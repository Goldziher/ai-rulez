// Package tagresolve resolves a version constraint against the semantic-version
// tags of a git repository: it lists the remote's tags (annotated tags peeled to
// their commit), picks the highest tag a constraint allows, and detects a pinned
// tag that moved or vanished. It never fetches content and never writes the
// lock; callers pin the commit it returns.
package tagresolve

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/samber/oops"
)

// Rule codes of the semver family (docs/strict-validation.md, block AR730-AR739).
const (
	CodeUnsatisfiable   = "AR730"
	CodeConstraintBad   = "AR731"
	CodeTagMoved        = "AR732"
	CodeLockedTagMissed = "AR735"
)

// Error is a failure with a rule code, so callers and tests can tell the cases apart.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + " " + e.Msg }

func errorf(code, format string, args ...any) error {
	return oops.Wrap(&Error{Code: code, Msg: fmt.Sprintf(format, args...)})
}

// RawTag is one tag of a remote.
type RawTag struct {
	Name string
	// Object is what refs/tags/<name> points to: the tag object of an annotated
	// tag, the commit of a lightweight one.
	Object string
	// Commit is the commit the tag resolves to (the peeled object).
	Commit string
}

// Annotated reports whether the tag is an annotated tag object.
func (t RawTag) Annotated() bool { return t.Object != "" && t.Object != t.Commit }

// TagObject is the annotated tag object id, "" for a lightweight tag.
func (t RawTag) TagObject() string {
	if t.Annotated() {
		return t.Object
	}
	return ""
}

// Runner runs one git command and returns its standard output. The caller
// supplies the hardening (config, environment, token injection, redaction).
type Runner func(ctx context.Context, args ...string) (string, error)

// maxTagName bounds a tag name taken from an untrusted remote.
const maxTagName = 255

// ListTags lists the tags of url with their peeled commits, sorted by name.
func ListTags(ctx context.Context, run Runner, url string) ([]RawTag, error) {
	out, err := run(ctx, "ls-remote", "--tags", "--", url)
	if err != nil {
		return nil, oops.Wrapf(err, "list tags")
	}
	return ParseLsRemote(out), nil
}

// ParseLsRemote parses `git ls-remote --tags` output: "<sha>\trefs/tags/<name>"
// lines, each annotated tag followed by a "<name>^{}" line holding the commit.
// Lines that are not a tag are ignored.
func ParseLsRemote(out string) []RawTag {
	const prefix = "refs/tags/"
	objects := map[string]string{}
	peeled := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || !isSHA(sha) || !strings.HasPrefix(ref, prefix) {
			continue
		}
		name := ref[len(prefix):]
		if strings.HasSuffix(name, "^{}") {
			if name = strings.TrimSuffix(name, "^{}"); validName(name) {
				peeled[name] = sha
			}
			continue
		}
		if validName(name) {
			objects[name] = sha
		}
	}
	tags := make([]RawTag, 0, len(objects))
	for name, obj := range objects {
		commit := obj
		if p, ok := peeled[name]; ok {
			commit = p
		}
		tags = append(tags, RawTag{Name: name, Object: obj, Commit: commit})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags
}

func validName(name string) bool {
	if name == "" || len(name) > maxTagName {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Spec is what a source asks for.
type Spec struct {
	// Constraint is an npm-style range ("^1.2", "~2.1.0", ">=1.4.0 <2.0.0").
	Constraint string
	// TagPrefix selects the tags that belong to this source. Empty accepts a
	// plain version or one after a single "v".
	TagPrefix string
	// IncludePrerelease admits prerelease tags the constraint does not name.
	IncludePrerelease bool
}

// Candidate is a version tag.
type Candidate struct {
	Tag     RawTag
	Version semver.Version
}

// Selection is the outcome of resolving a Spec against a tag list.
type Selection struct {
	// Chosen is the highest tag the constraint allows.
	Chosen Candidate
	// Latest is the highest version tag of any kind of release the spec admits
	// (prereleases only with IncludePrerelease), whatever the constraint says.
	Latest Candidate
	// Notes report oddities: two tags naming one version, for example.
	Notes []string
}

// Candidates returns the version tags of tags, highest first; equal versions
// ("1.2.3" and "v1.2.3") order by tag name so the choice is deterministic.
func Candidates(tags []RawTag, prefix string) (cands []Candidate, nonSemver []string) {
	for _, t := range tags {
		v, ok := semver.ParseTag(t.Name, prefix)
		if !ok {
			nonSemver = append(nonSemver, t.Name)
			continue
		}
		cands = append(cands, Candidate{Tag: t, Version: v})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if c := cands[i].Version.Compare(cands[j].Version); c != 0 {
			return c > 0
		}
		return cands[i].Tag.Name < cands[j].Tag.Name
	})
	sort.Strings(nonSemver)
	return cands, nonSemver
}

// Select picks the highest tag spec allows. It fails with AR731 for a constraint
// that does not parse and AR730 when no tag satisfies it.
func Select(tags []RawTag, spec Spec) (*Selection, error) {
	c, err := semver.ParseConstraint(spec.Constraint)
	if err != nil {
		return nil, errorf(CodeConstraintBad, "version constraint %q is invalid: %s", spec.Constraint, cause(err))
	}
	cands, nonSemver := Candidates(tags, spec.TagPrefix)
	sel := &Selection{}
	foundLatest, foundChosen := false, false
	for _, cand := range cands {
		if !foundLatest && (spec.IncludePrerelease || !cand.Version.IsPrerelease()) {
			sel.Latest, foundLatest = cand, true
		}
		if !foundChosen && c.Check(cand.Version, spec.IncludePrerelease) {
			sel.Chosen, foundChosen = cand, true
		}
	}
	sel.Notes = duplicateNotes(cands)
	if !foundChosen {
		return nil, errorf(CodeUnsatisfiable, "%s", unsatisfiableMessage(spec, cands, nonSemver))
	}
	if !foundLatest {
		// Every tag is a prerelease and the spec excludes them, yet the constraint chose one.
		sel.Latest = sel.Chosen
	}
	return sel, nil
}

func cause(err error) string { return err.Error() }

func duplicateNotes(cands []Candidate) []string {
	var notes []string
	for i := 1; i < len(cands); i++ {
		if cands[i-1].Version.Compare(cands[i].Version) == 0 && cands[i-1].Version.Build == cands[i].Version.Build {
			notes = append(notes, fmt.Sprintf("tags %q and %q name the same version; using %q", cands[i-1].Tag.Name, cands[i].Tag.Name, cands[i-1].Tag.Name))
		}
	}
	return notes
}

const nearest = 3

func unsatisfiableMessage(spec Spec, cands []Candidate, nonSemver []string) string {
	where := ""
	if spec.TagPrefix != "" {
		where = fmt.Sprintf(" with prefix %q", spec.TagPrefix)
	}
	if len(cands) == 0 {
		msg := fmt.Sprintf("no semantic version tags%s to satisfy %q", where, spec.Constraint)
		if len(nonSemver) > 0 {
			msg += "; non-version tags: " + quoteList(nonSemver, nearest)
		}
		return msg + "; pin a commit SHA instead"
	}
	names := make([]string, 0, nearest)
	for _, c := range cands {
		if len(names) == nearest {
			break
		}
		names = append(names, c.Tag.Name)
	}
	hint := ""
	if !spec.IncludePrerelease {
		for _, c := range cands {
			if c.Version.IsPrerelease() {
				hint = "; prereleases are excluded unless the constraint names one or include_prerelease = true"
				break
			}
		}
	}
	return fmt.Sprintf("no tag%s satisfies %q (highest tags: %s)%s", where, spec.Constraint, quoteList(names, nearest), hint)
}

func quoteList(names []string, limit int) string {
	if len(names) > limit {
		names = names[:limit]
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}

// Status is the state of a pinned tag on the remote.
type Status int

const (
	// StatusOK means the tag still points to the pinned commit.
	StatusOK Status = iota
	// StatusMoved means the tag now points to another commit (AR732).
	StatusMoved
	// StatusMissing means the remote no longer has the tag (AR735).
	StatusMissing
)

// Check compares the pinned tag with the remote. It returns the remote's tag
// when there is one.
func Check(tags []RawTag, name, commit string) (Status, RawTag) {
	for _, t := range tags {
		if t.Name != name {
			continue
		}
		if t.Commit != commit {
			return StatusMoved, t
		}
		return StatusOK, t
	}
	return StatusMissing, RawTag{}
}

// Find returns the tag called name.
func Find(tags []RawTag, name string) (RawTag, bool) {
	for _, t := range tags {
		if t.Name == name {
			return t, true
		}
	}
	return RawTag{}, false
}

// MovedError is the AR732 failure for a pinned tag that points elsewhere.
func MovedError(name, locked, now string) error {
	return errorf(CodeTagMoved, "tag %q was pinned at %s but now points to %s; a moved tag is never followed silently (review the new commit, then `ai-rulez update --accept-moved-tag`)", name, short(locked), short(now))
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
