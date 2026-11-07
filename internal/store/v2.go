package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/GabiHert/deputy/internal/domain"
)

// ---- key/value ----

// KV returns a stored value and whether it exists.
func (s *Store) KV(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM kv WHERE key=?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// SetKV stores a value.
func (s *Store) SetKV(key, value string) error {
	return s.exec(`INSERT INTO kv(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
}

// DeleteKV removes a value.
func (s *Store) DeleteKV(key string) error { return s.exec(`DELETE FROM kv WHERE key=?`, key) }

// ---- message queries for the board engine and Claude chats ----

// MessageQuery filters Messages. Empty fields match everything.
type MessageQuery struct {
	Chat  string
	Since time.Time
	Text  string // matched against the body and voice transcripts, case-insensitive
	Limit int
}

// Messages returns messages, newest first.
func (s *Store) Messages(q MessageQuery) []domain.Message {
	query := `SELECT data FROM messages WHERE at >= ?`
	args := []any{q.Since.UnixMilli()}
	if q.Chat != "" {
		query += ` AND chat_jid = ?`
		args = append(args, q.Chat)
	}
	if t := strings.TrimSpace(q.Text); t != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(t) + "%"
		query += ` AND (json_extract(data,'$.text') LIKE ? ESCAPE '\' OR json_extract(data,'$.transcript') LIKE ? ESCAPE '\')`
		args = append(args, like, like)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	query += ` ORDER BY at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var v string
		var m domain.Message
		if rows.Scan(&v) == nil && jsonUnmarshal(v, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// HourCount is the number of messages in one weekday and hour, local time.
type HourCount struct {
	Day   int `json:"day"` // 0 = Sunday
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

// MessageHours counts messages per weekday and hour since t, optionally in one chat.
func (s *Store) MessageHours(since time.Time, chat string) []HourCount {
	query := `SELECT CAST(strftime('%w', at/1000, 'unixepoch', 'localtime') AS INTEGER),
	                 CAST(strftime('%H', at/1000, 'unixepoch', 'localtime') AS INTEGER), COUNT(*)
	          FROM messages WHERE at >= ?`
	args := []any{since.UnixMilli()}
	if chat != "" {
		query += ` AND chat_jid = ?`
		args = append(args, chat)
	}
	query += ` GROUP BY 1, 2 ORDER BY 1, 2`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []HourCount
	for rows.Next() {
		var h HourCount
		if rows.Scan(&h.Day, &h.Hour, &h.Count) == nil {
			out = append(out, h)
		}
	}
	return out
}

// ---- Claude chats (stored as JSON documents owned by the assistant package) ----

// SaveAssistantChat inserts or replaces a chat document.
func (s *Store) SaveAssistantChat(id string, updated time.Time, data []byte) error {
	return s.exec(`INSERT INTO assistant_chats(id,updated_at,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at, data=excluded.data`,
		id, updated.UnixMilli(), string(data))
}

// AssistantChats returns every chat document, most recently updated first.
func (s *Store) AssistantChats() [][]byte {
	return s.docs(`SELECT data FROM assistant_chats ORDER BY updated_at DESC`)
}

// AssistantChat returns one chat document.
func (s *Store) AssistantChat(id string) ([]byte, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM assistant_chats WHERE id=?`, id).Scan(&v); err != nil {
		return nil, ErrNotFound
	}
	return []byte(v), nil
}

// DeleteAssistantChat removes a chat and its messages.
func (s *Store) DeleteAssistantChat(id string) error {
	if err := s.exec(`DELETE FROM assistant_messages WHERE chat_id=?`, id); err != nil {
		return err
	}
	return s.exec(`DELETE FROM assistant_chats WHERE id=?`, id)
}

// SaveAssistantMessage inserts or replaces a message document, keeping its place in the chat.
func (s *Store) SaveAssistantMessage(id, chatID string, data []byte) error {
	return s.exec(`INSERT INTO assistant_messages(id,chat_id,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`,
		id, chatID, string(data))
}

// AssistantMessages returns a chat's message documents in order.
func (s *Store) AssistantMessages(chatID string) [][]byte {
	return s.docs(`SELECT data FROM assistant_messages WHERE chat_id=? ORDER BY seq`, chatID)
}

// AssistantMessage returns one message document.
func (s *Store) AssistantMessage(id string) ([]byte, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM assistant_messages WHERE id=?`, id).Scan(&v); err != nil {
		return nil, ErrNotFound
	}
	return []byte(v), nil
}

func (s *Store) docs(q string, args ...any) [][]byte {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil {
			out = append(out, []byte(v))
		}
	}
	return out
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

// LastFromMe is when the owner (or one of their agents) last wrote in a chat after since; zero if never.
func (s *Store) LastFromMe(chat string, since time.Time) time.Time {
	var at sql.NullInt64
	_ = s.db.QueryRow(`SELECT MAX(at) FROM messages WHERE chat_jid = ? AND at > ? AND json_extract(data,'$.fromMe') = 1`, chat, since.UnixMilli()).Scan(&at)
	if !at.Valid {
		return time.Time{}
	}
	return time.UnixMilli(at.Int64)
}
