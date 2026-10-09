package mcp

// Typed tool arguments. Each struct is the single source of a tool's input
// schema: the SDK infers the JSON schema from the field types, json tags
// (omitempty marks an argument optional) and jsonschema descriptions, rejects
// unknown arguments, and validates types before a handler runs.

// workDirArg is the working_directory argument every project tool takes.
type workDirArg struct {
	WorkingDirectory string `json:"working_directory,omitempty" jsonschema:"Project directory (defaults to the server's current directory, useful for polyrepo setups). Must be inside the directory the server was started in unless it runs with --allow-any-dir"`
}

// localArg selects the machine-local content tree.
type localArg struct {
	Local bool `json:"local,omitempty" jsonschema:"Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content"`
}

// overlayArg selects the machine-local config overlay.
type overlayArg struct {
	Local bool `json:"local,omitempty" jsonschema:"Apply to the machine-local config.local.* overlay instead of the shared config"`
}

// configSelArgs picks the configuration a read-only report runs against.
type configSelArgs struct {
	ConfigFile string `json:"config_file,omitempty" jsonschema:"Path to the root configuration file (optional)"`
	ConfigDir  string `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	NoLocal    bool   `json:"no_local,omitempty" jsonschema:"Ignore the machine-local config.local.* overlay and local/ content (the teammate view)"`
	workDirArg
}

// Project tools.

type generateIn struct {
	ConfigFile string `json:"config_file,omitempty" jsonschema:"Path to the root configuration file (optional)"`
	ConfigDir  string `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"Preview changes without writing files"`
	Recursive  bool   `json:"recursive,omitempty" jsonschema:"Generate for all subdirectories containing .ai-rulez/"`
	NoLocal    bool   `json:"no_local,omitempty" jsonschema:"Ignore the machine-local config.local.* overlay and local/ content (the teammate view)"`
	workDirArg
}

type cleanIn struct {
	ConfigFile    string `json:"config_file,omitempty" jsonschema:"Path to the root configuration file (optional)"`
	ConfigDir     string `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	DryRun        bool   `json:"dry_run,omitempty" jsonschema:"Preview what would be removed without deleting"`
	KeepGitignore bool   `json:"keep_gitignore,omitempty" jsonschema:"Leave the ai-rulez managed block in .gitignore"`
	KeepManifest  bool   `json:"keep_manifest,omitempty" jsonschema:"Leave the generated manifest in place"`
	workDirArg
}

type validateIn struct {
	ConfigFile  string   `json:"config_file,omitempty" jsonschema:"Path to the root configuration file to validate (optional)"`
	ConfigDir   string   `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	NoLocal     bool     `json:"no_local,omitempty" jsonschema:"Validate without the machine-local config.local.* overlay"`
	FailOn      string   `json:"fail_on,omitempty" jsonschema:"Lowest finding severity that makes the result an error: error (default), warning, info or none; the CLI's --fail-on"`
	Strict      bool     `json:"strict,omitempty" jsonschema:"Fail on warnings as well as errors (the same as fail_on warning); the CLI's --strict"`
	ConfigOnly  bool     `json:"config_only,omitempty" jsonschema:"Check the configuration file only and skip the content checks; the CLI's --config-only"`
	LintProfile string   `json:"lint_profile,omitempty" jsonschema:"Lint preset: default, strict or permissive (overrides [lint] profile); the CLI's --lint-profile"`
	Analyzers   []string `json:"analyzers,omitempty" jsonschema:"Run only these analyzers (replaces [lint] analyzers); the CLI's --analyzer"`
	workDirArg
}

type doctorIn struct {
	ConfigFile string `json:"config_file,omitempty" jsonschema:"Path to the root configuration file (optional)"`
	ConfigDir  string `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	Profile    string `json:"profile,omitempty" jsonschema:"Profile to render for the drift and gitignore checks"`
	Strict     bool   `json:"strict,omitempty" jsonschema:"Treat warnings as failures in the ok field"`
	NoLocal    bool   `json:"no_local,omitempty" jsonschema:"Ignore the machine-local config.local.* overlay and local/ content"`
	workDirArg
}

type verifiersIn struct {
	ConfigFile string `json:"config_file,omitempty" jsonschema:"Path to the root configuration file (optional)"`
	ConfigDir  string `json:"config_dir,omitempty" jsonschema:"Configuration directory name (default: .ai-rulez)"`
	Name       string `json:"name,omitempty" jsonschema:"Comma-separated verifier names to run (default: all)"`
	Strict     bool   `json:"strict,omitempty" jsonschema:"Treat failing warning-severity verifiers as failures in the ok field"`
	Since      string `json:"since,omitempty" jsonschema:"Evaluate only files changed since the merge base of this git revision and HEAD (plus uncommitted and untracked); a missing revision is an error"`
	Staged     bool   `json:"staged,omitempty" jsonschema:"Evaluate only staged changes"`
	Rule       string `json:"rule,omitempty" jsonschema:"Run only the verifiers that enforce this rule, skill, agent or command"`
	NoLocal    bool   `json:"no_local,omitempty" jsonschema:"Ignore the machine-local config.local.* overlay and local/ content"`
	workDirArg
}

type initIn struct {
	ProjectName      string   `json:"project_name,omitempty" jsonschema:"The name for the new project"`
	Providers        []string `json:"providers,omitempty" jsonschema:"A list of providers to enable (e.g., ['claude', 'cursor'])"`
	WithAgents       bool     `json:"with_agents,omitempty" jsonschema:"Include sample agent configurations"`
	AllProviders     bool     `json:"all_providers,omitempty" jsonschema:"Enable all supported providers"`
	PopularProviders bool     `json:"popular_providers,omitempty" jsonschema:"Enable a curated list of popular providers"`
	workDirArg
}

// Utility tools.

type noArgs struct{}

type showBuiltinIn struct {
	Name string `json:"name" jsonschema:"Builtin domain name (e.g., security, rust, typescript)"`
}

// Domain tools.

type createDomainIn struct {
	Name        string `json:"name" jsonschema:"Domain name (alphanumeric and underscores, 1-50 characters)"`
	Description string `json:"description,omitempty" jsonschema:"Optional domain description"`
	workDirArg
}

type nameIn struct {
	Name string `json:"name" jsonschema:"Name of the item"`
	workDirArg
}

// Content tools (rules, context, skills).

type contentCreateIn struct {
	Name     string   `json:"name" jsonschema:"Item name: the filename without the .md extension"`
	Content  string   `json:"content,omitempty" jsonschema:"Markdown content with optional YAML frontmatter"`
	Domain   string   `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	Priority string   `json:"priority,omitempty" jsonschema:"Priority level"`
	Targets  []string `json:"targets,omitempty" jsonschema:"Target providers (e.g., claude, cursor)"`
	localArg
	workDirArg
}

type contentUpdateIn struct {
	Name     string   `json:"name" jsonschema:"Item name: the filename without the .md extension"`
	Content  string   `json:"content" jsonschema:"New markdown content"`
	Domain   string   `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	Priority string   `json:"priority,omitempty" jsonschema:"Priority level"`
	Targets  []string `json:"targets,omitempty" jsonschema:"Target providers (e.g., claude, cursor)"`
	localArg
	workDirArg
}

type contentRefIn struct {
	Name   string `json:"name" jsonschema:"Item name: the filename without the .md extension"`
	Domain string `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	localArg
	workDirArg
}

type contentListIn struct {
	Domain string `json:"domain,omitempty" jsonschema:"Domain name (optional, lists root items if not specified)"`
	localArg
	workDirArg
}

// Check tools.

type checkCreateIn struct {
	Name        string   `json:"name" jsonschema:"Check filename without .md extension (letters, digits, '.', '_', '-')"`
	Content     string   `json:"content,omitempty" jsonschema:"Markdown body, or a full file with YAML frontmatter"`
	Description string   `json:"description,omitempty" jsonschema:"Short summary of the check"`
	Severity    string   `json:"severity,omitempty" jsonschema:"Severity level"`
	Tools       []string `json:"tools,omitempty" jsonschema:"Tool names the check may use"`
	Domain      string   `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	Targets     []string `json:"targets,omitempty" jsonschema:"Target providers (e.g., cursor, kilo)"`
	workDirArg
}

type checkUpdateIn struct {
	Name        string   `json:"name" jsonschema:"Check filename without .md extension"`
	Content     string   `json:"content,omitempty" jsonschema:"New markdown body (the existing frontmatter is kept), or a full file with YAML frontmatter. Optional when a field below is given"`
	Description string   `json:"description,omitempty" jsonschema:"Short summary of the check; given fields are set on the existing frontmatter"`
	Severity    string   `json:"severity,omitempty" jsonschema:"Severity level"`
	Tools       []string `json:"tools,omitempty" jsonschema:"Tool names the check may use"`
	Domain      string   `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	Targets     []string `json:"targets,omitempty" jsonschema:"Target presets or path globs (e.g., cursor, kilo, src/**)"`
	workDirArg
}

type checkRefIn struct {
	Name   string `json:"name" jsonschema:"Check filename without .md extension"`
	Domain string `json:"domain,omitempty" jsonschema:"Domain name (optional, uses root if not specified)"`
	workDirArg
}

type checkListIn struct {
	Domain string `json:"domain,omitempty" jsonschema:"Domain name (optional, lists root checks if not specified)"`
	workDirArg
}

// Include, installed skill, config and profile tools.

type addIncludeIn struct {
	Name          string   `json:"name" jsonschema:"Include name (unique identifier)"`
	Source        string   `json:"source" jsonschema:"Git URL or local filesystem path"`
	Path          string   `json:"path,omitempty" jsonschema:"Path within git repository (git sources only)"`
	Ref           string   `json:"ref,omitempty" jsonschema:"Git reference: branch, tag, or commit hash (git sources only)"`
	Include       []string `json:"include,omitempty" jsonschema:"Content types to include: rules, context, skills, agents, commands"`
	MergeStrategy string   `json:"merge_strategy,omitempty" jsonschema:"Merge strategy"`
	InstallTo     string   `json:"install_to,omitempty" jsonschema:"Installation target path (optional)"`
	overlayArg
	workDirArg
}

type overlayNameIn struct {
	Name string `json:"name" jsonschema:"Name of the include, skill or profile"`
	overlayArg
	workDirArg
}

type installSkillIn struct {
	Name   string `json:"name" jsonschema:"Skill name (unique identifier)"`
	Source string `json:"source" jsonschema:"Git URL or local filesystem path"`
	Path   string `json:"path,omitempty" jsonschema:"Path within repo to skill directory (defaults to skills/<name>)"`
	Ref    string `json:"ref,omitempty" jsonschema:"Git reference: branch, tag, or commit hash"`
	overlayArg
	workDirArg
}

type updateConfigIn struct {
	Name                  string            `json:"name,omitempty" jsonschema:"Project name"`
	Description           string            `json:"description,omitempty" jsonschema:"Project description"`
	Builtins              []string          `json:"builtins,omitempty" jsonschema:"List of builtin names to enable (replaces current builtins setting)"`
	Gitignore             bool              `json:"gitignore,omitempty" jsonschema:"Whether to update .gitignore when generating outputs"`
	AgentsMD              bool              `json:"agents_md,omitempty" jsonschema:"Render always-on rules and context once into a shared AGENTS.md and skills once into .agents/skills for tools that read them"`
	DefaultEffort         string            `json:"default_effort,omitempty" jsonschema:"Default reasoning effort for Claude Code subagents (low, medium, high, xhigh, max, inherit). Empty string clears the default."`
	DefaultEffortByPreset map[string]string `json:"default_effort_by_preset,omitempty" jsonschema:"Per-preset reasoning effort override (e.g. {\"codex\": \"high\", \"claude\": \"xhigh\"}). Each value must be one of low, medium, high, xhigh, max, inherit. Pass {} to clear."`
	RulesMode             string            `json:"rules_mode,omitempty" jsonschema:"Default rules output mode: split (one file per rule, default) or inline (rules embedded in the root file). Empty string clears it."`
	RulesModeByPreset     map[string]string `json:"rules_mode_by_preset,omitempty" jsonschema:"Per-preset rules mode override (e.g. {\"claude\": \"split\", \"cursor\": \"inline\"}). Each value must be split or inline. Pass {} to clear."`
	overlayArg
	workDirArg
}

type addProfileIn struct {
	Name    string   `json:"name" jsonschema:"Profile name (unique identifier)"`
	Domains []string `json:"domains" jsonschema:"List of domain names to include in the profile. A builtin pack is referenced as 'builtin:<name>' and is scoped to this profile."`
	overlayArg
	workDirArg
}

// Governance tools.

type listRolesIn struct {
	configSelArgs
}

type resolveRoleIn struct {
	Role  string `json:"role" jsonschema:"Role name (see list_roles)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum items returned (default 200, max 1000); the totals always count every item"`
	configSelArgs
}

type lockStatusIn struct {
	Kind          string   `json:"kind,omitempty" jsonschema:"List only the changes of this kind (default: all); in_sync still covers the whole lock"`
	Profile       string   `json:"profile,omitempty" jsonschema:"Profile whose outputs are compared (default: the profile recorded in the lock); also selects the serve view"`
	Role          string   `json:"role,omitempty" jsonschema:"Compare only this role's outputs and also check the skills it serves, as a view of their own (lock --role)"`
	Targets       string   `json:"targets,omitempty" jsonschema:"Also check the view that serves this preset's rendering of the skills (lock --targets)"`
	IncludeStatic bool     `json:"include_static,omitempty" jsonschema:"Also check the view that serves static skills too (lock --include-static)"`
	Sources       []string `json:"sources,omitempty" jsonschema:"Also check the view with these extra skill sources (lock --source)"`
	configSelArgs
}

type catalogIn struct {
	Kind  string `json:"kind,omitempty" jsonschema:"Only items of this kind (default: all)"`
	Role  string `json:"role,omitempty" jsonschema:"Only items this role keeps (see list_roles)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum items returned (default 200, max 1000); total_items and truncated report a cut"`
	configSelArgs
}

// Skills-serving tools.

type findSkillIn struct {
	Task  string `json:"task" jsonschema:"What you are about to do, in a sentence"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results (default 5, max 20)"`
	Role  string `json:"role,omitempty" jsonschema:"Rank the skills of this role first (default: the server's role)"`
}

type loadSkillIn struct {
	Name        string `json:"name" jsonschema:"Skill name from find_skill"`
	Path        string `json:"path,omitempty" jsonschema:"File inside the skill, for example references/FORMS.md (default SKILL.md)"`
	BudgetBytes int    `json:"budget_bytes,omitempty" jsonschema:"Return at most this many bytes of the file (default: the whole file)"`
}

type listSkillResourcesIn struct {
	Name   string `json:"name" jsonschema:"Skill name from find_skill"`
	Offset int    `json:"offset,omitempty" jsonschema:"Index of the first file to list (see next_offset in the result)"`
}

type searchSkillsIn struct {
	Query  string `json:"query,omitempty" jsonschema:"Words to look for; empty lists all skills"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results (default 10, max 50)"`
	Domain string `json:"domain,omitempty" jsonschema:"Only skills owned by this domain ('root' for skills in no domain)"`
}

type getSkillIn struct {
	Name string `json:"name" jsonschema:"Skill name or the skill:// URI of its SKILL.md"`
}

type readSkillFileIn struct {
	URI string `json:"uri" jsonschema:"skill://<name>/<path> URI from get_skill or skills/list"`
}
