# The Health Sync Shortcut

How the Owner builds the iOS Shortcut that sends each Day's steps and active kcal to Nando, as a Health Sync ([ADR 0015](adr/0015-steps-and-burned-calories-in-calories.md)). It is built by hand on the iPhone, once.

## 1. Create a Sync Key

In Nando's Settings, under Sync Keys, create one named "iPhone Health" and approve it with a Passkey. Copy the key: it is shown only once.

## 2. Build the Shortcut "Nando Health Sync"

In the Shortcuts app, a new Shortcut with these actions, repeated for **today** and **yesterday**:

1. **Find Health Samples** where Type is **Steps** and Start Date is **today** (for yesterday, **yesterday**).
2. **Calculate Statistics**: **Sum** of the samples → the Day's steps.
3. **Find Health Samples** where Type is **Active Energy**, same Day, and **Calculate Statistics**: **Sum** → the Day's active kcal.
4. **Format Date**: the Day as `yyyy-MM-dd`.

Then one **Get Contents of URL**:

- URL: `https://api.<domain>/api/calories/health-sync`
- Method: **POST**
- Headers: `Authorization: Bearer <Sync Key>`
- Request Body: **JSON**:

```json
{
  "days": [
    { "day": "2026-11-24", "steps": 9812, "activeKcal": 312 },
    { "day": "2026-11-23", "steps": 11402, "activeKcal": 380 }
  ]
}
```

Round steps and kcal to whole numbers. A Day with no samples sums to `0`, and is sent as that.

## 3. Run it on a schedule

In Shortcuts → Automation, add three **Time of Day** personal automations (12:00, 18:00, 23:30), each running "Nando Health Sync", with **Run Immediately** on and **Notify When Run** off.

iOS hides Health data while the iPhone is locked, so a run on a locked phone may find nothing. That's why it runs three times and resends yesterday.

## When it stops

If Nando gets no Health Sync for 48 hours, the Calories Assistant says so in Telegram. Check that the automations are still on and the Sync Key wasn't removed.
