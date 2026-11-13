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
	"context"
	"net/http"
	"slices"
	"time"

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
	lookup *service.LookupService
}

// Config is where the Module finds the outside services it looks Words up
// on. An empty field means the real one; tests set fakes.
type Config struct {
	// JishoURL is jisho.org, whose search API is at /api/v1/search/words.
	JishoURL string
	// KanjiAPIURL is kanjiapi.dev, which grades a Kanji at /v1/kanji/<kanji>.
	KanjiAPIURL string
	// KanjiAPIPace is how far apart kanjiapi.dev is asked; 0 means a
	// second, which it allows.
	KanjiAPIPace time.Duration
}

// NewJapaneseModule returns the Japanese Module reading its own data through
// db and asking Core for Core's data through core.
func NewJapaneseModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, cfg Config) (*Module, error) {
	if cfg.JishoURL == "" {
		cfg.JishoURL = "https://jisho.org"
	}
	if cfg.KanjiAPIURL == "" {
		cfg.KanjiAPIURL = "https://kanjiapi.dev"
	}
	if cfg.KanjiAPIPace == 0 {
		cfg.KanjiAPIPace = time.Second
	}
	words, err := repository.NewWords(ctx, db)
	if err != nil {
		return nil, err
	}
	kanji, err := repository.NewKanji(ctx, db)
	if err != nil {
		return nil, err
	}
	return &Module{
		search: service.NewSearch(words, kanji),
		words:  service.NewWords(words, repository.NewGroups(db), kanji),
		kanji:  service.NewKanji(kanji, repository.NewIrregularComponents(db), words),
		anki:   service.NewAnki(words, core),
		lookup: service.NewLookup(kanji, cfg.JishoURL, cfg.KanjiAPIURL, cfg.KanjiAPIPace),
	}, nil
}

// ToolProvider is what the Module offers Connectors: its Tools, for
// Core.AddToolProvider.
func (m *Module) ToolProvider() coremodel.ToolProvider {
	return toolProvider{m}
}

// toolProvider gathers the Tools of the Module's services.
type toolProvider struct{ m *Module }

// ToolsDescription says what the Module's Tools let a Connector do, for the
// consent page.
func (p toolProvider) ToolsDescription() string {
	return "search your Words and Kanji, and add and change your Words"
}

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.words.Tools(), p.m.search.Tools(), p.m.kanji.Tools(), p.m.lookup.Tools())
}

// Routes adds the Module's routes to mux, all under /api/japanese/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.search.Routes(mux)
	m.words.Routes(mux)
	m.kanji.Routes(mux)
	m.anki.Routes(mux)
	m.lookup.Routes(mux)
}
