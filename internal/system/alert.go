package system

import "os/exec"

// Alert shows msg in a dialog and waits for OK. It is for errors before the window exists.
func Alert(msg string) {
	_ = exec.Command("/usr/bin/osascript",
		"-e", "on run argv", "-e", `display alert "Positron" message (item 1 of argv) as critical`, "-e", "end run",
		msg).Run()
}
