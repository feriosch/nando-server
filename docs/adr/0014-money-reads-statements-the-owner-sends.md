# Money reads the Statements the Owner sends, through the Money Assistant

The Owner keeps money in Japan and Mexico: a bank account in each, a savings pot and a new NISA account, and three credit cards, one of which pays only the rent. They want every Transaction in Nando, so Claude can find trends and spending they could do without. No bank or card offers an API to one person, so Nando gets a **Money** Module and a **Money Assistant** with its own chat in the Messenger. Once a month the Owner sends each Account's **Statement** there, as a CSV, a PDF or screenshots. Claude reads it and records its Transactions through Money Tools. Nando still never calls a model.

## Decisions

- **Every Transaction is kept, not monthly totals.** Finding a forgotten Subscription or spending that crept up takes the individual lines. Each Account also keeps its balance at each month's end.
- **What Money holds:**
  - spending and income;
  - balances, so net worth;
  - savings and investments, as a balance only, never their holdings.
- **Three kinds of Transaction:** Expense, Income and **Transfer**.
  - A Transfer moves money between two of the Owner's Accounts and never counts as spending or income. Examples: a card's bill paid from a bank account, money put into savings or the NISA.
  - Taking cash out of a bank, or topping up an e-money app such as PayPay, is an Expense in **Cash & PayPay (unitemized)**: neither is an Account, so the trail ends there. A Suica top-up through a card is an Expense in Public Transport.
  - A Refund lowers spending in the Category it came back to.
  - A savings or investment Account's growth is its **Return**: its balance, less last month's, less what was Transferred in. It is neither Income nor saving, so the saving rate counts only what the Owner saved.
- **A Transaction counts in the month of its purchase date**, not the month it was billed. The cards close on different days, and spending trends are about when the Owner chose to spend.
  - A month is **Complete** once every Account has a Statement covering its last day.
  - **An Account can be set not to hold a month back.** This is for the card that only pays the rent: it closes on the 27th, so the Statement covering a month's last days comes almost a month later. Its charge is the same every month, so waiting for it would delay every Monthly Review by about two weeks for nothing.
  - A Transaction that appears late, on the next Statement, joins its own month, and the next Monthly Review says so.
- **A Statement is one Account and one period.**
  - Nando keeps the total or closing balance the Statement states, to check its Transactions against.
  - Sending a Statement for the same Account and period again replaces it, so overlapping files never count twice.
  - Money has **no Editable Window**: Statements arrive a month late and history is brought in from earlier, so any month can change. As with every Assistant, deleting is asked first (ADR 0008).
- **Amounts keep their own currency; totals are in yen.**
  - Nando fetches one **Rate** a month from the European Central Bank's reference rates (through Frankfurter, which needs no key): the month's average for Transactions, the last day's for balances.
  - It stores the Rate and never changes it, so old totals never move.
  - The Rate differs by a percent or two from what a bank charged, which is fine for trends.
- **Categories are a fixed list in Money's code**, so months always compare:
  - Rent; Water, Gas & Electricity; Phone & Internet; Groceries; Eating Out; Convenience Store; Public Transport; Travel; Subscriptions; Shopping; Personal Care; Beauty; Health; Entertainment; Education; Gifts; Fees & Interest; Cash & PayPay (unitemized); Other.
  - Income: Salary; Interest; Other.
  - **Beauty** stands apart from **Personal Care**: a dental cleaning is care, teeth whitening is beauty.
- **Subcategories grow as needed** under any Category, such as Fashion and Furniture under Shopping. Claude creates one when nothing fits and says so, and the Owner renames or merges them in the chat.
- **Each Subcategory is Essential or Optional**, and a Transaction can say otherwise. A Category can also carry a default mark in Money's settings, for what has no Subcategory there (#229): the first import left most spending unmarked without one. Optional spending is where unnecessary expenses are looked for, and the Monthly Review splits every month this way.
- **A large purchase that won't repeat can be a One-off** (#241). The Owner marks it through the Money Assistant, which asks in the Review about an Expense of at least ¥50,000 or MX$5,000 that is over twice its Category's 3-month average. It still counts in its month, but not in the comparisons with the months around it, so one laptop never makes the next three months read as below average. Nando never marks one by itself, and trips are never One-offs.
- **Merchants are remembered.** Claude cleans a Statement's raw text into a Merchant with its usual Subcategory, and the Owner's corrections are kept on the Merchant, so the next Statement gets it right. A Merchant that sells everything, such as an online store, has no usual Subcategory.
- **Claude reads every Statement and asks when it can't tell.**
  - It first looks for the receipt in the Owner's Gmail, under `Banks/*` and `Payments/*`, matching by date and amount. Those labels are what the Core Assistant already files (ADR 0011).
  - What is still unclear is asked about in one list, if it is over a threshold in Money's settings (¥3,000 and MX$300 to start). Smaller ones keep Claude's best guess.
  - Nothing waits on the Owner: an unclear line is saved at once as **Unconfirmed**, and becomes final once the Owner answers.
- **The Money Assistant reads Gmail and never changes it.** It reads `Banks/*` and `Payments/*` only, and points the Owner to the Core Assistant for anything else in the mailbox.
- **Subscriptions are their own list**, kept by Claude from repeating charges and receipts. It points out new ones, price changes and ones the Owner may have stopped using.
- **What the Owner sends:**
  - a CSV where the bank or card offers one;
  - a PDF where it offers only that;
  - screenshots for an app that offers neither.
  - The Messenger learns to read **CSV files**, in Core, as it learned PDFs (#135). This amends ADR 0008, which answered every file but photos and PDFs with a fixed line. Japanese banks write CSVs in Shift_JIS, so Nando turns them into UTF-8 text for the run.
  - Which Account takes which format, and its billing cycle, is learned from its first Statement and kept with the Account, never in the code.
- **Every Statement's file is kept** in a private Cloud Storage bucket in the project.
  - A Money Tool takes the Telegram `file_id`; Nando downloads the file and stores it with its own service account. Claude never holds cloud credentials.
  - The Statement keeps where its file is, so a month can be read again later.
  - This matters because banks keep history only a while (a passbook bank account only 2 months, a card about 12 to 15), and Telegram drops old files.
- **Nando never keeps full card or account numbers**, branch codes or the Owner's address from a Statement's header. An Account carries at most the last 4 digits, to tell two apart.
- **Statement Reminders, one per app the Owner opens.**
  - A Statement Reminder is a small Notification, on a day in Money's settings, asking for the Statements one app holds. One bank's app can hold an account, a card and a savings pot.
  - It is sent only while those Statements are missing, and again 3 days later if they still are.
  - Small asks on different days were chosen over one large monthly checklist, so it never becomes one big chore.
- **The Monthly Review is a Notification that asks for work** (ADR 0009), sent when a month becomes Complete and answered by the Assistants' scheduled task (ADR 0008). There is no Money scheduled task of its own. It covers:
  - spending against last month and the 3-month average;
  - Essential against Optional, and the Categories that moved most;
  - Subscriptions, and anything worth a look;
  - net worth, Return and the saving rate;
  - what is still Unconfirmed.
  - The Owner can ask the Money Assistant anything between Reviews.
- **History starts in January 2026**, sent through the same chat one Statement at a time, with no separate import.
- **The web app only reads Money**, like Calories and Travel:
  - a Home Card with the latest Complete month against the average, and net worth, shipped with the Module;
  - later, once a few months are in, a Money screen with each month by Category down to Subcategories and Transactions, Essential against Optional, Subscriptions, and net worth over time.
  - Changes happen in the chat.

## Considered Options

- **An aggregator** (Money Forward ME, Moneytree, Zaim; Belvo or similar in Mexico): rejected. Their APIs are for businesses. Money Forward ME exports a CSV only on its paid plan and only from its app. No aggregator offers one person access to the Mexican bank. Each would also mean handing bank logins to one more company.
- **The banks' own APIs**: rejected. Where one exists, it is open only to licensed intermediaries, not to a person.
- **Monthly totals per Category**: rejected. Cheaper to store, but trends and unnecessary expenses need the lines.
- **Counting by billing month**: rejected. It matches the bills, but splits one month's spending across two depending on which card paid.
- **Statements picked up from Gmail by themselves**: rejected for now. The Mexican card's statement email says the PDF is ready in the app but doesn't attach it. The Japanese card's per-purchase emails were turned off and lacked the details its CSV has. If an issuer starts attaching its statement, this can be revisited for that Account.
- **A daily Rate per Transaction, or Claude looking Rates up**: rejected. One stored Rate a month is enough for trends, gives the same answer every time and works for past months.
- **Keeping files only in Telegram**: rejected. Telegram drops old files, and some banks drop history after 2 months.
- **Beauty as a Subcategory of Shopping**: rejected. The Owner wants beauty told apart from basic care wherever it was bought, so it is a Category, and Essential and Optional cover the same need everywhere else.
- **One monthly checklist instead of Statement Reminders**: rejected by the Owner as one big chore.

## Consequences

- A new `money` database, added to `scripts/backup.sh`. It holds the settings, Accounts, Statements, Transactions, Merchants, Subcategories, Subscriptions and Rates.
- Core's list of Assistants gains `money`, with its own Telegram bot, `TELEGRAM_BOT_TOKEN_MONEY`, from Secret Manager. Its name and picture are set by hand in @BotFather.
- The Messenger reads CSV files for every Assistant, not only Money's.
- A new private Cloud Storage bucket, with no public access, which Nando's Cloud Run service account may write and read. Statement files are kept for good.
- Nando calls Frankfurter, an outside service and not a model, the way Japanese calls jisho.org and Travel calls SerpApi.
- The Assistants' scheduled task needs no new connector: Gmail is already there.
- Every Statement holds the Owner's financial data, and Claude reads it in a cloud session on the Owner's own subscription, as it already reads their mail.
- Each Statement and Monthly Review uses some of the Owner's Claude usage. Bringing in the history since January is the largest share, and is spread over as many messages as the Owner likes.
