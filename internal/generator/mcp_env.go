package generator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

var mcpEnvPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// projectRootPlaceholder is a generation-time token that resolves to the project
// root (the directory containing .ai-rulez/). It lets an MCP command or arg
// reference an absolute project path without hardcoding one; it is expanded in
// command, args, and env values. Because it resolves to a machine-specific
// absolute path, generated output carrying it must be gitignored or regenerated
// per machine.
const projectRootPlaceholder = "${PROJECT_ROOT}"

// projectRootEnvName is the env-variable name the placeholder resolves through,
// so a real PROJECT_ROOT (from --env, the process env, or .env) overrides it.
const projectRootEnvName = "PROJECT_ROOT"

func (g *Generator) resolveMCPEnv() error {
	return g.resolveMCPPlaceholders(true)
}

// resolveMCPEnvForPlugin resolves MCP placeholders for a plugin bundle. Bundles
// never carry headers, so header placeholders are left unresolved rather than
// requiring secrets the output will not contain.
func (g *Generator) resolveMCPEnvForPlugin() error {
	return g.resolveMCPPlaceholders(false)
}

func (g *Generator) resolveMCPPlaceholders(withHeaders bool) error {
	if len(g.config.MCPServers) == 0 || g.lockRender {
		return nil
	}

	dotenvValues, err := g.loadMCPDotenvValues()
	if err != nil {
		return err
	}

	var unresolved []string
	for serverName, server := range g.config.MCPServers {
		if !server.IsEnabled() {
			continue
		}
		unresolved = append(unresolved, g.resolveMCPServer(serverName, server, dotenvValues, withHeaders)...)
	}

	if len(unresolved) > 0 && !g.lenientMCP {
		sort.Strings(unresolved)
		// Names only (server, key, variable): a placeholder never holds a value.
		return oops.
			With("unresolved", unresolved).
			Hint("Set missing values with --env KEY=VALUE, an environment variable, or .env").
			Errorf("unresolved MCP env placeholders: %s", strings.Join(unresolved, "; "))
	}
	return nil
}

// resolveMCPServer expands ${PROJECT_ROOT} in a server's command and args, then
// resolves ${VAR} placeholders in its env and headers. It returns the unresolved placeholders
// it found, formatted for the aggregate error.
func (g *Generator) resolveMCPServer(
	serverName string, server *config.MCPServer, dotenvValues map[string]string, withHeaders bool,
) []string {
	// ${PROJECT_ROOT} in command/args resolves to the project root. It is
	// resolved here, in the single pre-render pass, so every preset/sidecar
	// reads the expanded value without per-renderer changes.
	server.Command = expandProjectRoot(server.Command, g.config.BaseDir)
	for i := range server.Args {
		server.Args[i] = expandProjectRoot(server.Args[i], g.config.BaseDir)
	}

	var unresolved []string
	if len(server.Env) > 0 {
		var secret []string
		var refs map[string]string
		server.Env, secret, unresolved, refs = g.resolvePlaceholderMap(serverName+".env", server.Env, dotenvValues, isSensitiveEnvName)
		server.SecretEnvKeys = mergeSortedKeys(server.SecretEnvKeys, secret)
		server.EnvRefs = mergeRefs(server.EnvRefs, refs)
	}
	if withHeaders && len(server.Headers) > 0 {
		var secret, missing []string
		var refs map[string]string
		server.Headers, secret, missing, refs = g.resolvePlaceholderMap(serverName+".headers", server.Headers, dotenvValues, isSensitiveHeaderName)
		server.SecretHeaderKeys = mergeSortedKeys(server.SecretHeaderKeys, secret)
		server.HeaderRefs = mergeRefs(server.HeaderRefs, refs)
		unresolved = append(unresolved, missing...)
	}
	return unresolved
}

// resolvePlaceholderMap resolves ${VAR} placeholders in every value of values
// from --env overrides, the process env, then dotenv values (and ${PROJECT_ROOT}
// as a last resort). It returns the resolved map, the sorted keys whose values
// are secret (placeholder-sourced or a sensitive key name), and the unresolved
// placeholders, each labeled "<field>.<key> references ${NAME}". refs maps each key
// whose value held a placeholder to that value as written, so a tool that expands
// environment references itself can be given the reference instead of the secret.
func (g *Generator) resolvePlaceholderMap(
	field string, values, dotenvValues map[string]string, sensitive func(string) bool,
) (resolved map[string]string, secretKeys, unresolved []string, refs map[string]string) {
	resolved = make(map[string]string, len(values))
	secret := make(map[string]bool)
	for key, value := range values {
		wasPlaceholder := false
		// allFromProcess stays true only while every placeholder resolved through
		// the process environment: only then can a tool be handed the reference,
		// since --env, .env and PROJECT_ROOT values are not in its environment.
		allFromProcess := true
		next := mcpEnvPlaceholderPattern.ReplaceAllStringFunc(value, func(match string) string {
			wasPlaceholder = true
			name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
			if replacement, ok := g.config.MCPEnvOverrides[name]; ok {
				allFromProcess = false
				return replacement
			}
			if replacement, ok := g.host().LookupEnv(name); ok {
				return replacement
			}
			allFromProcess = false
			if replacement, ok := dotenvValues[name]; ok {
				return replacement
			}
			// A bare ${PROJECT_ROOT} resolves to the project root unless a
			// real PROJECT_ROOT was supplied above.
			if name == projectRootEnvName && g.config.BaseDir != "" {
				return g.config.BaseDir
			}
			unresolved = append(unresolved, fmt.Sprintf("%s.%s references %s", field, key, match))
			return match
		})
		resolved[key] = next
		if wasPlaceholder || sensitive(key) {
			secret[key] = true
		}
		if wasPlaceholder && allFromProcess {
			if refs == nil {
				refs = map[string]string{}
			}
			refs[key] = value
		}
	}
	return resolved, sortedMapKeys(secret), unresolved, refs
}

// mergeRefs adds refs to existing, keeping what an earlier resolution recorded
// (a second pass sees only resolved values and so finds no placeholder).
func mergeRefs(existing, refs map[string]string) map[string]string {
	if len(refs) == 0 {
		return existing
	}
	if existing == nil {
		existing = make(map[string]string, len(refs))
	}
	for k, v := range refs {
		existing[k] = v
	}
	return existing
}

func (g *Generator) loadMCPDotenvValues() (map[string]string, error) {
	files := g.config.MCPEnvFiles
	if len(files) == 0 {
		files = []string{filepath.Join(g.config.BaseDir, ".env")}
	}

	values := make(map[string]string)
	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(g.config.BaseDir, path)
		}
		parsed, err := parseDotenvFile(path)
		if err != nil {
			return nil, err
		}
		for key, value := range parsed {
			values[key] = value
		}
	}
	return values, nil
}

func parseDotenvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read env file")
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, oops.With("path", path).With("line", lineNo).Errorf("invalid env file line")
		}
		key = strings.TrimSpace(key)
		if !isValidEnvName(key) {
			return nil, oops.With("path", path).With("line", lineNo).Errorf("invalid env variable name: %s", key)
		}
		values[key] = unquoteDotenvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "scan env file")
	}
	return values, nil
}

func unquoteDotenvValue(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote != '"' && quote != '\'') || value[len(value)-1] != quote {
		return value
	}
	unquoted := value[1 : len(value)-1]
	if quote == '"' {
		replacer := strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\t`, "\t", `\"`, `"`, `\\`, `\`)
		return replacer.Replace(unquoted)
	}
	return unquoted
}

func isValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (i == 0 && !isEnvNameStart(r)) || (i > 0 && !isEnvNamePart(r)) {
			return false
		}
	}
	return true
}

func isEnvNameStart(r rune) bool {
	return r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isEnvNamePart(r rune) bool {
	return isEnvNameStart(r) || (r >= '0' && r <= '9')
}

func isSensitiveHeaderName(name string) bool { return config.IsSensitiveHeaderName(name) }

func isSensitiveEnvName(name string) bool { return config.IsSensitiveEnvName(name) }

// expandProjectRoot replaces ${PROJECT_ROOT} with the project root. A server
// with no root set (or no placeholder) is returned unchanged.
func expandProjectRoot(value, root string) string {
	if root == "" || !strings.Contains(value, projectRootPlaceholder) {
		return value
	}
	return strings.ReplaceAll(value, projectRootPlaceholder, root)
}

// normalizeProjectRoot is expandProjectRoot's inverse, applied before hashing so
// the source hash stays identical across checkout roots and machines. It also
// catches a path the user hardcoded, which is the same machine-specific input.
func normalizeProjectRoot(value, root string) string {
	if root == "" || !strings.Contains(value, root) {
		return value
	}
	return strings.ReplaceAll(value, root, projectRootPlaceholder)
}

// mcpServerForSourceHash returns a copy of server safe to feed the source hash:
// secret env values are redacted, and any resolved project root is normalized
// back to ${PROJECT_ROOT} so the hash does not depend on the checkout location
// (see TestComputeSourceHash_StableAcrossBaseDirs).
func mcpServerForSourceHash(server *config.MCPServer, root string) *config.MCPServer {
	if server == nil {
		return nil
	}
	serverCopy := *server
	serverCopy.Command = normalizeProjectRoot(serverCopy.Command, root)
	if len(serverCopy.Args) > 0 {
		args := make([]string, len(serverCopy.Args))
		for i, arg := range serverCopy.Args {
			args[i] = normalizeProjectRoot(arg, root)
		}
		serverCopy.Args = args
	}
	if len(server.Env) > 0 {
		secretKeys := make(map[string]bool, len(server.SecretEnvKeys))
		for _, key := range server.SecretEnvKeys {
			secretKeys[key] = true
		}
		serverCopy.Env = make(map[string]string, len(server.Env))
		for key, value := range server.Env {
			if secretKeys[key] || isSensitiveEnvName(key) {
				serverCopy.Env[key] = "<redacted>"
			} else {
				serverCopy.Env[key] = normalizeProjectRoot(value, root)
			}
		}
	}
	if len(server.Headers) > 0 {
		secretKeys := make(map[string]bool, len(server.SecretHeaderKeys))
		for _, key := range server.SecretHeaderKeys {
			secretKeys[key] = true
		}
		serverCopy.Headers = make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			if secretKeys[key] || isSensitiveHeaderName(key) {
				serverCopy.Headers[key] = "<redacted>"
			} else {
				serverCopy.Headers[key] = value
			}
		}
	}
	serverCopy.SecretEnvKeys = nil
	serverCopy.SecretHeaderKeys = nil
	return &serverCopy
}

// mergeSortedKeys unions previously recorded secret keys with newly detected
// ones. A repeat resolve pass sees already-expanded values, so a key that was
// secret only because it held a placeholder would otherwise be forgotten.
func mergeSortedKeys(previous, current []string) []string {
	keys := make(map[string]bool, len(previous)+len(current))
	for _, key := range previous {
		keys[key] = true
	}
	for _, key := range current {
		keys[key] = true
	}
	return sortedMapKeys(keys)
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
