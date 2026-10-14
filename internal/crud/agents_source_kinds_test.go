package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		want  string // exact content of the created file
		add   func(op crud.Operator, req *crud.AddFileRequest) (*crud.FileResult, error)
	}{
		{crud.ContentTypeRules, "rules", "item.md",
			"---\ntype: Decision\ntitle: Item\nx-ai-rulez:\n  kind: rule\n  id: item\n  metadata:\n    priority: medium\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
				return op.AddRule(context.Background(), r)
			}},
		{crud.ContentTypeContext, "context", "item.md",
			"---\ntype: Concept\ntitle: Item\nx-ai-rulez:\n  kind: context\n  id: item\n  metadata:\n    priority: medium\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
				return op.AddContext(context.Background(), r)
			}},
		{crud.ContentTypeSkills, "skills", "item/SKILL.md",
			"---\ntype: Playbook\ntitle: Item\ndescription: An item\nx-ai-rulez:\n  kind: skill\n  id: item\n  metadata:\n    priority: medium\n    name: item\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
				return op.AddSkill(context.Background(), r)
			}},
		{crud.ContentTypeAgents, "agents", "item.md",
			"---\ntype: Reference\ntitle: Item\nx-ai-rulez:\n  kind: agent\n  id: item\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
				return op.AddAgent(context.Background(), r)
			}},
		{crud.ContentTypeCommands, "commands", "item.md",
			"---\ntype: Reference\ntitle: Item\nx-ai-rulez:\n  kind: command\n  id: item\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
				return op.AddCommand(context.Background(), r)
			}},
		{crud.ContentTypeChecks, "checks", "item.md",
			"---\ntype: Reference\ntitle: Item\nx-ai-rulez:\n  kind: check\n  id: item\n---\n\nBody\n",
			func(op crud.Operator, r *crud.AddFileRequest) (*crud.FileResult, error) {
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
					wantPath := filepath.Join(base, k.dir, filepath.FromSlash(k.file))

					// Act
					added, addErr := k.add(op, &crud.AddFileRequest{Name: "item", Domain: domain, Description: "An item", Content: "Body\n"})
					listed, listErr := op.ListFiles(ctx, domain, k.ftype)

					// Assert
					require.NoError(t, addErr)
					require.NoError(t, listErr)
					assert.Equal(t, &crud.FileResult{Name: "item", FullPath: wantPath, Type: k.ftype, Domain: domain}, added)
					body, readErr := os.ReadFile(wantPath)
					require.NoError(t, readErr)
					wantBody := k.want
					if domain != "" { // a domain item records its domain after its id
						wantBody = strings.Replace(wantBody, "  id: item\n", "  id: item\n  domain: "+domain+"\n", 1)
					}
					assert.Equal(t, wantBody, string(body))
					assert.Equal(t, []crud.FileInfo{{Name: "item", Path: wantPath, Type: k.ftype, Domain: domain}}, listed)
					assert.NoDirExists(t, filepath.Join(root, ".ai-rulez", k.dir), "nothing lands in the unused content tree")

					// Act: remove
					removeErr := op.RemoveFile(ctx, domain, k.ftype, "item")

					// Assert
					require.NoError(t, removeErr)
					assert.NoFileExists(t, wantPath)
					after, afterErr := op.ListFiles(ctx, domain, k.ftype)
					require.NoError(t, afterErr)
					assert.Empty(t, after)
				})
			}
		}
	}
}
