// Package store keeps Deputy's agents, chats, messages and runs in SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/domain"
	_ "github.com/mattn/go-sqlite3"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Store is the SQLite store. It is safe for concurrent use.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serializes writes; SQLite allows one writer
}

const schema = `
CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS agents (id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS chats (jid TEXT PRIMARY KEY, data TEXT NOT NULL, last_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS messages (chat_jid TEXT NOT NULL, id TEXT NOT NULL, at INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY (chat_jid, id));
CREATE INDEX IF NOT EXISTS messages_chat_at ON messages (chat_jid, at);
CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, at INTEGER NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS runs_at ON runs (at);
CREATE TABLE IF NOT EXISTS assistant_chats (id TEXT PRIMARY KEY, updated_at INTEGER NOT NULL, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS assistant_messages (seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, chat_id TEXT NOT NULL, data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS assistant_messages_chat ON assistant_messages (chat_id, seq);
`

// Open opens (and creates) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not set up the database: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not update the database: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) exec(q string, args ...any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(q, args...)
	return err
}

// ---- settings ----

// Settings returns the saved settings, or the defaults.
func (s *Store) Settings() domain.Settings {
	out := domain.DefaultSettings()
	var v string
	if err := s.db.QueryRow(`SELECT value FROM kv WHERE key='settings'`).Scan(&v); err == nil {
		_ = json.Unmarshal([]byte(v), &out)
	}
	return out
}

// SaveSettings saves the settings.
func (s *Store) SaveSettings(v domain.Settings) error {
	b, _ := json.Marshal(v)
	return s.exec(`INSERT INTO kv(key,value) VALUES('settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(b))
}

// ---- agents ----

// Agents returns every agent, oldest first.
func (s *Store) Agents() []domain.Agent {
	rows, err := s.db.Query(`SELECT data FROM agents`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Agent
	for rows.Next() {
		var v string
		var a domain.Agent
		if rows.Scan(&v) == nil && json.Unmarshal([]byte(v), &a) == nil {
			if a.Chats == nil {
				a.Chats = []string{}
			}
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Agent returns one agent.
func (s *Store) Agent(id string) (domain.Agent, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM agents WHERE id=?`, id).Scan(&v); err != nil {
		return domain.Agent{}, ErrNotFound
	}
	var a domain.Agent
	err := json.Unmarshal([]byte(v), &a)
	if a.Chats == nil {
		a.Chats = []string{}
	}
	return a, err
}

// SaveAgent inserts or replaces an agent.
func (s *Store) SaveAgent(a domain.Agent) error {
	b, _ := json.Marshal(a)
	return s.exec(`INSERT INTO agents(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, a.ID, string(b))
}

// DeleteAgent removes an agent; its runs stay in the history.
func (s *Store) DeleteAgent(id string) error { return s.exec(`DELETE FROM agents WHERE id=?`, id) }

// ---- chats ----

// Chats returns every known chat, most recent first.
func (s *Store) Chats() []domain.Chat {
	rows, err := s.db.Query(`SELECT data FROM chats ORDER BY last_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Chat
	for rows.Next() {
		var v string
		var c domain.Chat
		if rows.Scan(&v) == nil && json.Unmarshal([]byte(v), &c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// Chat returns one chat.
func (s *Store) Chat(jid string) (domain.Chat, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM chats WHERE jid=?`, jid).Scan(&v); err != nil {
		return domain.Chat{JID: jid}, ErrNotFound
	}
	var c domain.Chat
	err := json.Unmarshal([]byte(v), &c)
	return c, err
}

// SaveChat inserts or replaces a chat.
func (s *Store) SaveChat(c domain.Chat) error {
	b, _ := json.Marshal(c)
	return s.exec(`INSERT INTO chats(jid,data,last_at) VALUES(?,?,?) ON CONFLICT(jid) DO UPDATE SET data=excluded.data, last_at=excluded.last_at`,
		c.JID, string(b), c.LastAt.UnixMilli())
}

// ---- messages ----

// SaveMessage inserts or replaces a message.
func (s *Store) SaveMessage(m domain.Message) error {
	b, _ := json.Marshal(m)
	return s.exec(`INSERT INTO messages(chat_jid,id,at,data) VALUES(?,?,?,?) ON CONFLICT(chat_jid,id) DO UPDATE SET data=excluded.data`,
		m.ChatJID, m.ID, m.At.UnixMilli(), string(b))
}

// Message returns one message.
func (s *Store) Message(chat, id string) (domain.Message, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM messages WHERE chat_jid=? AND id=?`, chat, id).Scan(&v); err != nil {
		return domain.Message{}, ErrNotFound
	}
	var m domain.Message
	err := json.Unmarshal([]byte(v), &m)
	return m, err
}

// RecentMessages returns the last n messages of a chat, oldest first.
func (s *Store) RecentMessages(chat string, n int) []domain.Message {
	rows, err := s.db.Query(`SELECT data FROM messages WHERE chat_jid=? ORDER BY at DESC LIMIT ?`, chat, n)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var v string
		var m domain.Message
		if rows.Scan(&v) == nil && json.Unmarshal([]byte(v), &m) == nil {
			out = append(out, m)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ---- runs ----

// SaveRun inserts or replaces a run.
func (s *Store) SaveRun(r domain.Run) error {
	b, _ := json.Marshal(r)
	return s.exec(`INSERT INTO runs(id,at,agent_id,status,data) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status, data=excluded.data`,
		r.ID, r.At.UnixMilli(), r.AgentID, string(r.Status), string(b))
}

// Run returns one run.
func (s *Store) Run(id string) (domain.Run, error) {
	var v string
	if err := s.db.QueryRow(`SELECT data FROM runs WHERE id=?`, id).Scan(&v); err != nil {
		return domain.Run{}, ErrNotFound
	}
	var r domain.Run
	err := json.Unmarshal([]byte(v), &r)
	return r, err
}

// RunQuery filters Runs. Empty fields match everything.
type RunQuery struct {
	AgentID string
	Status  domain.RunStatus
	Since   time.Time
	Limit   int
}

// Runs returns runs, newest first.
func (s *Store) Runs(q RunQuery) []domain.Run {
	query := `SELECT data FROM runs WHERE at >= ?`
	args := []any{q.Since.UnixMilli()}
	if q.AgentID != "" {
		query += ` AND agent_id = ?`
		args = append(args, q.AgentID)
	}
	if q.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(q.Status))
	}
	query += ` ORDER BY at DESC`
	if q.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, q.Limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Run
	for rows.Next() {
		var v string
		var r domain.Run
		if rows.Scan(&v) == nil && json.Unmarshal([]byte(v), &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// CountRuns counts runs with status since t.
func (s *Store) CountRuns(status domain.RunStatus, since time.Time) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM runs WHERE status=? AND at>=?`, string(status), since.UnixMilli()).Scan(&n)
	return n
}

// FailRunning marks runs left "running" by a crash as failed.
func (s *Store) FailRunning() {
	for _, r := range s.Runs(RunQuery{Status: domain.RunRunning}) {
		r.Status = domain.RunFailed
		r.Error = "Deputy quit while this run was going."
		_ = s.SaveRun(r)
	}
}
