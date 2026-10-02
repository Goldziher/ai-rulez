package gitignore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceMarkedBlock(t *testing.T) {
	const (
		begin = "# BEGIN p1"
		end   = "# END p1"
		other = "# BEGIN p2\n/other\n# END p2\n"
		mine  = begin + "\n/a\n" + end + "\n"
	)
	tests := []struct {
		name    string
		content string
		block   string
		want    string
	}{
		{"appends when absent", "user\n", mine, "user\n\n" + mine},
		{"creates from empty", "", mine, mine},
		{"replaces only its own region", "user\n\n" + other + "\n" + begin + "\n/old\n" + end + "\ntail\n", mine,
			"user\n\n" + other + "\n" + mine + "\ntail\n"},
		{"leaves another project's block alone", other, mine, other + "\n" + mine},
		{"empty block removes the region", "user\n\n" + mine, "", "user\n"},
		{"removal keeps other blocks", other + "\n" + mine, "", other},
		{"removal is a no-op when absent", "user\n", "", "user\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ReplaceMarkedBlock(tt.content, begin, end, tt.block))
		})
	}
}
