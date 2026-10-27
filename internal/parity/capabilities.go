//nolint:goconst // a table names the same flags, commands and tools in many rows
package parity

// Capabilities is the table. It is a function so no package state can be
// changed behind a test's back.
func Capabilities() []Capability {
	var caps []Capability
	caps = append(caps, authoringCapabilities()...)
	caps = append(caps, contentCapabilities()...)
	caps = append(caps, buildCapabilities()...)
	caps = append(caps, reportCapabilities()...)
	caps = append(caps, governanceCapabilities()...)
	caps = append(caps, servingCapabilities()...)
	caps = append(caps, cliOnlyCapabilities()...)
	return caps
}

func authoringCapabilities() []Capability {
	return []Capability{
		{
			ID: "init", Kind: Paired, CLI: []string{"init"}, Tool: "init_project",
			Flags: []FlagPair{{Flag: "source"}},
			CLIFlags: []Exclusion{
				{Names: []string{"config-dir", "domains", "setup-hooks", "skip-content", "from", "force", "minimal"},
					Reason: "init seeds a project interactively or from a source; init_project writes the minimal config.toml and refuses to overwrite"},
			},
			ToolArgs: []Exclusion{
				{Names: []string{"project_name", "providers", "with_agents", "all_providers", "popular_providers"},
					Reason: "init takes the project name as a positional and has no provider selection flags"},
			},
		},
		{
			ID: "domain-add", Kind: Paired, CLI: []string{"domain add"}, Tool: "create_domain",
			Flags: []FlagPair{{Flag: "description"}},
		},
		{
			ID: "domain-remove", Kind: Paired, CLI: []string{"domain remove"}, Tool: "delete_domain",
		},
		{
			ID: "domain-list", Kind: Paired, CLI: []string{"domain list"}, Tool: "list_domains",
		},
		{
			ID: "include-add", Kind: Paired, CLI: []string{"include add"}, Tool: "add_include",
			Flags: []FlagPair{{Flag: "local"}, {Flag: "merge-strategy"}, {Flag: "path"}, {Flag: "ref"}, {Flag: "install-to"},
				{Flag: "include", TypeNote: "the CLI takes a comma-separated string of content types; the tool takes an array"}},
		},
		{
			ID: "include-remove", Kind: Paired, CLI: []string{"include remove"}, Tool: "remove_include",
			Flags: []FlagPair{{Flag: "local"}},
		},
		{
			ID: "include-list", Kind: Paired, CLI: []string{"include list"}, Tool: "list_includes",
		},
		{
			ID: "installed-skill-install", Kind: Paired, CLI: []string{"skill install"}, Tool: "install_skill",
			Flags: []FlagPair{{Flag: "local"}, {Flag: "path"}, {Flag: "ref"}, {Flag: "source"},
				{Flag: "include"}, {Flag: "exclude"}, {Flag: "name-prefix"}, {Flag: "trust"}, {Flag: "resources"}, {Flag: "version"},
				{Flag: "tag-prefix"}, {Flag: "include-prerelease"}, {Flag: "min-release-age"}, {Flag: "domain"},
				{Flag: "profile", Arg: "profiles"}, {Flag: "local-override"}},
		},
		{
			ID: "installed-skill-remove", Kind: Paired, CLI: []string{"skill remove"}, Tool: "uninstall_skill",
			Flags: []FlagPair{{Flag: "local"}},
		},
		{
			ID: "installed-skill-list", Kind: Paired, CLI: []string{"skill list"}, Tool: "list_installed_skills",
		},
		{
			ID: "profile-add", Kind: Paired, CLI: []string{"profile add"}, Tool: "add_profile",
			Flags: []FlagPair{{Flag: "local"}},
			CLIFlags: []Exclusion{
				{Names: []string{"set-default"}, Reason: "the tool does one thing per call; set_default_profile makes a profile the default"},
			},
			ToolArgs: []Exclusion{
				{Names: []string{"domains"}, Reason: "the CLI takes the domains as positional arguments after the profile name"},
			},
		},
		{
			ID: "profile-remove", Kind: Paired, CLI: []string{"profile remove"}, Tool: "remove_profile",
			Flags: []FlagPair{{Flag: "local"}},
		},
		{
			ID: "profile-set-default", Kind: Paired, CLI: []string{"profile set-default"}, Tool: "set_default_profile",
			Flags: []FlagPair{{Flag: "local"}},
		},
		{
			ID: "profile-list", Kind: Paired, CLI: []string{"profile list"}, Tool: "list_profiles",
		},
		{
			ID: "builtins-list", Kind: Paired, CLI: []string{"builtins list"}, Tool: "list_builtins",
		},
		{
			ID: "builtins-show", Kind: Paired, CLI: []string{"builtins show"}, Tool: "show_builtin",
		},
		{
			ID: "version", Kind: Paired, CLI: []string{"version"}, Tool: "get_version",
		},
		{
			ID: "config-read", Kind: MCPOnly, Tool: "read_config",
			Reason: "the command line reads config.toml with an editor or cat; a tool client has no filesystem, so it reads the parsed settings here",
		},
		{
			ID: "config-update", Kind: MCPOnly, Tool: "update_config",
			Reason: "the command line edits config.toml directly or through `local set`; a tool client needs a typed edit that keeps comments and ordering",
		},
	}
}

// contentCapabilities are the per-kind item tools: create, read, update, delete, list.
func contentCapabilities() []Capability {
	const (
		targetsNote = "the CLI takes a comma-separated string of providers; the tool takes an array"
	)
	var caps []Capability
	for _, kind := range []struct {
		name, plural string
		local        bool
		add          []FlagPair
		cliFlags     []Exclusion
		toolArgs     []Exclusion
	}{
		{name: "rule", plural: "rules", local: true,
			add: []FlagPair{{Flag: "priority"}, {Flag: "targets", TypeNote: targetsNote}}},
		{name: "context", plural: "context", local: true,
			add: []FlagPair{{Flag: "priority"}, {Flag: "targets", TypeNote: targetsNote}}},
		{name: "skill", plural: "skills", local: true,
			add: []FlagPair{{Flag: "description"}, {Flag: "priority"}, {Flag: "targets", TypeNote: targetsNote}},
			toolArgs: []Exclusion{{Names: []string{"scope", "harness", "allow_write"},
				Reason: "the scoped write path puts a skill in the harness's own directory of a nested scope, behind the consent flag allow_write; the command line user edits that file directly"}}},
		{name: "check", plural: "checks",
			add: []FlagPair{{Flag: "description"}, {Flag: "severity"}, {Flag: "targets", TypeNote: targetsNote},
				{Flag: "tools", TypeNote: targetsNote}}},
		{name: "agent", plural: "agents", local: true, add: []FlagPair{{Flag: "description"}}},
		{name: "command", plural: "commands", local: true, add: []FlagPair{{Flag: "description"}}},
	} {
		caps = append(caps, itemCapabilities(kind.name, kind.plural, kind.local, kind.add, kind.toolArgs)...)
	}
	return caps
}

func itemCapabilities(name, plural string, local bool, addFlags []FlagPair, toolArgs []Exclusion) []Capability {
	var localFlag []FlagPair
	if local {
		localFlag = []FlagPair{{Flag: "local"}}
	}
	listTool := "list_" + plural
	create := Capability{
		ID: name + "-add", Kind: Paired, CLI: []string{"add " + name}, Tool: "create_" + name,
		Flags:    append(append([]FlagPair{{Flag: "domain"}, {Flag: "content"}}, localFlag...), addFlags...),
		ToolArgs: toolArgs,
	}
	list := Capability{
		ID: name + "-list", Kind: Paired, CLI: []string{"list " + plural}, Tool: listTool,
		Flags: append([]FlagPair{{Flag: "domain"}}, localFlag...),
	}
	remove := Capability{
		ID: name + "-remove", Kind: Paired, CLI: []string{"remove " + name}, Tool: "delete_" + name,
		Flags: append([]FlagPair{{Flag: "domain"}}, localFlag...),
	}
	read := Capability{
		ID: name + "-read", Kind: Paired, CLI: []string{"show " + name}, Tool: "read_" + name,
		Flags: append([]FlagPair{{Flag: "domain"}}, localFlag...),
	}
	update := Capability{
		ID: name + "-update", Kind: MCPOnly, Tool: "update_" + name,
		Reason: "`edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call",
	}
	edit := Capability{
		ID: name + "-edit", Kind: CLIOnly, CLI: []string{"edit " + name},
		Reason: "partial field edits and stdin input; update_" + name + " is the whole-item replacement for a tool client",
	}
	return []Capability{create, list, remove, read, update, edit}
}

func buildCapabilities() []Capability {
	return []Capability{
		{
			ID: "generate", Kind: Paired, CLI: []string{"generate"}, Tool: "generate_outputs",
			Flags: []FlagPair{{Flag: "profile"}, {Flag: "role"}, {Flag: "check"}, {Flag: "offline"}, {Flag: "dry-run"},
				{Flag: "recursive"}, {Flag: "exclude"}, {Flag: "no-local"}, {Flag: "no-self-mcp"}},
			CLIFlags: []Exclusion{
				{Names: []string{"watch", "if-configured"}, Reason: "process-bound or hook-bound: a tool call returns once"},
				{Names: []string{"refresh-remote", "accept-findings"}, Reason: "move pins to newer remote versions and accept security findings: network pinning and trust decisions an agent must not make"},
				{Names: []string{"emit-plan"}, Reason: "writes the plan to a file or stdout; generate_outputs dry_run returns the plan in the result"},
				{Names: []string{"locked", "frozen", "verify-tags", "allow-local-drift", "force", "strict-config"},
					Reason: "gates that turn drift into an exit code or overwrite files; an agent runs generate_outputs check for drift and never forces"},
				{Names: []string{"env", "env-file"}, Reason: "pass secrets from the caller's environment; a tool call must not accept them"},
				{Names: []string{"user", "plugin", "gitignore"}, Reason: "write outside the project tree or into user scope; not exposed to tools"},
				{Names: []string{"plugin-name", "marketplace-name"}, Reason: "select plugins for --plugin, which is not exposed to tools"},
			},
		},
		{
			ID: "clean", Kind: Paired, CLI: []string{"clean"}, Tool: "clean_outputs",
			Flags: []FlagPair{{Flag: "dry-run"}, {Flag: "keep-gitignore"}, {Flag: "keep-manifest"}},
			CLIFlags: []Exclusion{
				{Names: []string{"include-edited", "user", "profile"}, Reason: "removes hand-edited or user-scope files; not exposed to tools"},
			},
		},
		{
			ID: "validate", Kind: Paired, CLI: []string{"validate"}, Tool: "validate_config", Behavior: "validate",
			Flags: []FlagPair{{Flag: "fail-on"}, {Flag: "strict"}, {Flag: "config-only"}, {Flag: "lint-profile"},
				{Flag: "analyzer", Arg: "analyzers"}, {Flag: "no-local"}},
			CLIFlags: lintCLIFlags(true),
		},
		{
			ID: "scan", Kind: Paired, CLI: []string{"scan"}, Tool: "scan_content", Behavior: "scan",
			Flags:    []FlagPair{{Flag: "fail-on"}, {Flag: "lint-profile"}, {Flag: "no-local"}},
			CLIFlags: lintCLIFlags(false),
		},
		{
			ID: "doctor", Kind: Paired, CLI: []string{"doctor"}, Tool: "doctor",
			Flags: []FlagPair{{Flag: "profile"}, {Flag: "strict"}, {Flag: "no-local"}},
			CLIFlags: []Exclusion{
				{Names: []string{"mcp-user-scope"},
					Reason: "reads the developer's harness files outside the project; the MCP server is confined to its root"},
			},
		},
		{
			ID: "judge", Kind: Paired, CLI: []string{"judge"}, Tool: "judge",
			Flags: []FlagPair{{Flag: "categories"}, {Flag: "fail-on"}, {Flag: "min-confidence"}, {Flag: "changed"}, {Flag: "since"}, {Flag: "impact"},
				{Flag: "attach-rules"}, {Flag: "attach-claude-md"}, {Flag: "explain-drops"}, {Flag: "allow-partial"}, {Flag: "estimate"},
				{Flag: "redact"}, {Flag: "allow-llm"}},
			CLIFlags: []Exclusion{
				{Names: []string{"model", "adjudicator-model", "model-for", "provider", "price-input", "price-output", "max-cost", "max-tokens", "max-calls", "concurrency", "rpm"},
					Reason: "where content goes and what a run may spend are the user's; they come from the user config file and flags the user types, never from a tool caller"},
				{Names: []string{"no-cache", "cache-readonly"}, Reason: "cache policy of the command line run"},
				{Names: []string{"allow-prompt-override"}, Reason: "trusting a repository prompt that replaces a reviewer's instructions is a human decision"},
				{Names: []string{"dry-run"}, Reason: "an alias of the estimate flag, which the tool takes as estimate"},
				{Names: []string{"skill", "skill-for"},
					Reason: "the command takes NAME or path:DIR and CATEGORY=NAME strings; the tool takes the skills and skill_for objects, which can also carry inline Markdown"},
			},
			ToolArgs: []Exclusion{
				{Names: []string{"paths"}, Reason: "the positional arguments of the command"},
				{Names: []string{"skills", "skill_for"},
					Reason: "objects with name, path or inline_markdown (reference data, never configured text); the command's --skill and --skill-for take strings"},
			},
		},
		{
			ID: "verifiers-run", Kind: Paired, CLI: []string{"verifiers run"}, Tool: "run_verifiers",
			Flags: []FlagPair{{Flag: "name", TypeNote: "the CLI takes a repeatable list; the tool takes one comma-separated string"}, {Flag: "strict"}, {Flag: "since"}, {Flag: "staged"}, {Flag: "rule"},
				{Flag: "no-local"}},
			CLIFlags: []Exclusion{
				{Names: []string{"allow-exec", "allow-llm", "gate-llm", "max-cost", "estimate"},
					Reason: "running a verifier's program or a model judge is a human decision; the tool runs the deterministic subset"},
				{Names: []string{"all", "output", "profile", "role", "fail-on", "strict-applicability"},
					Reason: "report-shaping flags of the command line run; the tool reports every result in one document"},
			},
		},
		{
			ID: "verifiers-list", Kind: Paired, CLI: []string{"verifiers list"}, Tool: "list_verifiers",
			Flags: []FlagPair{{Flag: "no-local"}},
		},
	}
}

// lintCLIFlags are the flags of `validate` and `scan` that no tool takes.
// withFix adds the autofix flags, which only validate has.
func lintCLIFlags(withFix bool) []Exclusion {
	writes := []string{"dry-run", "write-baseline", "update-baseline", "baseline", "baseline-reason", "reason", "reason-for", "min-severity", "rule", "strict-baseline", "scanner-baseline", "output"}
	if withFix {
		writes = append(writes, "fix", "fix-unsafe")
	}
	out := []Exclusion{
		{Names: writes, Reason: "writes files (autofixes, baselines, report files); a tool is read-only"},
		{Names: []string{"changed", "since", "since-depth", "since-max-files", "tracked-only", "repo-root", "no-scan-cache", "today", "show-suppressed"},
			Reason: "git-diff and git-index scoping (the tool honors [lint] tracked_only), cache and presentation controls of the command line run"},
		{Names: []string{"external", "allow-egress", "recursive"},
			Reason: "runs scanner programs, possibly with network egress, or walks other projects"},
		{Names: []string{"path", "glob", "exclude", "outputs"},
			Reason: "name files outside the content tree from the command line's working directory, or render the generated files; the tool lints the configured content"},
	}
	if withFix {
		out = append(out,
			Exclusion{
				Names:  []string{"explain", "verifiers", "offline", "show-policy", "approvals-base"},
				Reason: "prints another report (explain a code, policy_show, run_verifiers) or compares against git history",
			},
			Exclusion{
				Names:  []string{"mcp-user-scope"},
				Reason: "reads the developer's harness files outside the project; the MCP server is confined to its root",
			},
		)
	}
	return out
}

func reportCapabilities() []Capability {
	return []Capability{
		{
			ID: "tokens", Kind: Paired, CLI: []string{"tokens"}, Tool: "token_report", Behavior: "tokens",
			Flags: []FlagPair{{Flag: "profile"}, {Flag: "role"}, {Flag: "by-role"}, {Flag: "compare-profiles"}, {Flag: "tokenizer"},
				{Flag: "budget"}, {Flag: "no-local"}},
		},
		{
			ID: "cost", Kind: Paired, CLI: []string{"cost"}, Tool: "cost_report", Behavior: "cost",
			Flags: []FlagPair{{Flag: "profile"}, {Flag: "target"}, {Flag: "top"}, {Flag: "budget"}, {Flag: "on-demand-budget"},
				{Flag: "tokenizer"}, {Flag: "no-local"}},
		},
		{
			ID: "sbom", Kind: Paired, CLI: []string{"sbom"}, Tool: "sbom", Behavior: "sbom",
			Flags: []FlagPair{{Flag: "type"}, {Flag: "files"}, {Flag: "profile"}, {Flag: "role"}, {Flag: "include-outputs"},
				{Flag: "no-approvals"}},
			CLIFlags: []Exclusion{
				{Names: []string{"output", "check"}, Reason: "writes or compares a file; the tool returns the document"},
				{Names: []string{"format"}, Reason: "selects text or json for the command's report; the tool always returns structured content"},
				{Names: []string{"online"}, Reason: "contacts remote sources; tools read the lock and the cache only"},
				{Names: []string{"verify", "redact-reviewers"}, Reason: "need the signing backend or a secret from the caller's environment"},
				{Names: []string{"require-lock", "strict-pins"}, Reason: "CI gates that turn a finding into exit 2; the document carries the findings"},
				{Names: []string{"timestamp"}, Reason: "the tool returns the reproducible document, which has no timestamp"},
			},
		},
		{
			ID: "okf-validate", Kind: Paired, CLI: []string{"okf validate"}, Tool: "okf_validate", Behavior: "okf-validate",
			Flags: []FlagPair{{Flag: "fail-on"}},
			ToolArgs: []Exclusion{
				{Names: []string{"bundle"}, Reason: "the CLI takes the bundle as a positional argument; the tool reads a local directory only, never a git URL"},
			},
		},
		{
			ID: "catalog", Kind: Paired, CLI: []string{"catalog"}, Tool: "catalog",
			Flags: []FlagPair{{Flag: "role"}, {Flag: "no-local"}},
			CLIFlags: []Exclusion{
				{Names: []string{"html", "check", "clean", "base-title", "indexable", "max-items-per-page", "render-markdown", "allow-findings", "include-excerpt", "no-owners", "with-eval", "with-usage", "schema-version"},
					Reason: "writes the static site tree or joins evaluation and usage files; the tool returns the catalog document"},
			},
			ToolArgs: []Exclusion{
				{Names: []string{"kind", "limit"}, Reason: "narrow and cap the document so a model's context is not flooded; the CLI pipes to jq"},
			},
		},
		{
			ID: "roles-list", Kind: Paired, CLI: []string{"roles list"}, Tool: "list_roles",
			ToolArgs: []Exclusion{noLocalToolArg()},
		},
		{
			ID: "roles-resolve", Kind: Paired, CLI: []string{"roles resolve"}, Tool: "resolve_role",
			ToolArgs: []Exclusion{
				{Names: []string{"role"}, Reason: "the CLI takes the role as the positional <name>"},
				{Names: []string{"limit"}, Reason: "caps the item list for a model's context"},
				noLocalToolArg(),
			},
		},
	}
}

func governanceCapabilities() []Capability {
	return []Capability{
		{
			ID: "lock-check", Kind: Paired, CLI: []string{"lock"}, Mode: "check", Tool: "lock_status", Behavior: "lock-check",
			ToolArgs: []Exclusion{noLocalToolArg()},
			Flags: []FlagPair{{Flag: "kind", EnumNote: "the usage text of --kind lists the refresh kinds; with --check it also filters by content"}, {Flag: "profile"}, {Flag: "role"}, {Flag: "targets"}, {Flag: "include-static"},
				{Flag: "source", Arg: "sources"}},
		},
		{
			ID: "lock-diff", Kind: Paired, CLI: []string{"lock"}, Mode: "diff", Tool: "lock_diff",
			ToolArgs: []Exclusion{noLocalToolArg()},
			Flags:    []FlagPair{{Flag: "profile"}, {Flag: "role"}, {Flag: "targets"}, {Flag: "include-static"}, {Flag: "source", Arg: "sources"}},
		},
		{
			ID: "lock-outdated", Kind: Paired, CLI: []string{"lock"}, Mode: "outdated", Tool: "lock_outdated",
			ToolArgs: []Exclusion{
				noLocalToolArg(),
				{Names: []string{"names"}, Reason: "the CLI takes the sources as positional arguments"},
			},
			Flags: []FlagPair{{Flag: "kind", EnumNote: "the usage text of --kind lists the refresh kinds of lock; the tool takes the kinds of a version constraint"}},
			CLIFlags: []Exclusion{
				{Names: []string{"fail-on-outdated", "offline"}, Reason: "the verdict is the exit code, and the tool reads the network by design (a read-only server lists it)"},
			},
		},
		{
			ID: "update-plan", Kind: Paired, CLI: []string{"update"}, Mode: "dry-run", Tool: "update_plan",
			ToolArgs: []Exclusion{
				noLocalToolArg(),
				{Names: []string{"names"}, Reason: "the CLI takes the sources as positional arguments"},
			},
			Flags:    []FlagPair{{Flag: "kind", EnumNote: "the tool takes the kinds of a version constraint"}},
			CLIFlags: updateConsentFlags(),
		},
		{
			ID: "update", Kind: Paired, CLI: []string{"update"}, Tool: "update",
			ToolArgs: []Exclusion{
				noLocalToolArg(),
				{Names: []string{"names"}, Reason: "the CLI takes the sources as positional arguments"},
			},
			Flags:    []FlagPair{{Flag: "kind", EnumNote: "the tool takes the kinds of a version constraint"}},
			CLIFlags: append(updateConsentFlags(), Exclusion{Names: []string{"dry-run"}, Reason: "update_plan is the dry run"}),
		},
		{
			ID: "approvals-status", Kind: Paired, CLI: []string{"approve"}, Mode: "list", Tool: "approvals_status", Behavior: "approvals",
			Flags: []FlagPair{{Flag: "all"}},
		},
		{
			ID: "policy-show", Kind: Paired, CLI: []string{"validate"}, Mode: "show-policy", Tool: "policy_show", Behavior: "policy",
			Flags: []FlagPair{{Flag: "no-local"}},
		},
	}
}

func servingCapabilities() []Capability {
	return []Capability{
		{
			ID: "search", Kind: Paired, CLI: []string{"search"}, Tool: "find_skill", Server: Skills, Behavior: "search",
			Flags: []FlagPair{{Flag: "limit"}, {Flag: "role"}},
			CLIFlags: []Exclusion{
				{Names: []string{"mode", "explain", "eval", "from-evals", "k", "baseline", "output", "min", "max-flips", "dry-run", "rebuild", "items", "min-count", "purge"},
					Reason: "embedding, evaluation and index maintenance modes spend network or write files; find_skill ranks with the configured mode"},
				{Names: []string{"profile", "targets", "domain", "allow", "deny", "source", "include-static", "offline", "frozen", "allow-exec"},
					Reason: "select the served catalog; a serve-mode server fixes them when it starts, so a tool call cannot widen what is served"},
			},
			ToolArgs: []Exclusion{
				{Names: []string{"task"}, Reason: "the CLI takes the query as positional words"},
				{Names: []string{"path", "scope", "recursive"}, Reason: "narrow the ranking to what applies to a path or scope of a repository the server indexed with --discover-skills; the CLI search does not index the working tree"},
			},
		},
		{ID: "search-skills", Kind: MCPOnly, Tool: "search_skills", Server: Skills,
			Reason: "lexical listing for a client that only speaks tools; the ranker the CLI `search` shares is find_skill"},
		{ID: "load-skill", Kind: MCPOnly, Tool: "load_skill", Server: Skills,
			Reason: "reads one served skill file under the session budget; the CLI reads the file from disk"},
		{ID: "list-skill-resources", Kind: MCPOnly, Tool: "list_skill_resources", Server: Skills,
			Reason: "lists the files of a served skill for a client that has no skill:// resource support"},
		{ID: "get-skill", Kind: MCPOnly, Tool: "get_skill", Server: Skills,
			Reason: "returns a served skill with provenance and digests to a client that only speaks tools"},
		{ID: "read-skill-file", Kind: MCPOnly, Tool: "read_skill_file", Server: Skills,
			Reason: "reads a supporting file of a served skill by its skill:// URI"},
	}
}

func cliOnlyCapabilities() []Capability {
	only := func(id, reason string, cli ...string) Capability {
		return Capability{ID: id, Kind: CLIOnly, CLI: cli, Reason: reason}
	}
	return []Capability{
		only("config-layers", "lists the config.<name>.toml files beside config.toml (in .agents/.ai-rulez/ for an .agents project) and which are active; a tool client reads the merged settings through read_config", "config layers"),
		only("lock-write", "writes the lock, the trust anchor of the project, after network pinning and scan acceptance; an agent must not mint trust",
			"lock"),
		only("approve-write", "approving, revoking, denying and signing are identity-bound human attestations; an agent must not make them",
			"approve"),
		only("sign", "signs with a key, an OIDC identity or a transparency log: credentials and egress", "sign", "trust update"),
		only("verify", "verifies signatures, attestations and downloaded artifacts with the signing backend; the drift half is generate_outputs check and doctor",
			"verify"),
		only("marketplace", "adds, removes and updates installed marketplaces: network fetches that rewrite the configuration and the lock", "marketplace *"),
		only("plugin", "installs, removes and updates plugins imported from marketplaces: network fetches, hook and MCP imports that run code", "plugin *"),
		only("installed-skill-update", "re-pins installed skills over the network and rewrites the lock", "skill update"),
		only("convert-migrate", "one-shot conversions that read untrusted tool files and rewrite the tree; gated by --write or a preview",
			"convert", "migrate *"),
		only("okf-export-import", "write a whole bundle or import a foreign one into the tree", "export okf", "import okf"),
		only("publish", "builds and signs distribution artifacts and may push them", "publish *"),
		only("eval-improve", "run models and harness subprocesses and spend money", "eval *", "improve *"),
		only("review", "spends model tokens in its default mode; the offline heuristics are not exposed yet", "review *", "rubric *"),
		only("telemetry", "consent and network egress of usage data", "telemetry *"),
		only("llm", "model credentials and cost estimates for network calls", "llm *"),
		only("scanners", "run external scanner programs, possibly with egress", "scanners *"),
		only("verifiers-other", "calibrate, suggest and test run models or programs; explain prints the help text of a verifier",
			"verifiers calibrate", "verifiers explain", "verifiers suggest", "verifiers test"),
		only("local-overlay", "the machine-local overlay is per developer; tools reach it through the local argument of the item tools and update_config",
			"local *"),
		only("roles-show", "a presentation of one role; resolve_role returns what the role keeps", "roles show"),
		only("catalog-diff", "compares two catalogs through git subprocesses", "catalog diff"),
		only("list-placement", "prints where skills and commands land per harness; generate_outputs dry_run returns the same plan", "list"),
		only("cache", "inspects and deletes the index cache the MCP server keeps in the user cache directory; a tool client has no use for the host's disk layout, and deleting is a human decision", "cache *"),
		only("mcp", "starts the MCP server itself", "mcp"),
		only("guard", "a hook entry point the harness calls on tool use", "guard"),
		only("session-start", "a hook entry point the harness calls when a session starts; it runs generate", "session-start"),
	}
}

// updateConsentFlags are the flags of update that give consent to a refusal or
// widen what it touches. A tool call cannot carry them: a person decides on the
// command line, after reading the refusal the tool returned.
func updateConsentFlags() []Exclusion {
	return []Exclusion{
		{Names: []string{"accept-findings", "accept-moved-tag", "allow-downgrade"},
			Reason: "consent to a refusal (scan findings, AR732, a downgrade) stays with a person on the command line; the tool returns the refusal"},
		{Names: []string{"major", "write-config"}, Reason: "taking a new major version rewrites config.toml; that stays with a person on the command line"},
		{Names: []string{"offline"}, Reason: "update reads the remote's tags; the tool has no offline mode"},
	}
}

func noLocalToolArg() Exclusion {
	return Exclusion{Names: []string{"no_local"}, Reason: "the command always reads the view that includes the machine-local overlay; the tool can ask for the teammate view"}
}
