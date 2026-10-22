package main

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/keys"
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
	tt.Key(0, ui.KeyK) // no modifier: still waiting
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
	tt.Click("Done")
	if a.ui.picking {
		t.Error("closed")
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
	want := []string{"Shun", "", "iTerm2", "Safari", "", "Shortcuts Enabled", "Settings…", "Check for Updates…", "", "Quit Shun"}
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
