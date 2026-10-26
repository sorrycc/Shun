package main

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/cloudsync"
	"shun/internal/keys"
	"shun/internal/mon"
	"shun/internal/store"
	"shun/internal/usage"
)

func newTestApp(t testing.TB) *app {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AppExists = func(p string) bool { return p != "/Applications/Gone.app" }
	p, _ := store.OpenPrefs(dir)
	a := &app{store: s, prefs: p, usage: usage.Open(dir), icons: newIconCache(), trust: func() bool { return true }}
	t.Cleanup(a.usage.SaveNow) // before dir goes, rather than as it goes
	a.icons.draw = func(string) *ui.Bitmap { return nil }
	a.cloud = newCloud(a, dir)
	a.cloud.onMain = func(fn func()) { fn() }
	a.ui.init()
	iterm := store.NewShortcut("/Applications/iTerm.app", "com.googlecode.iterm2", "iTerm2")
	iterm.Combo = &keys.Combo{KeyCode: 8, Modifiers: keys.CarbonOption}
	s.Add(iterm)
	s.Add(store.NewShortcut("/Applications/Safari.app", "com.apple.Safari", "Safari"))
	s.Add(store.NewShortcut("/Applications/Gone.app", "com.example.gone", "Gone"))
	return a
}

// shot saves the tester's frame to $SHUN_SHOTS/name.png, and in the dark
// theme to name-dark.png, to look at: once animations are over.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHUN_SHOTS")
	if dir == "" {
		return
	}
	save := func(name string) {
		tt.Advance(2 * time.Second)
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 78})
	save(name)
	tt.SetDark(true)
	save(name + "-dark")
	tt.SetDark(false)
	tt.SetTitleBar(ui.TitleBar{})
}

func TestShortcutsTab(t *testing.T) {
	a := newTestApp(t)
	a.usage.Log("com.googlecode.iterm2")
	tt := ui.NewTester(a.view, 600, 520)
	for _, text := range []string{"iTerm2", "Safari", "⌥", "C", "Record Shortcut", "App not found", "1× past 7 days",
		"1 of 3 active"} {
		if !tt.HasText(text) {
			t.Errorf("no %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "shortcuts")
}

func TestRecordShortcut(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	safari := a.store.Shortcuts[1]
	if err := tt.Click("Record Shortcut"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Type Shortcut") {
		t.Fatalf("not recording: %q", tt.Texts())
	}
	tt.HoldModifiers(ui.Ctrl | ui.Super) // shows the modifiers held
	if tt.HasText("Type Shortcut") || !tt.HasText("⌃") || !tt.HasText("⌘") {
		t.Errorf("held modifiers not shown: %q", tt.Texts())
	}
	shot(t, tt, "recording")
	tt.HoldModifiers(0)
	if !tt.HasText("Press keys with ⌘ ⌥ ⌃ or ⇧ · esc cancels") || tt.HasText("No shortcut") {
		t.Errorf("no hint while recording: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyK) // no modifier: still waiting, and it shakes
	if a.ui.recording != safari.ID {
		t.Error("stopped recording on a key without a modifier")
	}
	tt.Key(ui.Alt, ui.KeyC)
	sc, _ := a.store.Get(safari.ID)
	if sc.Combo == nil || *sc.Combo != (keys.Combo{KeyCode: 8, Modifiers: keys.CarbonOption}) {
		t.Fatalf("combo %v", sc.Combo)
	}
	if iterm, _ := a.store.Get(a.store.Shortcuts[0].ID); iterm.Combo != nil {
		t.Error("iTerm2 keeps ⌥C")
	}
	if !tt.HasText("Reassigned from iTerm2") {
		t.Errorf("no notice in %q", tt.Texts())
	}
	shot(t, tt, "reassigned")
}

func TestRecorderEscapeAndClear(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	tt.Click("Record Shortcut")
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Type Shortcut") {
		t.Error("Escape cancels")
	}
	if err := tt.Click("Clear Shortcut"); err != nil {
		t.Fatal(err)
	}
	if a.store.Shortcuts[0].Combo != nil {
		t.Error("cleared")
	}
}

func TestToggleAndRemove(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	if err := tt.Click("Enable Safari"); err != nil {
		t.Fatal(err)
	}
	if a.store.Shortcuts[1].IsEnabled {
		t.Error("Safari is off")
	}
	tt.Click("Safari")
	shot(t, tt, "selected")
	if err := tt.Click("Remove Shortcut"); err != nil {
		t.Fatal(err)
	}
	if len(a.store.Shortcuts) != 2 || a.store.HasApp("com.apple.Safari") {
		t.Errorf("shortcuts %+v", a.store.Shortcuts)
	}
	if !tt.HasText("Removed Safari") {
		t.Fatalf("no toast: %q", tt.Texts())
	}
	shot(t, tt, "remove")
	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	if len(a.store.Shortcuts) != 3 || a.store.Shortcuts[1].BundleIdentifier != "com.apple.Safari" {
		t.Errorf("not back in its place: %+v", a.store.Shortcuts)
	}
}

func TestEmptyStates(t *testing.T) {
	a := newTestApp(t)
	a.store.Remove(a.store.Shortcuts[0].ID, a.store.Shortcuts[1].ID, a.store.Shortcuts[2].ID)
	tt := ui.NewTester(a.view, 600, 520)
	if !tt.HasText("No shortcuts yet") || !tt.HasText("Add App…") {
		t.Errorf("texts %q", tt.Texts())
	}
	shot(t, tt, "empty")
	tt.Click("Insights")
	if !tt.HasText("No usage yet") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestOtherTabs(t *testing.T) {
	a := newTestApp(t)
	a.usage.Log("com.googlecode.iterm2")
	a.usage.Log("com.apple.Safari")
	tt := ui.NewTester(a.view, 600, 520)

	tt.Click("Insights")
	for _, text := range []string{"Most Used", "Last 7 Days", "Switches", "Clear Usage Data…"} {
		if !tt.HasText(text) {
			t.Errorf("no %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "insights")

	tt.Click("General")
	if !tt.HasText("Launch at Login") || !tt.HasText("Enable All Shortcuts") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("Enable All Shortcuts"); err != nil {
		t.Fatal(err)
	}
	if a.prefs.GlobalEnabled() {
		t.Error("shortcuts are off")
	}
	shot(t, tt, "general")

	tt.Click("Privacy")
	if !tt.HasText("Telemetry is only active in packaged release builds.") {
		t.Errorf("texts %q", tt.Texts())
	}
	shot(t, tt, "privacy")
}

func TestImportRecipeFile(t *testing.T) {
	a := newTestApp(t)
	path := filepath.Join(t.TempDir(), "x.hotrecipe")
	os.WriteFile(path, []byte(`{"version":9,"entries":[]}`), 0o644)
	tt := ui.NewTester(a.view, 600, 520)
	a.importRecipeFile(path)
	tt.Frame()
	if !tt.HasText("Recipe Import") {
		t.Errorf("no alert: %q", tt.Texts())
	}
}

func TestComboOf(t *testing.T) {
	c, ok := comboOf(ui.InputEvent{Key: ui.KeyUnknown, PhysicalKey: ui.KeyK, Mods: ui.Alt | ui.Shift})
	if !ok || c != (keys.Combo{KeyCode: 0x28, Modifiers: keys.CarbonOption | keys.CarbonShift}) {
		t.Errorf("combo %+v %v", c, ok)
	}
	if _, ok := comboOf(ui.InputEvent{Key: ui.KeyK}); ok {
		t.Error("a key without modifiers")
	}
}

func TestFilterShortcuts(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("saf")
	tt.Frame()
	if !tt.HasText("Safari") || tt.HasText("iTerm2") || !tt.HasText("1 of 3") {
		t.Fatalf("filtered by name: %q", tt.Texts())
	}
	if a.ui.list.Reorder != nil {
		t.Error("rows drag while filtered")
	}
	shot(t, tt, "filtered")
	if err := tt.Click("Clear Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Type("googlecode") // iTerm2's bundle ID
	tt.Frame()
	if !tt.HasText("iTerm2") || tt.HasText("Safari") {
		t.Fatalf("filtered by bundle ID: %q", tt.Texts())
	}
	tt.Click("iTerm2")
	if err := tt.Click("Remove Shortcut"); err != nil {
		t.Fatal(err)
	}
	if a.store.HasApp("com.googlecode.iterm2") || !a.store.HasApp("com.apple.Safari") {
		t.Errorf("removed the wrong one: %+v", a.store.Shortcuts)
	}
	tt.Click("Undo")
	tt.Click("Filter Apps")
	tt.Type("zzz")
	tt.Frame()
	if !tt.HasText("No apps match “googlecodezzz”") {
		t.Fatalf("no empty state: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if a.ui.query != "" || !tt.HasText("Safari") || !tt.HasText("iTerm2") || a.ui.list.Reorder == nil {
		t.Errorf("Escape clears: query %q, texts %q", a.ui.query, tt.Texts())
	}
}

func TestPicker(t *testing.T) {
	a := newTestApp(t)
	p := &a.ui.picker
	p.apps = []mygo.InstalledApp{
		{Name: "Calculator", ID: "com.apple.calculator", Path: "/System/Applications/Calculator.app"},
		{Name: "Calendar", ID: "com.apple.iCal", Path: "/System/Applications/Calendar.app"},
		{Name: "Safari", ID: "com.apple.Safari", Path: "/Applications/Safari.app"},
	}
	p.loadedAt = time.Now()
	tt := ui.NewTester(a.view, 600, 520)
	if err := tt.Click("Add App"); err != nil {
		t.Fatal(err)
	}
	tt.Type("cal")
	tt.Frame()
	if !tt.HasText("Calculator") || !tt.HasText("Calendar") || tt.HasText("com.apple.Safari") {
		t.Fatalf("results %q", tt.Texts())
	}
	shot(t, tt, "picker")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter) // adds Calendar
	if !a.store.HasApp("com.apple.iCal") || a.store.HasApp("com.apple.calculator") {
		t.Errorf("shortcuts %+v", a.store.Shortcuts)
	}
	if err := tt.Click("Calculator"); err != nil {
		t.Fatal(err)
	}
	if !a.store.HasApp("com.apple.calculator") {
		t.Error("a click adds")
	}
	tt.Key(0, ui.KeyEnter) // Calculator again: it has a shortcut, and its badge shakes
	if p.nudges != 1 || p.nudged != "/System/Applications/Calculator.app" || len(a.store.Shortcuts) != 5 {
		t.Errorf("nudges %d of %q, %d shortcuts", p.nudges, p.nudged, len(a.store.Shortcuts))
	}
	if err := tt.Click("Clear Search"); err != nil {
		t.Fatal(err)
	}
	if p.query != "" || !tt.HasText("Safari") {
		t.Errorf("query %q, texts %q", p.query, tt.Texts())
	}
	tt.Click("Done")
	if a.ui.picking {
		t.Error("closed")
	}
}

func TestPickerScanning(t *testing.T) {
	a := newTestApp(t)
	a.ui.picking, a.ui.picker.loading = true, true
	tt := ui.NewTester(a.view, 600, 520)
	if _, ok := tt.Find("Scanning applications…"); !ok {
		t.Errorf("no placeholder rows: %q", tt.Texts())
	}
	shot(t, tt, "scanning")
}

func TestRowContextMenu(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	if err := tt.RightClick("iTerm2"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Contains(got, "Turn Off") || !slices.Contains(got, "Clear Shortcut") {
		t.Fatalf("menu %q", got)
	}
	if err := tt.ChooseMenuItem("Turn Off"); err != nil {
		t.Fatal(err)
	}
	if a.store.Shortcuts[0].IsEnabled {
		t.Error("iTerm2 is on")
	}
	tt.RightClick("iTerm2")
	if err := tt.ChooseMenuItem("Clear Shortcut"); err != nil {
		t.Fatal(err)
	}
	if a.store.Shortcuts[0].Combo != nil {
		t.Error("iTerm2 keeps its keys")
	}
}

func TestBanners(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	if tt.HasText("Shortcuts are paused") || tt.HasText("Switch apps instantly") {
		t.Fatalf("banner while all is well: %q", tt.Texts())
	}
	a.setGlobalEnabled(false)
	tt.Frame()
	if !tt.HasText("Shortcuts are paused") {
		t.Fatalf("no paused banner in %q", tt.Texts())
	}
	shot(t, tt, "paused")
	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if !a.prefs.GlobalEnabled() {
		t.Error("Resume leaves shortcuts paused")
	}
	a.trust = func() bool { return false }
	a.ui.trustedAt = time.Time{}
	tt.Frame()
	if !tt.HasText("Switch apps instantly") || !tt.HasText("Open Settings…") {
		t.Fatalf("no permission banner in %q", tt.Texts())
	}
	shot(t, tt, "untrusted")
}

func TestPerDay(t *testing.T) {
	for n, want := range map[float32]string{0: "0", 0.29: "0.3", 3: "3", 12.4: "12"} {
		if got := perDay(n); got != want {
			t.Errorf("perDay(%v) = %q, want %q", n, got, want)
		}
	}
	for _, c := range []struct {
		today, avg float32
		want       string
	}{{2, 0.3, "1.7 above average"}, {0, 2, "2 below average"}, {1, 1.02, "on average"}} {
		if got := againstAverage(c.today, c.avg); got != c.want {
			t.Errorf("againstAverage(%v, %v) = %q, want %q", c.today, c.avg, got, c.want)
		}
	}
}

func TestTrayItems(t *testing.T) {
	a := newTestApp(t)
	labels := func() (l []string, acc map[string]string) {
		acc = map[string]string{}
		for _, it := range a.trayItems() {
			l = append(l, it.Label)
			acc[it.Label] = it.Accelerator
		}
		return
	}
	l, acc := labels()
	// Gone's app is missing: not listed.
	want := []string{"Shun", "", "iTerm2", "Safari", "", "Shortcuts Enabled", "Settings…", "Sign In to Sync…", "Check for Updates…", "", "Quit Shun"}
	if !slices.Equal(l, want) {
		t.Fatalf("items %q, want %q", l, want)
	}
	if acc["iTerm2"] != "Alt+C" || acc["Safari"] != "" {
		t.Errorf("accelerators %q", acc)
	}
	a.prefs.SetGlobalEnabled(false)
	if _, acc := labels(); acc["iTerm2"] != "" {
		t.Error("paused shortcuts show their keys")
	}
}

// TestReduceMotionRests checks that, with less motion asked, a recorder
// waiting for keys draws no frames of its own.
func TestReduceMotionRests(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Click("Record Shortcut")
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("frames drawn while the recorder waits")
	}
	if a.ui.recording == "" {
		t.Error("not recording")
	}
}

func TestAccountTab(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 600, 520)
	tt.Click("Account")
	if !tt.HasText("Sync Your Shortcuts") || !tt.HasText("Sign In…") {
		t.Errorf("signed out: texts %q", tt.Texts())
	}
	shot(t, tt, "account")

	a.cloud.signin = &signin{code: "NNCD-QHTC", url: "https://mon.example/device", expires: time.Now().Add(5 * time.Minute),
		cancel: func() {}}
	tt.Frame()
	for _, text := range []string{"NNCD-QHTC", "Open Browser Again", "Cancel"} {
		if !tt.HasText(text) {
			t.Errorf("signing in: no %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "account-code")
	tt.Click("Cancel")
	if a.cloud.signin != nil {
		t.Error("Cancel left the sign-in")
	}

	a.cloud.engine.SignedIn(&mon.User{ID: "u1", Name: "Ada Lovelace", Email: "ada@example.com"})
	tt.Frame()
	for _, text := range []string{"Ada Lovelace", "ada@example.com", "Not synced yet.", "Sync Now", "Sign Out"} {
		if !tt.HasText(text) {
			t.Errorf("signed in: no %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "account-signed-in")
	if items := a.trayItems(); !slices.ContainsFunc(items, func(it *mygo.MenuItem) bool {
		return it.Label == "Syncing as Ada Lovelace…"
	}) {
		t.Error("the menu doesn't name the account")
	}
}

func TestCloudApply(t *testing.T) {
	a := newTestApp(t)
	a.cloud.resolve = func(bundleID, name string) (string, bool) { return "", false }
	l := cloudLocal{a.cloud}
	read, _ := l.Read()
	s := cloudsync.Of(a.store.Shortcuts, nil)
	// The account has Safari first, with keys, iTerm2 not, and Xcode new.
	s.Order = []string{"com.apple.Safari", "com.example.gone", "com.apple.dt.Xcode"}
	delete(s.Shortcuts, "com.googlecode.iterm2")
	s.Shortcuts["com.apple.Safari"] = []byte(`{"name":"Safari","keys":{"carbonKeyCode":1,"carbonModifiers":256},"enabled":true}`)
	s.Shortcuts["com.apple.dt.Xcode"] = []byte(`{"name":"Xcode","enabled":false}`)
	s.Settings = map[string]json.RawMessage{"globalEnabled": json.RawMessage("false")}
	safari := a.store.Shortcuts[1]
	if err := l.Apply(read, s); err != nil {
		t.Fatal(err)
	}
	got := a.store.Shortcuts
	if len(got) != 3 || got[0].ID != safari.ID || got[0].Combo == nil || got[2].DisplayName != "Xcode" || got[2].IsEnabled {
		t.Errorf("shortcuts = %+v", got)
	}
	if a.prefs.GlobalEnabled() {
		t.Error("Enable All Shortcuts didn't sync")
	}
	if err := l.Apply(read, s); err != cloudsync.ErrStale {
		t.Errorf("a second Apply of the same read = %v, want ErrStale", err)
	}
}
