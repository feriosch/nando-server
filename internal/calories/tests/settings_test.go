package tests

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/feriosch/nando-server/internal/calories/model"
)

func TestCaloriesNotificationsAreOffUntilTheOwnerTurnsThemOn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	want := model.SettingsJSON{NoEntriesReminder: off("14:00"), EveningSummary: off("21:00")}
	if got := get[model.SettingsJSON](phone, "/api/calories/settings"); !reflect.DeepEqual(got, want) {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestOwnerSavesWhenCaloriesNotificationsAreSent(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	saved := model.SettingsJSON{NoEntriesReminder: on("13:30"), EveningSummary: on("22:00")}

	phone.must(http.MethodPut, "/api/calories/settings", saved, http.StatusNoContent)

	if got := get[model.SettingsJSON](phone, "/api/calories/settings"); !reflect.DeepEqual(got, saved) {
		t.Errorf("settings = %+v, want %+v", got, saved)
	}
}

func TestInvalidCaloriesNotificationSettingsAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	valid := map[string]any{"enabled": true, "time": "20:00"}

	for _, body := range []any{
		"settings",
		map[string]any{"noEntriesReminder": valid},
		map[string]any{"eveningSummary": valid},
		map[string]any{"noEntriesReminder": valid, "eveningSummary": map[string]any{"enabled": true, "time": "20:15"}},
		map[string]any{"noEntriesReminder": map[string]any{"enabled": true, "time": "24:00"}, "eveningSummary": valid},
		map[string]any{"noEntriesReminder": map[string]any{"enabled": true}, "eveningSummary": valid},
		map[string]any{"noEntriesReminder": valid, "eveningSummary": map[string]any{"time": "21:00"}},
	} {
		if rec := phone.do(http.MethodPut, "/api/calories/settings", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT settings %v: status = %d, want 400", body, rec.Code)
		}
	}
}
