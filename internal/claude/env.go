// Copied from github.com/brienze1/quant (internal/application/service/job_manager.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Positron; see NOTICE.md.

package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const envMarker = "__POSITRON_ENV_BEGIN__"

var (
	shellTimeout = 10 * time.Second
	// fallbackDirs are checked after $HOME/.local/bin and $HOME/.claude/local.
	fallbackDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	runShell     = execShell
)

func execShell(ctx context.Context, shell string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Stdin = nil
	// A new session has no controlling tty, so an interactive shell never stops on
	// terminal access when the app runs from a terminal. Its leader also leads a new
	// process group: a shell rc may start background children that keep stdout open, so
	// kill the whole group on timeout and stop waiting for the pipe shortly after.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// LoginShellEnv returns the environment of the user's interactive login shell. An app
// started from Finder has a minimal PATH; the shell's PATH is where claude lives, and
// the native installer adds its folder in ~/.zshrc, which only an interactive shell reads.
func LoginShellEnv() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	home, _ := os.UserHomeDir()
	return loginShellEnvWith(runShell, home, shell)
}

func loginShellEnvWith(run func(context.Context, string, ...string) ([]byte, error), home, shell string) []string {
	script := "printf '%s\\n' " + envMarker + "; env -0"
	env := shellEnv(run, shell, "-l", "-i", "-c", script)
	if len(env) == 0 {
		env = shellEnv(run, shell, "-l", "-c", script)
	}
	if len(env) == 0 {
		env = os.Environ()
	}
	return withClaudeOnPath(singlePath(env), home)
}

func shellEnv(run func(context.Context, string, ...string) ([]byte, error), shell string, args ...string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	output, err := run(ctx, shell, args...)
	if err != nil {
		return nil
	}
	_, after, found := strings.Cut(string(output), envMarker+"\n")
	if !found {
		return nil
	}
	var env []string
	for _, line := range strings.Split(after, "\x00") {
		if strings.Contains(line, "=") {
			env = append(env, line)
		}
	}
	return env
}

// singlePath keeps only the last PATH entry, the one LookPath reads.
func singlePath(env []string) []string {
	last := -1
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			last = i
		}
	}
	out := make([]string, 0, len(env))
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") && i != last {
			continue
		}
		out = append(out, e)
	}
	return out
}

func withClaudeOnPath(env []string, home string) []string {
	if _, err := LookPath(env, "claude"); err == nil {
		return env
	}
	var dirs []string
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"))
	}
	dirs = append(dirs, fallbackDirs...)
	for _, dir := range dirs {
		if _, err := LookPath([]string{"PATH=" + dir}, "claude"); err != nil {
			continue
		}
		out := make([]string, 0, len(env)+1)
		replaced := false
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, "PATH="); ok && !replaced {
				e = "PATH=" + dir + string(os.PathListSeparator) + v
				replaced = true
			}
			out = append(out, e)
		}
		if !replaced {
			out = append(out, "PATH="+dir)
		}
		return out
	}
	return env
}

// CleanEnv drops the API credentials so the run uses the person's subscription login.
func CleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") || strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// LookPath searches the PATH entry of env (not the app's own) for an executable file.
func LookPath(env []string, name string) (string, error) {
	var path string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			path = v
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", errors.New(name + " not found in PATH")
}
