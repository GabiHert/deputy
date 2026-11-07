//go:build darwin

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "menubar_darwin.h"
*/
import "C"

func install()         { C.DeputyMenuBarInstall() }
func setWaiting(n int) { C.DeputyMenuBarSetWaiting(C.int(n)) }
func setUnread(n int)  { C.DeputyMenuBarSetUnread(C.int(n)) }
func focused() bool    { return C.DeputyAppFocused() != 0 }

//export deputyMenuBarAction
func deputyMenuBarAction(action C.int) { dispatchActive(int(action)) }
