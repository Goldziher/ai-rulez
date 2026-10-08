package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Codes of the Agentic Resource Discovery manifest check (docs/ard.md). They are
// reported by `validate --strict` for a project with an [ard] table, and by
// `publish` when the ard emitter runs. The block is AR9S; the literals repeat
// internal/ard so the registry test can see them, and TestARDCodesMatchPackage
// keeps them in step.
const (
	CodeARDSchema      = "AR9S0"
	CodeARDIdentifier  = "AR9S1"
	CodeARDEntry       = "AR9S2"
	CodeARDQueries     = "AR9S3"
	CodeARDNotDeclared = "AR9S4"
)

// WithARD supplies the findings of the manifest the project would publish.
func WithARD(findings []ard.Finding) Option {
	return func(r *runner) { r.ard = findings }
}

// checkARD reports the manifest findings, anchored to the configuration file
// that declares [ard], and an ard emitter without an [ard] table.
func (r *runner) checkARD() {
	if r.cfg.ConfigDir == "" {
		return
	}
	file := filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)
	if r.cfg.ARD == nil {
		if p := r.cfg.Publish; p != nil {
			for _, e := range p.Emitters {
				if e.Name == config.PublishEmitterARD {
					r.add(CodeARDNotDeclared, file, 1, "[[publish.emitters]] names ard but the configuration has no [ard] table with publisher and namespace")
					return
				}
			}
		}
		return
	}
	for _, f := range r.ard {
		sev := SeverityError
		if f.Severity != ard.SeverityError {
			sev = SeverityWarning
		}
		msg := f.Message
		if f.Identifier != "" && f.Path != "" {
			msg = f.Identifier + " " + f.Path + ": " + msg
		}
		r.addWithSeverity(ard.Code(f.Rule), sev, file, 1, "%s", msg)
	}
}

func registerARD(s *ruleSet) {
	for _, code := range []string{CodeARDSchema, CodeARDIdentifier, CodeARDEntry, CodeARDQueries, CodeARDNotDeclared} {
		SetAnalyzer(code, AnalyzerPlugin, ScopeBundle)
	}
	s.addRules(
		RuleInfo{CodeARDSchema, "ard-manifest-invalid", SeverityError, "the ard.json manifest fails the vendored ARD entry schema (a required field is missing or has the wrong type)"},
		RuleInfo{CodeARDIdentifier, "ard-identifier-invalid", SeverityError, "an ARD identifier breaks the urn:air grammar: [ard] publisher is not a fully qualified domain name, a namespace or name has an illegal character, or two resources share an identifier"},
		RuleInfo{CodeARDEntry, "ard-entry-invalid", SeverityError, "an ARD entry has both or neither of url and data, or its url is not an absolute https URL"},
		RuleInfo{CodeARDQueries, "ard-queries-out-of-range", SeverityWarning, "an ARD entry has no representativeQueries or fewer than 2 or more than 5; registries build their search index from them"},
		RuleInfo{CodeARDNotDeclared, "ard-not-declared", SeverityError, "the ard emitter is requested but the configuration has no [ard] table with publisher and namespace"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeARDSchema: {
			Why:  "A registry validates ard.json against the entry schema and drops what fails it, so the manifest is checked against the schema pinned from the spec repository before it is published.",
			Bad:  "An entry without a displayName, or with a tags value that is not a list",
			Good: "Let `ai-rulez publish --emit ard` build the file instead of editing it by hand",
		},
		CodeARDIdentifier: {
			Why:  "An identifier is urn:air:<publisher>:<namespace>:<name> and its publisher must be the fully qualified domain name that serves the manifest; a registry rejects anything else, and two entries with one identifier are ambiguous.",
			Bad:  "`[ard] publisher = \"localhost\"`, a skill named `my skill`, or a skill and an MCP server both named `docs`",
			Good: "`publisher = \"acme.example\"`, `namespace = \"tools\"`, and one name per resource",
		},
		CodeARDEntry: {
			Why:  "An entry points at its artifact with url or carries it inline in data, never both and never neither, and the manifest is served over HTTPS so its urls must be too.",
			Bad:  "A skill whose file is served from `http://example.com/SKILL.md`",
			Good: "Publish from a tagged GitHub release, or set `[ard] base_url` to an https location",
		},
		CodeARDQueries: {
			Why:  "Registries build their semantic index from representativeQueries, and the spec asks for two to five. An entry without them is hard to find; more than five are cut.",
			Bad:  "An MCP server with no queries, or a skill with one trigger and no eval cases",
			Good: "Add `triggers` or `representative_queries` to the skill, eval cases that expect it to trigger, or `[ard.queries]` entries for a server",
		},
		CodeARDNotDeclared: {
			Why:  "The identifiers need a publisher and a namespace, so the emitter cannot guess them.",
			Bad:  "`[[publish.emitters]] name = \"ard\"` without an `[ard]` table",
			Good: "Add `[ard]` with `publisher` (an FQDN) and `namespace`",
		},
	})
}
