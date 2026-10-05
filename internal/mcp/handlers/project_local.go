package handlers

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// updateLocalConfig applies update_config fields to the machine-local overlay
// instead of the shared config. Clearing a field (empty string or empty map,
// including name and description) removes the local override rather than
// writing an empty one.
func updateLocalConfig(ctx context.Context, request *ToolRequest, dir string) (*mcp.CallToolResult, error) {
	// Apply to an empty config so only the fields the caller sent are known.
	scratch := &config.Config{}
	updated, err := applyConfigUpdates(scratch, request)
	if err != nil {
		return ToolError(err)
	}
	if len(updated) == 0 {
		return ToolSuccess(map[string]interface{}{
			keySuccess:   true,
			keyOperation: opUpdateConfig,
			keyMessage:   "No fields to update",
			keyUpdated:   updated,
		})
	}

	doc, err := config.OpenLocalDocInDir(dir)
	if err != nil {
		return ToolError(err)
	}
	defer doc.Close()
	if err := setLocalFields(doc, scratch, updated); err != nil {
		return ToolError(err)
	}
	if err := doc.Save(ctx); err != nil {
		return ToolError(fmt.Errorf("failed to save local config: %w", err))
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: opUpdateConfig,
		keyMessage:   "Local config updated successfully",
		keyUpdated:   updated,
		"local":      true,
		"file":       doc.Path,
	})
}

func setLocalFields(doc *config.LocalDoc, scratch *config.Config, updated []string) error {
	for _, field := range updated {
		path, value, cleared, ok := localFieldValue(scratch, field)
		if !ok {
			return fmt.Errorf("update_config field %q cannot be written to the local config", field)
		}
		var err error
		if cleared {
			err = doc.Unset(path)
		} else {
			err = doc.Set(path, value)
		}
		if err != nil {
			return err //nolint:wrapcheck // already contextual
		}
	}
	return nil
}

// localField maps an update_config field to its overlay key path and value;
// cleared reports that the field was emptied.
type localField func(c *config.Config) (path []string, value any, cleared bool)

var localFields = map[string]localField{
	keyName: func(c *config.Config) ([]string, any, bool) {
		return []string{keyName}, c.Name, c.Name == ""
	},
	keyDescription: func(c *config.Config) ([]string, any, bool) {
		return []string{keyDescription}, c.Description, c.Description == ""
	},
	keyBuiltins: func(c *config.Config) ([]string, any, bool) {
		if c.Builtins == nil {
			return []string{keyBuiltins}, nil, true
		}
		return []string{keyBuiltins}, c.Builtins.Names, false
	},
	keyGitignore: func(c *config.Config) ([]string, any, bool) {
		return []string{keyGitignore}, c.Gitignore != nil && *c.Gitignore, false
	},
	"default_effort": func(c *config.Config) ([]string, any, bool) {
		path := []string{keyDefaults, "effort"}
		if c.Defaults == nil || c.Defaults.Effort == "" {
			return path, nil, true
		}
		return path, c.Defaults.Effort, false
	},
	"default_effort_by_preset": func(c *config.Config) ([]string, any, bool) {
		path := []string{keyDefaults, "effort_by_preset"}
		if c.Defaults == nil || len(c.Defaults.EffortByPreset) == 0 {
			return path, nil, true
		}
		return path, c.Defaults.EffortByPreset, false
	},
	"rules_mode": func(c *config.Config) ([]string, any, bool) {
		path := []string{keyRules, "mode"}
		if c.Rules == nil || c.Rules.Mode == "" {
			return path, nil, true
		}
		return path, c.Rules.Mode, false
	},
	"rules_mode_by_preset": func(c *config.Config) ([]string, any, bool) {
		path := []string{keyRules, "mode_by_preset"}
		if c.Rules == nil || len(c.Rules.ModeByPreset) == 0 {
			return path, nil, true
		}
		return path, c.Rules.ModeByPreset, false
	},
}

func localFieldValue(c *config.Config, field string) (path []string, value any, cleared, ok bool) {
	fn := localFields[field]
	if fn == nil {
		return nil, nil, false, false
	}
	path, value, cleared = fn(c)
	return path, value, cleared, true
}
