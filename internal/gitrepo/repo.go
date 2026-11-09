// Package gitrepo sets up and commits an agent's knowledge repo.
package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is one knowledge repo on disk.
type Repo struct {
	Dir   string
	EnvFn func() []string // environment for git; nil uses the app's own
}

func (r Repo) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	if r.EnvFn != nil {
		cmd.Env = r.EnvFn()
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	if r.EnvFn == nil {
		cmd.Env = append(os.Environ(), cmd.Env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

const claudeMD = `# Positron agent

You are a WhatsApp agent run by Positron on behalf of the person who owns this repo.
Your instructions are in INSTRUCTIONS.md. Follow them.

@INSTRUCTIONS.md

## Your memory

This repo is your memory. Each run starts fresh, so anything you don't write down is forgotten.

- Read knowledge/ before you answer, starting with knowledge/people.md.
- When you learn something that will matter later (a fact, a decision, who someone is, a question
  that keeps coming back), write it to a file under knowledge/, one topic per file. Update a file
  rather than adding a near copy.
- Keep files short and factual. Note where a fact came from and when.
- You may refine INSTRUCTIONS.md when the person corrects you in a chat.
- Don't run git. Positron commits your changes after each run.
- .positron/ holds chat media for this run. It is not committed. Copy an image out of it only if you
  need to keep it.
`

const defaultInstructions = `Describe who this agent is and how it should behave in its chats.

For example: who it speaks for, what it should answer, what it should leave alone, its tone and
language, and anything it must never say.
`

// Init makes dir a knowledge repo if it is not one yet: git init, CLAUDE.md, INSTRUCTIONS.md, knowledge/.
func (r Repo) Init(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Join(r.Dir, "knowledge"), 0o755); err != nil {
		return fmt.Errorf("could not create the repo folder: %w", err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil {
		if _, err := r.git(ctx, "init", "-q", "-b", "main"); err != nil {
			return err
		}
	}
	write := func(name, content string) error {
		p := filepath.Join(r.Dir, name)
		if _, err := os.Stat(p); err == nil {
			return nil
		}
		return os.WriteFile(p, []byte(content), 0o644)
	}
	for name, content := range map[string]string{
		"CLAUDE.md":           claudeMD,
		"INSTRUCTIONS.md":     defaultInstructions,
		"knowledge/people.md": "# People\n\nWho is who in the chats.\n",
	} {
		if err := write(name, content); err != nil {
			return err
		}
	}
	if err := r.ensureIgnore(); err != nil {
		return err
	}
	_, _, _ = r.Commit(ctx, "Set up Positron agent repo")
	return nil
}

func (r Repo) ensureIgnore() error {
	p := filepath.Join(r.Dir, ".gitignore")
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), ".positron/") {
		return nil
	}
	s := string(b)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return os.WriteFile(p, []byte(s+".positron/\n.DS_Store\n"), 0o644)
}

// Instructions reads INSTRUCTIONS.md.
func (r Repo) Instructions() string {
	b, _ := os.ReadFile(filepath.Join(r.Dir, "INSTRUCTIONS.md"))
	return string(b)
}

// SetInstructions writes INSTRUCTIONS.md.
func (r Repo) SetInstructions(s string) error {
	return os.WriteFile(filepath.Join(r.Dir, "INSTRUCTIONS.md"), []byte(s), 0o644)
}

// Commit stages everything and commits it. It returns the short hash and the changed files,
// or "" when there was nothing to commit.
func (r Repo) Commit(ctx context.Context, msg string) (string, []string, error) {
	status, err := r.git(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil || status == "" {
		return "", nil, err
	}
	var files []string
	for _, line := range strings.Split(status, "\n") {
		// The output is trimmed, so the first line may have lost its leading status space:
		// split on the first space after the status letters instead of a fixed column.
		line = strings.TrimSpace(line)
		if sp := strings.IndexByte(line, ' '); sp > 0 {
			f := strings.TrimSpace(line[sp:])
			if i := strings.Index(f, " -> "); i >= 0 {
				f = f[i+4:]
			}
			files = append(files, strings.Trim(f, `"`))
		}
	}
	if _, err := r.git(ctx, "add", "-A"); err != nil {
		return "", nil, err
	}
	args := []string{"-c", "user.name=Positron", "-c", "user.email=positron@localhost", "commit", "-q", "-m", msg}
	if _, err := r.git(ctx, args...); err != nil {
		return "", nil, err
	}
	sha, err := r.git(ctx, "rev-parse", "--short", "HEAD")
	return sha, files, err
}

// Push pushes the current branch to its upstream, or to origin.
func (r Repo) Push(ctx context.Context) error {
	if r.Remote(ctx) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	_, err := r.git(ctx, "push", "-q", "-u", "origin", "HEAD")
	return err
}

// Remote returns origin's address without the scheme, like "github.com/GabiHert/kb", or "".
func (r Repo) Remote(ctx context.Context) string {
	u, err := r.git(ctx, "remote", "get-url", "origin")
	if err != nil || u == "" {
		return ""
	}
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if strings.HasPrefix(u, "git@") {
		u = strings.Replace(strings.TrimPrefix(u, "git@"), ":", "/", 1)
	}
	return u
}

// Stats counts knowledge files and commits, and finds the last commit time.
type Stats struct {
	Exists      bool
	Files       int
	Commits     int
	LastLearned time.Time
	Remote      string
}

// Stats reads the repo's numbers.
func (r Repo) Stats(ctx context.Context) Stats {
	var s Stats
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil {
		return s
	}
	s.Exists = true
	_ = filepath.WalkDir(filepath.Join(r.Dir, "knowledge"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			s.Files++
		}
		return nil
	})
	if n, err := r.git(ctx, "rev-list", "--count", "HEAD"); err == nil {
		s.Commits, _ = strconv.Atoi(n)
	}
	if t, err := r.git(ctx, "log", "-1", "--format=%ct"); err == nil {
		if sec, err := strconv.ParseInt(t, 10, 64); err == nil {
			s.LastLearned = time.Unix(sec, 0)
		}
	}
	s.Remote = r.Remote(ctx)
	return s
}

// Change is one file changed by one commit.
type Change struct {
	Commit  string    `json:"commit"`
	At      time.Time `json:"at"`
	Message string    `json:"message"`
	File    string    `json:"file"`
	Kind    string    `json:"kind"` // "added" | "modified" | "deleted" | "renamed"
}

// Log lists files changed under knowledge/ and INSTRUCTIONS.md since t, newest first, at most limit.
func (r Repo) Log(ctx context.Context, since time.Time, limit int) []Change {
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil {
		return nil
	}
	out, err := r.git(ctx, "log", fmt.Sprintf("--since=%d", since.Unix()), "--name-status", "--no-renames",
		"--format=%x1e%h%x1f%ct%x1f%s", "--", "knowledge", "INSTRUCTIONS.md")
	if err != nil {
		return nil
	}
	kinds := map[string]string{"A": "added", "M": "modified", "D": "deleted"}
	var changes []Change
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		head := strings.SplitN(lines[0], "\x1f", 3)
		if len(head) < 3 {
			continue
		}
		sec, _ := strconv.ParseInt(head[1], 10, 64)
		for _, l := range lines[1:] {
			parts := strings.SplitN(strings.TrimSpace(l), "\t", 2)
			if len(parts) != 2 {
				continue
			}
			k := kinds[parts[0][:1]]
			if k == "" {
				k = "modified"
			}
			changes = append(changes, Change{Commit: head[0], At: time.Unix(sec, 0), Message: head[2], File: parts[1], Kind: k})
			if limit > 0 && len(changes) >= limit {
				return changes
			}
		}
	}
	return changes
}

// WriteFile writes a file inside the repo, refusing paths that leave it or touch .git.
func (r Repo) WriteFile(rel, content string) error {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") ||
		clean == ".git" || strings.HasPrefix(clean, ".git"+string(filepath.Separator)) {
		return fmt.Errorf("%q is not a path inside the repo", rel)
	}
	p := filepath.Join(r.Dir, clean)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

// ReadFile reads a file inside the repo.
func (r Repo) ReadFile(rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("%q is not a path inside the repo", rel)
	}
	b, err := os.ReadFile(filepath.Join(r.Dir, clean))
	return string(b), err
}

// KnowledgeFiles lists files under knowledge/, relative to the repo.
func (r Repo) KnowledgeFiles() []string {
	var out []string
	root := filepath.Join(r.Dir, "knowledge")
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if rel, e := filepath.Rel(r.Dir, p); e == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return out
}
