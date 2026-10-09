package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
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

The path of the created directory is printed on stdout; --format json prints a
document with the name and path instead.

This creates a domain directory structure with rules, context, and skills subdirectories.
Domains allow you to organize rules, context, and skills by functional areas or teams.`,
	Args: cobra.ExactArgs(1),
	RunE: runDomainAdd,
}

var domainRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove a domain",
	Long: `Remove a domain and all its contents.

A domain that a profile still lists is not removed: remove the profile (or the
domain from it) first. Use --yes to skip confirmation prompts. The removed path is
printed on stdout; --format json prints a document with the name and path instead.`,
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
	addResultFormat(domainAddCmd.Flags())
	addResultFormat(domainRemoveCmd.Flags())

	// Add flags to domain remove command
	addYesFlag(domainRemoveCmd.Flags(), &domainForce, "Skip confirmation prompts")

	// Add flags to domain list command
	addJSONFormat(domainListCmd.Flags(), &domainJSON, "j")
}

func runDomainAdd(cmd *cobra.Command, args []string) error {
	out := outFor(cmd)
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	result, err := op.AddDomain(cmdContext(), &crud.AddDomainRequest{
		Name:        args[0],
		Description: domainDescription,
	})
	if err != nil {
		return failMsg("Failed to add domain", err)
	}

	out.Info("Domain added successfully\n")
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: "domain", Name: result.Name, Path: result.Path,
	}, true)
}

func runDomainRemove(cmd *cobra.Command, args []string) error {
	out := outFor(cmd)
	name := args[0]
	ctx := cmdContext()
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	// Refuse before asking: a missing domain or one a profile still lists is no
	// question for the user.
	if err := op.CheckDomainRemovable(ctx, name); err != nil {
		return failMsg("Failed to remove domain", err)
	}
	if err := confirmRemovalUnlessYes(domainForce, "domain", name, "Operation canceled"); err != nil {
		return fail(err)
	}
	path := op.DomainPath(name)
	if err := op.RemoveDomain(ctx, name); err != nil {
		return failMsg("Failed to remove domain", err)
	}

	out.Info("Domain removed successfully\n")
	return reportChange(out, out.JSON(), changeResult{
		Status: statusRemoved, Type: "domain", Name: name, Path: path,
	}, true)
}

func runDomainList(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(false)
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
