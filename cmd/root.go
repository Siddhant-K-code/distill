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
	Use:   "govs",
	Short: "GoVectorSync - High-performance vector ingestion with semantic deduplication",
	Long: `GoVectorSync (govs) is a production-grade CLI tool for ingesting massive 
datasets of vector embeddings into Pinecone with client-side semantic deduplication.

Features:
  - SIMD-accelerated cosine distance calculations (AVX2/AVX-512)
  - Custom K-Means clustering for semantic deduplication
  - Worker pool pattern for maximum throughput
  - Exponential backoff retry logic

Environment Variables:
  PINECONE_API_KEY    Your Pinecone API key (required for sync)
  PINECONE_INDEX      Default index name
  PINECONE_NAMESPACE  Default namespace`,
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.govs.yaml)")
	rootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")

	// Bind to viper
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
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
		viper.SetConfigName(".govs")
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
