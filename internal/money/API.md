# Money API

The Money Module keeps the Owner's money across Japan and Mexico, from the Statements the Owner sends the Money Assistant ([ADR 0014](../../docs/adr/0014-money-reads-statements-the-owner-sends.md)). The Money Assistant reads and writes everything through the Module's [Tools](#tools), over the Owner's Connector. The web app only reads Money, through routes under `/api/money/`, which come with the Home Card (#213).

The repo is public, so nothing here names a real Account, Merchant or amount of the Owner's. Those are the Owner's data, held only in the `money` database.

## Settings

One document:

```json
{
  "askAbove": {"JPY": 3000, "MXN": 300}
}
```

- `askAbove`: per currency, the whole amount above which a Transaction Claude can't tell is asked about. Below it, Claude's best guess is kept, Unconfirmed (ADR 0014). ¥3,000 and MX$300 until changed. Each must be above 0, and only the Owner's currencies, JPY and MXN, are taken.

## Tools

- **`money_get_settings`**: the settings, as above. Read-only.
- **`money_save_settings`** (`askAbove`): changes only the currencies given, such as `{"askAbove": {"JPY": 5000}}`, and answers the settings. A currency other than JPY or MXN, an amount of 0 or less, or no `askAbove` is refused, and nothing changes.

## Database

The `money` database holds `settings`, one document.
