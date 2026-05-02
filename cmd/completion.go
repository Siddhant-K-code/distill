package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for the Distill CLI.

Bash:
  # One-time setup (requires bash-completion package):
  $ distill completion bash > /etc/bash_completion.d/distill

  # Per-session:
  $ source <(distill completion bash)

Zsh:
  # If shell completion is not already enabled, run:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  $ distill completion zsh > "${fpath[1]}/_distill"

Fish:
  $ distill completion fish > ~/.config/fish/completions/distill.fish

PowerShell:
  PS> distill completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> distill completion powershell > distill.ps1
  # and source this file from your PowerShell profile.
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell %q: choose bash, zsh, fish, or powershell", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
