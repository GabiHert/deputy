// Adapted from Notted (internal/integration/install/install.go); see NOTICE.md.

package update

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

//go:embed install.sh
var installScript []byte

// startInstaller writes install.sh to a new temp folder and starts it in its own session, so it
// outlives Positron; it does not wait. extraEnv carries the POSITRON_* overrides tests set.
func startInstaller(pid int, newApp, installedApp, logPath string, extraEnv []string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("cannot write %s: %w", logPath, err)
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", logPath, err)
	}
	defer logf.Close()

	dir, err := os.MkdirTemp("", "positron-install-")
	if err != nil {
		return err
	}
	scriptPath := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(scriptPath, installScript, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}

	home, _ := os.UserHomeDir()
	cmd := exec.Command("/bin/bash", scriptPath, strconv.Itoa(pid), newApp, installedApp)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home}, extraEnv...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
