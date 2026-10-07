package airulez_test

import (
	"context"
	"fmt"
	"log"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// Plan what ai-rulez would generate for sources that exist only in memory: no
// checkout, no process and no write.
func Example() {
	ctx := context.Background()

	ws := airulez.NewMemWorkspace()
	ws.Set(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\nagents_md = false\n", 0o644)
	ws.Set(".ai-rulez/rules/style.md", "# Style\n\nBe concise.\n", 0o644)

	project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
	if err != nil {
		log.Fatal(err)
	}
	plan, err := project.Plan(ctx, airulez.PlanOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range plan.Files {
		if f.Action == "write" {
			fmt.Println(f.Action, f.Path)
		}
	}
	// Output:
	// write .claude/rules/style.md
	// write CLAUDE.md
}

// Apply a plan to a directory, or only check whether the directory already
// holds what the sources render.
func ExampleProject_Generate() {
	ctx := context.Background()
	dir, err := tempProject()
	if err != nil {
		log.Fatal(err)
	}
	ws, err := airulez.DirWorkspace(dir)
	if err != nil {
		log.Fatal(err)
	}
	project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
	if err != nil {
		log.Fatal(err)
	}

	before, _ := project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Check})
	fmt.Println("drift before:", len(before.Drift))

	if _, err := project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Write}); err != nil {
		log.Fatal(err)
	}
	project, _ = airulez.Load(ctx, airulez.Options{Workspace: ws})
	after, _ := project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Check})
	fmt.Println("drift after:", len(after.Drift))
	// Output:
	// drift before: 2
	// drift after: 0
}
