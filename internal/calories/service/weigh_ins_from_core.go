package service

import (
	"context"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// weighInsFromCore names the migration that takes what Core kept of
// Weigh-ins until they moved to Calories (ADR 0012).
const weighInsFromCore = "weigh-ins-from-core"

// TakeWeighInsFromCore copies the Weigh-ins and the Weigh-in reminder Core
// kept until ADR 0012 into the Module, once: a week that already has a
// Weigh-in, and settings that already have a reminder, keep theirs. It is
// marked done only once Core had something to take, so a Weigh-in the Owner
// deletes later never comes back. Running it again before then is safe.
func TakeWeighInsFromCore(ctx context.Context, core coremodel.CoreService, weighIns model.WeighInsRepository,
	settings *SettingsService, migrations model.MigrationsRepository) error {
	done, err := migrations.Done(ctx, weighInsFromCore)
	if err != nil || done {
		return err
	}
	left, err := core.WeighInsLeft(ctx)
	if err != nil {
		return err
	}
	if len(left.WeighIns) == 0 && left.Reminder == nil {
		return nil
	}
	for _, weighIn := range left.WeighIns {
		if err := weighIns.AddMissing(ctx, model.WeighIn{Week: weighIn.Week, Kg: weighIn.Kg}); err != nil {
			return err
		}
	}
	if left.Reminder != nil {
		if err := settings.takeWeighInReminder(ctx, *left.Reminder); err != nil {
			return err
		}
	}
	return migrations.MarkDone(ctx, weighInsFromCore)
}
