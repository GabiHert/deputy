//go:build darwin

package mediacapture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework WebKit
void DeputyAllowMediaCapture(void);
*/
import "C"

func allow() { C.DeputyAllowMediaCapture() }
