package lint

import (
	"strings"
	"testing"
)

func TestProjectTraps_RootLayoutReadsNoTrapDirectory(t *testing.T) {
	// Arrange: a traps/ folder at the project root belongs to the project, not to ai-rulez,
	// and a malformed file in it must not be reported as a trap definition.
	const badTrap = "[[trap]]\nname = \"x\"\nharness = \"t\"\nmessage = \"m\"\n[trap.scope]\ndir = \"../out\"\n[trap.predicate]\nkind = \"size-over\"\n"
	tests := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"directory layout reports the bad row", map[string]string{".ai-rulez/config.toml": trapConfig(`"claude"`, ""), ".ai-rulez/traps/b.toml": badTrap}, 1},
		{"root layout never reads traps/", map[string]string{"ai-rulez.toml": trapConfig(`"claude"`, ""), "traps/b.toml": badTrap}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := codeHits(trapFindings(t, tc.files), CodeProjectTrap)

			// Assert
			if len(got) != tc.want {
				t.Fatalf("want %d AR9CA findings, got %d: %+v", tc.want, len(got), got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Message, "inside the project") {
				t.Errorf("unexpected message %q", got[0].Message)
			}
		})
	}
}
