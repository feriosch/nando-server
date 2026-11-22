package tests

import (
	"context"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// leftByCore writes what Core kept before Weigh-ins moved to Calories (ADR
// 0012), as the old server stored it: Weigh-ins in Core's weigh_ins, and the
// Weigh-in reminder inside Core's settings, Sunday at 07:30.
func (n *nando) leftByCore() {
	n.t.Helper()
	ctx := context.Background()
	core := n.client.Database("core")
	if _, err := core.Collection("weigh_ins").InsertMany(ctx, []bson.D{
		{{Key: "_id", Value: "2026-10-12"}, {Key: "kg", Value: 72.8}},
		{{Key: "_id", Value: "2026-10-19"}, {Key: "kg", Value: 72.4}},
	}); err != nil {
		n.t.Fatal(err)
	}
	if _, err := core.Collection("settings").UpdateOne(ctx, bson.D{{Key: "_id", Value: "settings"}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "timeZone", Value: "UTC"},
		{Key: "heightCm", Value: 175},
		{Key: "weighInReminder", Value: bson.D{{Key: "enabled", Value: true}, {Key: "weekday", Value: 0}, {Key: "time", Value: "07:30"}}},
	}}}, options.UpdateOne().SetUpsert(true)); err != nil {
		n.t.Fatal(err)
	}
}

func TestOnStartCaloriesTakesTheWeighInsAndReminderCoreKept(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.leftByCore()

	phone.restart()

	if got, want := phone.weighIns(), []weighInJSON{{Week: "2026-10-19", Kg: 72.4}, {Week: "2026-10-12", Kg: 72.8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Weigh-ins = %+v, want Core's %+v", got, want)
	}
	if got, want := get[weightCardJSON](phone, "/api/calories/weight-card"),
		(weightCardJSON{Latest: &weighInJSON{Week: "2026-10-19", Kg: 72.4}, BMI: fptr(23.6), Change: fptr(-0.4)}); !reflect.DeepEqual(got, want) {
		t.Errorf("Weight card = %s, want %s", showCard(got), showCard(want))
	}
	if got, want := get[settingsJSON](phone, "/api/calories/settings").WeighInReminder,
		(weighInReminderJSON{Enabled: true, Weekday: "sunday", Time: "07:30"}); got != want {
		t.Errorf("Weigh-in reminder = %+v, want Core's %+v", got, want)
	}
}

func TestCaloriesTakesCoresWeighInsOnlyOnce(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.leftByCore()
	phone.restart()

	// After the move, the Owner deletes this week's and changes the reminder.
	callTool[map[string]any](t, phone.tools(), "calories_delete_weigh_in", map[string]any{"day": "2026-10-21"})
	phone.saveWeighInReminder(weighInReminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"})

	phone.restart()

	if got, want := phone.weighIns(), []weighInJSON{{Week: "2026-10-12", Kg: 72.8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Weigh-ins after another start = %+v, want this week's still deleted: %+v", got, want)
	}
	if got, want := get[settingsJSON](phone, "/api/calories/settings").WeighInReminder,
		(weighInReminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"}); got != want {
		t.Errorf("Weigh-in reminder after another start = %+v, want the Owner's change kept: %+v", got, want)
	}
}

func TestCaloriesSettingsSavedBeforeTheMoveKeepTheirNotificationsAndGainCoresReminder(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.saveCaloriesSettings(on("13:30"), on("22:00"))
	// As saved before the Weigh-in reminder moved here: no weighInReminder at all.
	if _, err := n.client.Database("calories").Collection("settings").UpdateOne(context.Background(), bson.D{},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "weighInReminder", Value: ""}}}}); err != nil {
		t.Fatal(err)
	}
	n.leftByCore()

	phone.restart()

	want := settingsJSON{
		NoEntriesReminder: on("13:30"),
		EveningSummary:    on("22:00"),
		WeighInReminder:   weighInReminderJSON{Enabled: true, Weekday: "sunday", Time: "07:30"},
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestWithNothingLeftByCoreCaloriesStartsEmpty(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	phone.restart()

	if got := phone.weighIns(); len(got) != 0 {
		t.Errorf("Weigh-ins = %+v, want none", got)
	}
	if got := get[settingsJSON](phone, "/api/calories/settings").WeighInReminder; got != defaultWeighInReminder {
		t.Errorf("Weigh-in reminder = %+v, want the default %+v", got, defaultWeighInReminder)
	}
}
