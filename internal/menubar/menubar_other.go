//go:build !darwin

package menubar

func install()         {}
func setWaiting(_ int) {}
func setUnread(_ int)  {}
func focused() bool    { return false }
