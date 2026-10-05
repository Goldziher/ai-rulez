package lint

import (
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
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
	// The machine-local overlay is merged into the config, so a secret key there counts too.
	ext := filepath.Ext(path)
	local := strings.TrimSuffix(path, ext) + ".local" + ext
	mainText, mainTable := r.readLLMTable(path)
	_, localTable := r.readLLMTable(local)
	if mainTable == nil && localTable == nil && r.cfg.LLM == nil {
		return
	}
	r.flagSecretKeys(path, mainText, mainTable)
	if localTable != nil {
		localText, _ := r.readLLMTable(local)
		r.flagSecretKeys(local, localText, localTable)
	}
	if r.cfg.LLM == nil {
		return
	}
	lineOf := func(needle string) int { return lineIn(mainText, needle) }
	if ignored := r.cfg.LLM.PrivilegedKeys(); len(ignored) > 0 {
		r.docs[path] = doc{lines: mainText}
		r.add(CodeLLMUntrustedKey, path, lineOf(ignored[0]), "llm: %s", llm.IgnoredKeysMessage(ignored))
	}
	for _, problem := range r.cfg.LLM.Validate() {
		r.docs[path] = doc{lines: mainText}
		r.add(CodeLLMConfigInvalid, path, lineOf("[llm]"), "llm: %s", problem)
	}
}

// readLLMTable returns the lines of a config file and its parsed [llm] table (nil when absent).
func (r *runner) readLLMTable(path string) (lines []string, table map[string]any) {
	data, err := os.ReadFile(path) //nolint:gosec // the project's own config file
	if err != nil {
		return nil, nil
	}
	var raw map[string]any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		_ = toml.Unmarshal(data, &raw) //nolint:errcheck // a parse error is reported by the loader
	default:
		_ = yaml.Unmarshal(data, &raw) //nolint:errcheck // a parse error is reported by the loader
	}
	table, _ = raw["llm"].(map[string]any) //nolint:errcheck // nil map is fine
	return strings.Split(string(data), "\n"), table
}

func (r *runner) flagSecretKeys(path string, text []string, table map[string]any) {
	for key := range table {
		for _, bad := range literalSecretKeys {
			if strings.EqualFold(key, bad) {
				r.docs[path] = doc{lines: text}
				r.add(CodeLLMConfigInvalid, path, lineIn(text, key), "llm.%s holds a secret in the config file; remove it and name an environment variable with llm.api_key_env instead", key)
			}
		}
	}
}

func lineIn(text []string, needle string) int {
	for i, l := range text {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}
