// Package project loads a configuration the way the command line and the MCP
// server do: with the include and installed-skill resolvers wired in, the default
// preset registry and the git token the CLI was given. internal/config cannot
// import internal/includes or the preset packages (the dependency runs the other
// way), so something above them has to join them; doing it here keeps config free
// of process-wide registration (issue #229, step S5) and keeps the token a value
// the caller passes rather than something the resolvers read from the viper
// singleton.
//
// Callers that fetch nothing (a service loading from a snapshot, a test) can use
// internal/config directly and pass no resolvers.
package project

import (
	"context"

	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/registry"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// gitTokenKey is the viper key the root command binds to --token and AI_RULEZ_GIT_TOKEN.
const gitTokenKey = "git_token"

// Resolvers returns the include and installed-skill resolvers of this process.
// The OKF include scan runs the default security rules, so an OKF include is
// refused content a skill would be refused.
func Resolvers() config.Resolvers {
	return includes.Resolvers(viper.GetString(gitTokenKey), lint.OKFScanner(nil))
}

// Options returns opts preceded by the resolvers and the registry of this process
// (the git token comes from --token or AI_RULEZ_GIT_TOKEN). An explicit
// config.WithResolvers or config.WithRegistry in opts wins, since it comes later.
func Options(opts ...config.LoadOption) []config.LoadOption {
	all := make([]config.LoadOption, 0, len(opts)+2)
	all = append(all, config.WithResolvers(Resolvers()), config.WithRegistry(registry.Default()))
	return append(all, opts...)
}

// Load is config.LoadConfig with the process resolvers.
func Load(ctx context.Context, baseDir string, opts ...config.LoadOption) (*config.Config, error) {
	return config.LoadConfig(ctx, baseDir, Options(opts...)...) //nolint:wrapcheck // already contextual
}

// LoadDir is config.LoadConfigFromDir with the process resolvers.
func LoadDir(ctx context.Context, baseDir, configDirName string, opts ...config.LoadOption) (*config.Config, error) {
	return config.LoadConfigFromDir(ctx, baseDir, configDirName, Options(opts...)...) //nolint:wrapcheck // already contextual
}

// LoadFile is config.LoadConfigFromFile with the process resolvers.
func LoadFile(ctx context.Context, path string, opts ...config.LoadOption) (*config.Config, error) {
	return config.LoadConfigFromFile(ctx, path, Options(opts...)...) //nolint:wrapcheck // already contextual
}
