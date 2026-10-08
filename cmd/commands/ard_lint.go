package commands

import (
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	pemit "github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
)

// ardPlaceholderBase stands in for the location of the published files when
// validate cannot know it (no [ard] base_url, no plugin repository); only the
// shape of the manifest is judged, never where it will be served.
const ardPlaceholderBase = "https://ard.invalid"

// ardLintOptions checks the manifest `publish --emit ard` would write: it maps
// the project the way publish does, with a fixed release time and a placeholder
// location, and hands the findings to strict lint as AR9S*. Nothing is
// reported for a project without an [ard] table.
func ardLintOptions(cfg *config.Config) []lint.Option {
	if cfg.ARD == nil {
		return nil
	}
	input := ardLintInput(cfg)
	model, _, err := pemit.ARDModel(input)
	if err != nil {
		return []lint.Option{lint.WithARD([]ard.Finding{{Rule: ard.RuleEntry, Severity: ard.SeverityError, Message: err.Error()}})}
	}
	findings, err := ard.Check(model)
	if err != nil {
		return []lint.Option{lint.WithARD([]ard.Finding{{Rule: ard.RuleSchema, Severity: ard.SeverityError, Message: err.Error()}})}
	}
	return []lint.Option{lint.WithARD(findings)}
}

func ardLintInput(cfg *config.Config) pemit.Input {
	in := pemit.Input{ARD: ardInput(cfg, "", time.Unix(0, 0).UTC())}
	if p := cfg.Plugin; p != nil {
		in.Repo, in.Tag = publish.RepoFromURL(p.Repository), "v"+p.Version
		in.Plugins = []pemit.Plugin{{
			Name: p.Name, Description: p.Description, Version: p.Version, Category: p.Category, Keywords: p.Keywords,
			BundleFile: p.Name + "-" + p.Version + ".tar.gz",
		}}
	}
	if (in.Repo == "" || in.Tag == "") && in.ARD.BaseURL == "" {
		in.ARD.BaseURL = ardPlaceholderBase
	}
	return in
}
