// Package menubar puts Deputy's icon and menu in the macOS menu bar.
package menubar

import "sync"

// Handlers are called, each on its own goroutine, when the person picks a menu item.
type Handlers struct {
	Activity func()
	Open     func()
	Quit     func()
	Inbox    func() // "N unread messages"
}

// Menu actions, shared with the Objective-C side.
const (
	actionActivity = iota
	actionOpen
	actionQuit
	actionInbox
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

// SetUnread shows how many WhatsApp messages are unread, in the menu and on the Dock icon.
func (i *Item) SetUnread(n int) { setUnread(n) }

// Focused reports whether one of Deputy's windows has the keyboard focus right now.
func Focused() bool { return focused() }

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
	case actionInbox:
		fn = h.Inbox
	}
	if fn != nil {
		go fn()
	}
}
