package tests

import (
	"strings"
	"testing"
)

// merchantJSON is a Merchant, as the Tools read it.
type merchantJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	RawTexts    []string `json:"rawTexts"`
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory"`
}

func TestAMerchantIsFoundByItsRawTextAndLearnsOnlyWhenTaught(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	snacks := saveSubcategory(t, session, map[string]any{"category": "convenience_store", "name": "Snacks", "essential": false})
	lunch := saveSubcategory(t, session, map[string]any{"category": "convenience_store", "name": "Lunch", "essential": true})
	store := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{
		"name": "Corner Store", "rawTexts": []string{"ｺｰﾅｰｽﾄｱ ﾃｽﾄ"}, "category": "convenience_store", "subcategory": snacks.ID,
	})

	found := callTool[merchantJSON](t, session, "money_find_merchant", map[string]any{"rawText": "ｺｰﾅｰｽﾄｱ ﾃｽﾄ"})
	if found.ID != store.ID || found.Subcategory != snacks.ID {
		t.Errorf("by its raw text: %+v; want the Corner Store, usually Snacks", found)
	}
	if byName := callTool[merchantJSON](t, session, "money_find_merchant", map[string]any{"rawText": "corner store"}); byName.ID != store.ID {
		t.Errorf("by its name: %+v; want the Corner Store", byName)
	}
	if msg := toolError(t, session, "money_find_merchant", map[string]any{"rawText": "UNKNOWN"}); !strings.Contains(msg, "no Merchant") {
		t.Errorf("an unknown raw text: %q", msg)
	}

	_, lines := oneStatement(t, session,
		map[string]any{"kind": "expense", "day": "2026-10-03", "amount": 450, "rawText": "ｺｰﾅｰｽﾄｱ ﾃｽﾄ2", "category": "convenience_store", "subcategory": snacks.ID, "merchant": store.ID},
		map[string]any{"kind": "expense", "day": "2026-10-04", "amount": 650, "rawText": "ｺｰﾅｰｽﾄｱ ﾃｽﾄ2", "category": "convenience_store", "subcategory": snacks.ID, "merchant": store.ID},
	)
	// A one-off exception stays one-off.
	callTool[whatItIs](t, session, "money_update_transaction", map[string]any{"id": lines[0].ID, "subcategory": lunch.ID})
	if m := callTool[merchantJSON](t, session, "money_find_merchant", map[string]any{"rawText": "ｺｰﾅｰｽﾄｱ ﾃｽﾄ"}); m.Subcategory != snacks.ID {
		t.Errorf("after a plain update: usually %q, want Snacks still", m.Subcategory)
	}
	// A correction that should stick teaches it, raw text and all.
	callTool[whatItIs](t, session, "money_update_transaction", map[string]any{"id": lines[1].ID, "subcategory": lunch.ID, "teachMerchant": true})
	m := callTool[merchantJSON](t, session, "money_find_merchant", map[string]any{"rawText": "ｺｰﾅｰｽﾄｱ ﾃｽﾄ2"})
	if m.ID != store.ID || m.Subcategory != lunch.ID {
		t.Errorf("after teachMerchant: %+v; want the Corner Store, usually Lunch, found by the new raw text", m)
	}
}

func TestAMergeMovesEverythingOfOneSubcategoryIntoAnother(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	clothes := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": "Clothes", "essential": false})
	fashion := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": "Fashion", "essential": false})
	merchant := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": "Shop", "category": "shopping", "subcategory": clothes.ID})
	subscription := callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{
		"merchant": merchant.ID, "amount": 1000, "currency": "JPY", "every": "monthly", "lastCharged": "2026-11-01",
		"category": "shopping", "subcategory": clothes.ID, "essential": false,
	})
	_, lines := oneStatement(t, session,
		map[string]any{"kind": "expense", "day": "2026-10-03", "amount": 4000, "rawText": "SHOP", "category": "shopping", "subcategory": clothes.ID},
	)

	callTool[map[string]any](t, session, "money_merge_subcategories", map[string]any{"from": clothes.ID, "into": fashion.ID})

	if got := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"subcategory": fashion.ID}); len(got) != 1 || got[0].ID != lines[0].ID {
		t.Errorf("Fashion's Transactions = %+v; want the moved one", got)
	}
	if got := callTool[[]merchantJSON](t, session, "money_list_merchants", nil); len(got) != 1 || got[0].Subcategory != fashion.ID {
		t.Errorf("merchants = %+v; want the Shop usually Fashion", got)
	}
	if got := callTool[[]subscriptionJSON](t, session, "money_list_subscriptions", nil); len(got) != 1 || got[0].ID != subscription.ID || got[0].Subcategory != fashion.ID {
		t.Errorf("subscriptions = %+v; want it in Fashion", got)
	}
	for _, c := range callTool[[]categoryJSON](t, session, "money_list_categories", nil) {
		for _, s := range c.Subcategories {
			if s.ID == clothes.ID {
				t.Errorf("Clothes is still there after the merge")
			}
		}
	}
}

func TestASubcategoryMergesOnlyIntoOneOfTheSameKind(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bonus := saveSubcategory(t, session, map[string]any{"category": "salary", "name": "Bonus", "essential": true})
	family := saveSubcategory(t, session, map[string]any{"category": "gifts", "name": "Family", "essential": false})

	if msg := toolError(t, session, "money_merge_subcategories", map[string]any{"from": bonus.ID, "into": family.ID}); !strings.Contains(msg, "income") {
		t.Errorf("merging income into an expense: %q; want it refused", msg)
	}
}

func TestAMerchantsCategoryChangedAloneClearsItsSubcategory(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	fashion := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": "Fashion", "essential": false})
	shop := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": "Shop", "category": "shopping", "subcategory": fashion.ID})
	streaming := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": "Streaming", "category": "subscriptions"})
	sub := callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{
		"merchant": streaming.ID, "amount": 990, "currency": "JPY", "every": "monthly", "lastCharged": "2026-11-01",
		"category": "shopping", "subcategory": fashion.ID, "essential": false,
	})

	moved := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"id": shop.ID, "category": "beauty"})
	if moved.Category != "beauty" || moved.Subcategory != "" {
		t.Errorf("Merchant moved to Beauty: %+v; want its Subcategory cleared", moved)
	}
	movedSub := callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{"id": sub.ID, "category": "subscriptions"})
	if movedSub.Category != "subscriptions" || movedSub.Subcategory != "" {
		t.Errorf("Subscription moved: %+v; want its Subcategory cleared", movedSub)
	}
}
