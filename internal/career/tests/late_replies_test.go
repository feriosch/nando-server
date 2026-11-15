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
	)
	suggestReply(t, phone, conversations["mori"], "late", "Hello Mori-san!\nMy deepest apologies for the late response.", at(-45, 10, 0))
	suggestReply(t, phone, conversations["ueda"], "late", "Hello Ueda-san!", at(-400, 10, 0))
	suggestReply(t, phone, conversations["tanaka"], "answer", "Thank you for reaching out!", at(-3, 10, 15))
	suggestReply(t, phone, conversations["abe"], "late", "Hello Abe-san!", at(-90, 10, 0))

	late := get[[]waitingJSON](phone, "/api/career/late-replies")
	var names []string
	for _, w := range late {
		names = append(names, w.Name)
	}
	if want := []string{"ueda", "abe", "mori"}; !slices.Equal(names, want) {
		t.Fatalf("Late Replies list %q; want the Recruiters with a late one, by how old their message is, oldest first: %q", names, want)
	}
	if mori := late[2]; mori.ID != recruiters["mori"] || mori.Organisation != "mori Agency" || !mori.LastMessageAt.Equal(at(-45, 10, 0)) || mori.FirstLine != "Hello Mori-san!" {
		t.Errorf("Mori is listed as %+v; want their id, agency, when their message came and the reply's first line", mori)
	}
}
