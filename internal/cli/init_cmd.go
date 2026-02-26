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

	promptsDir := filepath.Join(base, "prompts")
	for _, d := range []string{base, promptsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	yamlContent := config.DefaultConfigYAML
	if global {
		yamlContent = config.DefaultGlobalConfigYAML
	}

	if err := writeIfNotExists(filepath.Join(base, "config.yaml"), yamlContent); err != nil {
		return err
	}

	if !global {
		if err := writeIfNotExists(filepath.Join(base, "rules.yaml"), config.DefaultRulesYAML); err != nil {
			return err
		}
		if err := writeIfNotExists(filepath.Join(base, ".gitignore"), "# Ignore API keys in config\n# config.yaml\n\n# Review reports\nreports/\n"); err != nil {
			return err
		}
	}

	fmt.Printf("Initialized %s\n", base)
	fmt.Printf("  config.yaml  — LLM, review, and output settings\n")
	if !global {
		fmt.Printf("  rules.yaml   — custom review rules\n")
	}
	fmt.Printf("  prompts/     — prompt template overrides\n")

	return nil
}

func writeIfNotExists(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("  exists: %s (skipped)\n", path)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
