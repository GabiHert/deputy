// Package menubar puts Deputy's icon and menu in the macOS menu bar.
package menubar

import "sync"

// Handlers are called, each on its own goroutine, when the person picks a menu item.
type Handlers struct {
	Activity func()
	Open     func()
	Quit     func()
}

// Menu actions, shared with the Objective-C side.
const (
	actionActivity = iota
	actionOpen
	actionQuit
)

// Item is the menu bar icon.
type Item struct {
	mu       sync.Mutex
	handlers Handlers
}

// New returns an item that shows nothing until Install is called.
func New() *Item { return &Item{} }

// Install shows the icon and routes the menu to h. Call it once.
func (i *Item) Install(h Handlers) {
	i.mu.Lock()
	i.handlers = h
	i.mu.Unlock()
	active.Store(i)
	install()
}

// SetWaiting shows how many replies wait for approval.
func (i *Item) SetWaiting(n int) { setWaiting(n) }

func (i *Item) dispatch(action int) {
	i.mu.Lock()
	h := i.handlers
	i.mu.Unlock()
	var fn func()
	switch action {
	case actionActivity:
		fn = h.Activity
	case actionOpen:
		fn = h.Open
	case actionQuit:
		fn = h.Quit
	}
	if fn != nil {
		go fn()
	}
}
