package cmd

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Siddhant-K-code/distill/pkg/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage stateful context windows",
	Long: `Create and manage token-budgeted context windows for AI agent sessions.

Entries are deduplicated on push, compressed as they age, and evicted
when the token budget is exceeded.

Examples:
  distill session create --max-tokens 128000
  distill session push --session-id abc --role user --content "Fix the bug"
  distill session context --session-id abc
  distill session delete --session-id abc`,
}

var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new session",
	RunE:  runSessionCreate,
}

var sessionPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push entries to a session",
	RunE:  runSessionPush,
}

var sessionContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Read the current context window",
	RunE:  runSessionContext,
}

var sessionDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a session and all its entries",
	RunE:  runSessionDelete,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionCreateCmd, sessionPushCmd, sessionContextCmd, sessionDeleteCmd)

	// Shared flags
	sessionCmd.PersistentFlags().String("db", "", "SQLite database path (default: distill-sessions.db)")

	// Create flags
	sessionCreateCmd.Flags().String("session-id", "", "Session ID (auto-generated if empty)")
	sessionCreateCmd.Flags().Int("max-tokens", 128000, "Token budget for the session")
	sessionCreateCmd.Flags().Float64("dedup-threshold", 0.15, "Cosine distance threshold for dedup")
	sessionCreateCmd.Flags().Int("preserve-recent", 10, "Always keep last N entries uncompressed")

	// Push flags
	sessionPushCmd.Flags().String("session-id", "", "Session ID")
	sessionPushCmd.Flags().String("role", "user", "Entry role (user, assistant, tool, system)")
	sessionPushCmd.Flags().String("content", "", "Entry content")
	sessionPushCmd.Flags().String("source", "", "Entry source (e.g. file_read, search)")
	sessionPushCmd.Flags().Float64("importance", 0.5, "Entry importance (0-1)")
	_ = sessionPushCmd.MarkFlagRequired("session-id")
	_ = sessionPushCmd.MarkFlagRequired("content")

	// Context flags
	sessionContextCmd.Flags().String("session-id", "", "Session ID")
	sessionContextCmd.Flags().Int("max-tokens", 0, "Max tokens to return (0 = all)")
	sessionContextCmd.Flags().String("role", "", "Filter by role")
	_ = sessionContextCmd.MarkFlagRequired("session-id")

	// Delete flags
	sessionDeleteCmd.Flags().String("session-id", "", "Session ID")
	_ = sessionDeleteCmd.MarkFlagRequired("session-id")
}

func runSessionCreate(cmd *cobra.Command, _ []string) error {
	store, err := sessionStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sessionID, _ := cmd.Flags().GetString("session-id")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	threshold, _ := cmd.Flags().GetFloat64("dedup-threshold")
	preserveRecent, _ := cmd.Flags().GetInt("preserve-recent")

	sess, err := store.Create(context.Background(), session.CreateRequest{
		SessionID:      sessionID,
		MaxTokens:      maxTokens,
		DedupThreshold: threshold,
		PreserveRecent: preserveRecent,
	})
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(sess)
}

func runSessionPush(cmd *cobra.Command, _ []string) error {
	store, err := sessionStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sessionID, _ := cmd.Flags().GetString("session-id")
	role, _ := cmd.Flags().GetString("role")
	content, _ := cmd.Flags().GetString("content")
	source, _ := cmd.Flags().GetString("source")
	importance, _ := cmd.Flags().GetFloat64("importance")

	result, err := store.Push(context.Background(), session.PushRequest{
		SessionID: sessionID,
		Entries: []session.PushEntry{
			{
				Role:       role,
				Content:    content,
				Source:     source,
				Importance: importance,
			},
		},
	})
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(result)
}

func runSessionContext(cmd *cobra.Command, _ []string) error {
	store, err := sessionStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sessionID, _ := cmd.Flags().GetString("session-id")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	role, _ := cmd.Flags().GetString("role")

	result, err := store.Context(context.Background(), session.ContextRequest{
		SessionID: sessionID,
		MaxTokens: maxTokens,
		Role:      role,
	})
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(result)
}

func runSessionDelete(cmd *cobra.Command, _ []string) error {
	store, err := sessionStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sessionID, _ := cmd.Flags().GetString("session-id")

	result, err := store.Delete(context.Background(), sessionID)
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(result)
}

// sessionStoreFromFlags creates a session store from CLI flags.
func sessionStoreFromFlags(cmd *cobra.Command) (*session.SQLiteStore, error) {
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = viper.GetString("session.db_path")
	}
	return newSessionStore(dbPath)
}

// newSessionStore creates a session store with the given DB path,
// applying viper config overrides. Used by CLI, API, and MCP.
func newSessionStore(dbPath string) (*session.SQLiteStore, error) {
	if dbPath == "" {
		dbPath = "distill-sessions.db"
	}
	cfg := session.DefaultConfig()

	threshold := viper.GetFloat64("session.dedup_threshold")
	if threshold > 0 {
		cfg.DefaultDedupThreshold = threshold
	}

	maxTokens := viper.GetInt("session.max_tokens")
	if maxTokens > 0 {
		cfg.DefaultMaxTokens = maxTokens
	}

	return session.NewSQLiteStore(dbPath, cfg)
}
