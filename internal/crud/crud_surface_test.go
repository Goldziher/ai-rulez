package crud_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

func TestValidatePriority_AcceptsEveryAdvertisedLevel(t *testing.T) {
	for _, p := range []string{"critical", "high", "medium", "low", "minimal", ""} {
		assert.NoError(t, crud.ValidatePriority(p), p)
	}
	assert.Error(t, crud.ValidatePriority("bogus"))
}

func TestAddRule_RejectsAnUnknownTarget(t *testing.T) {
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)

	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{Name: "r", Targets: []string{"claude", "bogus"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestAddRule_AcceptsPresetAndGlobTargets(t *testing.T) {
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)

	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{Name: "r", Targets: []string{"claude", "cursor", "src/**"}})

	require.NoError(t, err)
}

func TestAddSkill_ScaffoldPassesDescriptionLint(t *testing.T) {
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)

	res, err := op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "sk1"})

	require.NoError(t, err)
	content, err := op.ReadFileContent(res.FullPath)
	require.NoError(t, err)
	assert.Regexp(t, `description: .{20,}`, content)
}

func TestRemoveDomain_RefusesWhileAProfileUsesIt(t *testing.T) {
	ctx := context.Background()
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)
	_, err = op.AddDomain(ctx, &crud.AddDomainRequest{Name: "d1"})
	require.NoError(t, err)
	require.NoError(t, op.AddProfile(ctx, "p1", []string{"d1"}))

	err = op.RemoveDomain(ctx, "d1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "p1")
	domains, listErr := op.ListDomains(ctx)
	require.NoError(t, listErr)
	assert.Len(t, domains, 1, "the domain must survive a refused removal")

	require.NoError(t, op.RemoveProfile(ctx, "p1"))
	require.NoError(t, op.RemoveDomain(ctx, "d1"))
}

func TestValidateNewFileName_RejectsHiddenAndDashLeading(t *testing.T) {
	for _, n := range []string{".hidden", "-flag", "a b", "x.md"} {
		assert.Error(t, crud.ValidateNewFileName(n), n)
	}
	for _, n := range []string{"code-quality", "a.b", "r_1", "Style2"} {
		assert.NoError(t, crud.ValidateNewFileName(n), n)
	}
}

func TestContentPath_NamesWhatRemoveDeletes(t *testing.T) {
	ctx := context.Background()
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)
	rule, err := op.AddRule(ctx, &crud.AddFileRequest{Name: "r"})
	require.NoError(t, err)
	skill, err := op.AddSkill(ctx, &crud.AddFileRequest{Name: "sk"})
	require.NoError(t, err)

	assert.Equal(t, rule.FullPath, op.ContentPath("", crud.ContentTypeRules, "r"))
	assert.Equal(t, skill.FullPath[:len(skill.FullPath)-len("/SKILL.md")], op.ContentPath("", crud.ContentTypeSkills, "sk"))
}
