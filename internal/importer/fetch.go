package importer

import (
	"fmt"
	"regexp"
	"strings"
)

// fullSHA matches a full git commit hash.
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// splitOrigin splits "file#field" into the finding's source and field.
func splitOrigin(origin string) (source, field string) {
	source, field, _ = strings.Cut(origin, "#")
	return source, field
}

// describe names a remote source for a message: URL, subdirectory and ref.
func (r Remote) describe() string {
	s := r.URL
	if r.Path != "" {
		s += " (" + r.Path + ")"
	}
	if r.Ref != "" {
		s += " at " + r.Ref
	}
	return s
}

// reportUnfetched records a needs-action finding for every remote source: without
// --fetch convert never uses the network, so the content is not imported.
func (p *Plan) reportUnfetched() {
	for _, rm := range p.Remotes {
		reason := fmt.Sprintf("%s is not fetched: convert uses the network only with --fetch; rerun with --fetch, or add the source by hand", rm.describe())
		if rm.Commit != "" {
			reason += " (the input's lock file pins it to " + rm.Commit[:12] + ")"
		}
		source, field := splitOrigin(rm.Origin)
		p.add(newFinding(StatusNeedsAction, source, field, "", reason))
	}
}
