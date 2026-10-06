package approval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestEvaluate_MaxAgeIsACeilingOnEveryApproval(t *testing.T) {
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	tests := []struct {
		name string
		rec  lockfile.Approval
		now  time.Time
		want string
	}{
		{"within max_age, no expiry", rec("include", "shared", digestB, "alice"), testNow, StatusOK},
		{"past approved_at + max_age without an expires date", rec("include", "shared", digestB, "alice"), testNow.AddDate(0, 0, 40), StatusExpired},
		{"an expires beyond the ceiling does not extend it", rec("include", "shared", digestB, "alice", expires("2099-01-01")), testNow.AddDate(0, 0, 40), StatusExpired},
		{"holds through the ceiling date", rec("include", "shared", digestB, "alice"), time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: approved 2026-10-01, max_age 30d => ceiling 2026-10-31
			p := Policy{Selectors: []string{"remote"}, MaxAge: 30 * 24 * time.Hour}

			// Act
			res := p.Evaluate([]lockfile.Approval{tt.rec}, include, tt.now)

			// Assert
			assert.Equal(t, tt.want, res.Status)
		})
	}
}
