package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/crud"
)

func localOperator(t *testing.T) (*crud.OperatorImpl, string) {
	t.Helper()
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".ai-rulez", "rules"), 0o755))
	op, err := crud.NewOperator(base)
	require.NoError(t, err)
	return op, filepath.Join(base, ".ai-rulez")
}

func TestLocalOperator_ContentRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		ftype  string
		domain string
		add    func(op *crud.OperatorImpl, req *crud.AddFileRequest) (*crud.FileResult, error)
		rel    string // path below .ai-rulez/local
	}{
		{"rule", crud.ContentTypeRules, "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddRule(context.Background(), r)
		}, "rules/item.md"},
		{"context", crud.ContentTypeContext, "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddContext(context.Background(), r)
		}, "context/item.md"},
		{"skill", crud.ContentTypeSkills, "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddSkill(context.Background(), r)
		}, "skills/item/SKILL.md"},
		{"agent", crud.ContentTypeAgents, "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddAgent(context.Background(), r)
		}, "agents/item.md"},
		{"command", crud.ContentTypeCommands, "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddCommand(context.Background(), r)
		}, "commands/item.md"},
		{"rule in a local domain", crud.ContentTypeRules, "team", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddRule(context.Background(), r)
		}, "domains/team/rules/item.md"},
		{"skill in a local domain", crud.ContentTypeSkills, "team", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddSkill(context.Background(), r)
		}, "domains/team/skills/item/SKILL.md"},
		{"agent in a local domain", crud.ContentTypeAgents, "team", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddAgent(context.Background(), r)
		}, "domains/team/agents/item.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			shared, cfgDir := localOperator(t)
			op := shared.Local()
			ctx := context.Background()

			// Act: add
			res, err := tt.add(op, &crud.AddFileRequest{Name: "item", Domain: tt.domain, Content: ""})

			// Assert: written below local/, nothing in the shared tree
			require.NoError(t, err)
			want := filepath.Join(cfgDir, "local", filepath.FromSlash(tt.rel))
			assert.Equal(t, want, res.FullPath)
			assert.FileExists(t, want)
			sharedFiles, err := shared.ListFiles(ctx, tt.domain, tt.ftype)
			if tt.domain == "" {
				require.NoError(t, err)
				assert.Empty(t, sharedFiles)
			} else {
				assert.Error(t, err, "the shared tree has no such domain")
			}

			// list
			listed, err := op.ListFiles(ctx, tt.domain, tt.ftype)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.Equal(t, "item", listed[0].Name)

			// update
			_, err = op.UpdateFile(ctx, tt.domain, tt.ftype, "item", "---\ndescription: d\n---\n\nUPDATED\n", "", nil)
			require.NoError(t, err)
			data, err := os.ReadFile(want)
			require.NoError(t, err)
			assert.Contains(t, string(data), "UPDATED")

			// duplicate add fails
			_, err = tt.add(op, &crud.AddFileRequest{Name: "item", Domain: tt.domain})
			assert.Error(t, err)

			// remove
			require.NoError(t, op.RemoveFile(ctx, tt.domain, tt.ftype, "item"))
			assert.NoFileExists(t, want)
		})
	}
}

func TestSharedOperator_RequiresExistingDomain(t *testing.T) {
	// Arrange
	op, _ := localOperator(t)

	// Act
	_, err := op.AddAgent(context.Background(), &crud.AddFileRequest{Name: "x", Domain: "ghost"})

	// Assert: only the local operator creates a domain on demand.
	assert.Error(t, err)
}
