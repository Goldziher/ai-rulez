package sbom_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

func TestReviewerRedactionKeyMakesTheTokenUnguessable(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\n", map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: f.digestOf("skill", "s"), Reviewer: "alice@example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z"})
	})
	token := func(o sbom.Options) string {
		bom, _ := f.cdx(o)
		return propValue(findComponent(bom, "ai-rulez:item:skill::s").Properties, "ai-rulez:approvers")
	}

	// Act
	unkeyed := token(sbom.Options{RedactReviewers: true})
	keyedA := token(sbom.Options{RedactReviewers: true, RedactKey: "secret-a"})
	keyedAgain := token(sbom.Options{RedactReviewers: true, RedactKey: "secret-a"})
	keyedB := token(sbom.Options{RedactReviewers: true, RedactKey: "secret-b"})

	// Assert
	assert.Equal(t, keyedA, keyedAgain, "stable for one key")
	assert.NotEqual(t, keyedA, keyedB)
	assert.NotEqual(t, unkeyed, keyedA, "a key changes the token, so it is not the public-salt hash")
	assert.Regexp(t, `^reviewer-[0-9a-f]{8}$`, keyedA)
}
