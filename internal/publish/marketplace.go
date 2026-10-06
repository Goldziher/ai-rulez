package publish

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/samber/oops"
)

// MarketplaceDir is the dist directory of the pinned marketplace indexes.
const MarketplaceDir = "marketplace"

// claudeIndexPath is where a Claude Code marketplace keeps its index, relative to the marketplace root.
const claudeIndexPath = ".claude-plugin/marketplace.json"

var (
	channelPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	refPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// Pin is the input of a pinned marketplace index.
type Pin struct {
	// Index is the generated .claude-plugin/marketplace.json of the bundle.
	Index []byte
	// IndexRoot is the marketplace root (the directory holding .claude-plugin),
	// relative to the project root; "" or "." is the project root itself.
	IndexRoot string
	// RepoPath is the project root relative to the git repository root; "" or
	// "." when they are the same.
	RepoPath string
	// Repo is OWNER/REPO, or HOST/OWNER/REPO for a host other than github.com.
	Repo string
	// Ref is the git ref the entries name: the release tag, or the channel's ref.
	Ref string
}

// ValidChannel reports whether name is a usable channel name.
func ValidChannel(name string) bool { return channelPattern.MatchString(name) }

// IndexPath is the dist path of the pinned index of a channel ("" is the
// default, unnamed channel).
func IndexPath(channel string) string {
	if channel == "" {
		return MarketplaceDir + "/" + claudeIndexPath
	}
	return MarketplaceDir + "/" + channel + "/" + claudeIndexPath
}

// pinnedSource is a Claude Code plugin source object (code.claude.com/docs/en/plugins/marketplace-reference,
// checked 2026-10-06): github, url and git-subdir sources take a branch or tag
// in ref and a full 40-character commit in sha, and Claude Code checks out sha.
type pinnedSource struct {
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
}

// PinIndex rewrites the relative sources of a Claude marketplace index into
// sources pinned to ref and commit, so the index points at one immutable
// state of the repository instead of whatever the default branch holds. Every
// other field of an entry, and of the index, is kept.
func PinIndex(p Pin, commit string) ([]byte, error) {
	if !commitPattern.MatchString(commit) {
		return nil, newError(CodeSource, ExitGate, "commit the changes first; a pinned index names a commit",
			"the source tree has no commit to pin the marketplace to")
	}
	if !refPattern.MatchString(p.Ref) || strings.Contains(p.Ref, "..") || strings.HasSuffix(p.Ref, "/") {
		return nil, newError(CodeConfig, ExitFailed, "", "invalid git ref %q for the pinned index", p.Ref)
	}
	host, slug, ok := splitRepo(p.Repo)
	if !ok {
		return nil, newError(CodeConfig, ExitFailed, "pass --repo OWNER/REPO", "cannot pin the marketplace to %q: not an OWNER/REPO repository", p.Repo)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(p.Index, &doc); err != nil {
		return nil, oops.Wrapf(err, "parse the marketplace index")
	}
	var plugins []map[string]json.RawMessage
	if err := json.Unmarshal(doc["plugins"], &plugins); err != nil {
		return nil, oops.Wrapf(err, "parse the marketplace plugins")
	}
	for i, entry := range plugins {
		var rel string
		if err := json.Unmarshal(entry["source"], &rel); err != nil {
			return nil, newError(CodePreflight, ExitGate, "", "plugin %d of the marketplace index has no relative source to pin", i)
		}
		dir, err := pinnedDir(p, rel)
		if err != nil {
			return nil, err
		}
		src, err := json.Marshal(sourceFor(host, slug, dir, p.Ref, commit))
		if err != nil {
			return nil, oops.Wrapf(err, "encode a pinned source")
		}
		entry["source"] = src
	}
	out, err := json.Marshal(plugins)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the pinned plugins")
	}
	doc["plugins"] = out
	return marshalJSON(doc)
}

// pinnedDir resolves a relative plugin source to its directory in the repository.
func pinnedDir(p Pin, rel string) (string, error) {
	clean := path.Clean(strings.TrimPrefix(rel, "./"))
	if rel == "" || path.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(rel, "\\") {
		return "", newError(CodeBundleUnsafe, ExitFailed, "", "marketplace source %q is not a relative path inside the repository", rel)
	}
	return path.Join(".", p.RepoPath, p.IndexRoot, clean), nil
}

func sourceFor(host, slug, dir, ref, commit string) pinnedSource {
	repoURL := "https://" + host + "/" + slug + ".git"
	switch {
	case dir == "." && host == "github.com":
		return pinnedSource{Source: "github", Repo: slug, Ref: ref, SHA: commit}
	case dir == ".":
		return pinnedSource{Source: "url", URL: repoURL, Ref: ref, SHA: commit}
	default:
		return pinnedSource{Source: "git-subdir", URL: repoURL, Path: dir, Ref: ref, SHA: commit}
	}
}

// splitRepo splits OWNER/REPO (github.com) or HOST/OWNER/REPO.
func splitRepo(repo string) (host, slug string, ok bool) {
	parts := strings.Split(repo, "/")
	switch len(parts) {
	case 2:
		return "github.com", repo, true
	case 3:
		return parts[0], parts[1] + "/" + parts[2], true
	}
	return "", "", false
}
