package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "distill",
	Short: "Distill - Reliability layer for LLM context with semantic deduplication",
	Long: `Distill is a reliability layer for LLM context that removes redundancy
before it reaches your model, improving output quality and determinism.

Features:
  - Agglomerative clustering for semantic deduplication
  - MMR re-ranking for diversity
  - ~12ms latency, no LLM calls
  - Deterministic, auditable results

Environment Variables:
  OPENAI_API_KEY      For text → embedding conversion
  PINECONE_API_KEY    For Pinecone backend
  QDRANT_URL          For Qdrant backend`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.distill.yaml)")
	rootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")

	// Bind to viper
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".distill")
	}

	// Read environment variables
	viper.SetEnvPrefix("PINECONE")
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
