package crud_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

func TestOperatorAddsListsAndRemovesEveryKindInTheAgentsTree(t *testing.T) {
	kinds := []struct {
		ftype string
		dir   string
		file  string // path of the item below its kind directory
		add   func(op crud.Operator, req *crud.AddFileRequest) (*crud.FileResult, error)
	}{
		{crud.ContentTypeRules, "rules", "item.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddRule(context.Background(), r)
		}},
		{crud.ContentTypeContext, "context", "item.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddContext(context.Background(), r)
		}},
		{crud.ContentTypeSkills, "skills", "item/SKILL.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddSkill(context.Background(), r)
		}},
		{crud.ContentTypeAgents, "agents", "item.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddAgent(context.Background(), r)
		}},
		{crud.ContentTypeCommands, "commands", "item.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddCommand(context.Background(), r)
		}},
		{crud.ContentTypeChecks, "checks", "item.md", func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddCheck(context.Background(), r)
		}},
	}
	for _, p := range agentsProjects {
		for _, k := range kinds {
			for _, domain := range []string{"", "backend"} {
				name := p.name + "/" + k.ftype
				if domain != "" {
					name += "/domain"
				}
				t.Run(name, func(t *testing.T) {
					// Arrange
					root := agentsProject(t, p.configPath, p.configBody)
					op, err := crud.NewOperator(root)
					require.NoError(t, err)
					ctx := context.Background()
					if domain != "" {
						_, err = op.AddDomain(ctx, &crud.AddDomainRequest{Name: domain, Description: "Backend"})
						require.NoError(t, err)
					}
					base := filepath.Join(root, ".agents")
					if domain != "" {
						base = filepath.Join(base, "domains", domain)
					}

					// Act
					added, addErr := k.add(op, &crud.AddFileRequest{Name: "item", Domain: domain, Description: "An item", Content: "Body\n"})
					listed, listErr := op.ListFiles(ctx, domain, k.ftype)

					// Assert
					require.NoError(t, addErr)
					require.NoError(t, listErr)
					assert.Equal(t, filepath.Join(base, k.dir, filepath.FromSlash(k.file)), added.FullPath)
					assert.FileExists(t, added.FullPath)
					require.Len(t, listed, 1)
					assert.Equal(t, "item", listed[0].Name)
					assert.NoDirExists(t, filepath.Join(root, ".ai-rulez", k.dir), "nothing lands in the unused content tree")

					// Act: remove
					removeErr := op.RemoveFile(ctx, domain, k.ftype, "item")

					// Assert
					require.NoError(t, removeErr)
					assert.NoFileExists(t, added.FullPath)
					after, afterErr := op.ListFiles(ctx, domain, k.ftype)
					require.NoError(t, afterErr)
					assert.Empty(t, after)
				})
			}
		}
	}
}
