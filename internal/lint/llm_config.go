package lint

import (
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/llm"
)

// literalSecretKeys are [llm] keys that would hold a key value; the config only
// takes the name of an environment variable (api_key_env).
var literalSecretKeys = []string{"api_key", "apikey", "key", "token", "secret", "password", "authorization"}

// checkLLMConfig reports an invalid [llm] table (AR9C0): an unknown backend, a
// literal secret where an environment variable name belongs, credentials in
// base_url, or negative limits.
func (r *runner) checkLLMConfig() {
	path := r.configFilePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path) //nolint:gosec // the project's own config file
	if err != nil {
		return
	}
	text := strings.Split(string(data), "\n")
	lineOf := func(needle string) int {
		for i, l := range text {
			if strings.Contains(l, needle) {
				return i + 1
			}
		}
		return 1
	}
	var raw map[string]any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		_ = toml.Unmarshal(data, &raw) //nolint:errcheck // a parse error is reported by the loader
	default:
		_ = yaml.Unmarshal(data, &raw) //nolint:errcheck // a parse error is reported by the loader
	}
	table, _ := raw["llm"].(map[string]any) //nolint:errcheck // nil map is fine
	if table == nil && r.cfg.LLM == nil {
		return
	}
	for key := range table {
		for _, bad := range literalSecretKeys {
			if strings.EqualFold(key, bad) {
				r.docs[path] = doc{lines: text}
				r.add(CodeLLMConfigInvalid, path, lineOf(key), "llm.%s holds a secret in the config file; remove it and name an environment variable with llm.api_key_env instead", key)
			}
		}
	}
	if r.cfg.LLM == nil {
		return
	}
	if ignored := r.cfg.LLM.PrivilegedKeys(); len(ignored) > 0 {
		r.docs[path] = doc{lines: text}
		r.add(CodeLLMUntrustedKey, path, lineOf(ignored[0]), "llm: %s", llm.IgnoredKeysMessage(ignored))
	}
	for _, problem := range r.cfg.LLM.Validate() {
		r.docs[path] = doc{lines: text}
		r.add(CodeLLMConfigInvalid, path, lineOf("[llm]"), "llm: %s", problem)
	}
}
