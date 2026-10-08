package commands

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var (
	domainDescription string
	domainForce       bool
	domainJSON        bool
)

var DomainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage domains",
	Long:  `Manage domains in your .ai-rulez/ configuration.`,
}

var domainAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new domain",
	Long: `Add a new domain with optional description.

This creates a domain directory structure with rules, context, and skills subdirectories.
Domains allow you to organize rules, context, and skills by functional areas or teams.`,
	Args: cobra.ExactArgs(1),
	Run:  runDomainAdd,
}

var domainRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove a domain",
	Long: `Remove a domain and all its contents.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runDomainRemove,
}

var domainListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all domains",
	Long:  `List all domains in your .ai-rulez/ configuration.`,
	Args:  cobra.NoArgs,
	Run:   runDomainList,
}

func init() {
	DomainCmd.AddCommand(domainAddCmd)
	DomainCmd.AddCommand(domainRemoveCmd)
	DomainCmd.AddCommand(domainListCmd)

	// Add flags to domain add command
	domainAddCmd.Flags().StringVarP(&domainDescription, "description", "s", "", "Domain description")

	// Add flags to domain remove command
	addYesFlag(domainRemoveCmd.Flags(), &domainForce, "Skip confirmation prompts")

	// Add flags to domain list command
	addJSONFormat(domainListCmd.Flags(), &domainJSON, "j")
}

func runDomainAdd(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	req := &crud.AddDomainRequest{
		Name:        name,
		Description: domainDescription,
	}

	result, err := op.AddDomain(ctx, req)
	if err != nil {
		fatal("Failed to add domain", err)
	}

	logger.Info("Domain added successfully",
		"name", result.Name,
		"path", result.Path,
	)
	if result.Description != "" {
		logger.Debug("Domain description", "description", result.Description)
	}
}

func runDomainRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	confirmRemovalUnlessYes(domainForce, "domain", name, "Operation canceled")

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	if err := op.RemoveDomain(ctx, name); err != nil {
		fatal("Failed to remove domain", err)
	}

	logger.Info("Domain removed successfully", "name", name)
}

func runDomainList(cmd *cobra.Command, args []string) {
	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}
	loadListedConfig(ctx)

	domains, err := op.ListDomains(ctx)
	if err != nil {
		fatal("Failed to list domains", err)
	}

	if len(domains) == 0 && !domainJSON {
		logger.Info("No domains found")
		return
	}

	if domainJSON {
		// Output as JSON
		output := make([]map[string]interface{}, len(domains))
		for i, domain := range domains {
			output[i] = map[string]interface{}{
				keyName:       domain.Name,
				keyPath:       domain.Path,
				"description": domain.Description,
			}
		}
		data, err := jsondoc.Marshal(output)
		if err != nil {
			fatal("Failed to marshal JSON", err)
		}
		fmt.Print(string(data))
	} else {
		// Output as human-readable table
		logger.Info("Domains:")
		for _, domain := range domains {
			logger.Info(fmt.Sprintf("  • %s", domain.Name))
			if domain.Description != "" {
				logger.Debug(fmt.Sprintf("    Description: %s", domain.Description))
			}
		}
	}
}
