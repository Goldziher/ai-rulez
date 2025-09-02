package crud

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/cobra"
)

var (
	sectionContent  string
	sectionPriority int
)

// AddSectionCmd adds a new section to the configuration
var AddSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Add a new section to configuration",
	Long: `Add a new section to your AI rules configuration file.
The section title is provided as an argument, and the content can be provided
via stdin or will open an editor for you to enter the section content.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddSection,
}

// UpdateSectionCmd updates an existing section
var UpdateSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Update an existing section",
	Long: `Update an existing section in your AI rules configuration file.
You can update the content, priority, or both. If no flags are provided,
you'll be prompted to enter new content via stdin.`,
	Args: cobra.ExactArgs(1),
	Run:  runUpdateSection,
}

// DeleteSectionCmd deletes a section
var DeleteSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Delete a section from configuration",
	Long:  `Delete a section by title from your AI rules configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteSection,
}

func init() {
	AddSectionCmd.Flags().IntVar(&sectionPriority, "priority", 5, "Priority of the section (1-10)")

	UpdateSectionCmd.Flags().StringVar(&sectionContent, "content", "", "New content for the section")
	UpdateSectionCmd.Flags().IntVar(&sectionPriority, "priority", 0, "New priority for the section")
}

func runAddSection(cmd *cobra.Command, args []string) {
	sectionTitle := args[0]
	configPath, cfg := loadConfiguration()

	for _, section := range cfg.Sections {
		if section.Name == sectionTitle {
			fmt.Printf("❌ Section '%s' already exists\n", sectionTitle)
			os.Exit(1)
		}
	}

	if sectionContent == "" {
		fmt.Println("Enter section content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}
		sectionContent = content
	}

	newSection := config.Section{
		Name:     sectionTitle,
		Content:  sectionContent,
		Priority: sectionPriority,
	}
	cfg.Sections = append(cfg.Sections, newSection)

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Added section '%s' with priority %d to %s\n", sectionTitle, sectionPriority, configPath)
}

func runUpdateSection(cmd *cobra.Command, args []string) {
	sectionTitle := args[0]
	configPath, cfg := loadConfiguration()

	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Name == sectionTitle {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		fmt.Printf("❌ Section '%s' not found\n", sectionTitle)
		os.Exit(1)
	}

	if sectionContent != "" {
		cfg.Sections[sectionIndex].Content = sectionContent
	} else if !cmd.Flags().Changed("priority") {
		fmt.Println("Enter new section content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}
		cfg.Sections[sectionIndex].Content = content
	}

	if sectionPriority > 0 {
		cfg.Sections[sectionIndex].Priority = sectionPriority
	}

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Updated section '%s' in %s\n", sectionTitle, configPath)
}

func runDeleteSection(cmd *cobra.Command, args []string) {
	sectionTitle := args[0]
	configPath, cfg := loadConfiguration()

	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Name == sectionTitle {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		fmt.Printf("❌ Section '%s' not found\n", sectionTitle)
		os.Exit(1)
	}

	cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Deleted section '%s' from %s\n", sectionTitle, configPath)
}
