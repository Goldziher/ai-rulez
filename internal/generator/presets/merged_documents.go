package presets

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

const (
	// aiRulezLatest is the npm package spec of ai-rulez's own MCP server entry.
	aiRulezLatest = "ai-rulez@latest"
	// keySchema is the top-level $schema key of a settings document.
	keySchema = "$schema"
	// keyServers is the map of MCP servers inside Xum's mcp.jsonc and OpenCode's mcp key.
	keyServers = "servers"
)

// Base-relative paths of the JSON settings documents preset generators share with
// their consumer: ai-rulez owns a few top-level keys and the user owns the rest,
// so these are read-modify-written rather than replaced (#185).
//
// The set has to be declared rather than observed, because the generator consults
// it precisely when a render did NOT emit the path — a document gated out by
// has-MCP-servers is absent from the render's outputs, and deleting it as stale
// would destroy the user's own settings.
const (
	MergedDocAgentsSettings = ".agents/settings.json"
	MergedDocGeminiSettings = ".gemini/settings.json"
	MergedDocMCPJSON        = ".mcp.json"
	MergedDocXumMCP         = ".xum/mcp.jsonc"
	MergedDocOpencodeConfig = "opencode.json"
	MergedDocPiMCP          = ".pi/mcp.json"
	MergedDocCursorMCP      = ".cursor/mcp.json"
	MergedDocVSCodeMCP      = ".vscode/mcp.json"
	MergedDocCodexConfig    = ".codex/config.toml"
	MergedDocDevinMCP       = ".devin/mcp_config.json"
	MergedDocAgentsMCP      = ".agents/mcp_config.json"
)

// MergedDocCodexHooks and MergedDocCursorHooks (hooks_documents.go) are the hooks
// documents of the codex and cursor presets.

// mergedDocumentPaths is the registry backing MergedDocumentPaths. Every path a
// preset passes to applyMergedDocument must appear here; applyMergedDocument
// fails loudly otherwise, so a new merged document cannot silently skip the
// stale-deletion guard.
var mergedDocumentPaths = []string{
	MergedDocAgentsSettings,
	MergedDocGeminiSettings,
	MergedDocMCPJSON,
	MergedDocXumMCP,
	MergedDocOpencodeConfig,
	MergedDocPiMCP,
	MergedDocCursorMCP,
	MergedDocVSCodeMCP,
	MergedDocCodexConfig,
	MergedDocDevinMCP,
	MergedDocAgentsMCP,
	MergedDocCodexHooks,
	MergedDocCursorHooks,
	MergedDocAntigravityHooks,
	MergedDocDevinHooks,
	MergedDocCursorBugbot,
	MergedDocCursorCLI,
	MergedDocVSCodeSettings,
	MergedDocDevinConfig,
}

// MergedDocumentPaths returns every base-relative, slash-separated path that a
// preset generator renders as a merged JSON document. The generator unions this
// with the equivalent set derived from provider sidecar specs.
func MergedDocumentPaths() []string {
	paths := make([]string, len(mergedDocumentPaths))
	copy(paths, mergedDocumentPaths)
	sort.Strings(paths)

	return paths
}

// MCPServerEntry builds the canonical MCP server entry shared by the tools
// whose config uses the `mcpServers` object with a stdio `command/args/env`
// form and, for remote servers, `url` plus optional `headers`. It is the shape
// pi (`url`, `headers`, `description`) and Gemini (`url`, `headers`) use; a
// tool that keys remote transport differently (Claude's `type`, Gemini's
// `httpUrl`) still builds its own entry. Returns nil for a server with nothing
// to launch or connect to.
func MCPServerEntry(server *config.MCPServer) map[string]interface{} {
	if server == nil {
		return nil
	}
	entry := map[string]interface{}{}
	switch server.GetTransport() {
	case config.TransportHTTP, config.TransportSSE:
		if server.URL == "" {
			return nil
		}
		entry["url"] = server.URL
		if server.Description != "" {
			entry[keyDescription] = server.Description
		}
		if len(server.Headers) > 0 {
			entry[keyHeaders] = server.Headers
		}
	default:
		if server.Command == "" {
			return nil
		}
		entry[keyCommand] = server.Command
		if len(server.Args) > 0 {
			entry[keyArgs] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
	}
	return entry
}

// MCPServersByKey maps the configured MCP servers onto the `mcpServers` object
// that tools with the shared stdio/url shape load, skipping any server that
// cannot be expressed (see MCPServerEntry). Pi, its only reader, expands ${VAR} in
// env and headers itself, so a placeholder that resolved from the process
// environment is written as that reference rather than as its value.
func MCPServersByKey(cfg *config.Config) map[string]interface{} {
	servers := map[string]interface{}{}
	if cfg == nil {
		return servers
	}
	for name, server := range cfg.MCPServers {
		if entry := MCPServerEntry(server); entry != nil {
			servers[name] = BracedMCPEntry(entry, server)
		}
	}
	return servers
}

// piMCPServers is the `mcpServers` object of pi's .pi/mcp.json.
func piMCPServers(cfg *config.Config) map[string]interface{} {
	return MCPServersByKey(cfg)
}

// applyMergedDocument merges the owned keys into the JSON document at path,
// rejecting a path the registry does not know about.
//
// The check is what keeps the registry honest: a preset that starts merging a new
// document without registering it would otherwise generate correctly and then have
// that document deleted as stale on the next run whose gate drops it.
//
// An empty path is accepted: it means "render a fresh document" rather than
// naming a file, so there is nothing on disk for the stale-deletion guard to
// protect and the registry has no bearing on it.
func applyMergedDocument(cfg *config.Config, path string, owned []jsonmerge.OwnedKey) (jsonmerge.Result, error) {
	return applyMergedDocumentAs(cfg, path, docmerge.FormatJSON, owned)
}

// applyMergedDocumentAs is applyMergedDocument for a document of another format.
func applyMergedDocumentAs(cfg *config.Config, path string, format docmerge.Format, owned []jsonmerge.OwnedKey) (jsonmerge.Result, error) {
	if path != "" && !isRegisteredMergedDocument(path) {
		return jsonmerge.Result{}, oops.
			With("path", path).
			Hint("Add the base-relative path to mergedDocumentPaths in presets/merged_documents.go").
			Errorf("merged JSON document path is not registered")
	}

	return docmerge.ApplyWith(cfg.ReadExisting, path, format, owned)
}

// mergeSource records what a merged output was rendered from, so that presets
// writing one document with different keys are combined rather than conflicting
// (see config.MergeSource).
func mergeSource(path string, format docmerge.Format, result jsonmerge.Result) *config.MergeSource {
	if len(result.Owned) == 0 {
		return nil
	}
	return &config.MergeSource{Path: path, Format: string(format), Owned: result.Owned}
}

// isRegisteredMergedDocument reports whether path ends in one of the registered
// base-relative paths. Matching on the tail rather than computing a relative path
// keeps callers from having to thread baseDir through their render helpers, and is
// unambiguous because every registered entry is itself a full tail from baseDir.
func isRegisteredMergedDocument(path string) bool {
	slashed := filepath.ToSlash(path)
	for _, relPath := range mergedDocumentPaths {
		if slashed == relPath || strings.HasSuffix(slashed, "/"+relPath) {
			return true
		}
	}

	return false
}

// LegacyMergeClaims is what clean may take back out of the merged document at the
// base-relative path rel when no record of what it merged exists (a document
// written by 4.23.0 or earlier, or a fresh clone). It is deliberately narrow, and
// every claim is guarded by a value: the entries the current config would render
// for the preset(s) that write the document (a hand-written server of the same
// name with another value is the user's), the self-registration entry, the
// context.fileName forms ai-rulez wrote and an opencode $schema it left as the
// only key. A server removed from the config before the first run that records
// claims is not recognized and stays.
//
// The documents .claude/settings.json and the providers' .mcp.json are covered by
// providers.LegacyMergeClaims.
func LegacyMergeClaims(rel string, cfg *config.Config) []jsonmerge.Claim {
	if cfg == nil {
		return nil
	}
	selfEntry := jsonmerge.Claim{
		Path:   []string{keyMCPServers, "ai-rulez"},
		Equals: map[string]any{keyCommand: cmdNPX, keyArgs: []string{"-y", aiRulezLatest, keyMCP}},
	}

	if claims, ok := settingsLegacyClaims(rel, cfg, selfEntry); ok {
		return claims
	}
	if claims, ok := mcpLegacyClaims(rel, cfg, selfEntry); ok {
		return claims
	}
	return harnessConfigLegacyClaims(rel, cfg)
}

// settingsLegacyClaims is LegacyMergeClaims for the documents that carry
// permissions or hooks; ok is false when rel is not one of them.
func settingsLegacyClaims(rel string, cfg *config.Config, selfEntry jsonmerge.Claim) (claims []jsonmerge.Claim, ok bool) {
	switch rel {
	case MergedDocCursorCLI:
		return permissionsLegacyClaims(cfg, config.HarnessCursor), true
	case MergedDocVSCodeSettings:
		return permissionsLegacyClaims(cfg, config.HarnessCopilot), true
	case MergedDocDevinConfig:
		// User scope merges [[hooks]] into the same document (settingsOutputs).
		return append(permissionsLegacyClaims(cfg, "devin"), hooksLegacyClaims(cfg, config.HarnessDevin)...), true
	case MergedDocGeminiSettings:
		return append(geminiLegacyClaims(cfg, selfEntry), permissionsLegacyClaims(cfg, config.HarnessGemini)...), true
	case MergedDocCodexHooks:
		return hooksLegacyClaims(cfg, config.HarnessCodex), true
	case MergedDocCursorHooks:
		return hooksLegacyClaims(cfg, config.HarnessCursor), true
	case MergedDocAntigravityHooks:
		return hooksLegacyClaims(cfg, config.HarnessAntigravity), true
	case MergedDocDevinHooks:
		return hooksLegacyClaims(cfg, config.HarnessDevin), true
	}
	return nil, false
}

// mcpLegacyClaims is LegacyMergeClaims for the MCP server documents; ok is false
// when rel is not one of them. A document whose render fails is claimed as ok
// with no claims, as it always was.
func mcpLegacyClaims(rel string, cfg *config.Config, selfEntry jsonmerge.Claim) (claims []jsonmerge.Claim, ok bool) {
	switch rel {
	case MergedDocAgentsSettings:
		claims := []jsonmerge.Claim{selfEntry}
		if len(cfg.MCPServers) > 0 {
			if result, err := (&AntigravityPresetGenerator{}).renderSettingsJSON("", cfg); err == nil {
				claims = append(claims, result.Claims...)
			}
		}
		return claims, true
	case MergedDocMCPJSON:
		return sharedMCPJSONLegacyClaims(cfg), true
	case MergedDocXumMCP:
		return memberClaimsOf([]string{keyServers}, xumServers(cfg)), true
	case MergedDocPiMCP:
		return memberClaimsOf([]string{keyMCPServers}, piMCPServers(cfg)), true
	case MergedDocCursorMCP:
		return memberClaimsOf([]string{keyMCPServers}, mcpEntries(cfg, nativeMCPEntry)), true
	case MergedDocDevinMCP:
		return memberClaimsOf([]string{keyMCPServers}, mcpEntries(cfg, devinMCPEntry)), true
	case MergedDocVSCodeMCP:
		return memberClaimsOf([]string{keyServers}, mcpEntries(cfg, VSCodeMCPEntry)), true
	case MergedDocAgentsMCP:
		if result, err := (&AntigravityPresetGenerator{}).renderMCPConfigJSON("", cfg); err == nil {
			return append([]jsonmerge.Claim{selfEntry}, result.Claims...), true
		}
		return nil, true
	}
	return nil, false
}

// sharedMCPJSONLegacyClaims is the record of the project .mcp.json entries the
// cursor and copilot presets render.
func sharedMCPJSONLegacyClaims(cfg *config.Config) []jsonmerge.Claim {
	if len(cfg.MCPServers) == 0 {
		return nil
	}
	var claims []jsonmerge.Claim
	if result, err := (&CursorPresetGenerator{}).renderMCPJSON("", cfg); err == nil {
		claims = append(claims, result.Claims...)
	}
	if result, err := (&CopilotPresetGenerator{}).renderMCPJSON("", cfg); err == nil {
		claims = append(claims, result.Claims...)
	}
	return claims
}

// harnessConfigLegacyClaims is LegacyMergeClaims for the codex and opencode
// config documents.
func harnessConfigLegacyClaims(rel string, cfg *config.Config) []jsonmerge.Claim {
	switch rel {
	case MergedDocCodexConfig:
		claims := memberClaimsOf([]string{codexKeyMCPServers}, mcpEntries(cfg, CodexMCPEntry))
		if effort := MapEffort(codexPresetName, ResolveGlobalEffort(codexPresetName, cfg)); effort != "" {
			claims = append(claims, jsonmerge.Claim{Path: []string{codexKeyReasoningEffort}, Equals: effort})
		}
		return claims
	case MergedDocOpencodeConfig:
		claims := memberClaimsOf([]string{opencodeMCPKey}, (&OpencodePresetGenerator{}).mcpServersValue(cfg))
		claims = append(claims,
			jsonmerge.Claim{Path: []string{opencodeInstructionsKey}, Elements: opencodeLocalEntries()},
			jsonmerge.Claim{Path: []string{keySchema}, Equals: opencodeSchemaURL, Alone: true})
		return append(claims, permissionsLegacyClaims(cfg, config.HarnessOpencode)...)
	}
	return nil
}

// geminiLegacyClaims is LegacyMergeClaims for .gemini/settings.json.
func geminiLegacyClaims(cfg *config.Config, selfEntry jsonmerge.Claim) []jsonmerge.Claim {
	claims := []jsonmerge.Claim{selfEntry}
	if len(cfg.MCPServers) > 0 {
		claims = append(claims, memberClaimsOf([]string{keyMCPServers}, (&GeminiPresetGenerator{}).mcpServersValue(cfg))...)
	}
	for _, names := range ownedContextFileNames() {
		claims = append(claims, jsonmerge.Claim{Path: geminiContextFileNamePath, Equals: names})
	}
	return append(claims, hooksLegacyClaims(cfg, config.HarnessGemini)...)
}

// memberClaimsOf is the record a merge of value as the members at path would leave.
func memberClaimsOf(path []string, value map[string]interface{}) []jsonmerge.Claim {
	if len(value) == 0 {
		return nil
	}
	result, err := jsonmerge.Apply("", []jsonmerge.OwnedKey{{Path: path, Value: value, Members: true}})
	if err != nil {
		return nil
	}
	return result.Claims
}

// projectRelative is the slash-separated path of a document relative to the
// project root, which is how manifests and claims key it. A scope run renders
// into the scope directory, so its root is the scope's project root.
func projectRelative(cfg *config.Config, path string) string {
	root := cfg.BaseDir
	if cfg.Run != nil && cfg.Run.Scope != nil && cfg.Run.Scope.RootDir != "" {
		root = cfg.Run.Scope.RootDir
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
