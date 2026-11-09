//go:build darwin

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "menubar_darwin.h"
*/
import "C"

func install()         { C.PositronMenuBarInstall() }
func setWaiting(n int) { C.PositronMenuBarSetWaiting(C.int(n)) }
func setUnread(n int)  { C.PositronMenuBarSetUnread(C.int(n)) }
func focused() bool    { return C.PositronAppFocused() != 0 }

//export positronMenuBarAction
func positronMenuBarAction(action C.int) { dispatchActive(int(action)) }
