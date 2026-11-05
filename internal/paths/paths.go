// Package paths says where Deputy keeps its files.
package paths

import (
	"os"
	"path/filepath"
)

// Home is $DEPUTY_HOME, else ~/Library/Application Support/Deputy.
func Home() string {
	if v := os.Getenv("DEPUTY_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".deputy"
	}
	return filepath.Join(home, "Library", "Application Support", "Deputy")
}

// DBPath is Deputy's own database: agents, chats, messages and runs.
func DBPath() string { return filepath.Join(Home(), "deputy.sqlite") }

// WhatsAppDBPath is the WhatsApp session store.
func WhatsAppDBPath() string { return filepath.Join(Home(), "whatsapp.sqlite") }

// MediaDir holds downloaded photos and voice notes, one folder per chat.
func MediaDir() string { return filepath.Join(Home(), "media") }

// ModelsDir holds the speech models.
func ModelsDir() string { return filepath.Join(Home(), "models") }

// LockPath is the single-instance lock.
func LockPath() string { return filepath.Join(Home(), "deputy.lock") }

// LogPath is the app log.
func LogPath() string { return filepath.Join(Home(), "deputy.log") }
