package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// rulesync `sources` (declarative sources, `rulesync install`): git repositories
// whose skills and rules rulesync copies into .rulesync/. They are read from the
// documentation (docs/guide/declarative-sources.md): `source` is owner/repo,
// owner/repo@ref:path or a git URL (transport "git"); `skills` and `rules` select
// names ("*" for all), `ref`, `path` and `rulesPath` locate them. The npm
// transport is not a git source and is not imported.

const (
	rulesyncDefaultSkillsPath = skillsDir
	rulesyncDefaultRulesPath  = rulesDir
	transportGit              = "git"
	transportNPM              = "npm"
	transportGitHub           = "github"
)

type rulesyncSource struct {
	url, ref, path, rulesPath string
	skills, rules             []string
}

// parseRulesyncSource reads one `sources` entry. reason is set when it cannot be
// imported.
func parseRulesyncSource(e map[string]any) (s rulesyncSource, reason string) {
	source := stringOf(e["source"])
	if source == "" {
		return s, "the entry has no source"
	}
	transport := strings.ToLower(stringOf(e["transport"]))
	s.skills, s.rules = listOf(e[skillsDir]), listOf(e[rulesDir])
	s.ref, s.path, s.rulesPath = stringOf(e["ref"]), stringOf(e["path"]), stringOf(e["rulesPath"])
	switch transport {
	case transportNPM:
		return s, "an npm package is not a git source; install it and copy the skills into skills/"
	case transportGit:
		s.url = source
	case "", transportGitHub:
		// owner/repo, owner/repo@ref, owner/repo:path or owner/repo@ref:path
		spec, atPath, _ := strings.Cut(source, ":")
		spec, atRef, _ := strings.Cut(spec, "@")
		s.ref = firstNonEmpty(s.ref, atRef)
		s.path = firstNonEmpty(s.path, atPath)
		if !githubShorthand.MatchString(spec) {
			return s, fmt.Sprintf("source %q is not an owner/repo", source)
		}
		s.url = "https://github.com/" + spec
	default:
		return s, fmt.Sprintf("transport %q is not a documented rulesync source transport", transport)
	}
	if reason := gitSourceProblem(s.url); reason != "" {
		return s, "source " + reason
	}
	if s.path == "" {
		s.path = rulesyncDefaultSkillsPath
	}
	if s.rulesPath == "" {
		s.rulesPath = rulesyncDefaultRulesPath
	}
	for _, p := range []string{s.path, s.rulesPath} {
		if reason := checkSubpath(strings.Trim(p, "/")); reason != "" {
			return s, "path " + reason
		}
	}
	s.path = strings.Trim(s.path, "/")
	s.rulesPath = strings.Trim(s.rulesPath, "/")
	return s, ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// readRulesyncLock returns the commits rulesync.lock pinned, by source key.
func (b *rulesyncPlanner) readRulesyncLock() map[string]string {
	out := map[string]string{}
	if _, ok := b.r.exists(rulesyncLockFile); !ok {
		return out
	}
	data, err := b.r.read(rulesyncLockFile)
	if err != nil {
		return out
	}
	var doc struct {
		Sources map[string]struct {
			ResolvedRef string `json:"resolvedRef"`
		} `json:"sources"`
	}
	if json.Unmarshal(trimBOM(data), &doc) != nil {
		return out
	}
	for k, v := range doc.Sources {
		if gitutil.IsCommitSHA(v.ResolvedRef) {
			out[k] = v.ResolvedRef
		}
	}
	return out
}

// importSources records the remote sources of rulesync.jsonc. Nothing is fetched
// here: each becomes a Remote that convert reads only with --fetch.
func (b *rulesyncPlanner) importSources(raw json.RawMessage) {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		b.p.add(newFinding(StatusUnsupported, rulesyncConfigFile, "sources", "", "sources is not a list of objects"))
		return
	}
	pinned := b.readRulesyncLock()
	for i, e := range entries {
		name := stringOf(e["source"])
		if name == "" {
			name = fmt.Sprintf("#%d", i)
		}
		field := "sources." + name
		s, reason := parseRulesyncSource(e)
		if reason != "" {
			b.p.add(newFinding(StatusUnsupported, rulesyncConfigFile, field, "", reason))
			continue
		}
		commit := pinned[name]
		origin := rulesyncConfigFile + "#" + field
		if len(s.skills) > 0 || len(s.rules) == 0 {
			b.p.Remotes = append(b.p.Remotes, Remote{Kind: remoteSkills, Origin: origin, URL: s.url, Ref: s.ref, Commit: commit, Path: s.path, Skills: s.skills})
		}
		if len(s.rules) > 0 {
			b.p.Remotes = append(b.p.Remotes, Remote{Kind: remoteRules, Origin: origin, URL: s.url, Ref: s.ref, Commit: commit, Path: s.rulesPath, Rules: s.rules})
		}
	}
}
