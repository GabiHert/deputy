package domain

import "testing"

func TestRouter(t *testing.T) {
	const (
		dm1   = "111@s.whatsapp.net"
		dm2   = "222@lid"
		dm3   = "333@s.whatsapp.net"
		group = "999@g.us"
	)
	agents := []Agent{
		{ID: "all", AllDirect: true, Except: []string{dm3}},
		{ID: "picked", Chats: []string{dm2, group}},
	}
	r := NewRouter(agents)
	cases := []struct {
		jid    string
		want   string
		viaAll bool
	}{
		{dm1, "all", true},
		{dm2, "picked", false}, // a picked agent wins over all direct chats
		{dm3, "", false},       // skipped
		{group, "picked", false},
		{"888@g.us", "", false}, // groups never go to all direct chats
		{"status@broadcast", "", false},
	}
	for _, c := range cases {
		a, via, ok := r.Owner(c.jid)
		if a.ID != c.want || via != c.viaAll || ok != (c.want != "") {
			t.Errorf("%s: got %q via=%v ok=%v, want %q via=%v", c.jid, a.ID, via, ok, c.want, c.viaAll)
		}
	}
}
