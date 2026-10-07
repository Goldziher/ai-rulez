package lint

import "testing"

func TestReferenceUnknownPluginProvided(t *testing.T) {
	const market = "\n[[marketplaces]]\nname = \"basemind\"\nsource = \"Goldziher/basemind\"\n"
	tests := []struct {
		name        string
		body        string
		extraConfig string
		wantErrors  int
		wantAny     int
	}{
		{"marketplace declared", "See the `multi-agent-room` skill for coordinating a team.", market, 0, 1},
		{"plugin declared", "Run `/bm-init` to refresh.", market + "\n[[plugins]]\nmarketplace = \"basemind\"\nname = \"basemind\"\n", 0, 1},
		{"file mentions the plugin", "Install the basemind Claude Code plugin.\nRe-run `basemind init` (or `/bm-init`) afterwards.", "", 0, 1},
		{"true positive: no plugin context", "See the `ghost-skill` skill.", "", 1, 1},
		{"true positive: slash command", "Then run `/ghost-cmd`.", "", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, tt.extraConfig, nil))
			gitAdd(t, root)
			fs := lintDir(t, root)
			errs, all := 0, 0
			for _, f := range fs {
				if f.Code == CodeReferenceUnknown {
					all++
					if f.Severity == SeverityError {
						errs++
					}
				}
			}
			if errs != tt.wantErrors || all != tt.wantAny {
				t.Errorf("AR301 errors=%d all=%d, want %d/%d:\n%s", errs, all, tt.wantErrors, tt.wantAny, dump(fs))
			}
		})
	}
}
