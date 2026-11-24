# The Health Sync Shortcut

How the Owner builds the iOS Shortcut that sends today's steps and active kcal to Nando, as a Health Sync ([ADR 0015](adr/0015-steps-and-burned-calories-in-calories.md)). It is built by hand on the iPhone, once.

## 1. Create a Sync Key

In Nando's Settings, under **Sync Keys**, keep the name "iPhone Health" and tap **Create with Face ID**. Tap **Copy**: the key is shown only once. If it is lost or leaks, remove it there and create another.

## 2. Build the Shortcut "Nando Health Sync"

In the Shortcuts app, a new Shortcut with these actions, in order:

```
Current Date
Find Health Samples   Type is Steps, Start Date is today
If   Health Samples has any value
   Calculate Statistics   Sum of Health Samples
   Round                  Sum to Integer
   Set Variable           Steps = Rounded Number
   Find Health Samples    Type is Active Calories, Start Date is today
   Calculate Statistics   Sum of Health Samples
   Round                  Sum to Integer
   Set Variable           KCal = Rounded Number
   Format Date            Current Date, Custom: yyyy-MM-dd
   Set Variable           FormattedToday = Formatted Date
   Text                   {"days":[{"day":"[FormattedToday]","steps":[Steps],"activeKcal":[KCal]}]}
   Set Variable           TodayBody = Text
   Get Contents of URL    https://api.nanders.app/api/calories/health-sync
End If
```

- **Get Contents of URL**, under its **›**: Method **POST**; Headers `Authorization` = `Bearer <Sync Key>` (one space after Bearer) and `Content-Type` = `application/json`; Request Body **File**, set to the **TodayBody** variable. Left on **JSON** with no fields, it sends `{}`, which Nando refuses with "days must have at least one Day".
- In the **Text**, only the date is in quotes: `steps` and `activeKcal` are whole numbers, which is what the **Round** actions are for.
- Shortcuts can't rename an action's result, so each value is kept with **Set Variable**, and picked by that name. A new action fills itself with the result of the action right above it, which is the easiest way to wire one up.
- The **If** matters: iOS hides Health data while the iPhone is locked, so a run then finds no samples, and without the If it would send 0 steps over the Day's real count.

Run it once from the editor. It answers nothing when it works. Allow it to read **Steps** and **Active Energy**, and to connect to `api.nanders.app` (**Always Allow**). Today's Day view in Nando then shows **iPhone, Synced HH:MM** under Activity.

If Nando's steps are higher than the Health app's, another app is writing steps too: add the filter **Source is iPhone** to the Steps search.

## 3. Run it on a schedule

In Shortcuts → **Automation** → **+** → **Time of Day**, five daily automations, at **12:00, 18:00, 22:00, 23:00 and 23:55**, each running "Nando Health Sync", with **Run Immediately** on and **Notify When Run** off. The late ones are close together so the evening's steps still land if the iPhone is locked at one of them.

Steps walked after the 23:55 run aren't counted, and a run on a locked iPhone sends nothing, so the Day keeps the numbers of its last run that did. When a Day ends short, tell the Calories Assistant its steps and active kcal from the Health app ("yesterday: 12,400 steps, 410 kcal"), and it corrects the Day.

A Day with no steps at all, such as one the iPhone stays home, sends nothing either, as the If skips it. Two such Days in a row bring the "Steps haven't synced" Notification below, though nothing is broken.

## When something is wrong

To see a value mid-run, add **Show Result** (or **Quick Look**) with it, then **Stop This Shortcut** so no request is sent; remove both afterwards. What Nando answers:

- `days must have at least one Day`: the body isn't the Text; see Request Body above.
- `steps and activeKcal must be whole numbers: round them in the Shortcut`: a number wasn't rounded.
- `body must be {"days": …}`: the Text isn't valid JSON, such as a number in quotes or a quote missing around the date.
- `sign in first` or `that Sync Key is unknown or was removed`: the Authorization header is wrong, or the key was removed in Settings.

If Nando gets no Health Sync for 48 hours, the Calories Assistant says so in Telegram: check that the automations are still on and the Sync Key is still listed in Settings.
