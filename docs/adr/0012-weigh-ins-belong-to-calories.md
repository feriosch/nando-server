# Weigh-ins belong to Calories

This supersedes ADR 0003. Weigh-ins, the BMI derived from them, the Weight Home Card and the weekly Weigh-in reminder move from Core to the Calories Module. The Owner's height stays in Core's settings.

ADR 0003 kept Weigh-ins in Core because more than one Module would read them: Calories, and the planned Gym Routine Tracker. That turned out wrong twice. The Gym Routine Tracker won't read weight, so Calories is the only Module that does. And with the Assistants (ADR 0008), Core's ownership split one habit across two chats: the Owner told the Calories Assistant what they ate and the Core Assistant what they weighed, and the Weigh-in reminder arrived in the Core chat. Weight is part of how the Owner tracks eating, so it lives where eating is tracked.

## Decisions

- **Calories owns Weigh-ins**: its own `weigh_ins` collection, the same rules as before (one per week, Monday to Sunday in the Owner Time Zone, kg to 1 decimal, only this week's and last week's can change), and BMI, derived from the latest Weigh-in and the Owner's height.
- **Only the Calories Assistant writes them**, like the rest of Calories (#141): `calories_log_weigh_in`, `calories_delete_weigh_in`, `calories_list_weigh_ins`. A Tool takes any Day of the week, today's when left out. The web app only reads: `GET /api/calories/weigh-ins` and the Weight Home Card's `GET /api/calories/weight-card`.
- **The Weigh-in reminder is a Calories Notification**, weekly at a weekday and time in the Calories settings, sent in the Calories Assistant's chat, where a reply with a weight is logged.
- **The height stays in Core.** It is a fixed fact about the Owner, set once in Core's Settings Section. Calories reads it through `CoreService.HeightCm`, as it reads Today.
- **Core keeps what it held, read-only, for one release.** At startup Calories takes the Weigh-ins and the reminder Core kept (`CoreService.WeighInsLeft`), never overwriting its own, and records in its `migrations` collection that it did, so a Weigh-in deleted later never comes back. Core's `weigh_ins` collection and the reminder in its settings are dropped in a later release, once the move is known to be good. Until then, rolling back loses nothing logged before the move.

## Considered Options

- **Keep Weigh-ins in Core and only show them under Calories in the app**: rejected. It moves the card but not the chat: logging weight would still be the Core Assistant's, which is what felt wrong.
- **Give the Core Assistant Weigh-in Tools**: rejected for the same reason: two Assistants for one habit.
- **Move the height too**: rejected. It doesn't change, and it is a fact about the Owner rather than about eating.
- **A one-off migration script**: rejected for a migration at startup, which leaves no window where the new server runs without the Owner's history, and which is safe to run again.

## Consequences

- If a future Module needs the Owner's weight, it can't read it from Calories: Modules never depend on each other. This decision would then need revisiting.
- Core's Settings Section keeps the time zone, height and Gmail Digest. `PUT /api/settings` ignores a `weighInReminder` sent by a web app from before the move.
- In the week of the move, the Owner may get the Weigh-in reminder twice, once from each chat.
