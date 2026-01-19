package main

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for arrgo.

To load completions:

Bash:
  $ source <(arrgo completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ arrgo completion bash > /etc/bash_completion.d/arrgo
  # macOS:
  $ arrgo completion bash > $(brew --prefix)/etc/bash_completion.d/arrgo

Zsh:
  $ source <(arrgo completion zsh)
  # To load completions for each session, execute once:
  $ arrgo completion zsh > "${fpath[1]}/_arrgo"

Fish:
  $ arrgo completion fish | source
  # To load completions for each session, execute once:
  $ arrgo completion fish > ~/.config/fish/completions/arrgo.fish

PowerShell:
  PS> arrgo completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> arrgo completion powershell > arrgo.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
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
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
