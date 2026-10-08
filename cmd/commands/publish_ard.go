package commands

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	pemit "github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

// ardInput maps the project to what the ard emitter lists: the [ard] table, the
// root skills with their frontmatter and eval prompts, the enabled MCP servers
// (the project's and the plugin's) and updatedAt, which is the release time and
// never the clock. It is nil without an [ard] table. repoPath is the project's
// directory relative to the repository root ("" at the root).
func ardInput(cfg *config.Config, repoPath string, updatedAt time.Time) *pemit.ARDInput {
	a := cfg.ARD
	if a == nil {
		return nil
	}
	in := &pemit.ARDInput{
		Publisher: a.Publisher, Namespace: a.Namespace, BaseURL: a.BaseURL, PluginType: a.PluginType,
		Queries: a.Queries, UpdatedAt: updatedAt,
	}
	prompts := evalPrompts(cfg.ConfigDir)
	if cfg.Content != nil {
		for i := range cfg.Content.Skills {
			f := &cfg.Content.Skills[i]
			in.Skills = append(in.Skills, ardSkill(cfg, f, repoPath, prompts))
		}
	}
	seen := map[string]bool{}
	for _, s := range cfg.EffectiveMCPServers() {
		if !s.IsEnabled() || seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		in.Servers = append(in.Servers, pemit.ARDServer{
			Name: s.Name, Description: s.Description, Transport: s.GetTransport(), URL: s.URL, Command: s.Command, Args: s.Args,
		})
	}
	if cfg.Plugin != nil {
		for _, s := range cfg.Plugin.MCP {
			if s.Disabled || seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			in.Servers = append(in.Servers, pemit.ARDServer{
				Name: s.Name, Transport: s.Transport, URL: s.URL, Command: s.Command, Args: s.Args,
			})
		}
	}
	return in
}

func ardSkill(cfg *config.Config, f *config.ContentFile, repoPath string, prompts map[string][]string) pemit.ARDSkill {
	text := skillText(cfg, f)
	front, _ := skillsearch.SplitSkill([]byte(text))
	name := f.Name
	if n, ok := front["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	}
	desc, _ := front["description"].(string)      //nolint:errcheck // a missing key is the empty string
	meta, _ := front["metadata"].(map[string]any) //nolint:errcheck // a missing key is the empty map
	version, _ := meta["version"].(string)        //nolint:errcheck // a missing key is the empty string
	return pemit.ARDSkill{
		Name: name, Description: strings.TrimSpace(desc), Version: strings.TrimSpace(version),
		Keywords:    skillsearch.FrontList(front["keywords"]),
		Triggers:    skillsearch.FrontList(front["triggers"]),
		Queries:     skillsearch.FrontList(front["representative_queries"]),
		EvalPrompts: prompts[f.Name],
		Source:      ardSkillSource(cfg, f, repoPath), Body: text,
	}
}

// skillText is the SKILL.md as written, frontmatter included: the loaded
// content is the body alone. It falls back to the body when the file cannot be read.
func skillText(cfg *config.Config, f *config.ContentFile) string {
	p := f.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(cfg.BaseDir, p)
	}
	if data, err := os.ReadFile(p); err == nil { //nolint:gosec // the project's own skill file
		return string(data)
	}
	return f.Content
}

// ardSkillSource is the slash path of a skill's SKILL.md from the repository root.
func ardSkillSource(cfg *config.Config, f *config.ContentFile, repoPath string) string {
	p := f.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(cfg.BaseDir, p)
	}
	rel, err := filepath.Rel(cfg.BaseDir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Join(filepath.Base(cfg.ConfigDir), "skills", f.Name, "SKILL.md"))
	}
	return filepath.ToSlash(filepath.Join(repoPath, rel))
}

// evalPrompts are the prompts of the eval cases that expect the skill to
// trigger, by skill id, in the order the cases are read (path order). Cases that
// cannot be read are skipped: `validate` reports them.
func evalPrompts(configDir string) map[string][]string {
	skills, err := evals.FindSkills(configDir)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for i := range skills {
		cases, _ := evals.LoadCases(&skills[i]) //nolint:errcheck // the problems are validate's to report
		for j := range cases {
			if cases[j].Expects() && strings.TrimSpace(cases[j].Prompt) != "" {
				out[skills[i].ID] = append(out[skills[i].ID], cases[j].Prompt)
			}
		}
	}
	return out
}

// repoRelative is the project directory relative to the repository root, "" at
// the root or outside a repository.
func repoRelative(top, baseDir string) string {
	if top == "" {
		return ""
	}
	rel := gitutil.RepoRelative(top, baseDir)
	if rel == "." {
		return ""
	}
	return rel
}
