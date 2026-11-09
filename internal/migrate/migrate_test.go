package migrate

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/positron/internal/update"
	_ "github.com/mattn/go-sqlite3"
)

// oldHome builds a Deputy data folder the way Deputy left it: its database (with a WAL file),
// the WhatsApp session, media and the MCP token.
func oldHome(t *testing.T, root string) string {
	t.Helper()
	old := filepath.Join(root, "Deputy")
	if err := os.MkdirAll(filepath.Join(old, "media", "chat1"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(old, "whatsapp.sqlite"), "session")
	write(t, filepath.Join(old, "mcp-token"), "tok\n")
	write(t, filepath.Join(old, "deputy.log"), "log line\n")
	write(t, filepath.Join(old, "media", "chat1", "m1.jpg"), "jpeg")

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(old, "deputy.sqlite")+"?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(old, "media", "chat1", "m1.jpg")
	for _, q := range []string{
		`CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE messages (chat_jid TEXT NOT NULL, id TEXT NOT NULL, at INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY (chat_jid, id))`,
		`INSERT INTO kv VALUES ('settings', '{"notify":true}')`,
		`INSERT INTO messages VALUES ('chat1', 'm1', 1, '{"mediaPath":"` + media + `"}')`,
		`INSERT INTO messages VALUES ('chat1', 'm2', 2, '{"text":"no media"}')`,
		`INSERT INTO messages VALUES ('chat1', 'm3', 3, '{"mediaPath":"` + filepath.Join(root, "Deputy2", "x.jpg") + `"}')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// Leave the last write in the WAL, as a Deputy that was killed would.
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages VALUES ('chat1', 'm4', 4, '{"mediaPath":"` + media + `"}')`); err != nil {
		t.Fatal(err)
	}
	// Copy the files while the connection is open, so the WAL is still there.
	snap := filepath.Join(root, "snap")
	copyDir(t, old, snap)
	db.Close()
	if err := os.RemoveAll(old); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(snap, old); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(old, "deputy.sqlite-wal")); err != nil {
		t.Fatalf("the fixture has no WAL file: %v", err)
	}
	return old
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if fi.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func plan(root string) Plan {
	return Plan{
		OldHome: filepath.Join(root, "Deputy"),
		NewHome: filepath.Join(root, "Positron"),
		Renames: DefaultRenames,
		DBFile:  "positron.sqlite",
		Running: func() bool { return false },
	}
}

func TestMoveFolder(t *testing.T) {
	root := t.TempDir()
	old := oldHome(t, root)
	agent := filepath.Join(root, "LaunchAgents", "tech.bhlabs.deputy.plist")
	if err := os.MkdirAll(filepath.Dir(agent), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, agent, "<plist/>")

	p := plan(root)
	p.OldLaunchAgent = agent
	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Moved || !res.LoginItem {
		t.Fatalf("result = %+v, want Moved and LoginItem", res)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the old folder is still there: %v", err)
	}
	if _, err := os.Stat(agent); !os.IsNotExist(err) {
		t.Fatalf("the old LaunchAgent is still there: %v", err)
	}
	nw := p.NewHome
	if got := read(t, filepath.Join(nw, "whatsapp.sqlite")); got != "session" {
		t.Fatalf("whatsapp.sqlite = %q", got)
	}
	if got := read(t, filepath.Join(nw, "mcp-token")); got != "tok\n" {
		t.Fatalf("mcp-token = %q", got)
	}
	if got := read(t, filepath.Join(nw, "positron.log")); got != "log line\n" {
		t.Fatalf("positron.log = %q", got)
	}
	if got := read(t, filepath.Join(nw, "media", "chat1", "m1.jpg")); got != "jpeg" {
		t.Fatalf("media = %q", got)
	}
	for _, gone := range []string{"deputy.sqlite", "deputy.sqlite-wal", "deputy.sqlite-shm", "deputy.log"} {
		if _, err := os.Stat(filepath.Join(nw, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", gone)
		}
	}

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(nw, "positron.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := map[string]string{
		"m1": `{"mediaPath":"` + filepath.Join(nw, "media", "chat1", "m1.jpg") + `"}`,
		"m2": `{"text":"no media"}`,
		"m3": `{"mediaPath":"` + filepath.Join(root, "Deputy2", "x.jpg") + `"}`,       // another folder: unchanged
		"m4": `{"mediaPath":"` + filepath.Join(nw, "media", "chat1", "m1.jpg") + `"}`, // was only in the WAL
	}
	rows, err := db.Query(`SELECT id, data FROM messages`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			t.Fatal(err)
		}
		got[id] = data
	}
	if len(got) != len(want) {
		t.Fatalf("messages = %v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %s, want %s", id, got[id], w)
		}
	}
	var settings string
	if err := db.QueryRow(`SELECT value FROM kv WHERE key='settings'`).Scan(&settings); err != nil || settings != `{"notify":true}` {
		t.Fatalf("settings = %q, %v", settings, err)
	}
}

func TestNothingToMove(t *testing.T) {
	root := t.TempDir()
	res, err := plan(root).Run(context.Background())
	if err != nil || res.Moved {
		t.Fatalf("no old folder: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(root, "Positron")); !os.IsNotExist(err) {
		t.Fatal("the new folder was created")
	}
}

func TestNewFolderWins(t *testing.T) {
	root := t.TempDir()
	old := oldHome(t, root)
	p := plan(root)
	if err := os.MkdirAll(p.NewHome, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.NewHome, "whatsapp.sqlite"), "new session")
	res, err := p.Run(context.Background())
	if err != nil || res.Moved {
		t.Fatalf("new folder exists: %+v, %v", res, err)
	}
	if got := read(t, filepath.Join(p.NewHome, "whatsapp.sqlite")); got != "new session" {
		t.Fatalf("the new folder changed: %q", got)
	}
	if got := read(t, filepath.Join(old, "whatsapp.sqlite")); got != "session" {
		t.Fatalf("the old folder changed: %q", got)
	}
}

func TestOldAppRunning(t *testing.T) {
	root := t.TempDir()
	old := oldHome(t, root)
	p := plan(root)
	p.Running = func() bool { return true }
	_, err := p.Run(context.Background())
	if !errors.Is(err, ErrOldRunning) {
		t.Fatalf("err = %v, want ErrOldRunning", err)
	}
	if _, err := os.Stat(filepath.Join(old, "deputy.sqlite")); err != nil {
		t.Fatalf("the old folder changed: %v", err)
	}
	if _, err := os.Stat(p.NewHome); !os.IsNotExist(err) {
		t.Fatal("the new folder was created")
	}
}

// TestInterrupted: a move cut short after the files inside were renamed is finished next time.
func TestInterrupted(t *testing.T) {
	root := t.TempDir()
	old := oldHome(t, root)
	p := plan(root)
	p.beforeMove = func() error { return errors.New("power cut") }
	if _, err := p.Run(context.Background()); err == nil {
		t.Fatal("want the injected error")
	}
	if _, err := os.Stat(p.NewHome); !os.IsNotExist(err) {
		t.Fatal("the new folder exists after an interrupted move")
	}
	p.beforeMove = nil
	res, err := p.Run(context.Background())
	if err != nil || !res.Moved {
		t.Fatalf("second run: %+v, %v", res, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the old folder is still there")
	}
	if got := read(t, filepath.Join(p.NewHome, "positron.log")); got != "log line\n" {
		t.Fatalf("positron.log = %q", got)
	}
}

// fakeSecurity is a stand-in for /usr/bin/security that keeps generic passwords as files in dir.
func fakeSecurity(t *testing.T) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(t.TempDir(), "security")
	script := `#!/bin/sh
store=` + dir + `
key() { printf '%s' "$1|$2" | tr '/ ' '__'; }
parse() { s=; a=; w=; while [ $# -gt 0 ]; do case "$1" in -s) s=$2; shift;; -a) a=$2; shift;; -w) if [ $# -gt 1 ]; then w=$2; shift; fi;; esac; shift; done; }
case "$1" in
find-generic-password) shift; parse "$@"; f="$store/$(key "$s" "$a")"; [ -f "$f" ] || exit 44; cat "$f"; echo ;;
delete-generic-password) shift; parse "$@"; rm -f "$store/$(key "$s" "$a")" ;;
-i) read -r line; eval "set -- $line"; shift; parse "$@"; printf '%s' "$w" > "$store/$(key "$s" "$a")" ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

func keyPlan(bin string) *KeyMove {
	return &KeyMove{
		From: update.Keychain{Service: "Deputy updates", Bin: bin}, FromAccount: "GabiHert/deputy",
		To: update.Keychain{Service: "Positron updates", Bin: bin}, ToAccount: "GabiHert/positron",
	}
}

const token = "github_pat_0123456789abcdefghijABCDEFGHIJ"

func TestMoveToken(t *testing.T) {
	bin, _ := fakeSecurity(t)
	ctx := context.Background()
	k := keyPlan(bin)
	if err := k.From.Set(ctx, k.FromAccount, token); err != nil {
		t.Fatal(err)
	}
	p := plan(t.TempDir())
	p.Keys = k
	res, err := p.Run(ctx)
	if err != nil || !res.MovedToken {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got := k.To.Get(ctx, k.ToAccount); got != token {
		t.Fatalf("new token = %q", got)
	}
	if got := k.From.Get(ctx, k.FromAccount); got != "" {
		t.Fatalf("old token is still there: %q", got)
	}
}

func TestTokenNewWins(t *testing.T) {
	bin, _ := fakeSecurity(t)
	ctx := context.Background()
	k := keyPlan(bin)
	newer := "github_pat_newnewnewnewnewnewnewnew"
	if err := k.From.Set(ctx, k.FromAccount, token); err != nil {
		t.Fatal(err)
	}
	if err := k.To.Set(ctx, k.ToAccount, newer); err != nil {
		t.Fatal(err)
	}
	p := plan(t.TempDir())
	p.Keys = k
	res, err := p.Run(ctx)
	if err != nil || res.MovedToken {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got := k.To.Get(ctx, k.ToAccount); got != newer {
		t.Fatalf("new token = %q", got)
	}
}

func TestNoToken(t *testing.T) {
	bin, dir := fakeSecurity(t)
	p := plan(t.TempDir())
	p.Keys = keyPlan(bin)
	res, err := p.Run(context.Background())
	if err != nil || res.MovedToken {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("keychain items were written: %v", ents)
	}
}

// TestTokenWhileOldRuns: the token still moves when the folder cannot, since it does not depend on it.
func TestTokenWhileOldRuns(t *testing.T) {
	bin, _ := fakeSecurity(t)
	ctx := context.Background()
	k := keyPlan(bin)
	if err := k.From.Set(ctx, k.FromAccount, token); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	oldHome(t, root)
	p := plan(root)
	p.Keys = k
	p.Running = func() bool { return true }
	res, err := p.Run(ctx)
	if !errors.Is(err, ErrOldRunning) || !res.MovedToken {
		t.Fatalf("result = %+v, %v", res, err)
	}
}

func TestDefault(t *testing.T) {
	t.Setenv("HOME", "/Users/someone")
	t.Setenv("POSITRON_HOME", "")
	p, ok := Default()
	if !ok {
		t.Fatal("Default() is off without POSITRON_HOME")
	}
	if p.OldHome != "/Users/someone/Library/Application Support/Deputy" || p.NewHome != "/Users/someone/Library/Application Support/Positron" {
		t.Fatalf("homes = %q, %q", p.OldHome, p.NewHome)
	}
	if p.OldLaunchAgent != "/Users/someone/Library/LaunchAgents/tech.bhlabs.deputy.plist" {
		t.Fatalf("launch agent = %q", p.OldLaunchAgent)
	}
	if p.Keys == nil || p.Keys.From.Service != "Deputy updates" || p.Keys.FromAccount != "GabiHert/deputy" ||
		p.Keys.To.Service != "Positron updates" || p.Keys.ToAccount != "GabiHert/positron" {
		t.Fatalf("keys = %+v", p.Keys)
	}
	t.Setenv("POSITRON_HOME", "/tmp/elsewhere")
	if _, ok := Default(); ok {
		t.Fatal("Default() moves data into a POSITRON_HOME")
	}
}

func TestRenamesCoverDatabaseFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range DefaultRenames {
		seen[r[0]] = strings.HasPrefix(r[1], "positron.")
	}
	for _, f := range []string{"deputy.sqlite", "deputy.sqlite-wal", "deputy.sqlite-shm", "deputy.log"} {
		if !seen[f] {
			t.Errorf("%s is not renamed to positron.*", f)
		}
	}
}
