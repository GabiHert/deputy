package menubar

import "sync/atomic"

// active is the item the native menu reports to.
var active atomic.Pointer[Item]

func dispatchActive(action int) {
	if i := active.Load(); i != nil {
		i.dispatch(action)
	}
}
