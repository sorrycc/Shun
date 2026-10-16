package hotkey

import (
	"testing"

	"shun/internal/keys"
)

type fakeSystem struct {
	hidden map[int]bool
	dead   map[int]bool
}

func (f *fakeSystem) IsHidden(pid int) bool  { return f.hidden[pid] }
func (f *fakeSystem) IsRunning(pid int) bool { return !f.dead[pid] }

var optC = keys.Combo{KeyCode: 8, Modifiers: keys.CarbonOption}

func newEngine() (*Engine, *fakeSystem) {
	sys := &fakeSystem{hidden: map[int]bool{}, dead: map[int]bool{}}
	e := NewEngine(sys)
	e.SetBindings([]Binding{{optC, Entry{"com.google.Chrome", "/Applications/Google Chrome.app"}}})
	return e, sys
}

// capsLock is a flag outside the modifiers, which matching ignores.
const capsLock = 0x10000

func TestLaunchActivateHide(t *testing.T) {
	e, _ := newEngine()
	a, ok := e.Match(8, keys.FlagOption|capsLock)
	if !ok || a.Kind != Launch || a.Path != "/Applications/Google Chrome.app" {
		t.Fatalf("not running: %+v %v", a, ok)
	}
	e.Launched(Proc{42, "com.google.Chrome"})
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Activate || a.PID != 42 {
		t.Fatalf("running: %+v", a)
	}
	e.Activated(Proc{42, "com.google.Chrome"})
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Hide || a.PID != 42 {
		t.Fatalf("frontmost: %+v", a)
	}
	// The frontmost app is forgotten after a hide: the next press shows it.
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Activate {
		t.Fatalf("after hide: %+v", a)
	}
}

func TestNoMatch(t *testing.T) {
	e, _ := newEngine()
	if _, ok := e.Match(8, 0); ok {
		t.Error("no modifiers")
	}
	if _, ok := e.Match(8, keys.FlagOption|keys.FlagShift); ok {
		t.Error("other modifiers")
	}
	if _, ok := e.Match(9, keys.FlagOption); ok {
		t.Error("other key")
	}
}

func TestStaleFrontmost(t *testing.T) {
	e, sys := newEngine()
	e.Launched(Proc{42, "com.google.Chrome"})
	e.Activated(Proc{42, "com.google.Chrome"})
	sys.hidden[42] = true
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Activate || a.PID != 42 {
		t.Errorf("hidden frontmost: %+v", a)
	}
}

func TestTerminated(t *testing.T) {
	e, sys := newEngine()
	e.Launched(Proc{42, "com.google.Chrome"})
	e.Launched(Proc{43, "com.google.Chrome"})
	e.Terminated(Proc{43, "com.google.Chrome"}, &Proc{42, "com.google.Chrome"})
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Activate || a.PID != 42 {
		t.Errorf("other instance: %+v", a)
	}
	e.Terminated(Proc{42, "com.google.Chrome"}, nil)
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Launch {
		t.Errorf("quit: %+v", a)
	}
	// A process that died without a notification yet is launched again.
	e.Launched(Proc{44, "com.google.Chrome"})
	sys.dead[44] = true
	if a, _ := e.Match(8, keys.FlagOption); a.Kind != Launch {
		t.Errorf("dead: %+v", a)
	}
}

func TestPausedAndBlocked(t *testing.T) {
	e, _ := newEngine()
	e.SetPaused(true)
	if _, ok := e.Match(8, keys.FlagOption); ok {
		t.Error("paused")
	}
	e.SetPaused(false)
	e.Activated(Proc{7, "com.apple.ScreenSharing"})
	if _, ok := e.Match(8, keys.FlagOption); ok {
		t.Error("Screen Sharing is frontmost")
	}
	e.Activated(Proc{8, "com.apple.finder"})
	if _, ok := e.Match(8, keys.FlagOption); !ok {
		t.Error("released")
	}
}

func TestHyperKey(t *testing.T) {
	e, _ := newEngine()
	hyper := keys.Combo{KeyCode: 2, Modifiers: keys.CarbonCmd | keys.CarbonShift | keys.CarbonOption | keys.CarbonControl}
	e.SetBindings([]Binding{{hyper, Entry{"com.googlecode.iterm2", "/Applications/iTerm.app"}}})
	if _, ok := e.Match(2, keys.FlagMask); !ok {
		t.Error("hyper key")
	}
	if e.Empty() {
		t.Error("not empty")
	}
}
