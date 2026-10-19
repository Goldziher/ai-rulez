package lint

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const osvUnreachable = "\U0001F7E1 SC4: OSV.dev unreachable, using static fallback (40 packages). Results may be incomplete."

func TestDropOfflineNotices(t *testing.T) {
	deny, allow := false, true
	tests := []struct {
		name     string
		egress   *bool
		profile  string
		found    []externalFinding
		wantRule []string
		wantDrop int
	}{
		{
			name: "egress false drops the unreachable notice and keeps real SC4 findings", egress: &deny, profile: "skillspector",
			found: []externalFinding{
				{Rule: "SC4", Message: osvUnreachable, File: "requirements.txt"},
				{Rule: "SC4", Message: "SC4: pyyaml 5.3 has CVE-2020-14343", File: "requirements.txt"},
				{Rule: "SC4", Message: osvUnreachable, File: "other/requirements.txt"},
			},
			wantRule: []string{"SC4"}, wantDrop: 2,
		},
		{
			name: "a different rule with the same text is kept", egress: &deny, profile: "skillspector",
			found:    []externalFinding{{Rule: "P1", Message: osvUnreachable}},
			wantRule: []string{"P1"},
		},
		{
			name: "egress true keeps the notice", egress: &allow, profile: "skillspector",
			found:    []externalFinding{{Rule: "SC4", Message: osvUnreachable}},
			wantRule: []string{"SC4"},
		},
		{
			name: "a profile without offline notices keeps everything", egress: &deny, profile: "agnix",
			found:    []externalFinding{{Rule: "SC4", Message: osvUnreachable}},
			wantRule: []string{"SC4"},
		},
		{
			name: "no profile keeps everything", egress: &deny,
			found:    []externalFinding{{Rule: "SC4", Message: osvUnreachable}},
			wantRule: []string{"SC4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sc := fromProfile(config.LintExternal{Name: "x", Profile: tt.profile, Egress: tt.egress})
			// Act
			got, dropped := dropOfflineNotices(sc, tt.found)
			// Assert
			require.Len(t, got, len(tt.wantRule))
			for i, rule := range tt.wantRule {
				assert.Equal(t, rule, got[i].Rule)
			}
			assert.Equal(t, tt.wantDrop, dropped)
		})
	}
}
