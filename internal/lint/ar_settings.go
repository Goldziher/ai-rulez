package lint

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// Codes for the rules that read the project's .claude/settings.json.
const (
	CodeSettingsPermissionsBroad = "AR039"
	CodeSettingsKeyTypo          = "AR040"
)

const (
	settingsKeyHooks  = "hooks"
	settingsKeyModel  = "model"
	settingsKeySchema = "$schema"
	modeBypass        = "bypassPermissions"
)

// claudeSettingsKeys are the top-level keys of a Claude Code settings file. AR040 only reports an
// unknown key that is one or two edits away from one of these, so a key added by a newer Claude
// Code release is not reported but a misspelled one is.
func claudeSettingsKeys() []string {
	return []string{
		settingsKeySchema, "agent", "allowedMcpServers", "allowManagedHooksOnly", "alwaysThinkingEnabled", "apiKeyHelper",
		"attribution", "autoUpdatesChannel", "availableModels", "awsAuthRefresh", "awsCredentialExport",
		"cleanupPeriodDays", "companyAnnouncements", "deniedMcpServers", "disableAllHooks", "disabledMcpjsonServers",
		"enableAllProjectMcpServers", "enabledMcpjsonServers", "enabledPlugins", "env", "extraKnownMarketplaces",
		"fileSuggestion", "forceLoginMethod", "forceLoginOrgUUID", settingsKeyHooks, "includeCoAuthoredBy", "includeGitInstructions",
		"language", settingsKeyModel, "outputStyle", "permissions", "plansDirectory", "prefersReducedMotion", "respectGitignore",
		"sandbox", "skipWebFetchPreflight", "spinnerTipsEnabled", "spinnerVerbs", "statusLine", "strictKnownMarketplaces",
		"teammateMode", "useAutoModeDuringPlan", "worktree",
	}
}

func claudePermissionKeys() []string {
	return []string{"additionalDirectories", "allow", "ask", "defaultMode", "deny", "disableBypassPermissionsMode"}
}

func registerArSettings(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeSettingsPermissionsBroad, "settings-permissions-broad", SeverityWarning, "the project's .claude/settings.json allows every tool or unrestricted Bash, or sets defaultMode bypassPermissions, for everyone who opens the repository"},
		RuleInfo{CodeSettingsKeyTypo, "settings-key-typo", SeverityInfo, "a key of the project's .claude/settings.json is one or two edits away from a known key, so it is probably a typo that Claude Code ignores silently"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeSettingsPermissionsBroad: {
			Why:  "A committed settings file applies to every contributor and every agent session. Allowing `Bash(*)` (or every tool) or starting in bypassPermissions mode removes the confirmation step for all of them, so any text the model reads can run commands unattended.",
			Bad:  "`\"permissions\": {\"allow\": [\"Bash(*)\"], \"defaultMode\": \"bypassPermissions\"}`",
			Good: "Allow the commands the project needs (`Bash(go test:*)`) and leave the mode at its default; keep personal shortcuts in `settings.local.json`",
		},
		CodeSettingsKeyTypo: {
			Why:  "Claude Code ignores a settings key it does not know, so a misspelling such as `permissionz` or `envv` silently drops the setting.",
			Bad:  "`\"permissionz\": {\"deny\": [\"Read(.env)\"]}`",
			Good: "`\"permissions\": {\"deny\": [\"Read(.env)\"]}`",
		},
	})
	s.addRunCheck(checkSettingsPosture, AnalyzerSecurity)
}

// broadAllowEntry reports whether a permissions.allow entry grants every tool or any shell command.
func broadAllowEntry(entry string) bool {
	switch strings.ToLower(strings.TrimSpace(entry)) {
	case "*", "bash", "bash(*)", "bash(**)", "bash(:*)":
		return true
	}
	return false
}

func checkSettingsPosture(r *runner) {
	file := filepath.Join(r.rootAbs(), dirClaude, "settings.json")
	data, err := readSmallFile(file)
	if err != nil {
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return
	}
	lines := r.fileLines(file)
	line := func(key string) int { return lineContaining(lines, quoteNeedle(key)) }
	checkSettingsKeys(r, file, "settings", root, claudeSettingsKeys(), line)
	var perms map[string]json.RawMessage
	if raw, ok := root["permissions"]; !ok || json.Unmarshal(raw, &perms) != nil {
		return
	}
	checkSettingsKeys(r, file, "permissions", perms, claudePermissionKeys(), line)
	checkBroadPermissions(r, file, perms, line)
}

// checkSettingsKeys reports each key of obj that is a near miss of a known key.
func checkSettingsKeys(r *runner, file, where string, obj map[string]json.RawMessage, known []string, line func(string) int) {
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		if slices.Contains(known, key) {
			continue
		}
		if near := closest(key, known, 2); near != "" {
			r.add(CodeSettingsKeyTypo, file, line(key), "%s key %q is not a Claude Code setting; did you mean %q?", where, key, near)
		}
	}
}

// checkBroadPermissions reports allow entries that grant everything and the bypass mode.
func checkBroadPermissions(r *runner, file string, perms map[string]json.RawMessage, line func(string) int) {
	var allow []string
	if raw, ok := perms["allow"]; ok && json.Unmarshal(raw, &allow) == nil {
		for _, entry := range allow {
			if broadAllowEntry(entry) {
				r.add(CodeSettingsPermissionsBroad, file, line(entry), "permissions.allow has %q, which runs any command or tool without asking; list the commands the project needs", entry)
			}
		}
	}
	var mode string
	if raw, ok := perms["defaultMode"]; ok && json.Unmarshal(raw, &mode) == nil && mode == modeBypass {
		r.add(CodeSettingsPermissionsBroad, file, line("defaultMode"), "permissions.defaultMode is bypassPermissions, so every session of this repository starts without confirmation prompts")
	}
}
