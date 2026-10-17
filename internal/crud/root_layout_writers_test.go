package crud

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootLayoutOperatorRefusesEveryContentWriter(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(op *OperatorImpl) error
	}{
		{"AddRule", func(op *OperatorImpl) error { _, err := op.AddRule(ctx, &AddFileRequest{Name: "r"}); return err }},
		{"AddContext", func(op *OperatorImpl) error { _, err := op.AddContext(ctx, &AddFileRequest{Name: "c"}); return err }},
		{"AddSkill", func(op *OperatorImpl) error {
			_, err := op.AddSkill(ctx, &AddFileRequest{Name: "s", Description: "d"})
			return err
		}},
		{"AddAgent", func(op *OperatorImpl) error { _, err := op.AddAgent(ctx, &AddFileRequest{Name: "a"}); return err }},
		{"AddCommand", func(op *OperatorImpl) error { _, err := op.AddCommand(ctx, &AddFileRequest{Name: "c"}); return err }},
		{"AddCheck", func(op *OperatorImpl) error { _, err := op.AddCheck(ctx, &AddFileRequest{Name: "c"}); return err }},
		{"AddDomain", func(op *OperatorImpl) error { _, err := op.AddDomain(ctx, &AddDomainRequest{Name: "d"}); return err }},
		{"UpdateFile", func(op *OperatorImpl) error {
			_, err := op.UpdateFile(ctx, "", ContentTypeRules, "r", "body", "", nil)
			return err
		}},
		{"UpdateFlatItem", func(op *OperatorImpl) error {
			_, err := op.UpdateFlatItem(ctx, "", ContentTypeAgents, "a", "body")
			return err
		}},
		{"UpdateCheck", func(op *OperatorImpl) error {
			_, err := op.UpdateCheck(ctx, "", "c", "body", true, CheckFields{})
			return err
		}},
		{"RemoveFile", func(op *OperatorImpl) error { return op.RemoveFile(ctx, "", ContentTypeRules, "r") }},
		{"RequireContent", func(op *OperatorImpl) error { return op.RequireContent(ctx, "", ContentTypeRules, "r") }},
		{"RemoveDomain", func(op *OperatorImpl) error { return op.RemoveDomain(ctx, "d") }},
		{"CheckDomainRemovable", func(op *OperatorImpl) error { return op.CheckDomainRemovable(ctx, "d") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := setupRootProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)

			// Act
			got := tt.call(op)

			// Assert
			require.Error(t, got)
			assert.Contains(t, got.Error(), "no .ai-rulez/ directory")
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			assert.Len(t, entries, 1, "only ai-rulez.toml is in the project")
		})
	}
}

func TestRootLayoutOperatorListsNothing(t *testing.T) {
	// Arrange
	dir := setupRootProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)

	// Act
	domains, domainErr := op.ListDomains(context.Background())
	rules, ruleErr := op.ListFiles(context.Background(), "", ContentTypeRules)

	// Assert
	require.NoError(t, domainErr)
	assert.Empty(t, domains)
	require.NoError(t, ruleErr)
	assert.Empty(t, rules)
}
