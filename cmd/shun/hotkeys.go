package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"

	"shun/internal/hotkey"
	"shun/internal/macos"
)

// hotkeys takes the keys of the shortcuts with an event tap, which decides
// and acts on its own thread in well under a millisecond, and lets keys
// through while Screen Sharing is in front. Without the Accessibility
// permission there is no event tap: MyGo's global shortcuts (Carbon) take
// the keys instead, slower, on the main thread.
type hotkeys struct {
	a      *app
	engine *hotkey.Engine
	tap    *macos.EventTap
	carbon []string // the accelerators registered without a tap
}

// system answers the engine's questions about processes.
type system struct{}

func (system) IsHidden(pid int) bool  { return macos.IsHidden(pid) }
func (system) IsRunning(pid int) bool { return macos.IsRunning(pid) }

func proc(a macos.App) hotkey.Proc { return hotkey.Proc{PID: a.PID, BundleID: a.BundleID} }

func newHotkeys(a *app) *hotkeys {
	return &hotkeys{a: a, engine: hotkey.NewEngine(system{})}
}

// start follows the running apps and starts taking keys. Call it on the
// main thread.
func (h *hotkeys) start() {
	var procs []hotkey.Proc
	for _, app := range macos.RunningApps() {
		procs = append(procs, proc(app))
	}
	h.engine.SetRunning(procs)
	h.engine.Activated(proc(macos.Frontmost()))
	macos.ObserveWorkspace(macos.WorkspaceHandlers{
		Launched: func(app macos.App) {
			h.engine.Launched(proc(app))
			h.a.appsChanged()
		},
		Terminated: func(app macos.App) {
			var next *hotkey.Proc
			if other, ok := macos.InstanceOf(app.BundleID); ok && other.PID != app.PID {
				p := proc(other)
				next = &p
			}
			h.engine.Terminated(proc(app), next)
			h.a.appsChanged()
		},
		Activated: func(app macos.App) { h.engine.Activated(proc(app)) },
	})
	// While Shun is in front its shortcut recorder takes the keys.
	mygo.App.OnDidBecomeActive(func() {
		h.engine.SetPaused(true)
		if h.tap == nil {
			h.refreshCarbon(nil)
		}
	})
	mygo.App.OnDidResignActive(func() {
		h.engine.SetPaused(false)
		if h.tap == nil {
			h.refresh()
		}
	})

	if !h.startTap() {
		log.Print("hotkeys: no Accessibility permission; global shortcuts take the keys until it is granted")
		go h.waitForPermission()
	}
	h.refresh()
}

// startTap starts the event tap, with the Accessibility permission. An
// untrusted process gets a tap that receives nothing, rather than an error,
// so the permission is asked first.
func (h *hotkeys) startTap() bool {
	if !macos.IsTrusted() {
		return false
	}
	tap, err := macos.StartEventTap(h.onKey)
	if err != nil {
		log.Printf("hotkeys: %v", err)
		return false
	}
	h.tap = tap
	return true
}

// waitForPermission switches from global shortcuts to the event tap once
// the user grants the Accessibility permission, without a relaunch.
func (h *hotkeys) waitForPermission() {
	for range time.Tick(2 * time.Second) {
		if !macos.IsTrusted() {
			continue
		}
		mygo.RunOnMain(func() {
			if h.tap == nil && h.startTap() {
				log.Print("hotkeys: Accessibility permission granted; the event tap takes the keys")
				h.refreshCarbon(nil)
				h.refresh()
			}
		})
		return
	}
}

// onKey runs on the tap's thread for every key press.
func (h *hotkeys) onKey(code int, flags uint64) bool {
	action, ok := h.engine.Match(code, flags)
	if !ok {
		return false
	}
	h.do(action)
	return true
}

// do carries out an action, on any thread.
func (h *hotkeys) do(action hotkey.Action) {
	switch action.Kind {
	case hotkey.Hide:
		macos.Hide(action.PID)
		return
	case hotkey.Activate:
		macos.Activate(action.PID)
	case hotkey.Launch:
		go func() {
			if err := mygo.Shell.LaunchApp(mygo.InstalledApp{ID: action.BundleID, Path: action.Path}); err != nil {
				log.Printf("hotkeys: launching %s: %v", action.Path, err)
			}
		}()
	}
	h.a.usage.Log(action.BundleID)
}

// refresh takes the keys of the enabled shortcuts, after a change.
func (h *hotkeys) refresh() {
	on := h.a.prefs.GlobalEnabled()
	var bindings []hotkey.Binding
	if on {
		for _, sc := range h.a.store.Shortcuts {
			if sc.IsEnabled && sc.Combo != nil {
				bindings = append(bindings, hotkey.Binding{
					Combo: *sc.Combo,
					Entry: hotkey.Entry{BundleID: sc.BundleIdentifier, Path: sc.Path()},
				})
			}
		}
	}
	h.engine.SetBindings(bindings)
	log.Printf("hotkeys: %d shortcuts (enabled: %v)", len(bindings), on)
	if h.tap != nil {
		h.tap.SetEnabled(on && len(bindings) > 0)
		return
	}
	h.refreshCarbon(bindings)
}

func (h *hotkeys) refreshCarbon(bindings []hotkey.Binding) {
	for _, acc := range h.carbon {
		mygo.GlobalShortcut.Unregister(acc)
	}
	h.carbon = h.carbon[:0]
	for _, b := range bindings {
		acc, ok := b.Combo.Accelerator()
		if !ok {
			log.Printf("hotkeys: %s cannot be a global shortcut", b.Combo)
			continue
		}
		entry := b.Entry
		if err := mygo.GlobalShortcut.Register(acc, func() { h.carbonToggle(entry) }); err != nil {
			log.Printf("hotkeys: %s: %v", acc, err)
			continue
		}
		h.carbon = append(h.carbon, acc)
	}
}

// carbonToggle does what the engine decides, without the workspace cache:
// a global shortcut runs on the main thread, where asking is cheap enough.
func (h *hotkeys) carbonToggle(e hotkey.Entry) {
	front := macos.Frontmost()
	if front.BundleID == "com.apple.ScreenSharing" {
		return
	}
	if front.BundleID == e.BundleID && !macos.IsHidden(front.PID) {
		h.do(hotkey.Action{Kind: hotkey.Hide, PID: front.PID, BundleID: e.BundleID})
	} else if app, ok := macos.InstanceOf(e.BundleID); ok {
		h.do(hotkey.Action{Kind: hotkey.Activate, PID: app.PID, BundleID: e.BundleID})
	} else {
		h.do(hotkey.Action{Kind: hotkey.Launch, Path: e.Path, BundleID: e.BundleID})
	}
}

// appsChanged marks the shortcuts whose apps are gone, as apps come and go.
func (a *app) appsChanged() {
	if a.store.RefreshStaleness() {
		a.invalidate()
	}
}
