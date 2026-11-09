//go:build darwin

package mediacapture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework WebKit
void PositronAllowMediaCapture(void);
*/
import "C"

func allow() { C.PositronAllowMediaCapture() }
