package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
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
	RunE: runDomainAdd,
}

var domainRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove a domain",
	Long: `Remove a domain and all its contents.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runDomainRemove,
}

var domainListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all domains",
	Long:  `List all domains in your .ai-rulez/ configuration.`,
	Args:  cobra.NoArgs,
	RunE:  runDomainList,
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

func runDomainAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	req := &crud.AddDomainRequest{
		Name:        name,
		Description: domainDescription,
	}

	result, err := op.AddDomain(ctx, req)
	if err != nil {
		return failMsg("Failed to add domain", err)
	}

	logger.Info("Domain added successfully", "name", result.Name)
	if result.Description != "" {
		logger.Debug("Domain description", "description", result.Description)
	}
	// The created path is the result, so a script can capture it.
	outFor(cmd).Resultln(result.Path)
	return nil
}

func runDomainRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := confirmRemovalUnlessYes(domainForce, "domain", name, "Operation canceled"); err != nil {
		return fail(err)
	}

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	if err := op.RemoveDomain(ctx, name); err != nil {
		return failMsg("Failed to remove domain", err)
	}

	logger.Info("Domain removed successfully", "name", name)
	return nil
}

func runDomainList(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	if err := loadListedConfig(ctx); err != nil {
		return err
	}

	domains, err := op.ListDomains(ctx)
	if err != nil {
		return failMsg("Failed to list domains", err)
	}

	if domainJSON {
		output := make([]map[string]interface{}, len(domains))
		for i, domain := range domains {
			output[i] = map[string]interface{}{
				keyName:       domain.Name,
				keyPath:       domain.Path,
				"description": domain.Description,
			}
		}
		return writeListJSON(out.Stdout(), output)
	}
	if len(domains) == 0 {
		out.Info("No domains found\n")
		return nil
	}
	out.Result("Domains:\n")
	for _, domain := range domains {
		out.Result("  • %s\n", domain.Name)
		if domain.Description != "" {
			out.Result("    Description: %s\n", domain.Description)
		}
	}
	return nil
}
