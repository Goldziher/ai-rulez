package config

import (
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// osView is workspace.OSView: a view of the real directory dir, for the
// path-based entry points (a content tree, a skill directory) that name their
// own root.
func osView(dir string) workspace.View { return workspace.OSView(dir) }

// View reads the workspace the configuration was loaded from. A Config built
// without one (a literal in a test) reads the real directory it names.
func (c *Config) View() workspace.View {
	if c == nil {
		return workspace.View{}
	}
	if c.Workspace != nil {
		return workspace.NewView(c.Workspace)
	}
	if c.BaseDir == "" {
		return workspace.View{}
	}
	return osView(c.BaseDir)
}

// ViewFor returns a view that can read dir: the config's workspace when dir is
// inside it, otherwise the real directory dir when the workspace is backed by the
// real file system. Included content that lives in a cache outside the project is
// read that way.
func (c *Config) ViewFor(dir string) workspace.View { return c.View().For(dir) }

// Log is the logger of this config's host (the CLI's when none was given). It is
// safe on a nil Config.
func (c *Config) Log() logger.Logger {
	if c == nil {
		return logger.Std()
	}
	return c.Host.Logger()
}

// Warn logs a warning through the host logger. The advice of a generate run goes
// through Diag instead, which a command can silence or de-duplicate; this is for
// what must always be shown. Safe on a nil Config.
func (c *Config) Warn(msg string, args ...any) {
	c.Log().Warn(msg, args...)
}

// OnceKey reports whether this is the first call for key in the life of this
// config's collector, which a second render by the same command (a clean plan and
// the clean) does not reset. Safe on a nil Config.
func (c *Config) OnceKey(key string) bool {
	if c == nil {
		return true
	}
	return c.Diag.Sticky(key)
}

// WarnOnce logs msg once through the host logger, whatever sink the run's
// advice is sent to. Safe on a nil Config.
func (c *Config) WarnOnce(key, msg string, args ...any) {
	if c.OnceKey(key) {
		c.Log().Warn(msg, args...)
	}
}

// Collector is the config's warning collector (nil, which stands for the process
// default one, outside a load). Safe on a nil Config.
func (c *Config) Collector() *diag.Collector {
	if c == nil {
		return nil
	}
	return c.Diag
}
