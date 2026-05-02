package ollama

import (
	"time"

	"github.com/Siddhant-K-code/distill/pkg/embedding"
)

func init() {
	embedding.RegisterFactory(embedding.ProviderOllama, func(cfg embedding.ProviderConfig) (embedding.Provider, error) {
		return NewClient(Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			Timeout: time.Duration(0), // uses defaultTimeout
		}), nil
	})
}
