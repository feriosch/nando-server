package model

import "context"

// IrregularComponentsRepository reads the irregular_components collection:
// the Components that aren't a Kanji, each stored by its keyword.
type IrregularComponentsRepository interface {
	// GetManyByKeywords returns those of keywords that name an irregular Component.
	GetManyByKeywords(ctx context.Context, keywords []string) ([]string, error)
}
