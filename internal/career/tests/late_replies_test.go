package tests

import (
	"slices"
	"testing"
)

func TestLateRepliesListTheRecruitersWhoseMessageIsOldestFirst(t *testing.T) {
	_, phone := signedInBrowser(t)
	conversations, recruiters := fileThreads(t, phone,
		thread("mori", message("m1", "other", at(-45, 10, 0), "Are you open to new roles?")),
		thread("ueda", message("u1", "other", at(-400, 10, 0), "A new role in Osaka!")),
		thread("tanaka", message("t1", "other", at(-3, 10, 15), "I have a backend role at a fintech.")),
		thread("abe", message("a1", "other", at(-90, 10, 0), "Hello from Abe.")),
		secondThread("abe", message("a2", "other", at(-35, 10, 0), "Abe again, with another role.")),
	)
	suggestReply(t, phone, conversations["2-mori"], "late", "Hello Mori-san!\nMy deepest apologies for the late response.", at(-45, 10, 0))
	suggestReply(t, phone, conversations["2-ueda"], "late", "Hello Ueda-san!", at(-400, 10, 0))
	suggestReply(t, phone, conversations["2-tanaka"], "answer", "Thank you for reaching out!", at(-3, 10, 15))
	suggestReply(t, phone, conversations["2-abe"], "late", "Hello Abe-san!", at(-90, 10, 0))
	suggestReply(t, phone, conversations["3-abe"], "late", "Hello again, Abe-san!", at(-35, 10, 0))

	late := get[[]waitingJSON](phone, "/api/career/late-replies")
	var names []string
	for _, w := range late {
		names = append(names, w.Name)
	}
	if want := []string{"ueda", "abe", "mori"}; !slices.Equal(names, want) {
		t.Fatalf("Late Replies list %q; want the Recruiters with a late one, once each, by how old their oldest such message is, oldest first: %q", names, want)
	}
	if abe := late[1]; !abe.LastMessageAt.Equal(at(-90, 10, 0)) || abe.FirstLine != "Hello Abe-san!" {
		t.Errorf("Abe is listed as %+v; want their oldest message waiting for a Late Reply, and that reply", abe)
	}
	if mori := late[2]; mori.ID != recruiters["mori"] || mori.Organisation != "mori Agency" || !mori.LastMessageAt.Equal(at(-45, 10, 0)) || mori.FirstLine != "Hello Mori-san!" {
		t.Errorf("Mori is listed as %+v; want their id, agency, when their message came and the reply's first line", mori)
	}
}
