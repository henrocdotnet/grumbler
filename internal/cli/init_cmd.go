package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/henrocdotnet/grumbler/internal/config"
)

func init() {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold configuration directory",
		Long:  "Creates config.yaml, rules.yaml, and prompts/ in the project or global config directory.",
		RunE:  runInit,
	}

	initCmd.Flags().Bool("global", false, "Scaffold global config (~/.config/grumbler/)")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	global, _ := cmd.Flags().GetBool("global")

	if global {
		return initDir(config.GlobalDir(), true)
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	return initDir(filepath.Join(dir, config.Dir), false)
}

func initDir(base string, global bool) error {
	if base == "" {
		return fmt.Errorf("cannot determine config directory")
	}

	var err error
	if global {
		err = config.InitGlobal(base)
	} else {
		err = config.InitProject(base)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Initialized %s\n", base)
	fmt.Printf("  config.yaml  — LLM, review, and output settings\n")
	if !global {
		fmt.Printf("  rules.yaml   — custom review rules\n")
	}
	fmt.Printf("  prompts/     — prompt template overrides\n")

	return nil
}
