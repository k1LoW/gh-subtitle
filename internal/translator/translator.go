package translator

import "context"

// TranslationInput represents a single item to translate.
type TranslationInput struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// TranslationOutput represents a single translated item.
type TranslationOutput struct {
	Key  string `json:"key"`
	Text string `json:"text"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Translator translates batches of text to a target language.
type Translator interface {
	TranslateBatch(ctx context.Context, lang string, items []TranslationInput) ([]TranslationOutput, error)
	Close() error
}
