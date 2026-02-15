// Package config provides configuration file support for Distill.
// It handles loading, validation, and environment variable interpolation
// for distill.yaml configuration files.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the full Distill configuration.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	Dedup     DedupConfig     `mapstructure:"dedup"`
	Retriever RetrieverConfig `mapstructure:"retriever"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Host         string        `mapstructure:"host"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// EmbeddingConfig holds embedding provider settings.
type EmbeddingConfig struct {
	Provider  string `mapstructure:"provider"`
	Model     string `mapstructure:"model"`
	BatchSize int    `mapstructure:"batch_size"`
}

// DedupConfig holds deduplication settings.
type DedupConfig struct {
	Threshold float64 `mapstructure:"threshold"`
	Method    string  `mapstructure:"method"`
	Linkage   string  `mapstructure:"linkage"`
	Lambda    float64 `mapstructure:"lambda"`
	EnableMMR bool    `mapstructure:"enable_mmr"`
}

// RetrieverConfig holds vector DB settings.
type RetrieverConfig struct {
	Backend   string `mapstructure:"backend"`
	Index     string `mapstructure:"index"`
	Host      string `mapstructure:"host"`
	Namespace string `mapstructure:"namespace"`
	TopK      int    `mapstructure:"top_k"`
	TargetK   int    `mapstructure:"target_k"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	APIKeys []string `mapstructure:"api_keys"`
}

// TelemetryConfig holds observability settings.
type TelemetryConfig struct {
	Tracing TracingConfig `mapstructure:"tracing"`
}

// TracingConfig holds OpenTelemetry tracing settings.
type TracingConfig struct {
	Enabled    bool    `mapstructure:"enabled"`
	Exporter   string  `mapstructure:"exporter"`
	Endpoint   string  `mapstructure:"endpoint"`
	SampleRate float64 `mapstructure:"sample_rate"`
	Insecure   bool    `mapstructure:"insecure"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8080,
			Host:         "0.0.0.0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
		},
		Embedding: EmbeddingConfig{
			Provider:  "openai",
			Model:     "text-embedding-3-small",
			BatchSize: 100,
		},
		Dedup: DedupConfig{
			Threshold: 0.15,
			Method:    "agglomerative",
			Linkage:   "average",
			Lambda:    0.5,
			EnableMMR: true,
		},
		Retriever: RetrieverConfig{
			Backend: "pinecone",
			TopK:    50,
			TargetK: 8,
		},
		Auth: AuthConfig{
			APIKeys: []string{},
		},
		Telemetry: TelemetryConfig{
			Tracing: TracingConfig{
				Enabled:    false,
				Exporter:   "otlp",
				Endpoint:   "localhost:4317",
				SampleRate: 1.0,
				Insecure:   true,
			},
		},
	}
}

// Load reads configuration from the given viper instance and returns
// a validated Config. Environment variables in string values are
// interpolated using ${VAR} syntax.
func Load(v *viper.Viper) (*Config, error) {
	cfg := DefaultConfig()

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Interpolate environment variables in string fields
	interpolateConfig(cfg)

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromFile reads a specific config file and returns a validated Config.
func LoadFromFile(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	return Load(v)
}

// Validate checks the configuration for errors and returns a descriptive
// error if any field is invalid.
func Validate(cfg *Config) error {
	var errs []string

	// Server validation
	if cfg.Server.Port < 0 || cfg.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port: must be between 0 and 65535, got %d", cfg.Server.Port))
	}
	if cfg.Server.ReadTimeout < 0 {
		errs = append(errs, "server.read_timeout: must be non-negative")
	}
	if cfg.Server.WriteTimeout < 0 {
		errs = append(errs, "server.write_timeout: must be non-negative")
	}

	// Embedding validation
	validProviders := map[string]bool{"openai": true, "": true}
	if !validProviders[cfg.Embedding.Provider] {
		errs = append(errs, fmt.Sprintf("embedding.provider: unsupported provider %q (supported: openai)", cfg.Embedding.Provider))
	}
	if cfg.Embedding.BatchSize < 0 {
		errs = append(errs, "embedding.batch_size: must be non-negative")
	}

	// Dedup validation
	if cfg.Dedup.Threshold < 0 || cfg.Dedup.Threshold > 1 {
		errs = append(errs, fmt.Sprintf("dedup.threshold: must be between 0 and 1, got %f", cfg.Dedup.Threshold))
	}
	validMethods := map[string]bool{"agglomerative": true, "": true}
	if !validMethods[cfg.Dedup.Method] {
		errs = append(errs, fmt.Sprintf("dedup.method: unsupported method %q (supported: agglomerative)", cfg.Dedup.Method))
	}
	validLinkages := map[string]bool{"single": true, "complete": true, "average": true, "": true}
	if !validLinkages[cfg.Dedup.Linkage] {
		errs = append(errs, fmt.Sprintf("dedup.linkage: unsupported linkage %q (supported: single, complete, average)", cfg.Dedup.Linkage))
	}
	if cfg.Dedup.Lambda < 0 || cfg.Dedup.Lambda > 1 {
		errs = append(errs, fmt.Sprintf("dedup.lambda: must be between 0 and 1, got %f", cfg.Dedup.Lambda))
	}

	// Retriever validation
	validBackends := map[string]bool{"pinecone": true, "qdrant": true, "": true}
	if !validBackends[cfg.Retriever.Backend] {
		errs = append(errs, fmt.Sprintf("retriever.backend: unsupported backend %q (supported: pinecone, qdrant)", cfg.Retriever.Backend))
	}
	if cfg.Retriever.TopK < 0 {
		errs = append(errs, "retriever.top_k: must be non-negative")
	}
	if cfg.Retriever.TargetK < 0 {
		errs = append(errs, "retriever.target_k: must be non-negative")
	}

	// Telemetry validation
	validExporters := map[string]bool{"otlp": true, "stdout": true, "none": true, "": true}
	if !validExporters[cfg.Telemetry.Tracing.Exporter] {
		errs = append(errs, fmt.Sprintf("telemetry.tracing.exporter: unsupported exporter %q (supported: otlp, stdout, none)", cfg.Telemetry.Tracing.Exporter))
	}
	if cfg.Telemetry.Tracing.SampleRate < 0 || cfg.Telemetry.Tracing.SampleRate > 1 {
		errs = append(errs, fmt.Sprintf("telemetry.tracing.sample_rate: must be between 0 and 1, got %f", cfg.Telemetry.Tracing.SampleRate))
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// envVarPattern matches ${VAR} or ${VAR:-default} syntax.
var envVarPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// InterpolateEnv replaces ${VAR} and ${VAR:-default} patterns in a string
// with the corresponding environment variable values.
func InterpolateEnv(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		varName := parts[1]
		defaultVal := ""
		if len(parts) >= 3 {
			defaultVal = parts[2]
		}

		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		if defaultVal != "" {
			return defaultVal
		}
		return match
	})
}

// interpolateConfig applies environment variable interpolation to all
// string fields in the config.
func interpolateConfig(cfg *Config) {
	cfg.Server.Host = InterpolateEnv(cfg.Server.Host)
	cfg.Embedding.Provider = InterpolateEnv(cfg.Embedding.Provider)
	cfg.Embedding.Model = InterpolateEnv(cfg.Embedding.Model)
	cfg.Dedup.Method = InterpolateEnv(cfg.Dedup.Method)
	cfg.Dedup.Linkage = InterpolateEnv(cfg.Dedup.Linkage)
	cfg.Retriever.Backend = InterpolateEnv(cfg.Retriever.Backend)
	cfg.Retriever.Index = InterpolateEnv(cfg.Retriever.Index)
	cfg.Retriever.Host = InterpolateEnv(cfg.Retriever.Host)
	cfg.Retriever.Namespace = InterpolateEnv(cfg.Retriever.Namespace)

	for i, key := range cfg.Auth.APIKeys {
		cfg.Auth.APIKeys[i] = InterpolateEnv(key)
	}

	cfg.Telemetry.Tracing.Exporter = InterpolateEnv(cfg.Telemetry.Tracing.Exporter)
	cfg.Telemetry.Tracing.Endpoint = InterpolateEnv(cfg.Telemetry.Tracing.Endpoint)
}

// GenerateTemplate returns a YAML template string with all available
// configuration options and their defaults, suitable for writing to
// a distill.yaml file.
func GenerateTemplate() string {
	return `# Distill Configuration
# See: https://github.com/Siddhant-K-code/distill

server:
  port: 8080
  host: 0.0.0.0
  read_timeout: 30s
  write_timeout: 60s

embedding:
  provider: openai
  model: text-embedding-3-small
  batch_size: 100

dedup:
  threshold: 0.15
  method: agglomerative
  linkage: average
  lambda: 0.5
  enable_mmr: true

retriever:
  backend: pinecone    # pinecone or qdrant
  index: ""
  host: ""             # required for qdrant
  namespace: ""
  top_k: 50
  target_k: 8

auth:
  api_keys:
    # - ${DISTILL_API_KEY}

telemetry:
  tracing:
    enabled: false
    exporter: otlp       # otlp, stdout, or none
    endpoint: localhost:4317
    sample_rate: 1.0     # 0.0 to 1.0
    insecure: true
`
}
