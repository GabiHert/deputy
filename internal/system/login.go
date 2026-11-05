// Package system holds small macOS integrations.
package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const label = "tech.bhlabs.deputy"

// LoginItem opens Deputy at login through a LaunchAgent.
type LoginItem struct{}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// appPath is the .app bundle holding the running binary, or "" when not in a bundle.
func appPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+4]
	}
	return ""
}

// Set turns opening at login on or off.
func (LoginItem) Set(on bool) error {
	p := plistPath()
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	app := appPath()
	if app == "" {
		return fmt.Errorf("Deputy must run from its app bundle (make install) to open at login")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>/usr/bin/open</string><string>-g</string><string>-a</string><string>%s</string></array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`, label, app)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(plist), 0o644)
}
