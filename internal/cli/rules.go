package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/henrocdotnet/grumbler/internal/rules"
)

func init() {
	rulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "Manage review rules",
	}

	rulesCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List loaded rules",
			RunE:  runRulesList,
		},
		&cobra.Command{
			Use:   "import [file]",
			Short: "Import rules from a YAML file",
			Args:  cobra.ExactArgs(1),
			RunE:  runRulesImport,
		},
	)

	rootCmd.AddCommand(rulesCmd)
}

func runRulesList(_ *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	loaded, err := rules.LoadRules(dir)
	if err != nil {
		return err
	}

	if len(loaded) == 0 {
		fmt.Println("No rules loaded. Run 'init' to create a rules file.")
		return nil
	}

	fmt.Printf("Loaded %d rule(s):\n\n", len(loaded))
	for _, r := range loaded {
		fmt.Printf("  [%s] %s (%s)\n", r.Severity, r.Name, r.ID)
		if r.FileGlob != "" {
			fmt.Printf("       glob: %s\n", r.FileGlob)
		}
		fmt.Printf("       %s\n\n", r.Guidance)
	}

	return nil
}

func runRulesImport(_ *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	imported, err := rules.LoadRulesFromFile(args[0])
	if err != nil {
		return err
	}

	if len(imported) == 0 {
		fmt.Println("No rules found in file.")
		return nil
	}

	// Load existing rules and merge
	existing, _ := rules.LoadRules(dir)

	// Build ID set for dedup
	seen := make(map[string]bool, len(existing))
	for _, r := range existing {
		seen[r.ID] = true
	}

	var added int
	for _, r := range imported {
		if !seen[r.ID] {
			existing = append(existing, r)
			seen[r.ID] = true
			added++
		}
	}

	if err := rules.SaveRules(dir, existing); err != nil {
		return err
	}

	fmt.Printf("Imported %d new rule(s) (%d skipped as duplicates)\n", added, len(imported)-added)
	return nil
}
