package model

import coreutils "github.com/feriosch/nando-server/internal/core/utils"

// ScheduledTaskRepository reads and writes the scheduled_task collection:
// when Nando last woke the scheduled task that works through Word Requests,
// and when a run was last active, under the rules Core's utils.ScheduledTask
// shares with every Module.
type ScheduledTaskRepository = coreutils.FireRecord
