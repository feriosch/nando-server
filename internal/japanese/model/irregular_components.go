package model

import "context"

// IrregularComponentsRepository reads the irregular_components collection:
// the Components that aren't a Kanji, each stored by its keyword.
type IrregularComponentsRepository interface {
	// Stored returns those of keywords that name an irregular Component.
	Stored(ctx context.Context, keywords []string) ([]string, error)
}
