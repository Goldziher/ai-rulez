package config

import (
	"os"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// JudgeResolution is the effective [judge] setup: the repository table with its user-scope-only keys
// removed, and the user table those keys come from.
type JudgeResolution struct {
	// Repo is the repository table (nil when absent) without the user-scope-only keys.
	Repo *JudgeConfig
	// User is the user config file's table (nil when absent).
	User *JudgeConfig
	// IgnoredRepoKeys names the user-scope-only keys the repository set; they have no effect.
	IgnoredRepoKeys []string
	// UserFile is the user config path that was read ("" when none exists).
	UserFile string
}

func loadUserJudge(path string) (*JudgeConfig, error) {
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
		Judge *JudgeConfig `toml:"judge"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse user config")
	}
	return doc.Judge, nil
}

// ResolveJudge layers the repository [judge] table and the user config file's. A repository (and its
// local overlay) cannot choose where content goes or what a run spends: models, spend caps, rate
// limits, consent and the prompt-override switch are taken from the user config only, and the keys the
// repository set are reported. getenv may be nil (os.Getenv). c may be nil: a project with no
// configuration still has a user table.
func (c *Config) ResolveJudge(getenv func(string) string) (JudgeResolution, error) {
	if getenv == nil {
		getenv = func(name string) string { return ambient.Getenv(nil, name) }
	}
	var out JudgeResolution
	if c != nil && c.Judge != nil {
		repo := *c.Judge
		out.IgnoredRepoKeys = repo.userOnlyKeys()
		repo.Models, repo.MaxCostUSD, repo.MaxTokens, repo.MaxCalls = nil, 0, 0, 0
		repo.Concurrency, repo.RPM, repo.AllowLLM, repo.AllowPromptOverride = 0, 0, false, false
		out.Repo = &repo
	}
	path := UserConfigFile(getenv)
	user, err := loadUserJudge(path)
	if err != nil {
		return out, err
	}
	if user != nil {
		out.UserFile = path
		out.User = user
		if problems := user.Validate(ambient.Clock(nil).Now()); len(problems) > 0 {
			return out, oops.Errorf("user config [judge]: %s", problems[0])
		}
	}
	return out, nil
}
