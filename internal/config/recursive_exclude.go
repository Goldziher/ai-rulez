package config

import (
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// LoadRecursiveExclude reads recursive_exclude from the configuration of the
// project at dir (.agents/ai-rulez.toml, .ai-rulez/config.toml,
// .config/ai-rulez/config.toml or ai-rulez.toml, as discovery finds them; an
// explicit configDirName is honored exactly), without loading the rest of the
// configuration: a recursive run needs it before it knows which configurations
// exist. A directory with no configuration has none.
func LoadRecursiveExclude(dir, configDirName string) ([]string, error) {
	file, err := FindConfigFileUntil(dir, configDirName, dir)
	if err != nil {
		if errors.Is(err, ErrNoConfig) {
			return nil, nil
		}
		return nil, err
	}
	data, err := os.ReadFile(file) //nolint:gosec // the project's own config file
	if err != nil {
		return nil, oops.With("path", file).Wrapf(err, "read config")
	}
	var doc struct {
		RecursiveExclude []string `toml:"recursive_exclude"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", file).Wrapf(err, "parse config")
	}
	return doc.RecursiveExclude, nil
}

// ExcludedFromRecursion reports whether rel, a slash-separated path relative to
// the directory a recursive run starts from, is or lies below a directory one
// of patterns names: a [path.Match] glob such as "svc/*/gen" or a plain
// directory such as "tests/fixtures".
func ExcludedFromRecursion(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = path.Clean(filepath.ToSlash(pattern))
		for dir := path.Clean(rel); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if ok, _ := path.Match(pattern, dir); ok { //nolint:errcheck // a malformed pattern matches nothing
				return true
			}
		}
	}
	return false
}
