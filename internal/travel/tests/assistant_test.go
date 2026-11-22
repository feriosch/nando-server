package tests

import (
	"context"
	"testing"

	"github.com/feriosch/nando-server/internal/core/model"
)

func TestTheOwnersMessageToTheTravelBotWaitsInTheTravelTalk(t *testing.T) {
	n := newNando(t)

	n.ownerSays("travel", 9001, 51, "any cheap fares this week?")

	talk, err := n.core.Talk(context.Background(), "travel")
	if err != nil {
		t.Fatalf("Talk(travel): %v", err)
	}
	if got := talk.Messages; len(got) != 1 || got[0].From != model.FromOwner ||
		got[0].Text != "any cheap fares this week?" || got[0].State != model.MessageWaiting {
		t.Errorf("Travel Talk's messages = %+v; want the Owner's one message, waiting", got)
	}
	reactions := n.telegram.CallsTo("setMessageReaction")
	if len(reactions) != 1 || reactions[0].Token != travelBotToken {
		t.Errorf("setMessageReaction calls = %+v; want one, by the Travel bot", reactions)
	}
}
