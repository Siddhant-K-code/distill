package openai

import (
	"github.com/Siddhant-K-code/distill/pkg/embedding"
)

func init() {
	embedding.RegisterFactory(embedding.ProviderOpenAI, func(cfg embedding.ProviderConfig) (embedding.Provider, error) {
		return NewClient(Config{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
		})
	})
}
