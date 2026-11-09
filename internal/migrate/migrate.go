// Package migrate moves what Deputy, Positron's old name, kept on this Mac to Positron's places,
// once: the data folder (WhatsApp session, agents, chats, media, MCP token), the GitHub token in
// the keychain and the open-at-login LaunchAgent.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GabiHert/positron/internal/paths"
	"github.com/GabiHert/positron/internal/update"
	_ "github.com/mattn/go-sqlite3"
)

// ErrOldRunning means Deputy is open, so its folder cannot move yet.
var ErrOldRunning = errors.New("Deputy is still running. Quit Deputy (its menu bar icon → Quit), then open Positron again; Positron then brings over your WhatsApp link, agents and chats.")

// DefaultRenames are the files in the data folder that carried Deputy's name.
var DefaultRenames = [][2]string{
	{"deputy.sqlite", "positron.sqlite"},
	{"deputy.sqlite-wal", "positron.sqlite-wal"},
	{"deputy.sqlite-shm", "positron.sqlite-shm"},
	{"deputy.log", "positron.log"},
	{"deputy.lock", "positron.lock"},
}

// Plan says what to move.
type Plan struct {
	OldHome, NewHome string
	Renames          [][2]string // files inside the folder: {old name, new name}
	DBFile           string      // the database, after Renames, whose stored paths point into OldHome
	OldLaunchAgent   string      // Deputy's open-at-login LaunchAgent; "" leaves it
	Running          func() bool // whether Deputy is open
	Keys             *KeyMove    // nil moves no token

	beforeMove func() error // tests interrupt the move here
}

// KeyMove moves the GitHub token for updates from Deputy's keychain item to Positron's.
type KeyMove struct {
	From, To               update.Keychain
	FromAccount, ToAccount string
}

// Result says what moved.
type Result struct {
	Moved      bool // the data folder moved
	LoginItem  bool // Deputy opened at login; Positron should now
	MovedToken bool
}

// Default is the plan for this Mac. It is off (false) with POSITRON_HOME set: a separate
// Positron, like a test one, starts empty.
func Default() (Plan, bool) {
	if os.Getenv("POSITRON_HOME") != "" {
		return Plan{}, false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Plan{}, false
	}
	support := filepath.Join(home, "Library", "Application Support")
	return Plan{
		OldHome:        filepath.Join(support, "Deputy"),
		NewHome:        paths.Home(),
		Renames:        DefaultRenames,
		DBFile:         filepath.Base(paths.DBPath()),
		OldLaunchAgent: filepath.Join(home, "Library", "LaunchAgents", "tech.bhlabs.deputy.plist"),
		Running:        deputyRunning,
		Keys: &KeyMove{
			From: update.Keychain{Service: "Deputy updates"}, FromAccount: "GabiHert/deputy",
			To: update.Keychain{Service: update.KeychainService}, ToAccount: update.DefaultRepo,
		},
	}, true
}

// deputyRunning reports whether a process named Deputy runs for this user.
func deputyRunning() bool {
	return exec.Command("/usr/bin/pgrep", "-x", "-U", fmt.Sprint(os.Getuid()), "Deputy").Run() == nil
}

// Run moves what has not moved yet. The folder moves only when the new one does not exist, and
// never while Deputy runs (ErrOldRunning). The token moves whenever Deputy's is there and
// Positron has none.
func (p Plan) Run(ctx context.Context) (Result, error) {
	var res Result
	if p.Keys != nil {
		res.MovedToken = p.Keys.run(ctx)
	}
	if _, err := os.Lstat(p.NewHome); err == nil || !os.IsNotExist(err) {
		return res, nil
	}
	if fi, err := os.Stat(p.OldHome); err != nil || !fi.IsDir() {
		return res, nil
	}
	if p.Running != nil && p.Running() {
		return res, ErrOldRunning
	}
	// Everything is done inside the old folder first, so the final rename is the one step that
	// makes the move happen: cut short before it, the next launch starts over.
	for _, r := range p.Renames {
		from, to := filepath.Join(p.OldHome, r[0]), filepath.Join(p.OldHome, r[1])
		if _, err := os.Lstat(from); err != nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return res, fmt.Errorf("could not rename %s: %w", r[0], err)
		}
	}
	if p.DBFile != "" {
		if err := rewritePaths(filepath.Join(p.OldHome, p.DBFile), p.OldHome, p.NewHome); err != nil {
			return res, err
		}
	}
	if p.beforeMove != nil {
		if err := p.beforeMove(); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.NewHome), 0o755); err != nil {
		return res, err
	}
	if err := os.Rename(p.OldHome, p.NewHome); err != nil {
		return res, fmt.Errorf("could not move %s to %s: %w", p.OldHome, p.NewHome, err)
	}
	res.Moved = true
	if p.OldLaunchAgent != "" {
		if err := os.Remove(p.OldLaunchAgent); err == nil {
			res.LoginItem = true
		}
	}
	return res, nil
}

// rewritePaths points every stored path into oldHome at newHome. Deputy stored absolute paths,
// such as a message's media file, in JSON text columns.
func rewritePaths(db, oldHome, newHome string) error {
	if _, err := os.Stat(db); err != nil {
		return nil
	}
	conn, err := sql.Open("sqlite3", "file:"+db+"?_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	tables, err := textColumns(conn)
	if err != nil {
		return fmt.Errorf("could not read the database: %w", err)
	}
	from, to := oldHome+string(filepath.Separator), newHome+string(filepath.Separator)
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for table, cols := range tables {
		for _, c := range cols {
			q := fmt.Sprintf(`UPDATE %s SET %s = replace(%s, ?, ?) WHERE instr(%s, ?) > 0`, quote(table), quote(c), quote(c), quote(c))
			if _, err := tx.Exec(q, from, to, from); err != nil {
				return fmt.Errorf("could not update paths in %s: %w", table, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Fold the WAL into the database before the folder moves.
	_, _ = conn.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return nil
}

// textColumns lists each table's TEXT columns.
func textColumns(conn *sql.DB) (map[string][]string, error) {
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	out := map[string][]string{}
	for _, n := range names {
		cols, err := conn.Query(`SELECT name, type FROM pragma_table_info(?)`, n)
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var c, typ string
			if err := cols.Scan(&c, &typ); err != nil {
				cols.Close()
				return nil, err
			}
			if strings.Contains(strings.ToUpper(typ), "TEXT") {
				out[n] = append(out[n], c)
			}
		}
		cols.Close()
	}
	return out, nil
}

func quote(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }

// run moves the token; it reports whether one moved. A token Positron already has wins.
func (k *KeyMove) run(ctx context.Context) bool {
	tok := k.From.Get(ctx, k.FromAccount)
	if tok == "" || k.To.Get(ctx, k.ToAccount) != "" {
		return false
	}
	if err := k.To.Set(ctx, k.ToAccount, tok); err != nil {
		return false
	}
	_ = k.From.Delete(ctx, k.FromAccount)
	return true
}
