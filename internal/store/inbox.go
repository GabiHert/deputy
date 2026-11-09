package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/domain"
)

// migrations run once each, in order, on top of the base schema; PRAGMA user_version counts them.
// Add new steps at the end and never change old ones.
var migrations = []string{
	// 1: the inbox looks messages up by ID alone (receipts may name a chat by another JID) and
	// searches every chat newest first.
	`CREATE INDEX IF NOT EXISTS messages_id ON messages (id);
	 CREATE INDEX IF NOT EXISTS messages_at ON messages (at);`,
}

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion is the number of migrations applied.
func (s *Store) SchemaVersion() int {
	var v int
	_ = s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v
}

func scanMessages(rows *sql.Rows) []domain.Message {
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var v string
		var m domain.Message
		if rows.Scan(&v) == nil && json.Unmarshal([]byte(v), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// FindMessage returns a message in chat, or by ID alone when chat does not have it: WhatsApp can
// name the same direct chat by phone number or by LID.
func (s *Store) FindMessage(chat, id string) (domain.Message, error) {
	if m, err := s.Message(chat, id); err == nil {
		return m, nil
	}
	var v string
	if err := s.db.QueryRow(`SELECT data FROM messages WHERE id=? ORDER BY at DESC LIMIT 1`, id).Scan(&v); err != nil {
		return domain.Message{}, ErrNotFound
	}
	var m domain.Message
	err := json.Unmarshal([]byte(v), &m)
	return m, err
}

// UpdateMessage changes a stored message (see FindMessage) under the write lock. f reports
// whether it changed anything; only then is the message saved. It returns the message as saved.
func (s *Store) UpdateMessage(chat, id string, f func(*domain.Message) bool) (domain.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.FindMessage(chat, id)
	if err != nil {
		return m, false, err
	}
	if !f(&m) {
		return m, false, nil
	}
	b, _ := json.Marshal(m)
	_, err = s.db.Exec(`UPDATE messages SET data=? WHERE chat_jid=? AND id=?`, string(b), m.ChatJID, m.ID)
	return m, err == nil, err
}

// SaveMessages stores many messages in one transaction. Messages already stored are left alone
// (history syncs resend what Deputy has). It returns the ones it added.
func (s *Store) SaveMessages(ms []domain.Message) ([]domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	var added []domain.Message
	for _, m := range ms {
		b, _ := json.Marshal(m)
		res, err := tx.Exec(`INSERT INTO messages(chat_jid,id,at,data) VALUES(?,?,?,?) ON CONFLICT(chat_jid,id) DO NOTHING`,
			m.ChatJID, m.ID, m.At.UnixMilli(), string(b))
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = append(added, m)
		}
	}
	return added, tx.Commit()
}

// UpdateChat changes a chat under the write lock, creating it when missing.
func (s *Store) UpdateChat(jid string, f func(*domain.Chat)) (domain.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Chat(jid)
	if err != nil {
		c = domain.Chat{JID: jid}
	}
	f(&c)
	b, _ := json.Marshal(c)
	_, err = s.db.Exec(`INSERT INTO chats(jid,data,last_at) VALUES(?,?,?) ON CONFLICT(jid) DO UPDATE SET data=excluded.data, last_at=excluded.last_at`,
		c.JID, string(b), c.LastAt.UnixMilli())
	return c, err
}

// Cursor marks a place in a chat's messages, for paging back in time.
type Cursor struct {
	At time.Time
	ID string
}

// String encodes the cursor for the UI; "" means the newest end.
func (c Cursor) String() string {
	if c.At.IsZero() && c.ID == "" {
		return ""
	}
	return strconv.FormatInt(c.At.UnixMilli(), 10) + ":" + c.ID
}

// ParseCursor reads Cursor.String; "" and garbage give the newest end.
func ParseCursor(s string) Cursor {
	at, id, ok := strings.Cut(s, ":")
	if !ok {
		return Cursor{}
	}
	n, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return Cursor{}
	}
	return Cursor{At: time.UnixMilli(n), ID: id}
}

// MessagesBefore returns up to limit messages of chat older than the cursor, newest first, and
// the cursor for the next page ("" when there are no more).
func (s *Store) MessagesBefore(chat string, before Cursor, limit int) ([]domain.Message, Cursor) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT data FROM messages WHERE chat_jid=?`
	args := []any{chat}
	if !before.At.IsZero() {
		q += ` AND (at < ? OR (at = ? AND id < ?))`
		ms := before.At.UnixMilli()
		args = append(args, ms, ms, before.ID)
	}
	q += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, Cursor{}
	}
	out := scanMessages(rows)
	if len(out) <= limit {
		return out, Cursor{}
	}
	out = out[:limit]
	last := out[len(out)-1]
	return out, Cursor{At: last.At, ID: last.ID}
}

// OldestMessage is the oldest stored message of chat.
func (s *Store) OldestMessage(chat string) (domain.Message, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM messages WHERE chat_jid=? ORDER BY at ASC, id ASC LIMIT 1`, chat).Scan(&v); err != nil {
		return domain.Message{}, ErrNotFound
	}
	var m domain.Message
	err := json.Unmarshal([]byte(v), &m)
	return m, err
}

// UnreadAfter returns the messages of others in chat after t that were not deleted, newest first.
func (s *Store) UnreadAfter(chat string, t time.Time, limit int) []domain.Message {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT data FROM messages WHERE chat_jid=? AND at > ?
		AND COALESCE(json_extract(data,'$.fromMe'),0) = 0 AND COALESCE(json_extract(data,'$.revoked'),0) = 0
		ORDER BY at DESC LIMIT ?`, chat, t.UnixMilli(), limit)
	if err != nil {
		return nil
	}
	return scanMessages(rows)
}

// CountUnreadAfter counts the messages of others in chat after t that were not deleted.
func (s *Store) CountUnreadAfter(chat string, t time.Time) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE chat_jid=? AND at > ?
		AND COALESCE(json_extract(data,'$.fromMe'),0) = 0 AND COALESCE(json_extract(data,'$.revoked'),0) = 0`,
		chat, t.UnixMilli()).Scan(&n)
	return n
}

// TotalUnread adds up the unread counts of chats that are not muted at now.
func (s *Store) TotalUnread(now time.Time) int {
	n := 0
	for _, c := range s.Chats() {
		if c.Unread > 0 && !c.Muted(now) {
			n += c.Unread
		}
	}
	return n
}

// SearchMessages finds messages whose text, transcript or file name contains text (any case),
// newest first, in one chat or in all of them. Deleted messages are left out.
func (s *Store) SearchMessages(text, chat string, limit int) []domain.Message {
	t := strings.TrimSpace(text)
	if t == "" {
		return nil
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(t) + "%"
	q := `SELECT data FROM messages WHERE (json_extract(data,'$.text') LIKE ? ESCAPE '\'
		OR json_extract(data,'$.transcript') LIKE ? ESCAPE '\' OR json_extract(data,'$.fileName') LIKE ? ESCAPE '\')
		AND COALESCE(json_extract(data,'$.revoked'),0) = 0`
	args := []any{like, like, like}
	if chat != "" {
		q += ` AND chat_jid = ?`
		args = append(args, chat)
	}
	q += ` ORDER BY at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil
	}
	return scanMessages(rows)
}
