package emit

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/mcpcard"
)

// ARDEmitter is the name of the Agentic Resource Discovery emitter.
const ARDEmitter = "ard"

const (
	ardManifest   = "ard.json"
	ardSkillsDir  = "skills"
	ardSkillFile  = "SKILL.md"
	ardDefaultGit = "github.com"
	ardRawHost    = "raw.githubusercontent.com"
	ardAgentTag   = "agent-plugins"
	ardMaxQueries = ard.MaxQueries
)

// ARDSkill is a skill as the ard emitter lists it.
type ARDSkill struct {
	Name, Description, Version string
	// Keywords become tags.
	Keywords []string
	// Triggers, Queries (the skill's own representative_queries) and EvalPrompts
	// (prompts of eval cases that expect the skill to trigger) seed the entry's
	// representativeQueries, in that priority: Queries, EvalPrompts, Triggers.
	Triggers, Queries, EvalPrompts []string
	// Source is the slash path of the skill's SKILL.md from the repository root.
	Source string
	// Body is the SKILL.md text, written under skills/<name>/ for a base_url.
	Body string
}

// ARDServer is an MCP server as the ard emitter lists it. Environment values
// and header values are deliberately not carried: the manifest is public.
type ARDServer = mcpcard.Server

// ARDInput is the [ard] table and the resources the ard emitter lists.
type ARDInput struct {
	Publisher, Namespace string
	// BaseURL, when set, is where the skill files (and the plugin archive, when the
	// repository is not known) are served from.
	BaseURL string
	// PluginType overrides the media type of plugin entries.
	PluginType string
	// UpdatedAt is the updatedAt of every entry: the release time, never the clock.
	UpdatedAt time.Time
	// Queries override the representative queries of a resource, by name.
	Queries map[string][]string
	Skills  []ARDSkill
	Servers []ARDServer
}

// ardEmitter writes the Agentic Resource Discovery manifest (ard.json, spec v0.91, a
// proposal) for the skills, MCP servers and plugins of a release. With a
// base_url it also writes the skill files the manifest points at, so hosting is
// a copy of the output directory. The manifest is served from
// https://<publisher>/.well-known/ard.json; see docs/ard.md.
type ardEmitter struct{}

// init registers the ARD emitter and the MCP server card emitter that shares its builders.
func init() {
	register(ardEmitter{})
	register(mcpServerCard{})
}

func (ardEmitter) Name() string   { return ARDEmitter }
func (ardEmitter) Status() string { return StatusVerified }

func (ardEmitter) Emit(in Input) ([]File, []Finding, error) {
	m, files, notes, err := ardModel(in)
	if err != nil {
		return nil, nil, err
	}
	data, findings, err := ardBuild(m)
	if err != nil {
		return nil, nil, err
	}
	findings = append(findings, findingsOf(notes)...)
	files = append(files, File{Path: ardManifest, Data: data})
	out, err := finish(files)
	return out, findings, err
}

func ardBuild(m ard.Model) ([]byte, []Finding, error) {
	data, found, err := ard.Build(m)
	if err != nil {
		return nil, nil, ardBuildError(m, err)
	}
	var findings []Finding
	for _, f := range found {
		findings = append(findings, Finding{Message: ard.Code(f.Rule) + " " + f.Identifier + ": " + f.Message})
	}
	return data, findings, nil
}

// ardBuildError names the AR9S code of every problem that stopped the build.
func ardBuildError(m ard.Model, cause error) error {
	found, err := ard.Check(m)
	if err != nil {
		return oops.Wrapf(err, "build the ARD manifest")
	}
	var msgs []string
	for _, f := range found {
		if f.Severity == ard.SeverityError {
			msgs = append(msgs, ard.Code(f.Rule)+" "+f.Message)
		}
	}
	if len(msgs) == 0 {
		return oops.Wrapf(cause, "build the ARD manifest")
	}
	return oops.Errorf("ard: %s", strings.Join(msgs, "; "))
}

// ARDModel maps the input to the model the ard library builds and the skill files
// that go with it. It is what `validate --strict` checks and `publish` emits.
func ARDModel(in Input) (ard.Model, []File, error) {
	m, files, _, err := ardModel(in)
	return m, files, err
}

// ardModel is ARDModel plus a note for each MCP server left out because a
// server card describes remote servers only.
func ardModel(in Input) (ard.Model, []File, []string, error) {
	a := in.ARD
	if a == nil {
		return ard.Model{}, nil, nil, oops.Hint("add an [ard] table with publisher and namespace").
			Errorf("%s: the ard emitter needs an [ard] table", ard.CodeNotDeclared)
	}
	m := ard.Model{Publisher: a.Publisher, Namespace: a.Namespace, UpdatedAt: a.UpdatedAt}
	if a.PluginType != "" {
		m.MediaTypes = map[ard.Kind]string{ard.KindPlugin: a.PluginType}
	}
	var (
		files   []File
		errs    []error
		notes   []string
		perName = map[string][]string{}
	)
	skills := slices.Clone(a.Skills)
	slices.SortFunc(skills, func(x, y ARDSkill) int { return strings.Compare(x.Name, y.Name) })
	for i := range skills {
		s := &skills[i]
		link, err := ardSkillURL(in, a, s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		queries := ardQueries(a, s.Name, ard.QuerySources{Explicit: s.Queries, EvalPrompts: s.EvalPrompts, Triggers: s.Triggers})
		perName[s.Name] = queries
		m.Resources = append(m.Resources, ard.Resource{
			Kind: ard.KindSkill, Name: s.Name, Description: s.Description, Version: s.Version,
			Tags: s.Keywords, URL: link, RepresentativeQueries: queries,
		})
		if a.BaseURL != "" {
			files = append(files, File{Path: ardSkillsDir + "/" + s.Name + "/" + ardSkillFile, Data: []byte(s.Body)})
		}
	}
	cards, serverNotes, err := mcpcard.Build(a.Servers, mcpcard.Options{
		Namespace: mcpcard.NamespaceFromDomain(a.Publisher), Version: in.Version, HTTPSOnly: true,
	})
	if err != nil {
		errs = append(errs, oops.Errorf("%s: %w", ard.CodeEntry, err))
	}
	notes = append(notes, serverNotes...)
	for _, c := range cards {
		data, err := c.Map()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, name, _ := strings.Cut(c.Name, "/")
		m.Resources = append(m.Resources, ard.Resource{
			Kind: ard.KindMCPServer, Name: name, Description: c.Description, Data: data,
			RepresentativeQueries: ardQueries(a, name, ard.QuerySources{}),
		})
	}
	packages := map[string]bool{}
	for _, e := range AgentPluginEntries(in) {
		packages[e.Name] = true
	}
	for i := range in.Plugins {
		p := &in.Plugins[i]
		link, err := ardAssetURL(in, a, p.BundleFile)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r := ard.Resource{
			Kind: ard.KindPlugin, Name: p.Name, Description: p.Description, Version: p.Version,
			Tags: p.Keywords, URL: link,
			RepresentativeQueries: ardQueries(a, p.Name, ard.QuerySources{Explicit: roundRobin(skills, perName)}),
		}
		if p.Category != "" {
			r.Capabilities = []string{p.Category}
		}
		if packages[p.Name] {
			r.Tags = append(slices.Clone(r.Tags), ardAgentTag)
		}
		m.Resources = append(m.Resources, r)
	}
	if len(errs) > 0 {
		return ard.Model{}, nil, nil, errors.Join(errs...)
	}
	return m, files, notes, nil
}

// ardQueries is the configured override of a resource, or the derived queries.
func ardQueries(a *ARDInput, name string, src ard.QuerySources) []string {
	if over, ok := a.Queries[name]; ok {
		return ard.DeriveQueries(ard.QuerySources{Explicit: over})
	}
	return ard.DeriveQueries(src)
}

// roundRobin takes the first query of every skill, then the second of every
// skill, and so on, so a plugin's queries cover all its skills.
func roundRobin(skills []ARDSkill, perName map[string][]string) []string {
	var out []string
	for round := 0; len(out) < ardMaxQueries; round++ {
		took := false
		for i := range skills {
			if q := perName[skills[i].Name]; round < len(q) && len(out) < ardMaxQueries {
				out = append(out, q[round])
				took = true
			}
		}
		if !took {
			break
		}
	}
	return out
}

func ardSkillURL(in Input, a *ARDInput, s *ARDSkill) (string, error) {
	if a.BaseURL != "" {
		return joinURL(a.BaseURL, ardSkillsDir, s.Name, ardSkillFile), nil
	}
	host, repo, ok := splitRepo(in.Repo)
	if !ok || in.Tag == "" {
		return "", ardWhere("skill " + s.Name)
	}
	if host != ardDefaultGit {
		return "", oops.Hint("set [ard] base_url to where the skill files are served").
			Errorf("%s: skill %s: raw file URLs are only known for %s, not %s: set [ard] base_url", ard.CodeEntry, s.Name, ardDefaultGit, host)
	}
	return "https://" + ardRawHost + "/" + repo + "/" + escapePath(in.Tag) + "/" + escapePath(s.Source), nil
}

func ardAssetURL(in Input, a *ARDInput, file string) (string, error) {
	if host, repo, ok := splitRepo(in.Repo); ok && in.Tag != "" {
		return "https://" + host + "/" + repo + "/releases/download/" + escapePath(in.Tag) + "/" + url.PathEscape(file), nil
	}
	if a.BaseURL != "" {
		return joinURL(a.BaseURL, file), nil
	}
	return "", ardWhere("the plugin archive " + file)
}

func ardWhere(what string) error {
	return oops.Hint("publish a tagged release of a repository (--tag, --repo), or set [ard] base_url").
		Errorf("%s: cannot tell where %s is served: no release tag and repository, and no [ard] base_url", ard.CodeEntry, what)
}

// splitRepo reads "owner/name" (GitHub) or "host/owner/name".
func splitRepo(repo string) (host, path string, ok bool) {
	parts := strings.Split(repo, "/")
	switch len(parts) {
	case 2:
		return ardDefaultGit, repo, true
	case 3:
		return parts[0], parts[1] + "/" + parts[2], true
	}
	return "", "", false
}

func joinURL(base string, elems ...string) string {
	out := strings.TrimRight(base, "/")
	for _, e := range elems {
		out += "/" + url.PathEscape(e)
	}
	return out
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
