package importer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

const (
	skillsLockName    = "skills-lock"
	skillsLockFile    = "skills-lock.json"
	skillsLockVersion = 1
)

// skillsLockEntry mirrors LocalSkillLockEntry of the Vercel skills CLI
// (vercel-labs/skills, src/local-lock.ts, lock file version 1).
type skillsLockEntry struct {
	Source          string   `json:"source"`
	SourceURL       string   `json:"sourceUrl"`
	Ref             string   `json:"ref"`
	SourceType      string   `json:"sourceType"`
	SkillPath       string   `json:"skillPath"`
	ComputedHash    string   `json:"computedHash"`
	Subagents       []string `json:"subagents"`
	WellKnownDigest string   `json:"wellKnownDigest"`
}

type skillsLockDoc struct {
	Version int                        `json:"version"`
	Skills  map[string]json.RawMessage `json:"skills"`
}

var githubShorthand = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)

type skillsLockImporter struct{}

func (skillsLockImporter) Name() string { return skillsLockName }

func (skillsLockImporter) Description() string {
	return "skills-lock.json of the Vercel skills CLI, mapped to [[installed_skills]]"
}

func (skillsLockImporter) Detect(fsys fs.FS) []string {
	if _, ok := newReader(fsys).exists(skillsLockFile); ok {
		return []string{skillsLockFile}
	}
	return nil
}

func (skillsLockImporter) Plan(fsys fs.FS, opt Options) (*Plan, error) {
	p := &Plan{}
	data, err := newReader(fsys).read(skillsLockFile)
	if err != nil {
		return nil, oops.With("file", skillsLockFile).Wrapf(err, "%s", skillsLockFile)
	}
	var doc skillsLockDoc
	if err := json.Unmarshal(trimBOM(data), &doc); err != nil {
		return nil, oops.With("file", skillsLockFile).Wrapf(err, "%s is not valid JSON (%s)", skillsLockFile, CodeInvalid)
	}
	if doc.Version != skillsLockVersion {
		reason := fmt.Sprintf("lock file version %d is not the supported version %d", doc.Version, skillsLockVersion)
		if !opt.BestEffort {
			return nil, oops.With("file", skillsLockFile).Hint("Rerun with --best-effort to import the known fields").
				Errorf("%s: %s (%s); rerun with --best-effort to import the known fields", skillsLockFile, reason, CodeNeedsAction)
		}
		p.add(newFinding(StatusNeedsAction, skillsLockFile, "version", "", reason+"; known fields imported best effort"))
	}

	names := make([]string, 0, len(doc.Skills))
	for name := range doc.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := "skills." + name
		var e skillsLockEntry
		if err := json.Unmarshal(doc.Skills[name], &e); err != nil {
			p.add(newFinding(StatusUnsupported, skillsLockFile, field, "", "entry is not an object of the documented shape"))
			continue
		}
		importLockEntry(p, name, field, e)
	}
	return p, nil
}

func importLockEntry(p *Plan, name, field string, e skillsLockEntry) {
	clean, _ := safeName(name)
	if clean != name {
		p.add(newFinding(StatusApproximated, skillsLockFile, field, "installed_skills."+clean, "skill name renamed to a valid name"))
	}
	source, reason := lockSourceURL(e)
	if source == "" {
		p.add(newFinding(StatusUnsupported, skillsLockFile, field, "", reason))
		return
	}
	if reason := gitSourceProblem(source); reason != "" {
		p.add(newFinding(StatusUnsupported, skillsLockFile, field, "", "source "+reason))
		return
	}
	for _, have := range p.InstalledSkills {
		if have.Name == clean {
			p.add(newFinding(StatusUnsupported, skillsLockFile, field, "",
				fmt.Sprintf("not imported: its name collides with another entry as %q after sanitising; rename one in the lock and rerun", clean)))
			return
		}
	}
	skill := config.InstalledSkillConfig{Name: clean, Source: source, Ref: e.Ref}
	if e.SkillPath != "" {
		skill.Path = lockSkillDir(e.SkillPath)
		if skill.Path == "skills/"+clean {
			skill.Path = ""
		}
	}
	// The same check config load applies to installed_skills: a ref or path that
	// would be read as a git option or escape the repository is never written.
	if err := config.ValidateInstalledSkillFields(&skill); err != nil {
		p.add(newFinding(StatusUnsupported, skillsLockFile, field, "", redactedReason(err)))
		return
	}
	p.InstalledSkills = append(p.InstalledSkills, skill)
	p.add(newFinding(StatusMapped, skillsLockFile, field, "installed_skills."+clean, ""))

	if e.Ref == "" {
		p.add(newFinding(StatusNeedsAction, skillsLockFile, field+".ref", "installed_skills."+clean+".ref",
			"no ref recorded; the skill follows the default branch until `ai-rulez lock` pins it"))
	}
	if e.ComputedHash != "" {
		p.add(newFinding(StatusNeedsAction, skillsLockFile, field+".computedHash", "",
			"hash scheme differs from the ai-rulez lock (sha256 of file paths and contents, not a tree digest); not carried, run `ai-rulez lock`"))
	}
	if len(e.Subagents) > 0 {
		p.add(newFinding(StatusDropped, skillsLockFile, field+".subagents", "", "subagent targets have no ai-rulez equivalent"))
	}
	if e.WellKnownDigest != "" {
		p.add(newFinding(StatusDropped, skillsLockFile, field+".wellKnownDigest", "", "well-known digests are not carried; run `ai-rulez lock`"))
	}
}

// lockSourceURL turns a lock entry into a git URL. The skills CLI documents
// the source types github, node_modules and local; others are accepted only
// when their source is already a git URL.
func lockSourceURL(e skillsLockEntry) (url, reason string) {
	switch e.SourceType {
	case "node_modules":
		return "", "npm package skills are not git sources; install the package and copy the skill into skills/"
	case "local":
		return "", "local skills are not tracked remotely; copy the skill into skills/"
	}
	for _, cand := range []string{e.SourceURL, e.Source} {
		cand = strings.TrimSpace(cand)
		switch {
		case strings.Contains(cand, "://") || strings.HasPrefix(cand, "git@"):
			return cand, ""
		case e.SourceType == "github" && githubShorthand.MatchString(cand):
			return "https://github.com/" + cand, ""
		}
	}
	if e.SourceType == "github" || e.SourceType == "" {
		return "", "source " + fmt.Sprintf("%q", e.Source) + " is not an owner/repo or a git URL"
	}
	return "", "source type " + fmt.Sprintf("%q", e.SourceType) + " is not a documented git source"
}

var scpSource = regexp.MustCompile(`^git@[A-Za-z0-9][A-Za-z0-9.-]*:[^\s:][^\s]*$`)

// gitSourceProblem checks a git source before it goes into installed_skills.
// Only https://, ssh:// and git@host:path are accepted: not a leading dash or a
// transport helper (ext::), not file://, git:// or plain http, and no embedded
// credentials. It returns the reason a source is refused, or "".
func gitSourceProblem(src string) string {
	switch {
	case src == "":
		return "is empty"
	case strings.HasPrefix(src, "-"):
		return "starts with '-' and would be read as a git option"
	case strings.Contains(src, "::"):
		return "uses a git transport helper"
	}
	lower := strings.ToLower(src)
	switch {
	case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "ssh://"):
		u, err := url.Parse(src)
		if err != nil || u.Host == "" {
			return "is not a valid URL"
		}
		if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(src, "#") {
			return "carries a query string or fragment (a token often travels there); use a plain repository URL"
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword || u.Scheme == "https" {
				return "embeds credentials; use a ${VAR} reference or a credential helper"
			}
		}
		return ""
	case scpSource.MatchString(src):
		if strings.ContainsAny(src, "?#") {
			return "carries a query string or fragment; use a plain repository path"
		}
		return ""
	}
	return "must be an https://, ssh:// or git@host:path URL"
}

// redactedReason is the message of a validation error without its details.
func redactedReason(err error) string {
	return strings.TrimSpace(firstLine(err.Error()))
}

// lockSkillDir returns the directory of the skill inside its repository: the
// lock records the path of SKILL.md.
func lockSkillDir(skillPath string) string {
	p := strings.TrimSuffix(strings.TrimPrefix(path.Clean(skillPath), "./"), "/")
	if path.Base(p) == "SKILL.md" {
		p = path.Dir(p)
	}
	return p
}
