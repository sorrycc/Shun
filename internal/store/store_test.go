package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"shun/internal/keys"
)

// swiftShortcuts is shortcuts.json as the Swift app writes it.
const swiftShortcuts = `[
  {
    "bundleIdentifier" : "com.googlecode.iterm2",
    "bundleURL" : "file:\/\/\/Applications\/iTerm.app\/",
    "displayName" : "iTerm2",
    "id" : "6AC8498A-B41F-435E-9A82-B99AB1CB8376",
    "isEnabled" : true,
    "shortcutName" : "hotapp_6AC8498A-B41F-435E-9A82-B99AB1CB8376"
  },
  {
    "bundleIdentifier" : "com.google.Chrome.canary",
    "bundleURL" : "file:\/\/\/Applications\/Google%20Chrome%20Canary.app\/",
    "displayName" : "Chrome Canary",
    "id" : "516309A5-B870-431A-B994-5CB1C9E49AC1",
    "isEnabled" : false,
    "shortcutName" : "hotapp_516309A5-B870-431A-B994-5CB1C9E49AC1"
  }
]`

func openWith(t *testing.T, content string) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, shortcutsFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AppExists = func(string) bool { return true }
	s.RefreshStaleness()
	return s, dir
}

func TestReadsSwiftShortcuts(t *testing.T) {
	s, _ := openWith(t, swiftShortcuts)
	if s.FirstLaunch() {
		t.Error("not a first launch")
	}
	if len(s.Shortcuts) != 2 {
		t.Fatalf("got %d shortcuts", len(s.Shortcuts))
	}
	sc := s.Shortcuts[1]
	if sc.Path() != "/Applications/Google Chrome Canary.app" {
		t.Errorf("path %q", sc.Path())
	}
	if sc.IsEnabled || sc.DisplayName != "Chrome Canary" || sc.ShortcutName != "hotapp_516309A5-B870-431A-B994-5CB1C9E49AC1" {
		t.Errorf("shortcut %+v", sc)
	}
}

// What the Go app writes must decode as the Swift app's AppShortcut: every
// field it requires, with a URL Foundation reads.
func TestWritesSwiftCompatibleShortcuts(t *testing.T) {
	s, dir := openWith(t, swiftShortcuts)
	s.SetCombo(s.Shortcuts[0].ID, &keys.Combo{KeyCode: 2, Modifiers: keys.CarbonOption})
	data, err := os.ReadFile(filepath.Join(dir, shortcutsFile))
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(dir, shortcutsFile)); info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, want 0644 as the Swift app wrote", info.Mode().Perm())
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "bundleURL", "bundleIdentifier", "displayName", "shortcutName", "isEnabled"} {
		if _, ok := raw[1][field]; !ok {
			t.Errorf("missing %s", field)
		}
	}
	if raw[1]["bundleURL"] != "file:///Applications/Google%20Chrome%20Canary.app/" {
		t.Errorf("bundleURL %v", raw[1]["bundleURL"])
	}
	if _, ok := raw[0]["Stale"]; ok {
		t.Error("staleness is not saved")
	}
}

func TestNewShortcut(t *testing.T) {
	sc := NewShortcut("/Applications/Visual Studio Code.app", "com.microsoft.VSCode", "Code")
	if !regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`).MatchString(sc.ID) {
		t.Errorf("id %q", sc.ID)
	}
	if sc.ShortcutName != "hotapp_"+sc.ID || !sc.IsEnabled {
		t.Errorf("shortcut %+v", sc)
	}
	if sc.BundleURL != "file:///Applications/Visual%20Studio%20Code.app/" || sc.Path() != "/Applications/Visual Studio Code.app" {
		t.Errorf("url %q path %q", sc.BundleURL, sc.Path())
	}
}

func TestAddRemove(t *testing.T) {
	s, _ := openWith(t, "")
	if !s.FirstLaunch() {
		t.Error("first launch")
	}
	changes := 0
	s.OnChange(func() { changes++ })
	a := NewShortcut("/Applications/A.app", "a", "A")
	if !s.Add(a) || s.Add(NewShortcut("/Applications/A.app", "a", "A")) {
		t.Fatal("an app has one shortcut")
	}
	s.Add(NewShortcut("/Applications/B.app", "b", "B"))
	s.Remove(a.ID)
	if len(s.Shortcuts) != 1 || s.Shortcuts[0].BundleIdentifier != "b" || changes != 3 {
		t.Errorf("shortcuts %+v, %d changes", s.Shortcuts, changes)
	}
}

func TestInsertUndoesRemove(t *testing.T) {
	s, _ := openWith(t, "")
	a := NewShortcut("/Applications/A.app", "a", "A")
	a.Combo = &keys.Combo{KeyCode: 0, Modifiers: keys.CarbonOption}
	s.Add(a)
	s.Add(NewShortcut("/Applications/B.app", "b", "B"))
	s.Remove(a.ID)
	s.SetCombo(s.Shortcuts[0].ID, a.Combo) // B takes A's keys meanwhile
	if !s.Insert(0, a) || names(s) != "AB" {
		t.Fatalf("shortcuts %s", names(s))
	}
	if s.Shortcuts[0].Combo != nil || s.Shortcuts[1].Combo == nil {
		t.Error("B keeps the keys it took")
	}
	if s.Insert(0, a) {
		t.Error("A has a shortcut")
	}
}

func names(s *Store) string {
	var n []string
	for _, sc := range s.Shortcuts {
		n = append(n, sc.DisplayName)
	}
	return strings.Join(n, "")
}

func TestMove(t *testing.T) {
	s, _ := openWith(t, "")
	for _, n := range []string{"A", "B", "C", "D"} {
		s.Add(NewShortcut("/Applications/"+n+".app", n, n))
	}
	s.Move([]int{0}, 3) // A before D
	if got := names(s); got != "BCAD" {
		t.Errorf("got %s", got)
	}
	s.Move([]int{3}, 0) // D first
	if got := names(s); got != "DBCA" {
		t.Errorf("got %s", got)
	}
	s.Move([]int{0, 1}, 4) // to the end
	if got := names(s); got != "CADB" {
		t.Errorf("got %s", got)
	}
}

func TestSetComboReassigns(t *testing.T) {
	s, _ := openWith(t, swiftShortcuts)
	c := keys.Combo{KeyCode: 2, Modifiers: keys.CarbonOption}
	if from := s.SetCombo(s.Shortcuts[0].ID, &c); from != "" {
		t.Errorf("reassigned from %q", from)
	}
	if from := s.SetCombo(s.Shortcuts[1].ID, &c); from != "iTerm2" {
		t.Errorf("reassigned from %q", from)
	}
	if s.Shortcuts[0].Combo != nil || *s.Shortcuts[1].Combo != c {
		t.Errorf("combos %v %v", s.Shortcuts[0].Combo, s.Shortcuts[1].Combo)
	}
	s.SetCombo(s.Shortcuts[1].ID, nil)
	if s.Shortcuts[1].Combo != nil {
		t.Error("cleared")
	}
}

func TestStaleness(t *testing.T) {
	s, _ := openWith(t, swiftShortcuts)
	s.AppExists = func(p string) bool { return strings.Contains(p, "iTerm") }
	if !s.RefreshStaleness() || s.Shortcuts[0].Stale || !s.Shortcuts[1].Stale {
		t.Errorf("staleness %v %v", s.Shortcuts[0].Stale, s.Shortcuts[1].Stale)
	}
}

func TestRecipeRoundTrip(t *testing.T) {
	s, _ := openWith(t, swiftShortcuts)
	s.SetCombo(s.Shortcuts[0].ID, &keys.Combo{KeyCode: 2, Modifiers: keys.CarbonOption})
	data, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseRecipe(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 1 || r.Entries[0] != (RecipeEntry{"com.googlecode.iterm2", keys.CarbonOption, "iTerm2", 2}) {
		t.Errorf("recipe %+v", r)
	}
	if _, err := ParseRecipe([]byte(`{"version":2,"entries":[]}`)); err == nil {
		t.Error("a newer recipe is refused")
	}
}

func TestImport(t *testing.T) {
	s, _ := openWith(t, swiftShortcuts)
	s.SetCombo(s.Shortcuts[0].ID, &keys.Combo{KeyCode: 2, Modifiers: keys.CarbonOption})
	r := Recipe{Version: 1, Entries: []RecipeEntry{
		{BundleIdentifier: "com.googlecode.iterm2", DisplayName: "iTerm2", KeyCode: 3, CarbonModifiers: keys.CarbonOption},
		{BundleIdentifier: "missing", DisplayName: "Missing", KeyCode: 4, CarbonModifiers: keys.CarbonOption},
		{BundleIdentifier: "com.apple.Safari", DisplayName: "Safari", KeyCode: 2, CarbonModifiers: keys.CarbonOption},
		{BundleIdentifier: "com.apple.Notes", DisplayName: "Notes", KeyCode: 5, CarbonModifiers: keys.CarbonOption},
		{BundleIdentifier: "com.apple.Mail", DisplayName: "Mail", KeyCode: 5, CarbonModifiers: keys.CarbonOption},
	}}
	res := s.Import(r, func(id, name string) (string, bool) {
		if id == "missing" {
			return "", false
		}
		return "/Applications/" + name + ".app", true
	})
	want := ImportResult{Added: 3, SkippedDuplicate: 1, SkippedNotFound: 1, SkippedKeyConflict: 2}
	if res != want {
		t.Errorf("result %+v", res)
	}
	if got := res.Summary(); got != "Imported 3 of 5 shortcuts (1 duplicate, 1 not found, 2 without keys, taken by other shortcuts)." {
		t.Errorf("summary %q", got)
	}
	notes := s.Shortcuts[slices.IndexFunc(s.Shortcuts, func(sc Shortcut) bool { return sc.DisplayName == "Notes" })]
	if notes.Combo == nil || notes.Combo.KeyCode != 5 {
		t.Errorf("notes %+v", notes)
	}
}

type fakeDefaults map[string]any

func (d fakeDefaults) String(k string) (string, bool)    { v, ok := d[k].(string); return v, ok }
func (d fakeDefaults) Bool(k string) (bool, bool)        { v, ok := d[k].(bool); return v, ok }
func (d fakeDefaults) Strings(k string) ([]string, bool) { v, ok := d[k].([]string); return v, ok }

func TestMigrate(t *testing.T) {
	s, dir := openWith(t, swiftShortcuts)
	p, _ := OpenPrefs(dir)
	d := fakeDefaults{
		"KeyboardShortcuts_hotapp_6AC8498A-B41F-435E-9A82-B99AB1CB8376": `{"carbonKeyCode":2,"carbonModifiers":6912}`,
		"globalEnabled":                 false,
		"telemetry.dauEnabled":          false,
		"telemetry.installId":           "331FBFD3-A23A-43AE-8F2F-0252AA4C66F3",
		"telemetry.uploadedCrashes":     []string{"Shun-2026.ips"},
		"telemetry.crashReportsEnabled": true,
	}
	if !Migrate(s, p, d) {
		t.Fatal("did not migrate")
	}
	if c := s.Shortcuts[0].Combo; c == nil || *c != (keys.Combo{KeyCode: 2, Modifiers: 6912}) {
		t.Errorf("combo %v", c)
	}
	if s.Shortcuts[1].Combo != nil {
		t.Error("the second shortcut had no keys")
	}
	if p.GlobalEnabled() || p.UsageStatsEnabled() || !p.CrashReportsEnabled() {
		t.Error("preferences not imported")
	}
	if p.InstallID() != "331FBFD3-A23A-43AE-8F2F-0252AA4C66F3" || len(p.UploadedCrashes()) != 1 {
		t.Error("telemetry state not imported")
	}
	// Persisted, and once only.
	s2, _ := Open(dir)
	p2, _ := OpenPrefs(dir)
	if s2.Shortcuts[0].Combo == nil || p2.GlobalEnabled() {
		t.Error("migration not saved")
	}
	if Migrate(s2, p2, d) {
		t.Error("migrated twice")
	}
}

func TestPrepareDirMovesHotApp(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "HotApp")
	os.MkdirAll(legacy, 0o755)
	os.WriteFile(filepath.Join(legacy, "shortcuts.json"), []byte(swiftShortcuts), 0o644)
	os.WriteFile(filepath.Join(legacy, "usage.json"), []byte("[]"), 0o644)
	dir := filepath.Join(root, "Shun")
	os.MkdirAll(dir, 0o755) // MyGo creates it before
	if err := PrepareDir(dir); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "shortcuts.json")) || !exists(filepath.Join(dir, "usage.json")) || exists(legacy) {
		t.Error("legacy files not moved")
	}
}
