// Shun (瞬, "instant") switches to apps with global shortcuts: a shortcut
// brings its app to the front, launching it when it does not run, and hides
// it when it is in front already. It lives in the menu bar, and its Settings
// window, of MyGo's native UI, binds apps to keys.
package main

import (
	"context"
	_ "embed"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"shun/internal/hotkey"
	"shun/internal/macos"
	"shun/internal/store"
	"shun/internal/telemetry"
	"shun/internal/usage"
)

// defaultsDomain is the bundle ID of Shun, under which the Swift versions
// kept their preferences in UserDefaults.
const defaultsDomain = "com.shun.Shun"

//go:embed trayicon.png
var trayIcon []byte

// app is Shun's state. Everything but the hotkey thread's work, the usage
// log and the preferences is read and changed on the main thread.
type app struct {
	store *store.Store
	prefs *store.Prefs
	usage *usage.Store
	keys  *hotkeys

	win      *mygo.Window // Settings
	tray     *mygo.Tray
	windows  []*mygo.Window
	quitting bool

	icons *iconCache
	ui    settingsState
	trust func() bool // whether Shun has the Accessibility permission
}

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running Shun opens its Settings (OnSecondInstance)
	}
	// Development builds keep their data apart from the installed app's:
	// a `go run` binary is named "shun", which is the installed app's
	// directory on a case-insensitive disk.
	if mygo.IsDev() && mygo.App.Name() != "Shun Dev" {
		mygo.App.SetName("Shun Dev")
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.Use(native.Plugin)
	logToFile()

	a, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.OnWindowAllClosed(func() {}) // keep running in the menu bar
	mygo.App.OnSecondInstance(func([]string, string) { a.openSettings() })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.openSettings()
		}
	})
	mygo.App.OnWindowCreated(a.trackWindow)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.usage.SaveNow()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// logToFile writes the log of release builds to ~/Library/Logs/Shun/Shun.log
// as well, since an app opened from Finder has no terminal; the log of the
// run before is kept once it is over a megabyte, as Shun.log.1.
func logToFile() {
	if mygo.IsDev() {
		return
	}
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		return
	}
	path := filepath.Join(dir, "Shun.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// dataDir is where Shun keeps its files: ~/Library/Application Support/Shun,
// or SHUN_DATA_DIR.
func dataDir() (string, error) {
	if dir := os.Getenv("SHUN_DATA_DIR"); dir != "" {
		return dir, os.MkdirAll(dir, 0o755)
	}
	return mygo.App.Path(mygo.PathUserData)
}

func newApp() (*app, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	if err := store.PrepareDir(dir); err != nil {
		log.Printf("moving the HotApp directory: %v", err)
	}
	s, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	p, err := store.OpenPrefs(dir)
	if err != nil {
		return nil, err
	}
	a := &app{store: s, prefs: p, usage: usage.Open(dir), icons: newIconCache(), trust: macos.IsTrusted}
	a.ui.init()
	return a, nil
}

func (a *app) ready() {
	defaults := macos.Defaults{AppID: defaultsDomain}
	if store.Migrate(a.store, a.prefs, defaults) {
		if on, ok := defaults.Bool("SUEnableAutomaticChecks"); ok {
			updater.SetAutomaticChecks(on)
		}
	}

	a.keys = newHotkeys(a)
	a.store.OnChange(a.keys.refresh)
	a.store.OnChange(a.updateTrayMenu)
	// Activations are logged on the event tap's thread, which must not wait
	// for the main one.
	a.usage.OnChange(func() { go mygo.RunOnMain(a.invalidate) })
	a.keys.start()
	a.setUpTray()

	if !macos.IsTrusted() {
		go askForAccessibility()
	}
	if a.store.FirstLaunch() {
		time.AfterFunc(500*time.Millisecond, func() { mygo.RunOnMain(a.openSettings) })
	}
	go a.startTelemetry()
}

// window returns the Settings window, made the first time: an app of the
// menu bar mostly runs without it, and a window takes memory.
func (a *app) window() *mygo.Window {
	if a.win != nil {
		return a.win
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Shun Settings",
		Width:             600,
		Height:            560,
		MinWidth:          540,
		MinHeight:         420,
		Hidden:            true,
		DisableFullScreen: true,
		// The view draws the title and the tabs under the window controls,
		// as one toolbar.
		TitleBarStyle:   mygo.TitleBarHidden,
		BackgroundColor: "light-dark(#ffffff, #1e1e1e)",
		Content:         ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.win.Hide()
		}
	})
	a.icons.post = a.win.Update
	a.win.OnHide(func() {
		a.ui.reset()
		keep := make([]string, len(a.store.Shortcuts))
		for i, sc := range a.store.Shortcuts {
			keep[i] = sc.Path()
		}
		a.icons.clear(keep) // all but the shortcuts' icons, which show at once next time
	})
	return a.win
}

// setUpTray puts Shun's icon in the menu bar, with its menu.
func (a *app) setUpTray() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon, IconIsTemplate: true, ToolTip: "Shun"})
	if err != nil {
		log.Printf("no menu bar icon: %v", err)
		return
	}
	a.updateTrayMenu()
}

// updateTrayMenu builds the menu of the menu bar icon: the shortcuts,
// with their keys, which bring their apps forward, then Shun's own items.
func (a *app) updateTrayMenu() {
	if a.tray != nil {
		a.tray.SetMenu(mygo.NewMenu(a.trayItems()))
	}
}

// trayItems are the items of the menu bar menu.
func (a *app) trayItems() []*mygo.MenuItem {
	title := "Shun"
	if v := mygo.App.Version(); v != "" {
		title += " v" + v
	}
	items := []*mygo.MenuItem{{Label: title, Disabled: true}, mygo.Separator()}
	on := a.prefs.GlobalEnabled()
	n := 0
	for _, sc := range a.store.Shortcuts {
		if !sc.IsEnabled || sc.Stale {
			continue
		}
		if n++; n > maxTrayShortcuts {
			break
		}
		item := &mygo.MenuItem{Label: sc.DisplayName}
		if sc.Combo != nil && on {
			item.Accelerator, _ = sc.Combo.Accelerator()
		}
		entry := hotkey.Entry{BundleID: sc.BundleIdentifier, Path: sc.Path()}
		item.Click = func(*mygo.MenuItem, *mygo.Window) { bringForward(entry) }
		items = append(items, item)
	}
	if n > 0 {
		items = append(items, mygo.Separator())
	}
	items = append(items,
		&mygo.MenuItem{Label: "Shortcuts Enabled", Type: mygo.MenuItemCheckbox, Checked: on,
			Click: func(it *mygo.MenuItem, _ *mygo.Window) { a.setGlobalEnabled(it.Checked) }},
		&mygo.MenuItem{Label: "Settings…", Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		&mygo.MenuItem{Label: "Check for Updates…", Click: func(*mygo.MenuItem, *mygo.Window) { a.checkForUpdates() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "Quit Shun", Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	)
	return items
}

// maxTrayShortcuts is how many shortcuts the menu bar menu lists.
const maxTrayShortcuts = 20

// setGlobalEnabled turns every shortcut on or off.
func (a *app) setGlobalEnabled(on bool) {
	a.prefs.SetGlobalEnabled(on)
	if a.keys != nil {
		a.keys.refresh()
	}
	a.updateTrayMenu()
	a.invalidate()
}

// openSettings shows the Settings window in front. An app without a Dock
// icon cannot reliably come to the front on macOS 26 (Tahoe): it becomes a
// regular app while it shows a window, and an accessory again once it shows
// none (refreshActivationPolicy).
func (a *app) openSettings() {
	a.appsChanged()
	a.ui.loginItem = mygo.App.OpenAtLogin()
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
	win := a.window()
	if !win.IsVisible() {
		win.Center()
	}
	win.Show()
	win.Invalidate() // with what changed while it hid
	win.Focus()
	mygo.App.Focus()
}

func (a *app) checkForUpdates() {
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
	mygo.App.Focus()
	updater.CheckForUpdates()
}

// trackWindow watches every window, the update window's too, to drop the
// Dock icon once none shows.
func (a *app) trackWindow(w *mygo.Window) {
	a.windows = append(a.windows, w)
	w.OnHide(a.refreshActivationPolicy)
	w.OnClose(func(*mygo.CloseEvent) { a.refreshActivationPolicy() })
}

func (a *app) refreshActivationPolicy() {
	// After the window has left the screen.
	time.AfterFunc(100*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			for _, w := range a.windows {
				if w.IsVisible() {
					return
				}
			}
			mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		})
	})
}

// askForAccessibility explains the permission the event tap needs.
func askForAccessibility() {
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageWarning,
		Message: "Accessibility Permission Required",
		Detail: "Shun needs Accessibility access to switch apps instantly. Please grant access in " +
			"System Settings → Privacy & Security → Accessibility. Until then, shortcuts work more slowly.",
		Buttons: []string{"Open System Settings", "Later"},
	})
	if err == nil && res.Button == 0 {
		mygo.Shell.OpenExternal(accessibilitySettings)
	}
}

// accessibilitySettings opens System Settings at the Accessibility list.
const accessibilitySettings = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

// telemetryConfig returns where telemetry goes, from the Info.plist of
// release builds; false for others.
func telemetryConfig() (telemetry.Config, bool) {
	endpoint, key := macos.InfoString("TelemetryEndpointURL"), macos.InfoString("TelemetryIngestKey")
	if endpoint == "" || key == "" {
		return telemetry.Config{}, false
	}
	home, _ := os.UserHomeDir()
	osVersion, _ := syscall.Sysctl("kern.osproductversion")
	return telemetry.Config{
		Endpoint:    endpoint,
		IngestKey:   key,
		AppVersion:  mygo.App.Version(),
		OSVersion:   osVersion,
		Locale:      macos.LocaleIdentifier(),
		CrashDir:    filepath.Join(home, "Library/Logs/DiagnosticReports"),
		CrashPrefix: "Shun-",
	}, true
}

func (a *app) startTelemetry() {
	if c, ok := telemetryConfig(); ok {
		telemetry.Run(context.Background(), c, a.prefs)
	}
}
