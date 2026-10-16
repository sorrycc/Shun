package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/keys"
	"shun/internal/store"
	"shun/internal/usage"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AppExists = func(p string) bool { return p != "/Applications/Gone.app" }
	p, _ := store.OpenPrefs(dir)
	a := &app{store: s, prefs: p, usage: usage.Open(dir), icons: newIconCache()}
	a.icons.draw = func(string) *ui.Bitmap { return nil }
	a.ui.init()
	iterm := store.NewShortcut("/Applications/iTerm.app", "com.googlecode.iterm2", "iTerm2")
	iterm.Combo = &keys.Combo{KeyCode: 8, Modifiers: keys.CarbonOption}
	s.Add(iterm)
	s.Add(store.NewShortcut("/Applications/Safari.app", "com.apple.Safari", "Safari"))
	s.Add(store.NewShortcut("/Applications/Gone.app", "com.example.gone", "Gone"))
	return a
}

// shot saves the tester's frame to $SHUN_SHOTS/name.png, to look at.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHUN_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestShortcutsTab(t *testing.T) {
	a := newTestApp(t)
	a.usage.Log("com.googlecode.iterm2")
	tt := ui.NewTester(a.view, 580, 480)
	for _, text := range []string{"iTerm2", "Safari", "⌥C", "Record Shortcut", "App not found", "1× past 7 days"} {
		if !tt.HasText(text) {
			t.Errorf("no %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "shortcuts")
}

func TestRecordShortcut(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 580, 480)
	safari := a.store.Shortcuts[1]
	if err := tt.Click("Record Shortcut"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Type Shortcut") {
		t.Fatalf("not recording: %q", tt.Texts())
	}
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
	tt := ui.NewTester(a.view, 580, 480)
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
	tt := ui.NewTester(a.view, 580, 480)
	if err := tt.Click("Enable Safari"); err != nil {
		t.Fatal(err)
	}
	if a.store.Shortcuts[1].IsEnabled {
		t.Error("Safari is off")
	}
	tt.Click("Safari")
	if err := tt.Click("Remove Shortcut"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Remove shortcut for Safari?") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	shot(t, tt, "remove")
	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if len(a.store.Shortcuts) != 2 || a.store.HasApp("com.apple.Safari") {
		t.Errorf("shortcuts %+v", a.store.Shortcuts)
	}
}

func TestEmptyStates(t *testing.T) {
	a := newTestApp(t)
	a.store.Remove(a.store.Shortcuts[0].ID, a.store.Shortcuts[1].ID, a.store.Shortcuts[2].ID)
	tt := ui.NewTester(a.view, 580, 480)
	if !tt.HasText("No shortcuts configured") {
		t.Errorf("texts %q", tt.Texts())
	}
	tt.Click("Insights")
	if !tt.HasText("No usage data yet") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestOtherTabs(t *testing.T) {
	a := newTestApp(t)
	a.usage.Log("com.googlecode.iterm2")
	a.usage.Log("com.apple.Safari")
	tt := ui.NewTester(a.view, 580, 480)

	tt.Click("Insights")
	for _, text := range []string{"Most Used", "2 times", "Clear Usage Data"} {
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
}

func TestImportRecipeFile(t *testing.T) {
	a := newTestApp(t)
	path := filepath.Join(t.TempDir(), "x.hotrecipe")
	os.WriteFile(path, []byte(`{"version":9,"entries":[]}`), 0o644)
	tt := ui.NewTester(a.view, 580, 480)
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
	tt := ui.NewTester(a.view, 580, 480)
	if err := tt.Click("Add App"); err != nil {
		t.Fatal(err)
	}
	tt.Type("cal")
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
