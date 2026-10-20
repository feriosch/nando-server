# Body measurements live in Core, not in a Module

The Owner's height and weekly Weigh-ins are kept in Core rather than in their own "Weight" Module or inside Calories. They describe the Owner, not one activity, and more than one Module needs them: Calories now (BMI, and possibly calculating the Daily Limit later), and the planned Gym Routine Tracker. Because Modules may depend only on Core and never on each other, data that several Modules read has to sit in Core.

## Considered Options

- **A separate Weight Module**: rejected. Weight isn't something the Owner manages as a feature of its own, and Calories and Gym would then have to read another Module's data, which the Module rule forbids.
- **Inside Calories**: rejected for the same reason, once the Gym Module arrives.

## Consequences

- Core provides its own Body Home Card on Home and its own Settings section (height, the Weigh-in reminder). Neither appears in the hamburger menu's list of Modules.
- The Weigh-in reminder is a Core Notification, not a Module one.
- Core grows. Only facts about the Owner belong there; anything specific to one activity stays in its Module.
