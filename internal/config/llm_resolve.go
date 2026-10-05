package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/llm"
)

// LLMResolution is the effective [llm] setup and what the trust rule dropped.
type LLMResolution struct {
	Config llm.Config
	// Ignored names the user-scope-only keys the repository config (or its local
	// overlay) set; they have no effect. Set them in the user config file or via
	// AI_RULEZ_LLM_* instead.
	Ignored []string
	// UserFile is the user config path that was read ("" when none exists).
	UserFile string
}

// userLLMConfigPath is $XDG_CONFIG_HOME/ai-rulez/config.toml (XDG_CONFIG_HOME
// absolute) or ~/.config/ai-rulez/config.toml.
func userLLMConfigPath(getenv func(string) string) string {
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "ai-rulez", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ai-rulez", "config.toml")
}

// loadUserLLM reads the [llm] table of the user config file. A missing file or
// table is not an error.
func loadUserLLM(path string) (*llm.Config, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own config file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read user config")
	}
	var doc struct {
		LLM *llm.Config `toml:"llm"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse user config")
	}
	return doc.LLM, nil
}

// ResolveLLM layers the repository [llm] table (config plus local overlay), the
// user config file and the AI_RULEZ_LLM_* environment, applying the trust rule:
// allow_network, base_url, api_key_env and price overrides are honoured only
// from the user config file or the environment. getenv may be nil (os.Getenv).
func (c *Config) ResolveLLM(getenv func(string) string) (LLMResolution, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	var repo *llm.Config
	if c != nil {
		repo = c.LLM
	}
	path := userLLMConfigPath(getenv)
	user, err := loadUserLLM(path)
	if err != nil {
		return LLMResolution{}, err
	}
	merged, ignored := llm.Resolve(repo, user)
	out := LLMResolution{Ignored: ignored}
	if user != nil {
		out.UserFile = path
	}
	out.Config, err = merged.WithEnv(getenv)
	return out, err
}

// ResolvedLLM returns the effective [llm] table (zero value when absent) with the
// trust rule and the AI_RULEZ_LLM_* environment overrides applied.
func (c *Config) ResolvedLLM() (llm.Config, error) {
	r, err := c.ResolveLLM(nil)
	return r.Config, err
}
