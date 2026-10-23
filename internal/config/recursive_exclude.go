package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// LoadRecursiveExclude reads recursive_exclude from the config.toml of the
// project at dir, without loading the rest of the configuration: a recursive
// run needs it before it knows which configurations exist. A directory with no
// configuration has none.
func LoadRecursiveExclude(dir string) ([]string, error) {
	path := filepath.Join(dir, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path) //nolint:gosec // the project's own config file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read config")
	}
	var doc struct {
		RecursiveExclude []string `toml:"recursive_exclude"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse config")
	}
	return doc.RecursiveExclude, nil
}
