package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/GabiHert/deputy/internal/domain"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "db.sqlite")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func TestMigrationsRunOnceAndUpgradeOldDatabases(t *testing.T) {
	// A database from before migrations: base schema, user_version 0, with data.
	p := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite3", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages(chat_jid,id,at,data) VALUES('c@g.us','M1',1,'{"id":"M1","chatJid":"c@g.us","text":"oi"}')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ { // reopening must not fail on steps already applied
		s, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if v := s.SchemaVersion(); v != len(migrations) {
			t.Fatalf("version %d, want %d", v, len(migrations))
		}
		if m, err := s.Message("c@g.us", "M1"); err != nil || m.Text != "oi" {
			t.Fatalf("old data: %+v %v", m, err)
		}
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('messages_id','messages_at')`).Scan(&n)
		if n != 2 {
			t.Fatalf("indexes: %d", n)
		}
		s.Close()
	}
}

func TestMessagesBeforePagesNewestFirst(t *testing.T) {
	s, _ := open(t)
	base := time.UnixMilli(1_700_000_000_000)
	for i := 0; i < 7; i++ {
		// Two messages share each timestamp, so the cursor must break ties by ID.
		at := base.Add(time.Duration(i/2) * time.Second)
		if err := s.SaveMessage(domain.Message{ID: fmt.Sprintf("M%d", i), ChatJID: "c@g.us", At: at, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.SaveMessage(domain.Message{ID: "OTHER", ChatJID: "d@g.us", At: base})
	var got []string
	cur := Cursor{}
	for pages := 0; pages < 10; pages++ {
		ms, next := s.MessagesBefore("c@g.us", ParseCursor(cur.String()), 3)
		for _, m := range ms {
			got = append(got, m.ID)
		}
		if next.String() == "" {
			break
		}
		cur = next
	}
	want := "[M6 M5 M4 M3 M2 M1 M0]"
	if fmt.Sprint(got) != want {
		t.Fatalf("got %v, want %s", got, want)
	}
	if ms, next := s.MessagesBefore("c@g.us", Cursor{}, 7); len(ms) != 7 || next.String() != "" {
		t.Fatalf("an exact last page should have no cursor: %d %q", len(ms), next.String())
	}
	if o, err := s.OldestMessage("c@g.us"); err != nil || o.ID != "M0" {
		t.Fatalf("oldest %+v %v", o, err)
	}
}

func TestSaveMessagesKeepsWhatIsStored(t *testing.T) {
	s, _ := open(t)
	_ = s.SaveMessage(domain.Message{ID: "A", ChatJID: "c@g.us", At: time.Now(), Text: "live", Transcript: "kept"})
	added, err := s.SaveMessages([]domain.Message{
		{ID: "A", ChatJID: "c@g.us", At: time.Now(), Text: "from history", History: true},
		{ID: "B", ChatJID: "c@g.us", At: time.Now(), Text: "new", History: true},
	})
	if err != nil || len(added) != 1 || added[0].ID != "B" {
		t.Fatalf("added %+v %v", added, err)
	}
	if m, _ := s.Message("c@g.us", "A"); m.Transcript != "kept" || m.History {
		t.Fatalf("stored message changed: %+v", m)
	}
}

func TestFindAndUpdateMessageAcrossChatIDs(t *testing.T) {
	s, _ := open(t)
	_ = s.SaveMessage(domain.Message{ID: "X1", ChatJID: "5511@s.whatsapp.net", At: time.Now(), FromMe: true, Status: domain.StatusSent})
	m, ok, err := s.UpdateMessage("123@lid", "X1", func(m *domain.Message) bool {
		m.Status = m.Status.Upgrade(domain.StatusRead)
		return true
	})
	if err != nil || !ok || m.ChatJID != "5511@s.whatsapp.net" {
		t.Fatalf("%+v %v %v", m, ok, err)
	}
	if got, _ := s.Message("5511@s.whatsapp.net", "X1"); got.Status != domain.StatusRead {
		t.Fatalf("status %s", got.Status)
	}
	if _, ok, _ := s.UpdateMessage("5511@s.whatsapp.net", "X1", func(*domain.Message) bool { return false }); ok {
		t.Fatal("no change should not save")
	}
	if _, _, err := s.UpdateMessage("c", "missing", func(*domain.Message) bool { return true }); err != ErrNotFound {
		t.Fatalf("missing: %v", err)
	}
}

func TestUnreadQueriesAndSearch(t *testing.T) {
	s, _ := open(t)
	base := time.Now().Add(-time.Hour)
	add := func(id, text string, at time.Time, fromMe, revoked bool) {
		_ = s.SaveMessage(domain.Message{ID: id, ChatJID: "c@g.us", At: at, Text: text, FromMe: fromMe, Revoked: revoked})
	}
	add("1", "Bom dia", base, false, false)
	add("2", "almoço às 13h", base.Add(time.Minute), true, false)
	add("3", "Almoço confirmado", base.Add(2*time.Minute), false, false)
	add("4", "", base.Add(3*time.Minute), false, true)
	_ = s.SaveMessage(domain.Message{ID: "5", ChatJID: "d@g.us", At: base, Kind: domain.KindDocument, FileName: "almoco_100%.pdf"})
	if n := s.CountUnreadAfter("c@g.us", base); n != 1 {
		t.Fatalf("unread after: %d", n)
	}
	if ms := s.UnreadAfter("c@g.us", time.Time{}, 0); len(ms) != 2 || ms[0].ID != "3" {
		t.Fatalf("unread list %+v", ms)
	}
	if hits := s.SearchMessages("ALMOço", "", 10); len(hits) != 2 || hits[0].ID != "3" {
		t.Fatalf("search %+v", hits)
	}
	if hits := s.SearchMessages("100%", "", 10); len(hits) != 1 || hits[0].ID != "5" {
		t.Fatalf("file name search with a %% sign: %+v", hits)
	}
	if hits := s.SearchMessages("almoço", "d@g.us", 10); len(hits) != 0 {
		t.Fatalf("chat filter: %+v", hits)
	}
	_, _ = s.UpdateChat("c@g.us", func(c *domain.Chat) { c.Unread = 2 })
	_, _ = s.UpdateChat("d@g.us", func(c *domain.Chat) { c.Unread = 5; c.MutedUntil = domain.Forever })
	if n := s.TotalUnread(time.Now()); n != 2 {
		t.Fatalf("total unread %d", n)
	}
}

func TestReactionsAndStatus(t *testing.T) {
	var m domain.Message
	if !m.SetReaction(domain.Reaction{Sender: "a", Emoji: "👍"}) || !m.SetReaction(domain.Reaction{FromMe: true, Emoji: "❤️"}) {
		t.Fatal("adding reactions")
	}
	if m.SetReaction(domain.Reaction{Sender: "a", Emoji: "👍"}) {
		t.Fatal("the same reaction again is no change")
	}
	if !m.SetReaction(domain.Reaction{Sender: "a", Emoji: ""}) || len(m.Reactions) != 1 || !m.Reactions[0].FromMe {
		t.Fatalf("removing: %+v", m.Reactions)
	}
	if domain.StatusRead.Upgrade(domain.StatusDelivered) != domain.StatusRead || domain.MsgStatus("").Upgrade(domain.StatusSent) != domain.StatusSent {
		t.Fatal("status upgrade")
	}
}

func TestChatTimerAndViewOnceArePersisted(t *testing.T) {
	s, _ := open(t)
	at := time.Unix(1700000000, 0)
	if _, err := s.UpdateChat("g@g.us", func(c *domain.Chat) {
		c.Disappearing, c.DisappearingAt, c.DisappearingKnown = 604800, at, true
	}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Chat("g@g.us")
	if err != nil || c.Disappearing != 604800 || !c.DisappearingKnown || !c.DisappearingAt.Equal(at) {
		t.Fatalf("chat %+v %v", c, err)
	}
	if err := s.SaveMessage(domain.Message{ChatJID: "g@g.us", ID: "V1", At: at, Kind: domain.KindImage, ViewOnce: true}); err != nil {
		t.Fatal(err)
	}
	if m, err := s.Message("g@g.us", "V1"); err != nil || !m.ViewOnce {
		t.Fatalf("message %+v %v", m, err)
	}
}
