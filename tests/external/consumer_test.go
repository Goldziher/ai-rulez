// Package external is a separate Go module that consumes ai-rulez only through
// its public API (pkg/airulez), the way an embedding service would. It exists so
// that a change which breaks what an outside module can import or compile fails
// here, which no test inside the main module can show: the main module can reach
// internal/ and a consumer cannot.
//
// Run it with `go test ./...` from this directory (the main module's own test
// run skips it, being a different module).
package external

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

func TestAServiceCanPlanSourcesThatExistOnlyInMemory(t *testing.T) {
	// Arrange
	ctx := context.Background()
	ws := airulez.NewMemWorkspace()
	ws.Set(".ai-rulez/config.toml", "version = \"4.0\"\nname = \"consumer\"\npresets = [\"claude\", \"cursor\"]\n", 0o644)
	ws.Set(".ai-rulez/rules/style.md", "# Style\n\nBe concise.\n", 0o644)

	// Act
	project, err := airulez.Load(ctx, airulez.Options{Workspace: ws, Runner: airulez.DenyAll})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := project.Plan(ctx, airulez.PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// Assert
	if plan.Digest == "" || len(plan.Digest) != 64 {
		t.Errorf("Digest = %q, want a SHA-256 hex string", plan.Digest)
	}
	paths := map[string]bool{}
	for _, f := range plan.Files {
		paths[f.Path] = true
	}
	for _, want := range []string{"CLAUDE.md", ".cursor/rules/style.mdc"} {
		if !paths[want] {
			t.Errorf("plan lacks %s; has %v", want, plan.Files)
		}
	}
	doc, err := plan.JSON()
	if err != nil || !strings.Contains(string(doc), `"schema"`) {
		t.Errorf("JSON() = %q, %v", doc, err)
	}
}

func TestAServiceCannotWriteToAWorkspaceThatIsNotADirectory(t *testing.T) {
	ctx := context.Background()
	ws := airulez.NewMemWorkspace()
	ws.Set(".ai-rulez/config.toml", "version = \"4.0\"\nname = \"consumer\"\npresets = [\"claude\"]\n", 0o644)
	project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	_, err = project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Write})

	var apiErr *airulez.Error
	if !asError(err, &apiErr) || apiErr.Code != airulez.CodeDiskRequired {
		t.Fatalf("Generate = %v, want a %s error", err, airulez.CodeDiskRequired)
	}
}

func asError(err error, target **airulez.Error) bool {
	for err != nil {
		if e, ok := err.(*airulez.Error); ok { //nolint:errorlint // the facade returns its error directly
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
