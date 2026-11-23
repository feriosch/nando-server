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

## Accounts

An Account is one place the Owner's money sits or is spent from, in one currency:

```json
{
  "id": "6750…",
  "name": "Card",
  "kind": "card",
  "currency": "MXN",
  "institution": "Bank A",
  "last4": "1234",
  "note": "PDF statements, closes at month end",
  "trackedFrom": "2026-01",
  "closedFrom": "",
  "holdsMonth": true
}
```

- `kind`: `bank`, `card`, `savings` or `investment`. `currency`: `JPY` or `MXN`, which can't change once the Account has Statements.
- `last4`: exactly 4 digits or empty. Never a full card or account number (ADR 0014).
- `trackedFrom` and `closedFrom`: the first month tracked, and the first month no longer tracked (empty while open), as `YYYY-MM`. An Account is never deleted, only closed.
- `holdsMonth`: whether a month waits for its Statements to be Complete. True unless it is a card whose charge never changes and that closes mid-month, such as one that only pays the rent.

## Statements and Transactions

A Statement is one Account and one period (`from`, `to`, Days as `YYYY-MM-DD`), with its Transactions in it:

```json
{
  "id": "6751…",
  "account": "6750…",
  "from": "2026-10-01",
  "to": "2026-10-31",
  "source": "pdf",
  "fileIds": ["…"],
  "total": 1234.5,
  "closingBalance": null,
  "transactions": [
    {"id": "6752…", "kind": "expense", "day": "2026-10-03", "amount": 199.9, "rawText": "SHOP TEST", "note": ""},
    {"id": "6753…", "kind": "transfer", "day": "2026-10-27", "amount": 80000, "direction": "out", "rawText": "CARD", "note": "", "otherAccount": "6754…"}
  ]
}
```

- `source`: `csv`, `pdf` or `screenshots`. `fileIds`: the Telegram files it was read from.
- `files`: where each of those files is kept for good, in the private Statements bucket (ADR 0014), as `<account>/<from>_<to>/<save>/<n>`. Nando downloads each from Telegram as the Money bot and keeps it before the Statement is saved, so a Statement never points at a file that isn't there. A new folder each time it is sent: Nando only ever creates objects, never overwrites or deletes one.
- `total` (such as a card's billed amount) and `closingBalance` (the balance at `to`), each `null` when it states none. A closing balance at a month's last Day is the Account's month-end balance.
- Amounts are in the Account's currency: whole yen, or pesos to centavos. Nando keeps them in yen and centavos, so nothing is rounded. One above 1,000,000,000,000 is refused as misread.
- A Transaction's `kind` is `expense`, `income` or `transfer`. Its `day` is the purchase date, which decides its month, whatever month it was billed in. Its `amount` is positive, but negative for a refund, an `expense` coming back. A `transfer` has a `direction`, `in` to or `out` of the Account, and its `otherAccount` when known: never counted as spending or income.
- **Sending it again replaces it**: the same Account and period replaces the earlier Statement and all its Transactions, in one write.
- **Statements of one Account never overlap**: a period that overlaps another, without being the same, is refused, naming the other, so the Assistant asks the Owner which to keep.
- **A late Transaction**, dated before the period (it posted on a later bill), joins its own month. One another Statement of the Account already counts (the same day, amount and raw text) isn't counted twice: it is kept as a duplicate, left out of every answer, and counts instead if the other copy goes, with its Statement deleted or sent again without it, so no Transaction is lost. A Transaction dated after the period is refused.
- Money has **no Editable Window**: any period can be saved, replaced or deleted (ADR 0014).

## What a Transaction is

- **Categories** are a fixed list in the code (ADR 0014), each with a `key`, the `name` the Owner reads and its `kind`. For expenses: Rent (`rent`), Water, Gas & Electricity (`utilities`), Phone & Internet (`phone_internet`), Groceries (`groceries`), Eating Out (`eating_out`), Convenience Store (`convenience_store`), Public Transport (`public_transport`), Travel (`travel`), Subscriptions (`subscriptions`), Shopping (`shopping`), Personal Care (`personal_care`), Beauty (`beauty`), Health (`health`), Entertainment (`entertainment`), Education (`education`), Gifts (`gifts`), Fees & Interest (`fees_interest`), Cash & PayPay (unitemized) (`cash_paypay`) and Other (`other`). For income: Salary (`salary`), Interest (`interest`) and Other income (`other_income`).
- A **Subcategory** (`id`, `category`, `name`, `essential`) is a finer place inside one Category, with a name unique there in any case. It is Essential or Optional, and it can be renamed or merged, never moved.
- On each `expense` or `income`, as `money_save_statement` and `money_update_transaction` take them:
  - `category` must be a Category of its kind, and `subcategory`, if any, a Subcategory in it;
  - `merchant` is its Merchant;
  - `essential` is set only when this one Transaction differs from its Subcategory, and `unconfirmed` marks Claude's best guess awaiting the Owner.
  - A `transfer` has none of these, only `unconfirmed`.
  - Answers carry `essentialOverride`, and `essential`, what the Transaction counts as: its override, else its Subcategory's mark, else `null`, unmarked, so a missing mark shows rather than being guessed.
- A **Merchant** (`id`, `name`, `rawTexts`, `category`, `subcategory`) is who a Transaction was with, cleaned up from the raw texts it has been seen as, with what its Transactions usually are. One that sells everything has neither.
- A **Subscription** (`id`, `merchant`, `amount`, `currency`, `every`: `monthly` or `yearly`, `lastCharged`, `category`, `subcategory`, `essential`, `active`) is a charge that repeats. It is `late` when active and more than a week past its expected charge (a month or a year after `lastCharged`), so the Review can ask whether it was cancelled.

## Months

A month is **Complete** when every Account tracked in it (from `trackedFrom`, until `closedFrom`) that holds it has Statements reaching its last Day: one covers that Day, or, for a `savings` or `investment` Account, one states a balance on or after it.

## Tools

- **`money_get_settings`**: the settings, as above. Read-only.
- **`money_save_settings`** (`askAbove`): changes only the currencies given, such as `{"askAbove": {"JPY": 5000}}`, and answers the settings. A currency other than JPY or MXN, an amount of 0 or less, or no `askAbove` is refused, and nothing changes.
- **`money_save_account`** (`id` to change one, or `name`, `kind`, `currency` and `trackedFrom` for a new one; `institution`, `last4`, `note`, `closedFrom`, `holdsMonth` optional): answers the Account. Only the fields sent change.
- **`money_list_accounts`**: every Account, open and closed, by name. Read-only.
- **`money_save_statement`** (`account`, `from`, `to`, `source`, `fileIds`, `total`, `closingBalance`, `transactions`): saves the Statement as above, all at once or not at all. It answers the Statement, with `late`, how many Transactions were dated before the period, and `skipped`, how many of those another Statement already had; `missingFiles`, the files Telegram had dropped, which are left out; and `filesNote` when no files are kept, as without `MONEY_STATEMENTS_BUCKET`. If a file can't be kept for any other reason, nothing is saved, and the error says why.
- **`money_get_statement_file`** (`statement`, `n`, `pages`, `lines`): reads back the Statement's file `n` (1 for the first) from the bucket, as `messenger_get_file` would: a PDF's pages, a CSV's lines, or a screenshot as an image. Read-only.
- **`money_get_statement`** (`id`): the Statement with its Transactions. Read-only.
- **`money_list_statements`** (`account`, `fromMonth` and `toMonth` optional): the Account's Statements that reach into those months, by period, without their Transactions. Read-only.
- **`money_delete_statement`** (`id`): deletes the Statement and its Transactions. The Assistant asks the Owner first (ADR 0008).
- **`money_list_transactions`** (`account`, `fromMonth`, `toMonth`, `kind`, `category`, `subcategory`, `merchant`, `unconfirmed`, `text`, `limit`, all optional): Transactions by Day, each with its `statement` and `account`. `text` matches the raw text or note, in any case. At most `limit`, 200 unless given, and never more than 1000. Read-only.
- **`money_update_transaction`** (`id`, `note`, `otherAccount`, `category`, `subcategory`, `merchant`, `essential`, `unconfirmed`, `teachMerchant`): changes the fields given, and answers the Transaction.
  - `otherAccount` is for a `transfer` only, and empty clears it.
  - A `category` given without a `subcategory` clears the Subcategory. A `null` `essential` has it follow its Subcategory again.
  - `teachMerchant` also has its Merchant learn its raw text and usually take its Category and Subcategory: for a correction that should stick. Without it, an exception stays one-off.
- **`money_list_categories`**: every Category, in order, with its Subcategories. Read-only.
- **`money_save_subcategory`** (`id` to change one, or `category`, `name` and `essential` for a new one): answers it.
- **`money_merge_subcategories`** (`from`, `into`): moves every Transaction, Merchant and Subscription of `from` into `into`, and into its Category, then removes `from`. The Assistant asks the Owner first.
- **`money_save_merchant`** (`id` to change one, or `name` for a new one; `rawTexts` added, `category`, `subcategory`): answers it.
- **`money_find_merchant`** (`rawText`): the Merchant seen as it exactly, or else named it in any case; a Tool error when there is none. Read-only.
- **`money_list_merchants`**: every Merchant, by name. Read-only.
- **`money_save_subscription`** (`id` to change one, or `merchant`, `amount`, `currency`, `every`, `lastCharged` and `essential` for a new one; `category`, `subcategory`, `active`): answers it, with `late`.
- **`money_list_subscriptions`**: every Subscription, active ones first, then by last charge, each with `late`. Read-only.
- **`money_get_month`** (`month`): `complete`, and `missing`, the Accounts the month still waits for, and `late`, those that don't hold the month and haven't reached its end, each with its `id` and `name`. Read-only.

Something sent wrong is a Tool error saying what, and nothing changes.

## Database

The `money` database holds `settings`, one document; `accounts`; `statements`, each with its Transactions, with a unique index on Account and period and an index on its Transactions' IDs; `subcategories`; `merchants`, with an index on their raw texts; and `subscriptions`.
