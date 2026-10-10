package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

// CodeCredentialedSource flags a remote source whose URL embeds a credential in
// its userinfo. It is the security-check counterpart of the refusal `include
// add` and `skill install` apply: a source already committed to config.toml is
// reported so the run fails and the secret can be removed.
const CodeCredentialedSource = "AR035"

func registerArSources(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeCredentialedSource, "credential-in-source-url", SeverityError,
			"an include, installed skill or [[skill_sources]] URL carries a credential in its userinfo, so the secret is committed to config.toml in clear"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeCredentialedSource: {
			Why:  "A token embedded in a git URL is written into the committed config.toml, where everyone with repository access can read it; it can also reach the cached clone's config. The resolver redacts its own logs, but the file keeps the secret.",
			Bad:  "`source = \"https://user:ghp_secret@github.com/org/repo.git\"`",
			Good: "A URL without userinfo, authenticated with the git credential helper or the AI_RULEZ_GIT_TOKEN environment variable",
		},
	})
	SetAnalyzer(CodeCredentialedSource, AnalyzerSecurity, ScopeBundle)
	s.addRunCheck(checkCredentialedSources, AnalyzerSecurity)
}

// checkCredentialedSources reports AR035 for each include, installed skill or
// skill source in the configuration that carries a credential in its URL. The
// finding redacts the URL, so the secret is never echoed.
func checkCredentialedSources(r *runner) {
	path := r.configFilePath()
	if path == "" {
		path = filepath.Join(r.rootAbs(), ".ai-rulez", "config.toml")
	}
	lines := r.fileLines(path)
	base := filepath.Base(path)
	report := func(kind, name, source string) {
		if !includes.HasCredentials(source) {
			return
		}
		r.add(CodeCredentialedSource, path, lineContaining(lines, `"`+name+`"`),
			"%s %q carries a credential in its URL (%s), which is stored in clear in %s; keep the URL free of secrets and authenticate with the git credential helper or the AI_RULEZ_GIT_TOKEN environment variable",
			kind, name, includes.RedactURL(source), base)
	}
	for i := range r.cfg.Includes {
		report("include", r.cfg.Includes[i].Name, r.cfg.Includes[i].Source)
	}
	for i := range r.cfg.InstalledSkills {
		report("installed skill", r.cfg.InstalledSkills[i].Name, r.cfg.InstalledSkills[i].Source)
	}
	for i := range r.cfg.SkillSources {
		report("skill source", r.cfg.SkillSources[i].Name, r.cfg.SkillSources[i].URL)
	}
}
