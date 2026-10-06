package approval

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestEvaluate_ReviewLinkedRefIsCheckedOffline(t *testing.T) {
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	origin := func() (forge.Repo, bool) { return forge.Repo{Host: "github.com", Owner: "Acme", Name: "Repo"}, true }
	link := func(ref string) lockfile.Approval {
		return rec("include", "shared", digestB, "github:alice", withAssurance(lockfile.AssuranceReviewLinked, func(a *lockfile.Approval) { a.Ref = ref }))
	}
	tests := []struct {
		name   string
		policy Policy
		ref    string
		want   string
	}{
		{"any string is not a review link", Policy{Origin: origin}, "whatever", StatusUnverified},
		{"a pull request link without a review id", Policy{Origin: origin}, "https://github.com/Acme/Repo/pull/7", StatusUnverified},
		{"a review of another repository", Policy{Origin: origin}, "https://github.com/evil/Repo/pull/7#pullrequestreview-1", StatusUnverified},
		{"a review of this repository, in any letter case", Policy{Origin: origin}, "https://github.com/acme/repo/pull/7#pullrequestreview-1", StatusOK},
		{"no origin to compare with accepts a well-formed link", Policy{}, "https://github.com/o/r/pull/7#pullrequestreview-1", StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.policy.Selectors = []string{"remote"}

			// Act
			res := tt.policy.Evaluate([]lockfile.Approval{link(tt.ref)}, include, testNow)

			// Assert
			assert.Equal(t, tt.want, res.Status, res.Detail)
		})
	}
}
