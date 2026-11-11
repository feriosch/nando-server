// Package japanese is the Module for the Owner's Japanese study: their Words
// and Kanji.
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles requests
// (SearchService for the search screen, WordsService for the Word page, KanjiService for the Kanji page, AnkiService for the Anki deck files) and repository holds all database
// code. The services see only the repository and CoreService interfaces,
// never a database.
package japanese

import (
	"net/http"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/japanese/repository"
	"github.com/feriosch/nando-server/internal/japanese/service"
)

// Module is the Japanese Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	search *service.SearchService
	words  *service.WordsService
	kanji  *service.KanjiService
	anki   *service.AnkiService
}

// NewJapaneseModule returns the Japanese Module reading its own data through
// db and asking Core for Core's data through core.
func NewJapaneseModule(db *mongo.Database, core coremodel.CoreService) *Module {
	words := repository.NewWords(db)
	kanji := repository.NewKanji(db)
	return &Module{
		search: service.NewSearch(words, kanji),
		words:  service.NewWords(words, repository.NewGroups(db), kanji),
		kanji:  service.NewKanji(kanji, repository.NewIrregularComponents(db), words),
		anki:   service.NewAnki(words, core),
	}
}

// ToolProvider is what the Module offers Connectors: its Tools, for
// Core.AddToolProvider.
func (m *Module) ToolProvider() coremodel.ToolProvider {
	return toolProvider{m}
}

// toolProvider gathers the Tools of the Module's services.
type toolProvider struct{ m *Module }

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.words.Tools(), p.m.search.Tools(), p.m.kanji.Tools())
}

// Routes adds the Module's routes to mux, all under /api/japanese/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.search.Routes(mux)
	m.words.Routes(mux)
	m.kanji.Routes(mux)
	m.anki.Routes(mux)
}
