package generator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

var mcpEnvPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

var sensitiveEnvNameParts = [...]string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"KEY",
	"CREDENTIAL",
}

func (g *Generator) resolveMCPEnv() error {
	if len(g.config.MCPServers) == 0 {
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
		if len(server.Env) == 0 {
			continue
		}
		resolved := make(map[string]string, len(server.Env))
		secretKeys := make(map[string]bool)
		for key, value := range server.Env {
			wasPlaceholder := false
			next := mcpEnvPlaceholderPattern.ReplaceAllStringFunc(value, func(match string) string {
				wasPlaceholder = true
				name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
				if replacement, ok := g.config.MCPEnvOverrides[name]; ok {
					return replacement
				}
				if replacement, ok := os.LookupEnv(name); ok {
					return replacement
				}
				if replacement, ok := dotenvValues[name]; ok {
					return replacement
				}
				unresolved = append(unresolved, fmt.Sprintf("%s.env.%s references %s", serverName, key, match))
				return match
			})
			resolved[key] = next
			if wasPlaceholder || isSensitiveEnvName(key) {
				secretKeys[key] = true
			}
		}
		server.Env = resolved
		server.SecretEnvKeys = sortedMapKeys(secretKeys)
	}

	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return oops.
			With("unresolved", unresolved).
			Hint("Set missing values with --env KEY=VALUE, an environment variable, or .env").
			Errorf("unresolved MCP env placeholders")
	}
	return nil
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

func isSensitiveEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, part := range sensitiveEnvNameParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

func mcpServerForSourceHash(server *config.MCPServer) *config.MCPServer {
	if server == nil {
		return nil
	}
	serverCopy := *server
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
				serverCopy.Env[key] = value
			}
		}
	}
	serverCopy.SecretEnvKeys = nil
	return &serverCopy
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
