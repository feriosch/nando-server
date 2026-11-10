// Package japanese is the Module for the Owner's Japanese vocabulary and kanji
// study.
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles requests
// (SearchService for the search screen, WordsService for the Word page, KanjiService for the Kanji page) and repository holds all database
// code. The services see only the repository interfaces, never a database.
package japanese

import (
	"net/http"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/japanese/repository"
	"github.com/feriosch/nando-server/internal/japanese/service"
)

// Module is the Japanese Module. Its only access to data is its own database.
type Module struct {
	search *service.SearchService
	words  *service.WordsService
	kanji  *service.KanjiService
}

// NewJapaneseModule returns the Japanese Module reading its own data through
// db.
func NewJapaneseModule(db *mongo.Database) *Module {
	words := repository.NewWords(db)
	kanji := repository.NewKanji(db)
	return &Module{
		search: service.NewSearch(words, kanji),
		words:  service.NewWords(words, repository.NewGroups(db), kanji),
		kanji:  service.NewKanji(kanji, repository.NewIrregularComponents(db), words),
	}
}

// Routes adds the Module's routes to mux, all under /api/japanese/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.search.Routes(mux)
	m.words.Routes(mux)
	m.kanji.Routes(mux)
}
