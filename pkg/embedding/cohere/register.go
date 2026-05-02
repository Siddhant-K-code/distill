package cohere

import (
	"github.com/Siddhant-K-code/distill/pkg/embedding"
)

func init() {
	embedding.RegisterFactory(embedding.ProviderCohere, func(cfg embedding.ProviderConfig) (embedding.Provider, error) {
		return NewClient(Config{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
		})
	})
}
