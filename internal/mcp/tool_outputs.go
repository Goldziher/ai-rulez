package mcp

// Result shapes. Each struct documents the structuredContent a tool returns and
// is turned into the tool's outputSchema (see outputSchemaFor): every property
// is optional, arrays may be null, and unlisted properties are allowed, so a
// result can grow a field without breaking validation. List items stay generic
// objects; their fields are described by the tool descriptions.

type opBase struct {
	Success   bool   `json:"success,omitempty" jsonschema:"True when the operation succeeded"`
	Operation string `json:"operation,omitempty" jsonschema:"Name of the tool that produced the result"`
}

// mutationOut is the result of a create, update, delete, add or remove tool.
type mutationOut struct {
	opBase
	Name    string   `json:"name,omitempty" jsonschema:"Name of the item that changed"`
	Domain  string   `json:"domain,omitempty" jsonschema:"Domain the item belongs to (empty for root)"`
	Path    string   `json:"path,omitempty" jsonschema:"Path of the file that was written"`
	Source  string   `json:"source,omitempty" jsonschema:"Source of an include or installed skill, credentials removed"`
	Domains []string `json:"domains,omitempty" jsonschema:"Domains of a profile"`
	Updated []string `json:"updated,omitempty" jsonschema:"Settings that update_config changed"`
	Message string   `json:"message,omitempty" jsonschema:"Human-readable summary"`
}

// readOut is the result of a read tool.
type readOut struct {
	opBase
	Name    string `json:"name,omitempty" jsonschema:"Item name"`
	Domain  string `json:"domain,omitempty" jsonschema:"Domain the item belongs to (empty for root)"`
	Path    string `json:"path,omitempty" jsonschema:"Path of the file"`
	Content string `json:"content,omitempty" jsonschema:"File content"`
}

// listOut is the result of a list tool; the list itself is under the key the
// tool names (rules, checks, contexts, skills, domains, includes, ...).
type listOut struct {
	opBase
	Domain          string `json:"domain,omitempty" jsonschema:"Domain that was listed (empty for root)"`
	Count           int    `json:"count" jsonschema:"Number of items listed"`
	Rules           []any  `json:"rules,omitempty" jsonschema:"Rules (list_rules)"`
	Checks          []any  `json:"checks,omitempty" jsonschema:"Checks (list_checks)"`
	Contexts        []any  `json:"contexts,omitempty" jsonschema:"Context files (list_context)"`
	Skills          []any  `json:"skills,omitempty" jsonschema:"Skills (list_skills)"`
	Domains         []any  `json:"domains,omitempty" jsonschema:"Domains (list_domains)"`
	Includes        []any  `json:"includes,omitempty" jsonschema:"Includes (list_includes)"`
	InstalledSkills []any  `json:"installed_skills,omitempty" jsonschema:"Installed skills (list_installed_skills)"`
	Profiles        []any  `json:"profiles,omitempty" jsonschema:"Profiles (list_profiles)"`
}

// configOut is the result of read_config and update_config.
type configOut struct {
	opBase
	Name                  string            `json:"name,omitempty" jsonschema:"Project name"`
	Description           string            `json:"description,omitempty" jsonschema:"Project description"`
	Presets               []string          `json:"presets,omitempty" jsonschema:"Enabled presets"`
	Profiles              any               `json:"profiles,omitempty" jsonschema:"Profiles by name"`
	Builtins              any               `json:"builtins,omitempty" jsonschema:"Enabled builtins: a list of names or a boolean"`
	Includes              []any             `json:"includes,omitempty" jsonschema:"Include sources"`
	Gitignore             bool              `json:"gitignore,omitempty" jsonschema:"Whether generate updates .gitignore"`
	AgentsMD              bool              `json:"agents_md,omitempty" jsonschema:"Whether a shared AGENTS.md is rendered"`
	DefaultEffort         string            `json:"default_effort,omitempty" jsonschema:"Default reasoning effort"`
	DefaultEffortByPreset map[string]string `json:"default_effort_by_preset,omitempty" jsonschema:"Per-preset reasoning effort"`
	RulesMode             string            `json:"rules_mode,omitempty" jsonschema:"Default rules output mode"`
	RulesModeByPreset     map[string]string `json:"rules_mode_by_preset,omitempty" jsonschema:"Per-preset rules mode"`
	LocalOverlay          map[string]any    `json:"local_overlay,omitempty" jsonschema:"The machine-local overlay: its path and the key paths it sets, never values"`
	Updated               []string          `json:"updated,omitempty" jsonschema:"Settings that update_config changed"`
	Message               string            `json:"message,omitempty" jsonschema:"Human-readable summary"`
}

// generateOut is the result of generate_outputs.
type generateOut struct {
	Message     string   `json:"message,omitempty" jsonschema:"Human-readable summary"`
	Status      string   `json:"status,omitempty" jsonschema:"With check: ok or drift"`
	Roots       any      `json:"roots,omitempty" jsonschema:"With check: how many roots were compared"`
	Blocked     int      `json:"blocked,omitempty" jsonschema:"With check: differing files generate would refuse to overwrite"`
	Differing   []any    `json:"differing,omitempty" jsonschema:"With check: the generated files that differ, with kind and path"`
	NewCommands []string `json:"new_commands,omitempty" jsonschema:"Hook and MCP commands written for the first time, for review"`
	Results     []any    `json:"results,omitempty" jsonschema:"One entry per project (recursive runs)"`
	Plan        any      `json:"plan,omitempty" jsonschema:"The planned changes of a dry run"`
}

// cleanOut is the result of clean_outputs.
type cleanOut struct {
	Message           string   `json:"message,omitempty" jsonschema:"Human-readable summary"`
	Config            string   `json:"config,omitempty" jsonschema:"Configuration directory"`
	Profile           string   `json:"profile,omitempty" jsonschema:"Profile that was cleaned"`
	Files             []string `json:"files,omitempty" jsonschema:"Generated files removed"`
	Directories       []string `json:"directories,omitempty" jsonschema:"Generated directories removed"`
	ManifestRemoved   bool     `json:"manifest_removed,omitempty" jsonschema:"Whether the generated manifest was removed"`
	GitignoreStripped bool     `json:"gitignore_stripped,omitempty" jsonschema:"Whether the managed .gitignore block was removed"`
}

// validateOut is the result of validate_config: the lint document of
// `ai-rulez validate --format json` plus the verdict.
type validateOut struct {
	Valid    bool           `json:"valid" jsonschema:"False when the configuration is invalid or findings reach fail_on"`
	FailOn   string         `json:"fail_on,omitempty" jsonschema:"Severity threshold that was applied"`
	Config   string         `json:"config,omitempty" jsonschema:"Configuration directory"`
	Error    string         `json:"error,omitempty" jsonschema:"Why the configuration could not be validated"`
	Hint     string         `json:"hint,omitempty" jsonschema:"How to fix the error"`
	Warnings []string       `json:"warnings,omitempty" jsonschema:"Findings of severity warning, as file:line: CODE message"`
	Errors   []string       `json:"errors,omitempty" jsonschema:"Findings of severity error, as file:line: CODE message"`
	Roots    []string       `json:"roots,omitempty" jsonschema:"Roots that were linted"`
	Findings []any          `json:"findings,omitempty" jsonschema:"Every finding with code, name, severity, file, line and message"`
	Summary  map[string]any `json:"summary,omitempty" jsonschema:"Finding counts by severity and by code"`
	Risk     map[string]any `json:"risk,omitempty" jsonschema:"Advisory risk score"`
}

// doctorOut is the result of doctor.
type doctorOut struct {
	OK       bool           `json:"ok" jsonschema:"True when no finding fails the run"`
	Root     string         `json:"root,omitempty" jsonschema:"Project root that was checked"`
	Summary  map[string]int `json:"summary,omitempty" jsonschema:"Finding counts by severity"`
	Findings []any          `json:"findings,omitempty" jsonschema:"Findings with severity, message and hint"`
}

// verifiersOut is the result of run_verifiers.
type verifiersOut struct {
	OK        bool           `json:"ok" jsonschema:"True when no verifier failed and all could run"`
	CannotRun bool           `json:"cannot_run,omitempty" jsonschema:"True when a verifier could not be evaluated"`
	Mode      string         `json:"mode,omitempty" jsonschema:"Evaluation mode"`
	Root      string         `json:"root,omitempty" jsonschema:"Project root"`
	Summary   map[string]int `json:"summary,omitempty" jsonschema:"Result counts by status"`
	Results   []any          `json:"results,omitempty" jsonschema:"One result per verifier"`
}

type initOut struct {
	Message string `json:"message,omitempty" jsonschema:"Human-readable summary"`
	Path    string `json:"path,omitempty" jsonschema:"Path of the config.toml that was written"`
}

type versionOut struct {
	Version string `json:"version" jsonschema:"ai-rulez version"`
}

type builtinOut struct {
	opBase
	Name    string         `json:"name,omitempty" jsonschema:"Builtin domain name"`
	Content map[string]any `json:"content,omitempty" jsonschema:"Entries grouped by type (rules, context, skills)"`
}

// Governance documents mirror the CLI's --format json output.

type rolesOut struct {
	Roles []any `json:"roles,omitempty" jsonschema:"Roles with item counts and token estimates"`
}

type resolveRoleOut struct {
	Role  string `json:"role,omitempty" jsonschema:"Role name"`
	Items []any  `json:"items,omitempty" jsonschema:"Items the role keeps, capped at limit"`
}

type lockStatusOut struct {
	InSync  bool  `json:"in_sync" jsonschema:"True when ai-rulez.lock matches the sources and outputs"`
	Changes []any `json:"changes,omitempty" jsonschema:"Differences between the lock and the working tree"`
}

type catalogOut struct {
	Items      []any `json:"items,omitempty" jsonschema:"Catalog items, capped at limit"`
	TotalItems int   `json:"total_items,omitempty" jsonschema:"Number of items before the cap"`
	Truncated  bool  `json:"truncated,omitempty" jsonschema:"True when items was cut at limit"`
}

// Skills-serving results.

type skillSearchOut struct {
	Profile string `json:"profile,omitempty" jsonschema:"Profile served"`
	Task    string `json:"task,omitempty" jsonschema:"The task that was searched for"`
	Count   int    `json:"count" jsonschema:"Number of results"`
	Results []any  `json:"results,omitempty" jsonschema:"Ranked skills"`
	Role    string `json:"role,omitempty" jsonschema:"Role the ranking favored"`
	Hint    string `json:"hint,omitempty" jsonschema:"What to try when nothing matched"`
}

type skillOut struct {
	Name       string         `json:"name,omitempty" jsonschema:"Skill name"`
	URI        string         `json:"uri,omitempty" jsonschema:"skill:// URI"`
	Digest     string         `json:"digest,omitempty" jsonschema:"Digest of the skill or file"`
	Content    string         `json:"content,omitempty" jsonschema:"File text"`
	Path       string         `json:"path,omitempty" jsonschema:"File inside the skill"`
	Truncated  bool           `json:"truncated,omitempty" jsonschema:"True when the content or list was cut"`
	Resources  []any          `json:"resources,omitempty" jsonschema:"Supporting files"`
	Provenance map[string]any `json:"provenance,omitempty" jsonschema:"Where the skill came from and whether the lock vouches for it"`
}

// Report documents mirror the CLI's --format json output.

type tokenReportOut struct {
	Profile        string         `json:"profile,omitempty" jsonschema:"Profile reported on, or role:<name> for a role report"`
	Role           string         `json:"role,omitempty" jsonschema:"Role reported on"`
	Tokenizer      map[string]any `json:"tokenizer,omitempty" jsonschema:"The token counter used"`
	HeadlinePreset string         `json:"headline_preset,omitempty" jsonschema:"Runtime with the largest always-loaded surface"`
	HeadlineAlways int            `json:"headline_always,omitempty" jsonschema:"Always-loaded tokens of the headline runtime, listing included"`
	Runtimes       []any          `json:"runtimes,omitempty" jsonschema:"Per-runtime token surface split by when it is loaded"`
	Domains        []any          `json:"domains,omitempty" jsonschema:"Per-domain tokens"`
	Budget         map[string]any `json:"budget,omitempty" jsonschema:"The budget check, when a budget was given"`
	Notes          []string       `json:"notes,omitempty" jsonschema:"Scope and caveats of the counts"`
	Items          []any          `json:"items,omitempty" jsonschema:"One report per target when several were asked for"`
	SchemaVersion  int            `json:"schema_version,omitempty" jsonschema:"Version of this document"`
}

type costReportOut struct {
	SchemaVersion int    `json:"schema_version,omitempty" jsonschema:"Version of this document"`
	Profile       string `json:"profile,omitempty" jsonschema:"Profile reported on"`
	Target        string `json:"target,omitempty" jsonschema:"Preset whose runtime totals are reported"`
	Tokenizer     string `json:"tokenizer,omitempty" jsonschema:"The token counter used"`
	Always        int    `json:"always" jsonschema:"Always-loaded tokens of the target"`
	Conditional   int    `json:"conditional" jsonschema:"Conditionally loaded tokens of the target"`
	OnDemand      int    `json:"on_demand" jsonschema:"On-demand tokens of the target"`
	Items         []any  `json:"items,omitempty" jsonschema:"Every item with its token costs, most expensive first"`
	TopAlways     []any  `json:"top_always,omitempty" jsonschema:"Biggest always-loaded offenders"`
	TopOnDemand   []any  `json:"top_on_demand,omitempty" jsonschema:"Biggest on-demand offenders"`
}

type okfValidateOut struct {
	SchemaVersion int    `json:"schema_version,omitempty" jsonschema:"Version of this document"`
	Bundle        string `json:"bundle,omitempty" jsonschema:"The bundle directory as given"`
	OKFSpec       string `json:"okf_spec,omitempty" jsonschema:"OKF specification version checked against"`
	Concepts      int    `json:"concepts" jsonschema:"Number of concepts in the bundle"`
	IndexStyle    string `json:"index_style,omitempty" jsonschema:"Index scheme of the bundle: body or frontmatter"`
	Findings      []any  `json:"findings,omitempty" jsonschema:"Findings with code, name, severity, path, line and message"`
}

type approvalsStatusOut struct {
	SchemaVersion int            `json:"schema_version,omitempty" jsonschema:"Version of this document"`
	Policy        map[string]any `json:"policy,omitempty" jsonschema:"The governance policy the list was judged against"`
	Items         []any          `json:"items,omitempty" jsonschema:"Pinned items with their approval status"`
	Orphans       []any          `json:"orphans,omitempty" jsonschema:"Approvals whose content no longer exists"`
	Summary       map[string]int `json:"summary,omitempty" jsonschema:"Counts by status"`
}

type policyShowOut struct {
	SchemaVersion int               `json:"schema_version,omitempty" jsonschema:"Version of this document"`
	Mode          string            `json:"mode,omitempty" jsonschema:"warn when the policy only warns"`
	Layers        []any             `json:"layers,omitempty" jsonschema:"The policy layers, lowest first, with origin and digest"`
	Effective     map[string]any    `json:"effective,omitempty" jsonschema:"The merged policy"`
	Provenance    map[string]string `json:"provenance,omitempty" jsonschema:"Which layer set each key"`
	Overrides     map[string]any    `json:"overrides,omitempty" jsonschema:"What the configuration tried to loosen"`
	Violations    []any             `json:"violations,omitempty" jsonschema:"Policy violations of this configuration"`
}

type builtinsListOut struct {
	Builtins []any `json:"builtins,omitempty" jsonschema:"Builtin domains with category and description"`
	Count    int   `json:"count" jsonschema:"Number of builtin domains"`
}

type verifiersListOut struct {
	Verifiers []any `json:"verifiers,omitempty" jsonschema:"Declared verifiers with type, severity and the rule they enforce"`
	Count     int   `json:"count" jsonschema:"Number of verifiers"`
}

// sbomOut is the bill of materials: a CycloneDX or SPDX document. Its shape is
// owned by those specifications, so only the identifying fields are described.
type sbomOut struct {
	BOMFormat   string `json:"bomFormat,omitempty" jsonschema:"CycloneDX: the format name"`
	SpecVersion string `json:"specVersion,omitempty" jsonschema:"CycloneDX: the specification version"`
	SPDXVersion string `json:"spdxVersion,omitempty" jsonschema:"SPDX: the specification version"`
	Components  []any  `json:"components,omitempty" jsonschema:"CycloneDX: the components"`
	Packages    []any  `json:"packages,omitempty" jsonschema:"SPDX: the packages"`
}
