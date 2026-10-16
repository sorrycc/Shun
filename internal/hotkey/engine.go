// Package hotkey decides what a shortcut does: hide the app it brings
// forward when that app is frontmost, bring it forward when it runs, or
// launch it. The Engine holds what the event tap needs to decide in
// microseconds, under one lock, without the main thread: the shortcuts,
// the running apps and the frontmost one, kept current by the workspace's
// notifications.
package hotkey

import (
	"log"
	"sync"

	"shun/internal/keys"
)

// blockedFrontmost are the apps whose being frontmost lets every key
// through, so that Screen Sharing sends the keys to the remote Mac.
var blockedFrontmost = map[string]bool{
	"com.apple.ScreenSharing": true,
}

// Key is a key code with its modifiers, as event tap flags.
type Key struct {
	Code  int
	Flags uint64
}

// Entry is the app of a shortcut.
type Entry struct {
	BundleID string
	Path     string
}

// Proc is a running app.
type Proc struct {
	PID      int
	BundleID string
}

// System answers questions about running apps, for the Engine.
type System interface {
	// IsHidden reports whether the app of a process is hidden.
	IsHidden(pid int) bool
	// IsRunning reports whether a process still runs.
	IsRunning(pid int) bool
}

// ActionKind is what a shortcut does.
type ActionKind int

const (
	Hide ActionKind = iota
	Activate
	Launch
)

func (k ActionKind) String() string {
	return [...]string{"hide", "activate", "launch"}[k]
}

// Action is what a shortcut does, to which app.
type Action struct {
	Kind     ActionKind
	PID      int    // Hide and Activate
	Path     string // Launch
	BundleID string
}

// Engine resolves shortcuts. It is safe to use from any goroutine.
type Engine struct {
	mu      sync.Mutex
	sys     System
	table   map[Key]Entry
	paused  bool
	blocked bool
	running map[string]int // bundle ID → PID
	front   Proc
}

// NewEngine returns an engine without shortcuts.
func NewEngine(sys System) *Engine {
	return &Engine{sys: sys, table: map[Key]Entry{}, running: map[string]int{}}
}

// Binding is a shortcut for the engine.
type Binding struct {
	Combo keys.Combo
	Entry Entry
}

// SetBindings replaces the shortcuts; a later binding of the same keys
// wins.
func (e *Engine) SetBindings(bs []Binding) {
	table := make(map[Key]Entry, len(bs))
	for _, b := range bs {
		table[Key{b.Combo.KeyCode, b.Combo.CGFlags()}] = b.Entry
	}
	e.mu.Lock()
	e.table = table
	e.mu.Unlock()
}

// Empty reports whether there are no shortcuts.
func (e *Engine) Empty() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.table) == 0
}

// SetPaused stops the shortcuts while Shun is active, so that its shortcut
// recorder gets the keys.
func (e *Engine) SetPaused(p bool) {
	e.mu.Lock()
	e.paused = p
	e.mu.Unlock()
}

// SetRunning records the running apps, as Shun starts.
func (e *Engine) SetRunning(procs []Proc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	clear(e.running)
	for _, p := range procs {
		if p.BundleID != "" {
			e.running[p.BundleID] = p.PID
		}
	}
}

// Launched records an app that started.
func (e *Engine) Launched(p Proc) {
	if p.BundleID == "" {
		return
	}
	e.mu.Lock()
	e.running[p.BundleID] = p.PID
	e.mu.Unlock()
}

// Terminated forgets an app that quit; next is another instance of the same
// app that still runs, if any.
func (e *Engine) Terminated(p Proc, next *Proc) {
	if p.BundleID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if next != nil {
		e.running[p.BundleID] = next.PID
	} else if e.running[p.BundleID] == p.PID {
		delete(e.running, p.BundleID)
	}
}

// Activated records the frontmost app.
func (e *Engine) Activated(p Proc) {
	e.mu.Lock()
	was := e.blocked
	e.front = p
	e.blocked = blockedFrontmost[p.BundleID]
	now := e.blocked
	e.mu.Unlock()
	if was != now {
		state := "released"
		if now {
			state = "engaged"
		}
		log.Printf("hotkey: frontmost blocklist %s: %s", state, p.BundleID)
	}
}

// Match returns what the keys do, if they are a shortcut. flags may hold
// other bits than the modifiers'.
func (e *Engine) Match(code int, flags uint64) (Action, bool) {
	flags &= keys.FlagMask
	if flags == 0 {
		return Action{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.paused || e.blocked {
		return Action{}, false
	}
	entry, ok := e.table[Key{code, flags}]
	if !ok {
		return Action{}, false
	}
	// The app is frontmost: hide it.
	if e.front.BundleID == entry.BundleID && e.front.PID != 0 {
		front := e.front
		// Forget it, so that the next press cannot match a frontmost app
		// macOS did not change after the hide.
		e.front = Proc{}
		// It is hidden but still frontmost, as some Electron apps stay
		// when they swallow the focus: show it rather than hide it again.
		if e.sys.IsHidden(front.PID) {
			log.Printf("hotkey: stale frontmost: %s is hidden, activating", entry.BundleID)
			return Action{Kind: Activate, PID: front.PID, BundleID: entry.BundleID}, true
		}
		return Action{Kind: Hide, PID: front.PID, BundleID: entry.BundleID}, true
	}
	if pid, ok := e.running[entry.BundleID]; ok && e.sys.IsRunning(pid) {
		return Action{Kind: Activate, PID: pid, BundleID: entry.BundleID}, true
	}
	return Action{Kind: Launch, Path: entry.Path, BundleID: entry.BundleID}, true
}
