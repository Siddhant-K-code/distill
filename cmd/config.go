package cmd

import (
	"fmt"
	"os"

	"github.com/Siddhant-K-code/distill/pkg/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Distill configuration",
	Long:  `Commands for creating and validating distill.yaml configuration files.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a distill.yaml template",
	Long: `Creates a distill.yaml configuration file with all available options
and their default values.

Example:
  distill config init
  distill config init --output /etc/distill/distill.yaml`,
	RunE: runConfigInit,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate a distill.yaml configuration file",
	Long: `Reads and validates a configuration file, reporting any errors.

Example:
  distill config validate
  distill config validate distill.yaml
  distill config validate --config /etc/distill/distill.yaml`,
	RunE: runConfigValidate,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configValidateCmd)

	configInitCmd.Flags().StringP("output", "o", "distill.yaml", "output file path")
	configInitCmd.Flags().Bool("stdout", false, "print to stdout instead of file")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	toStdout, _ := cmd.Flags().GetBool("stdout")
	output, _ := cmd.Flags().GetString("output")

	template := config.GenerateTemplate()

	if toStdout {
		fmt.Print(template)
		return nil
	}

	// Check if file already exists
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file %s already exists (use --stdout to print to stdout)", output)
	}

	if err := os.WriteFile(output, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created %s\n", output)
	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	var cfgPath string

	if len(args) > 0 {
		cfgPath = args[0]
	} else if cfgFile != "" {
		cfgPath = cfgFile
	} else {
		// Search default locations
		candidates := []string{
			"distill.yaml",
			".distill.yaml",
		}
		home, err := os.UserHomeDir()
		if err == nil {
			candidates = append(candidates,
				home+"/.distill.yaml",
				home+"/distill.yaml",
			)
		}

		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				cfgPath = c
				break
			}
		}

		if cfgPath == "" {
			return fmt.Errorf("no config file found (try: distill config validate <file>)")
		}
	}

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed for %s:\n%v\n", cfgPath, err)
		os.Exit(1)
	}

	_ = cfg
	fmt.Fprintf(os.Stderr, "Config file %s is valid\n", cfgPath)
	return nil
}
