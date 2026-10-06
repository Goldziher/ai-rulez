package sbom_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

func TestApprovalAssuranceFollowsTheRecords(t *testing.T) {
	// Arrange: one asserted and one review-linked approval of the same skill
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\nmin_approvers = 2\n",
		map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	digest := f.digestOf("skill", "s")
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: digest, Reviewer: "alice@example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z"})
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: digest, Reviewer: "github:bob", Assurance: lockfile.AssuranceReviewLinked,
			Ref: "https://github.com/o/r/pull/7#pullrequestreview-1", ApprovedAt: "2026-03-05T05:06:07Z"})
	})

	// Act
	bom := f.build(sbom.Options{})
	doc, raw := f.spdx(sbom.Options{})

	// Assert: the weakest assurance of the counted approvals, and both records are annotated
	assert.Equal(t, "asserted", propValue(findComponent(bom, "ai-rulez:item:skill::s").Properties, "ai-rulez:approval-assurance"))
	requireValid(t, spdxSchema(t), raw)
	var comments []string
	for _, p := range doc.Packages {
		if p.Name == "skill/s" {
			for _, a := range p.Annotations {
				comments = append(comments, a.Annotator+" "+a.Comment)
			}
		}
	}
	require.Len(t, comments, 2)
	assert.Contains(t, comments, "Person: alice@example.org approved "+digest+" (assurance asserted)")
	assert.Contains(t, comments, "Person: github:bob approved "+digest+" (assurance review-linked)")
}

func TestApprovalAssuranceIsReviewLinkedWhenAllAre(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\n",
		map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: f.digestOf("skill", "s"), Reviewer: "github:bob", Assurance: lockfile.AssuranceReviewLinked,
			Ref: "https://github.com/o/r/pull/7#pullrequestreview-1", ApprovedAt: "2026-03-05T05:06:07Z"})
	})

	// Act
	bom := f.build(sbom.Options{})

	// Assert
	assert.Equal(t, "review-linked", propValue(findComponent(bom, "ai-rulez:item:skill::s").Properties, "ai-rulez:approval-assurance"))
}
