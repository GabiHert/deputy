// Package paths says where Positron keeps its files.
package paths

import (
	"os"
	"path/filepath"
)

// Home is $POSITRON_HOME, else ~/Library/Application Support/Positron.
func Home() string {
	if v := os.Getenv("POSITRON_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".positron"
	}
	return filepath.Join(home, "Library", "Application Support", "Positron")
}

// DBPath is Positron's own database: agents, chats, messages and runs.
func DBPath() string { return filepath.Join(Home(), "positron.sqlite") }

// WhatsAppDBPath is the WhatsApp session store.
func WhatsAppDBPath() string { return filepath.Join(Home(), "whatsapp.sqlite") }

// MediaDir holds downloaded photos and voice notes, one folder per chat.
func MediaDir() string { return filepath.Join(Home(), "media") }

// ModelsDir holds the speech models.
func ModelsDir() string { return filepath.Join(Home(), "models") }

// LockPath is the single-instance lock.
func LockPath() string { return filepath.Join(Home(), "positron.lock") }

// LogPath is the app log.
func LogPath() string { return filepath.Join(Home(), "positron.log") }

// UpdateDir is where Update Positron downloads, builds and stages the new version.
func UpdateDir() string { return filepath.Join(Home(), "update") }

// UpdateLogPath is the install script's log from the last update.
func UpdateLogPath() string { return filepath.Join(Home(), "update.log") }
