package tests

import (
	"net/http"
	"testing"
)

// settingsJSON is the Career settings, as the API reads and writes them.
type settingsJSON struct {
	Looking bool `json:"looking"`
}

func TestTheOwnerIsNotLookingUntilTheySayTheyAre(t *testing.T) {
	_, phone := signedInBrowser(t)

	if got := get[settingsJSON](phone, "/api/career/settings"); got.Looking {
		t.Errorf("looking = true, want false")
	}
}

func TestTheOwnerSaysTheyAreLooking(t *testing.T) {
	_, phone := signedInBrowser(t)

	phone.must(http.MethodPut, "/api/career/settings", settingsJSON{Looking: true}, http.StatusNoContent)

	if got := get[settingsJSON](phone, "/api/career/settings"); !got.Looking {
		t.Errorf("looking = false, want true")
	}
}

func TestCareerSettingsWithoutLookingAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t)

	for _, body := range []any{
		"settings",
		map[string]any{},
		map[string]any{"looking": "yes"},
	} {
		if rec := phone.do(http.MethodPut, "/api/career/settings", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT settings %v: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestAConnectorReadsWhetherTheOwnerIsLooking(t *testing.T) {
	_, phone := signedInBrowser(t)
	claude := phone.connect()
	phone.must(http.MethodPut, "/api/career/settings", settingsJSON{Looking: true}, http.StatusNoContent)

	if got := callTool[settingsJSON](t, claude, "career_get_settings", nil); !got.Looking {
		t.Errorf("career_get_settings: looking = false, want true")
	}
}
