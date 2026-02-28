package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "grumbler",
	Short:         "AI code review for local git diffs",
	Long:          "Multi-pass LLM review (review team, audit) applied to local git diffs.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().String("provider", "", "LLM provider override (anthropic|openai|google|claude-cli|gemini-cli)")
	rootCmd.PersistentFlags().String("model", "", "LLM model override")
	rootCmd.PersistentFlags().StringP("format", "f", "", "Output format (terminal|json|sarif)")
	rootCmd.PersistentFlags().String("config", "", "Path to config directory (default: .)")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
