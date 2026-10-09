package mcp

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// boolPtr returns a pointer to a bool value, used for optional annotation hints.
func boolPtr(b bool) *bool {
	return &b
}

// Every tool acts on the local project tree only, so none is open-world.

// readOnlyAnnotations returns tool annotations marking a tool as read-only and non-destructive.
func readOnlyAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
	}
}

// destructiveAnnotations returns tool annotations marking a tool as destructive.
func destructiveAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(true),
		OpenWorldHint:   boolPtr(false),
	}
}

// additiveAnnotations marks a writing tool that performs only additive,
// non-destructive updates (create/add operations). Without this, clients
// assume the MCP default DestructiveHint=true and may warn on a harmless create.
func additiveAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
}

// idempotentAnnotations marks a non-destructive writing tool whose repeated
// calls with the same arguments have no additional effect (update/set/generate).
func idempotentAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
	}
}

var priorityValues = []string{"critical", "high", "medium", "low", "minimal"}

// severityValues are the severity levels a check takes.
var severityValues = []string{"low", "medium", "high", "critical"}

// failOnValues are the thresholds `validate --fail-on` accepts.
func failOnValues() []string {
	return []string{string(lint.SeverityError), string(lint.SeverityWarning), string(lint.SeverityInfo), "none"}
}

// enumsOf is the enum restriction of one string property.
func enumsOf(property string, values []string) map[string][]string {
	return map[string][]string{property: values}
}

func (s *Server) registerTools() {
	s.registerProjectTools()
	s.registerUtilityTools()
	s.registerCRUDTools()
}

func (s *Server) registerProjectTools() {
	addTool[generateIn](s, toolSpec{
		name: "generate_outputs", title: "Generate Outputs",
		description: "Generate output files from the current configuration, respecting includes and extends",
		annotations: idempotentAnnotations(), output: generateOut{},
	}, handlers.GenerateOutputsHandler)

	addTool[cleanIn](s, toolSpec{
		name: "clean_outputs", title: "Clean Generated Outputs",
		description: "Remove the files produced by generate (the inverse of generate_outputs): generated assistant files, the generated manifest, and the ai-rulez managed .gitignore block. The .ai-rulez/ source tree is never touched.",
		annotations: destructiveAnnotations(), output: cleanOut{},
	}, handlers.CleanOutputsHandler)

	addTool[validateIn](s, toolSpec{
		name: "validate_config", title: "Validate Configuration",
		description: "Validate the configuration and lint its content with the same checks as `ai-rulez validate`: schema, structure, includes, policy, then every lint analyzer. Returns the verdict, warnings, errors and the full findings document. An invalid configuration, or findings at or above fail_on, is an error result. Read-only.",
		annotations: readOnlyAnnotations(), output: validateOut{},
		enums: map[string][]string{"fail_on": failOnValues(), "lint_profile": lint.ProfileNames()},
	}, handlers.ValidateConfigWith(s.validator))

	addTool[scanIn](s, toolSpec{
		name: "scan_content", title: "Scan Content",
		description: "Run the deterministic security checks of `ai-rulez scan` on the configuration: secret patterns, hidden or bidirectional characters, prompt-injection phrases, risky shell, credential access and unpinned remote sources. Nothing is fetched or executed. Returns the findings document of validate_config; findings at or above fail_on are an error result. Read-only.",
		annotations: readOnlyAnnotations(), output: validateOut{},
		enums: map[string][]string{"fail_on": failOnValues(), "lint_profile": lint.ProfileNames()},
	}, handlers.ScanContentWith(s.validator))

	addTool[doctorIn](s, toolSpec{
		name: "doctor", title: "Run Diagnostics",
		description: "Run read-only diagnostics: config validity, preset names, generated-output drift, gitignore coverage, shared settings documents, MCP env placeholders, hook scripts, lock file, and tool binaries. Returns findings by severity (error, warning, info).",
		annotations: readOnlyAnnotations(), output: doctorOut{},
	}, handlers.DoctorHandler)

	addTool[verifiersIn](s, toolSpec{
		name: "run_verifiers", title: "Run Verifiers",
		description: "Evaluate the deterministic repo checks declared as [[verifiers]] (file exists/absent, glob counts, regex present/absent, JSON/YAML/TOML key values, generated output in sync) and the rule-linked specs under .ai-rulez/verifiers/ (paired files, all/any/not). Read-only: nothing is executed or sent to a model, so a command predicate reports error (AR9H3, refused) and an llm verifier reports skipped (AR9H4), never pass. Returns one pass/fail/error/skipped/inactive/not_applicable result per verifier (inactive: outside the active profile or role) with its findings and the rule it enforces.",
		annotations: readOnlyAnnotations(), output: verifiersOut{},
	}, handlers.RunVerifiersHandler)

	s.registerGovernanceTools()
	s.registerReportTools()

	addTool[initIn](s, toolSpec{
		name: "init_project", title: "Initialize Project",
		description: "Initialize a new ai-rulez project in the current directory. Refuses to overwrite an existing configuration.",
		annotations: additiveAnnotations(), output: initOut{},
	}, handlers.InitProjectHandler)
}

func (s *Server) registerUtilityTools() {
	addTool[noArgs](s, toolSpec{
		name: "get_version", title: "Get Version", description: "Get the ai-rulez version",
		annotations: readOnlyAnnotations(), output: versionOut{},
	}, handlers.GetVersionHandler(s.version))

	addTool[noArgs](s, toolSpec{
		name: "list_builtins", title: "List Builtin Domains",
		description: "List the builtin domains that ship with ai-rulez, with category and whether each is auto-included (builtins list --format json)",
		annotations: readOnlyAnnotations(), output: builtinsListOut{},
	}, handlers.ListBuiltinsHandler)

	addTool[showBuiltinIn](s, toolSpec{
		name: "show_builtin", title: "Show Builtin Domain",
		description: "Show the full content of a builtin domain (rules, context, skills)",
		annotations: readOnlyAnnotations(), output: builtinOut{},
	}, handlers.ShowBuiltinHandler)
}

func (s *Server) registerCRUDTools() {
	s.registerCRUDDomainTools()
	s.registerCRUDRuleTools()
	s.registerCRUDCheckTools()
	s.registerCRUDContextTools()
	s.registerCRUDSkillTools()
	s.registerFlatContentTools()
	s.registerCRUDIncludeTools()
	s.registerCRUDInstalledSkillTools()
	s.registerCRUDConfigTools()
	s.registerCRUDProfileTools()
}

// registerCRUDDomainTools adds the domain tools.
func (s *Server) registerCRUDDomainTools() {
	addTool[createDomainIn](s, toolSpec{
		name: "create_domain", title: "Create Domain",
		description: "Create a new domain with subdirectories for rules, context, and skills",
		annotations: additiveAnnotations(), output: mutationOut{},
	}, handlers.CreateDomainHandler)

	addTool[nameIn](s, toolSpec{
		name: "delete_domain", title: "Delete Domain",
		description: "Delete a domain and all its contents",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.DeleteDomainHandler)

	addTool[workDirArg](s, toolSpec{
		name: "list_domains", title: "List Domains",
		description: "List all domains in the .ai-rulez directory",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListDomainsHandler)
}

// registerCRUDRuleTools adds the rule tools.
func (s *Server) registerCRUDRuleTools() {
	addTool[contentCreateIn](s, toolSpec{
		name: "create_rule", title: "Create Rule",
		description: "Create a new rule file with optional YAML frontmatter",
		annotations: additiveAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.CreateRuleHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "read_rule", title: "Read Rule", description: "Read the content of a rule file",
		annotations: readOnlyAnnotations(), output: readOut{}, readsConfig: true,
	}, handlers.ReadRuleHandler)

	addTool[contentUpdateIn](s, toolSpec{
		name: "update_rule", title: "Update Rule", description: "Update an existing rule file atomically",
		annotations: idempotentAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.UpdateRuleHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "delete_rule", title: "Delete Rule", description: "Delete a rule file",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.DeleteRuleHandler)

	addTool[contentListIn](s, toolSpec{
		name: "list_rules", title: "List Rules", description: "List all rules in the root or a specific domain",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListRulesHandler)
}

// registerCRUDCheckTools adds the check tools.
func (s *Server) registerCRUDCheckTools() {
	addTool[checkCreateIn](s, toolSpec{
		name: "create_check", title: "Create Check",
		description: "Create a new code-review check file (.ai-rulez/checks/<name>.md)",
		annotations: additiveAnnotations(), output: mutationOut{}, enums: enumsOf("severity", severityValues),
	}, handlers.CreateCheckHandler)

	addTool[checkRefIn](s, toolSpec{
		name: "read_check", title: "Read Check", description: "Read the content of a check file",
		annotations: readOnlyAnnotations(), output: readOut{}, readsConfig: true,
	}, handlers.ReadCheckHandler)

	addTool[checkUpdateIn](s, toolSpec{
		name: "update_check", title: "Update Check",
		description: "Update an existing check file atomically: give content, a field (description, severity, tools, targets), or both",
		annotations: idempotentAnnotations(), output: mutationOut{}, enums: enumsOf("severity", severityValues),
	}, handlers.UpdateCheckHandler)

	addTool[checkRefIn](s, toolSpec{
		name: "delete_check", title: "Delete Check", description: "Delete a check file",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.DeleteCheckHandler)

	addTool[checkListIn](s, toolSpec{
		name: "list_checks", title: "List Checks", description: "List all checks in the root or a specific domain",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListChecksHandler)
}

// registerCRUDContextTools adds the context tools.
func (s *Server) registerCRUDContextTools() {
	addTool[contentCreateIn](s, toolSpec{
		name: "create_context", title: "Create Context",
		description: "Create a new context file with optional YAML frontmatter",
		annotations: additiveAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.CreateContextHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "read_context", title: "Read Context", description: "Read the content of a context file",
		annotations: readOnlyAnnotations(), output: readOut{}, readsConfig: true,
	}, handlers.ReadContextHandler)

	addTool[contentUpdateIn](s, toolSpec{
		name: "update_context", title: "Update Context", description: "Update an existing context file atomically",
		annotations: idempotentAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.UpdateContextHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "delete_context", title: "Delete Context", description: "Delete a context file",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.DeleteContextHandler)

	addTool[contentListIn](s, toolSpec{
		name: "list_context", title: "List Context",
		description: "List all context files in the root or a specific domain with summaries",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListContextsHandler)
}

// registerCRUDSkillTools adds the skill tools.
func (s *Server) registerCRUDSkillTools() {
	addTool[skillCreateIn](s, toolSpec{
		name: "create_skill", title: "Create Skill",
		description: "Create a new skill file with optional YAML frontmatter",
		annotations: additiveAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.CreateSkillHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "read_skill", title: "Read Skill", description: "Read the content of a skill file",
		annotations: readOnlyAnnotations(), output: readOut{}, readsConfig: true,
	}, handlers.ReadSkillHandler)

	addTool[contentUpdateIn](s, toolSpec{
		name: "update_skill", title: "Update Skill", description: "Update an existing skill file atomically",
		annotations: idempotentAnnotations(), output: mutationOut{}, enums: enumsOf("priority", priorityValues),
	}, handlers.UpdateSkillHandler)

	addTool[contentRefIn](s, toolSpec{
		name: "delete_skill", title: "Delete Skill", description: "Delete a skill file",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.DeleteSkillHandler)

	addTool[contentListIn](s, toolSpec{
		name: "list_skills", title: "List Skills", description: "List all skill files in the root or a specific domain",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListSkillsHandler)
}

// registerFlatContentTools adds the agent and command tools: flat markdown files
// whose frontmatter carries their own settings.
func (s *Server) registerFlatContentTools() {
	for _, kind := range []struct {
		singular, plural, title string
		create, read, update    handlerFunc
		remove, list            handlerFunc
	}{
		{"agent", "agents", "Agent", handlers.CreateAgentHandler, handlers.ReadAgentHandler, handlers.UpdateAgentHandler, handlers.DeleteAgentHandler, handlers.ListAgentsHandler},
		{"command", "commands", "Command", handlers.CreateCommandHandler, handlers.ReadCommandHandler, handlers.UpdateCommandHandler, handlers.DeleteCommandHandler, handlers.ListCommandsHandler},
	} {
		addTool[flatCreateIn](s, toolSpec{
			name: "create_" + kind.singular, title: "Create " + kind.title,
			description: "Create a new " + kind.singular + " file in the root or a domain; a template is written when content is empty",
			annotations: additiveAnnotations(), output: mutationOut{},
		}, kind.create)
		addTool[contentRefIn](s, toolSpec{
			name: "read_" + kind.singular, title: "Read " + kind.title, description: "Read the content of " + indefinite(kind.singular) + " file",
			annotations: readOnlyAnnotations(), output: readOut{}, readsConfig: true,
		}, kind.read)
		addTool[flatUpdateIn](s, toolSpec{
			name: "update_" + kind.singular, title: "Update " + kind.title,
			description: "Replace the content of an existing " + kind.singular + " atomically",
			annotations: idempotentAnnotations(), output: mutationOut{},
		}, kind.update)
		addTool[contentRefIn](s, toolSpec{
			name: "delete_" + kind.singular, title: "Delete " + kind.title, description: "Delete " + indefinite(kind.singular) + " file",
			annotations: destructiveAnnotations(), output: mutationOut{},
		}, kind.remove)
		addTool[contentListIn](s, toolSpec{
			name: "list_" + kind.plural, title: "List " + kind.title + "s",
			description: "List all " + kind.singular + " files in the root or a specific domain",
			annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
		}, kind.list)
	}
}

func indefinite(noun string) string {
	if noun != "" && strings.ContainsRune("aeiou", rune(noun[0])) {
		return "an " + noun
	}
	return "a " + noun
}

// registerCRUDIncludeTools adds the include tools.
func (s *Server) registerCRUDIncludeTools() {
	addTool[addIncludeIn](s, toolSpec{
		name: "add_include", title: "Add Include",
		description: "Add a new include source (git URL or local path) to the configuration",
		annotations: additiveAnnotations(), output: mutationOut{},
		enums: map[string][]string{"merge_strategy": {"local-override", "include-override", "error"}},
	}, handlers.AddIncludeHandler)

	addTool[overlayNameIn](s, toolSpec{
		name: "remove_include", title: "Remove Include",
		description: "Remove an include source from the configuration",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.RemoveIncludeHandler)

	addTool[workDirArg](s, toolSpec{
		name: "list_includes", title: "List Includes",
		description: "List all include sources in the configuration",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListIncludesHandler)
}

// registerCRUDInstalledSkillTools adds the installed skill tools.
func (s *Server) registerCRUDInstalledSkillTools() {
	addTool[installSkillIn](s, toolSpec{
		name: "install_skill", title: "Install Skill",
		description: "Install a named skill from a git repository or local path",
		annotations: additiveAnnotations(), output: mutationOut{},
	}, handlers.InstallSkillHandler)

	addTool[overlayNameIn](s, toolSpec{
		name: "uninstall_skill", title: "Uninstall Skill",
		description: "Remove an installed skill from the configuration",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.UninstallSkillHandler)

	addTool[workDirArg](s, toolSpec{
		name: "list_installed_skills", title: "List Installed Skills",
		description: "List all installed skills",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListInstalledSkillsHandler)
}

// registerCRUDConfigTools adds the config tools.
func (s *Server) registerCRUDConfigTools() {
	addTool[workDirArg](s, toolSpec{
		name: "read_config", title: "Read Configuration",
		description: "Read the current project configuration and return its fields as structured JSON",
		annotations: readOnlyAnnotations(), output: configOut{},
	}, handlers.ReadConfigHandler)

	addTool[updateConfigIn](s, toolSpec{
		name: "update_config", title: "Update Configuration",
		description: "Update specific fields in the project configuration",
		annotations: idempotentAnnotations(), output: configOut{},
	}, handlers.UpdateConfigHandler)
}

// registerCRUDProfileTools adds the profile tools.
func (s *Server) registerCRUDProfileTools() {
	addTool[addProfileIn](s, toolSpec{
		name: "add_profile", title: "Add Profile",
		description: "Create a new profile with a set of domains",
		annotations: additiveAnnotations(), output: mutationOut{},
	}, handlers.AddProfileHandler)

	addTool[overlayNameIn](s, toolSpec{
		name: "remove_profile", title: "Remove Profile",
		description: "Remove a profile from the configuration",
		annotations: destructiveAnnotations(), output: mutationOut{},
	}, handlers.RemoveProfileHandler)

	addTool[overlayNameIn](s, toolSpec{
		name: "set_default_profile", title: "Set Default Profile",
		description: "Set a profile as the default",
		annotations: idempotentAnnotations(), output: mutationOut{},
	}, handlers.SetDefaultProfileHandler)

	addTool[workDirArg](s, toolSpec{
		name: "list_profiles", title: "List Profiles",
		description: "List all profiles in the configuration",
		annotations: readOnlyAnnotations(), output: listOut{}, readsConfig: true,
	}, handlers.ListProfilesHandler)
}

// registerGovernanceTools adds the read-only roles, lock and catalog tools. They
// never write and never use the network: remote includes resolve from the local
// cache only. Mutating lock operations (lock, update) stay CLI-only.
func (s *Server) registerGovernanceTools() {
	addTool[listRolesIn](s, toolSpec{
		name: "list_roles", title: "List Roles",
		description: "List the [[roles]] with their item counts and token estimates: the roles.json manifest (roles list --format json). Read-only, offline.",
		annotations: readOnlyAnnotations(), output: rolesOut{},
	}, handlers.ListRolesHandler)

	addTool[resolveRoleIn](s, toolSpec{
		name: "resolve_role", title: "Resolve Role",
		description: "Resolve what a person holding a role gets: the items it keeps with sizes, skill modes and delivery (roles resolve <role> --format json). Read-only, offline; the item list is capped at limit.",
		annotations: readOnlyAnnotations(), output: resolveRoleOut{},
	}, handlers.ResolveRoleHandler)

	addTool[lockStatusIn](s, toolSpec{
		name: "lock_status", title: "Lock Status",
		description: "Compare ai-rulez.lock with the sources, outputs, skill sources and served skills, without fetching anything (lock --check --format json). Read-only, offline; lock and update stay CLI-only. A project with no lock file is an error, as in the CLI.",
		annotations: readOnlyAnnotations(), output: lockStatusOut{},
		enums: map[string][]string{"kind": govview.LockKinds},
	}, handlers.LockStatusHandler(s.version, func(ctx context.Context, cfg *config.Config, lock *lockfile.File, views []handlers.LockView) []contentlock.Change {
		extras := make([]ServeSetup, 0, len(views))
		for _, v := range views {
			extras = append(extras, ServeSetup{Role: v.Role, Profile: v.Profile, Preset: v.Targets, IncludeStatic: v.IncludeStatic, Sources: v.Sources})
		}
		return DynamicLockChanges(ctx, cfg, lock, s.version, extras...)
	}))

	addTool[catalogIn](s, toolSpec{
		name: "catalog", title: "Catalog",
		description: "List every rule, skill, agent, command and context file with owner, version, tokens, digest, roles and lock status (catalog --format json). Read-only, offline; capped at limit items.",
		annotations: readOnlyAnnotations(), output: catalogOut{},
		enums: map[string][]string{"kind": append(append([]string(nil), config.RoleKinds...), contentlock.KindContext)},
	}, handlers.CatalogHandler(s.version))
}

// registerReportTools adds the read-only report tools: the twins of tokens, cost,
// sbom, okf validate, approve --list and validate --show-policy. They run the
// library code of the commands, never write and never use the network; the
// writes and decisions around them (approve, sign, lock) stay CLI-only.
func (s *Server) registerReportTools() {
	addTool[tokenReportIn](s, toolSpec{
		name: "token_report", title: "Token Report",
		description: "Report the prompt-token cost of the generated outputs, split by when an agent loads it: always, conditionally or on demand (tokens --format json). Rendered in memory, offline; one target answers with the report, several with {items}. A headline over budget is an error result carrying the report.",
		annotations: readOnlyAnnotations(), output: tokenReportOut{}, readsConfig: true,
		enums: map[string][]string{"tokenizer": tokens.Names()},
	}, handlers.TokenReportHandler)

	addTool[costReportIn](s, toolSpec{
		name: "cost_report", title: "Cost Report",
		description: "Report which rules, context, skills, agents and commands cost the most prompt tokens, biggest first (cost --format json). Offline. Exceeding budget or on_demand_budget is an error result carrying the report.",
		annotations: readOnlyAnnotations(), output: costReportOut{}, readsConfig: true,
		enums: map[string][]string{"tokenizer": tokens.Names()},
	}, handlers.CostReportHandler)

	addTool[sbomIn](s, toolSpec{
		name: "sbom", title: "Software Bill of Materials",
		description: "Return the bill of materials of the AI configuration as CycloneDX 1.6 or SPDX 2.3 JSON (sbom): authored items, remote includes and skill sources with their pins, MCP servers. Reproducible, built from the lock and the cache without the network and without the machine-local overlay. Read-only; writing and signing stay on the command line.",
		annotations: readOnlyAnnotations(), output: sbomOut{}, readsConfig: true,
		enums: map[string][]string{"format": {sbom.FormatCycloneDX, sbom.FormatSPDXJSON}, "files": {sbom.FilesNone, sbom.FilesSkills, sbom.FilesAll}},
	}, handlers.SBOMHandler(s.version))

	addTool[okfValidateIn](s, toolSpec{
		name: "okf_validate", title: "Validate OKF Bundle",
		description: "Check an OKF (Open Knowledge Format) bundle directory against the OKF v0.2 conformance rules and the hygiene checks AR9B0-AR9B9 (okf validate --format json). Local directories only. Findings at or above fail_on are an error result carrying the document.",
		annotations: readOnlyAnnotations(), output: okfValidateOut{},
		enums: map[string][]string{"fail_on": {"error", "warning", "info", "none"}},
	}, handlers.OKFValidateHandler)

	addTool[approvalsStatusIn](s, toolSpec{
		name: "approvals_status", title: "Approvals Status",
		description: "List what needs approval under [governance] and its status: ok, stale, missing, expired, unauthorized, denied (approve --list --format json). Read-only and offline. Approving, revoking and signing stay on the command line: an agent must not make attestations.",
		annotations: readOnlyAnnotations(), output: approvalsStatusOut{}, readsConfig: true,
	}, handlers.ApprovalsStatusHandler(s.version))

	addTool[policyShowIn](s, toolSpec{
		name: "policy_show", title: "Show Organization Policy",
		description: "Show the effective organization policy layers, the merged policy, where each key came from and what this configuration tried to loosen (validate --show-policy --format json). Read-only. A loosening attempt under an enforcing policy is an error result carrying the report.",
		annotations: readOnlyAnnotations(), output: policyShowOut{},
	}, handlers.PolicyShowWith(s.policyViewer))

	addTool[verifiersListIn](s, toolSpec{
		name: "list_verifiers", title: "List Verifiers",
		description: "List the declared verifiers with type, severity and the rule each enforces (verifiers list --format json)",
		annotations: readOnlyAnnotations(), output: verifiersListOut{}, readsConfig: true,
	}, handlers.ListVerifiersHandler)
}
