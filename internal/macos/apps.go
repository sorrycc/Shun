//go:build darwin

package macos

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// App is a running app.
type App struct {
	PID      int
	BundleID string
}

func appOf(obj id) App {
	if obj == 0 {
		return App{}
	}
	return App{
		PID:      int(int32(send(obj, "processIdentifier"))),
		BundleID: goString(send(obj, "bundleIdentifier")),
	}
}

func workspace() id { return send(class("NSWorkspace"), "sharedWorkspace") }

// RunningApps lists the running apps.
func RunningApps() []App {
	var apps []App
	withPool(func() {
		list := send(workspace(), "runningApplications")
		n := int(send(list, "count"))
		for i := range n {
			apps = append(apps, appOf(send(list, "objectAtIndex:", uintptr(i))))
		}
	})
	return apps
}

// Frontmost returns the frontmost app.
func Frontmost() (a App) {
	withPool(func() { a = appOf(send(workspace(), "frontmostApplication")) })
	return
}

// InstanceOf returns a running instance of an app, if any.
func InstanceOf(bundleID string) (a App, ok bool) {
	withPool(func() {
		list := send(class("NSRunningApplication"), "runningApplicationsWithBundleIdentifier:", uintptr(nsString(bundleID)))
		if send(list, "count") > 0 {
			a, ok = appOf(send(list, "objectAtIndex:", 0)), true
		}
	})
	return
}

func running(pid int) id {
	return send(class("NSRunningApplication"), "runningApplicationWithProcessIdentifier:", uintptr(pid))
}

// IsHidden reports whether the app of a process is hidden.
func IsHidden(pid int) (hidden bool) {
	withPool(func() {
		if app := running(pid); app != 0 {
			hidden = sendBool(app, "isHidden")
		}
	})
	return
}

// IsRunning reports whether a process runs.
func IsRunning(pid int) (ok bool) {
	withPool(func() {
		app := running(pid)
		ok = app != 0 && !sendBool(app, "isTerminated")
	})
	return
}

// Hide hides the app of a process.
func Hide(pid int) {
	withPool(func() {
		if app := running(pid); app != 0 {
			send(app, "hide")
		}
	})
}

const nsApplicationActivateIgnoringOtherApps = 1 << 1

// Activate brings the app of a process to the front: through the window
// server directly (SkyLight, a few milliseconds), else as AppKit does
// (Apple events, tens of milliseconds).
func Activate(pid int) {
	if fastActivate(pid) {
		return
	}
	withPool(func() {
		if app := running(pid); app != 0 {
			send(app, "activateWithOptions:", nsApplicationActivateIgnoringOtherApps)
		}
	})
}

var (
	skyOnce                    sync.Once
	getProcessForPID, setFront uintptr
)

// fastActivate activates with SkyLight's private SPI, which Thor and
// others use; false when it is missing or fails.
func fastActivate(pid int) bool {
	skyOnce.Do(func() {
		load()
		hi, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/Frameworks/HIServices.framework/HIServices", purego.RTLD_LAZY)
		if err != nil {
			return
		}
		sky, err := purego.Dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/SkyLight", purego.RTLD_LAZY)
		if err != nil {
			return
		}
		g, err1 := purego.Dlsym(hi, "GetProcessForPID")
		s, err2 := purego.Dlsym(sky, "_SLPSSetFrontProcessWithOptions")
		if err1 == nil && err2 == nil {
			getProcessForPID, setFront = g, s
		}
	})
	if getProcessForPID == 0 {
		return false
	}
	psnMu.Lock()
	defer psnMu.Unlock()
	p := uintptr(unsafe.Pointer(&psn))
	if r, _, _ := purego.SyscallN(getProcessForPID, uintptr(pid), p); int32(r) != 0 {
		return false
	}
	r, _, _ := purego.SyscallN(setFront, p, 0, 0)
	return int32(r) == 0
}

// psn is a ProcessSerialNumber, in a package variable, which never moves.
var (
	psnMu sync.Mutex
	psn   [2]uint32
)

// WorkspaceHandlers are called on the main thread when apps start, quit
// and come to the front.
type WorkspaceHandlers struct {
	Launched, Terminated, Activated func(App)
}

// ObserveWorkspace calls the handlers for the workspace's notifications,
// for as long as the app runs. Call it on the main thread.
func ObserveWorkspace(h WorkspaceHandlers) {
	withPool(func() {
		center := send(workspace(), "notificationCenter")
		observe := func(name string, fn func(App)) {
			if fn == nil {
				return
			}
			block := objc.NewBlock(func(_ objc.Block, note objc.ID) {
				withPool(func() {
					info := send(id(note), "userInfo")
					app := send(info, "objectForKey:", uintptr(nsString("NSWorkspaceApplicationKey")))
					fn(appOf(app))
				})
			})
			// The observer lasts as long as the app; so does the block.
			send(center, "addObserverForName:object:queue:usingBlock:", uintptr(nsString(name)), 0, 0, uintptr(block))
		}
		observe("NSWorkspaceDidLaunchApplicationNotification", h.Launched)
		observe("NSWorkspaceDidTerminateApplicationNotification", h.Terminated)
		observe("NSWorkspaceDidActivateApplicationNotification", h.Activated)
	})
}

// BundleInfo returns the bundle ID and name of the app at path.
func BundleInfo(path string) (bundleID, name string, ok bool) {
	withPool(func() {
		b := send(class("NSBundle"), "bundleWithPath:", uintptr(nsString(path)))
		if b == 0 {
			return
		}
		bundleID = goString(send(b, "bundleIdentifier"))
		for _, key := range []string{"CFBundleName", "CFBundleDisplayName"} {
			if name = goString(send(b, "objectForInfoDictionaryKey:", uintptr(nsString(key)))); name != "" {
				break
			}
		}
	})
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), ".app")
	}
	return bundleID, name, bundleID != ""
}

// AppPath returns where the app of a bundle ID is installed.
func AppPath(bundleID string) (path string, ok bool) {
	withPool(func() {
		u := send(workspace(), "URLForApplicationWithBundleIdentifier:", uintptr(nsString(bundleID)))
		if u != 0 {
			path = goString(send(u, "path"))
		}
	})
	return path, path != ""
}

// InfoString returns a string of the app's own Info.plist, or "".
func InfoString(key string) (s string) {
	withPool(func() {
		s = goString(send(send(class("NSBundle"), "mainBundle"), "objectForInfoDictionaryKey:", uintptr(nsString(key))))
	})
	return
}

// LocaleIdentifier returns the user's locale, such as en_US.
func LocaleIdentifier() (s string) {
	withPool(func() { s = goString(send(send(class("NSLocale"), "currentLocale"), "localeIdentifier")) })
	return
}

// IsTrusted reports whether Shun has the Accessibility permission, which
// the event tap needs.
func IsTrusted() bool {
	load()
	fn, err := purego.Dlsym(libAS, "AXIsProcessTrusted")
	if err != nil {
		log.Print(err)
		return false
	}
	r, _, _ := purego.SyscallN(fn)
	return byte(r) != 0
}
