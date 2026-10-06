package config

import (
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
