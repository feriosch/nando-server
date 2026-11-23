package tests

import (
	"context"
	"testing"

	"github.com/feriosch/nando-server/internal/core/model"
)

func TestTheOwnersMessageToTheMoneyBotWaitsInTheMoneyTalk(t *testing.T) {
	n := newNando(t)

	n.ownerSays("money", 9001, 51, "here is October's statement")

	talk, err := n.core.Talk(context.Background(), "money")
	if err != nil {
		t.Fatalf("Talk(money): %v", err)
	}
	if got := talk.Messages; len(got) != 1 || got[0].From != model.FromOwner ||
		got[0].Text != "here is October's statement" || got[0].State != model.MessageWaiting {
		t.Errorf("Money Talk's messages = %+v; want the Owner's one message, waiting", got)
	}
	reactions := n.telegram.CallsTo("setMessageReaction")
	if len(reactions) != 1 || reactions[0].Token != moneyBotToken {
		t.Errorf("setMessageReaction calls = %+v; want one, by the Money bot", reactions)
	}
}
