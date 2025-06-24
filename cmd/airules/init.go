package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/airules/internal/config"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules project",
	Long: `Initialize a new AI rules project with a basic configuration file
and example rules. This creates an ai_rules.yaml file in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := "My Project"
		if len(args) > 0 {
			projectName = args[0]
		}

		// Check if ai_rules.yaml already exists
		if _, err := os.Stat("ai_rules.yaml"); err == nil {
			fmt.Fprintf(os.Stderr, "Error: ai_rules.yaml already exists in current directory\n")
			os.Exit(1)
		}

		// Get template type from flag
		template, _ := cmd.Flags().GetString("template")

		// Create configuration based on template
		var cfg *config.Config
		switch template {
		case "react":
			cfg = createReactTemplate(projectName)
		case "typescript":
			cfg = createTypescriptTemplate(projectName)
		default:
			cfg = createBasicTemplate(projectName)
		}

		// Save configuration
		if err := config.SaveConfig(cfg, "ai_rules.yaml"); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating configuration file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Initialized new AI rules project: %s\n", projectName)
		fmt.Println("  - Created ai_rules.yaml")
		fmt.Println("  - Run 'ai_rules generate' to create rule files")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	initCmd.Flags().StringP("template", "t", "basic", "Template to use (basic, react, typescript)")
}

func createBasicTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "AI assistant rules configuration",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "Code Quality",
				Priority: 10,
				Content:  "Write clean, readable, and maintainable code following best practices.",
			},
			{
				Name:     "Documentation",
				Priority: 5,
				Content:  "Document functions, classes, and complex logic with clear comments.",
			},
			{
				Name:     "Testing",
				Priority: 5,
				Content:  "Write unit tests for all new functionality.",
			},
		},
	}
}

func createReactTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "React project AI assistant rules",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "React Best Practices",
				Priority: 10,
				Content:  "Use functional components with hooks. Prefer composition over inheritance.",
			},
			{
				Name:     "Component Structure",
				Priority: 10,
				Content:  "Keep components small and focused. Extract custom hooks for reusable logic.",
			},
			{
				Name:     "State Management",
				Priority: 5,
				Content:  "Use useState for local state, useContext for shared state, consider Redux for complex apps.",
			},
			{
				Name:     "Performance",
				Priority: 5,
				Content:  "Use React.memo, useMemo, and useCallback to optimize performance when needed.",
			},
			{
				Name:     "Testing",
				Priority: 5,
				Content:  "Write unit tests with React Testing Library. Test behavior, not implementation.",
			},
		},
	}
}

func createTypescriptTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "TypeScript project AI assistant rules",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "Type Safety",
				Priority: 10,
				Content:  "Use strict TypeScript settings. Avoid 'any' type unless absolutely necessary.",
			},
			{
				Name:     "Interface Design",
				Priority: 10,
				Content:  "Define clear interfaces for data structures. Use union types for controlled variations.",
			},
			{
				Name:     "Generic Programming",
				Priority: 5,
				Content:  "Use generics to create reusable, type-safe functions and classes.",
			},
			{
				Name:     "Error Handling",
				Priority: 5,
				Content:  "Use Result/Option patterns or proper error types instead of throwing exceptions.",
			},
			{
				Name:     "Documentation",
				Priority: 3,
				Content:  "Use TSDoc comments for public APIs. Document complex type definitions.",
			},
		},
	}
}
