package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitAndCommitListsFiles(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Dir: dir}
	ctx := context.Background()
	if err := r.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"CLAUDE.md", "INSTRUCTIONS.md", "knowledge/people.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "knowledge", "people.md"), []byte("# People\n\nAna has a cat.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "knowledge", "cats.md"), []byte("Miso\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".positron", "media"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".positron", "media", "x.jpg"), []byte("x"), 0o644)
	sha, files, err := r.Commit(ctx, "test")
	if err != nil || sha == "" {
		t.Fatalf("commit: %v %q", err, sha)
	}
	want := map[string]bool{"knowledge/people.md": true, "knowledge/cats.md": true}
	if len(files) != 2 || !want[files[0]] || !want[files[1]] {
		t.Fatalf("files %v", files)
	}
	if sha, _, _ := r.Commit(ctx, "nothing"); sha != "" {
		t.Fatal("empty commit")
	}
	if s := r.Stats(ctx); s.Commits != 2 || s.Files != 2 {
		t.Fatalf("stats %+v", s)
	}
}

func TestLogAndWriteFile(t *testing.T) {
	r := Repo{Dir: t.TempDir()}
	ctx := context.Background()
	if err := r.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile("knowledge/dates.md", "# Dates\n"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../x.md", "/etc/x", ".git/config", "."} {
		if err := r.WriteFile(bad, "x"); err == nil {
			t.Fatalf("%s should be refused", bad)
		}
	}
	if _, _, err := r.Commit(ctx, "add dates"); err != nil {
		t.Fatal(err)
	}
	log := r.Log(ctx, time.Now().Add(-time.Hour), 0)
	if len(log) < 2 || log[0].File != "knowledge/dates.md" || log[0].Kind != "added" || log[0].Message != "add dates" {
		t.Fatalf("log %+v", log)
	}
	if got := r.KnowledgeFiles(); len(got) != 2 {
		t.Fatalf("files %v", got)
	}
}
