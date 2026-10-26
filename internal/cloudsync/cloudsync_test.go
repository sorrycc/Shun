package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"shun/internal/keys"
	"shun/internal/mon"
	"shun/internal/store"
)

// fakeMon is Mon's settings API for one person, as Mon answers it.
type fakeMon struct {
	mu      sync.Mutex
	version int64
	data    json.RawMessage
	revoked bool
	puts    int
	// beforePut runs before a PUT is answered, as another Mac saving.
	beforePut func()
}

func (f *fakeMon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if f.revoked {
		f.mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method == "PUT" && f.beforePut != nil {
		hook := f.beforePut
		f.beforePut = nil
		f.mu.Unlock()
		hook()
		f.mu.Lock()
	}
	defer f.mu.Unlock()
	doc := func() map[string]any {
		return map[string]any{"app": App, "version": f.version, "data": f.data}
	}
	switch r.Method {
	case "GET":
		json.NewEncoder(w).Encode(doc())
	case "PUT":
		var body struct {
			BaseVersion int64           `json:"base_version"`
			Data        json.RawMessage `json:"data"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.BaseVersion != f.version {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{"error": "version_conflict", "current": doc()})
			return
		}
		f.puts++
		f.version++
		f.data = body.Data
		json.NewEncoder(w).Encode(doc())
	}
}

// save saves s as another Mac would.
func (f *fakeMon) save(s Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data, _ = json.Marshal(s)
	f.version++
}

func (f *fakeMon) snapshot(t *testing.T) Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, err := decode(f.data)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// fakeLocal is a Mac's shortcuts and preferences.
type fakeLocal struct {
	s       Snapshot
	applies int
	// beforeApply runs before an Apply, as the person changing a shortcut
	// as a sync runs.
	beforeApply func()
}

func (l *fakeLocal) Read() (Snapshot, error) { return l.s.clone(), nil }

func (l *fakeLocal) Apply(read, s Snapshot) error {
	if hook := l.beforeApply; hook != nil {
		l.beforeApply = nil
		hook()
	}
	if !l.s.Equal(read) {
		return ErrStale
	}
	l.s = s
	l.applies++
	return nil
}

func (l *fakeLocal) IsDefault(s Snapshot) bool { return len(s.Shortcuts) == 0 }
func (l *fakeLocal) Known(key string) bool     { return key != "future" }

// entry is a shortcut's JSON: its name, a key code with Command, or -1 for
// no keys, and whether it is on.
func entry(name string, key int, on bool) json.RawMessage {
	e := Entry{Name: name, Enabled: on}
	if key >= 0 {
		e.Keys = &keys.Combo{KeyCode: key, Modifiers: keys.CarbonCmd}
	}
	data, _ := json.Marshal(e)
	return data
}

// snap is a snapshot of shortcuts by bundle ID, in their order.
func snap(kv ...any) Snapshot {
	s := Snapshot{Shortcuts: map[string]json.RawMessage{}}
	for i := 0; i < len(kv); i += 2 {
		id := kv[i].(string)
		s.Shortcuts[id] = kv[i+1].(json.RawMessage)
		s.Order = append(s.Order, id)
	}
	return s
}

func newEngine(t *testing.T, f *fakeMon, l *fakeLocal) *Engine {
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := mon.New(srv.URL)
	c.Token = "mon_test"
	e := New(c, l, filepath.Join(t.TempDir(), "sync.json"))
	if err := e.SignedIn(&mon.User{ID: "u1", Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMerge(t *testing.T) {
	base := snap("safari", entry("Safari", 1, true), "notes", entry("Notes", 2, true), "mail", entry("Mail", 3, true))
	// Here Safari moved to the end and Notes went; there Mail changed keys
	// and Xcode came.
	local := snap("mail", entry("Mail", 3, true), "safari", entry("Safari", 1, false))
	remote := snap("safari", entry("Safari", 1, true), "notes", entry("Notes", 2, true), "mail", entry("Mail", 4, true),
		"xcode", entry("Xcode", 5, true))
	merged := Merge(base, local, remote)
	want := snap("mail", entry("Mail", 4, true), "safari", entry("Safari", 1, false), "xcode", entry("Xcode", 5, true))
	if !merged.Equal(want) {
		t.Errorf("merged = %s %v, want %s %v", merged.Shortcuts, merged.Order, want.Shortcuts, want.Order)
	}
}

func TestMergeGivesKeysToOneApp(t *testing.T) {
	base := snap("safari", entry("Safari", -1, true), "chrome", entry("Chrome", -1, true))
	local := snap("safari", entry("Safari", -1, true), "chrome", entry("Chrome", 1, true))
	remote := snap("safari", entry("Safari", 1, true), "chrome", entry("Chrome", -1, true))
	merged := Merge(base, local, remote)
	want := snap("safari", entry("Safari", -1, true), "chrome", entry("Chrome", 1, true))
	if !merged.Equal(want) {
		t.Errorf("merged = %s, want this Mac's change to keep the keys", merged.Shortcuts)
	}
}

func TestFirstSyncUploadsToAnEmptyAccount(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	res, err := e.Sync(context.Background(), Ask)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pushed || res.Applied {
		t.Errorf("result = %+v", res)
	}
	if got := f.snapshot(t); !got.Equal(l.s) {
		t.Errorf("account = %+v, want %+v", got, l.s)
	}
	if st := e.State(); st.Version != 1 || st.Base == nil || st.SyncedAt.IsZero() {
		t.Errorf("state = %+v", st)
	}
}

func TestFirstSyncOfANewMacTakesTheAccounts(t *testing.T) {
	f := &fakeMon{}
	f.save(snap("safari", entry("Safari", 1, true)))
	l := &fakeLocal{}
	e := newEngine(t, f, l)
	res, err := e.Sync(context.Background(), Ask)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Pushed || !l.s.Equal(f.snapshot(t)) {
		t.Errorf("result = %+v, local = %+v", res, l.s)
	}
}

func TestFirstSyncAsksWhenBothHaveShortcuts(t *testing.T) {
	f := &fakeMon{}
	f.save(snap("safari", entry("Safari", 1, true)))
	l := &fakeLocal{s: snap("notes", entry("Notes", 2, true))}
	e := newEngine(t, f, l)
	if _, err := e.Sync(context.Background(), Ask); !errors.Is(err, ErrNeedsChoice) {
		t.Fatalf("err = %v, want ErrNeedsChoice", err)
	}
	if _, err := e.Sync(context.Background(), UseAccount); err != nil {
		t.Fatal(err)
	}
	if !l.s.Equal(snap("safari", entry("Safari", 1, true))) {
		t.Errorf("UseAccount: local = %s", l.s.Shortcuts)
	}

	l2 := &fakeLocal{s: snap("notes", entry("Notes", 2, true))}
	e2 := newEngine(t, f, l2)
	if _, err := e2.Sync(context.Background(), UseLocal); err != nil {
		t.Fatal(err)
	}
	if got := f.snapshot(t); !got.Equal(l2.s) {
		t.Errorf("UseLocal: account = %s", got.Shortcuts)
	}
}

func TestSyncMergesBothSides(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	ctx := context.Background()
	if _, err := e.Sync(ctx, Ask); err != nil {
		t.Fatal(err)
	}
	f.save(snap("safari", entry("Safari", 1, true), "notes", entry("Notes", 2, true)))
	l.s.Shortcuts["safari"] = entry("Safari", 3, true)
	if _, err := e.Sync(ctx, Ask); err != nil {
		t.Fatal(err)
	}
	want := snap("safari", entry("Safari", 3, true), "notes", entry("Notes", 2, true))
	if !l.s.Equal(want) || !f.snapshot(t).Equal(want) {
		t.Errorf("local = %s, account = %s", l.s.Shortcuts, f.snapshot(t).Shortcuts)
	}
}

func TestPushWithoutChangesAsksNothing(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	ctx := context.Background()
	e.Sync(ctx, Ask)
	f.revoked = true // any request would fail
	if _, err := e.Push(ctx); err != nil {
		t.Errorf("push = %v", err)
	}
}

func TestSyncRetriesAConflict(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	ctx := context.Background()
	e.Sync(ctx, Ask)
	l.s.Shortcuts["safari"] = entry("Safari", 3, true)
	f.beforePut = func() { f.save(snap("safari", entry("Safari", 1, true), "notes", entry("Notes", 2, true))) }
	if _, err := e.Push(ctx); err != nil {
		t.Fatal(err)
	}
	want := snap("safari", entry("Safari", 3, true), "notes", entry("Notes", 2, true))
	if !f.snapshot(t).Equal(want) || !l.s.Equal(want) {
		t.Errorf("account = %s, local = %s", f.snapshot(t).Shortcuts, l.s.Shortcuts)
	}
}

func TestAChangeAsItSyncsStays(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	ctx := context.Background()
	e.Sync(ctx, Ask)
	f.save(snap("safari", entry("Safari", 1, true), "notes", entry("Notes", 2, true)))
	l.beforeApply = func() { l.s.Shortcuts["safari"] = entry("Safari", 1, false) }
	if _, err := e.Sync(ctx, Ask); err != nil {
		t.Fatal(err)
	}
	want := snap("safari", entry("Safari", 1, false), "notes", entry("Notes", 2, true))
	if !l.s.Equal(want) || !f.snapshot(t).Equal(want) {
		t.Errorf("local = %s, account = %s", l.s.Shortcuts, f.snapshot(t).Shortcuts)
	}
}

func TestUnknownPreferencesStay(t *testing.T) {
	f := &fakeMon{}
	s := snap("safari", entry("Safari", 1, true))
	s.Settings = map[string]json.RawMessage{"globalEnabled": json.RawMessage("true"), "future": json.RawMessage("1")}
	f.save(s)
	l := &fakeLocal{}
	e := newEngine(t, f, l)
	ctx := context.Background()
	e.Sync(ctx, Ask)
	// A Shun that doesn't know "future" leaves it out of what it reads.
	delete(l.s.Settings, "future")
	l.s.Settings["globalEnabled"] = json.RawMessage("false")
	if _, err := e.Sync(ctx, Ask); err != nil {
		t.Fatal(err)
	}
	got := f.snapshot(t).Settings
	if string(got["future"]) != "1" || string(got["globalEnabled"]) != "false" {
		t.Errorf("account settings = %s", got)
	}
}

func TestRevokedTokenForgetsTheAccount(t *testing.T) {
	f := &fakeMon{revoked: true}
	e := newEngine(t, f, &fakeLocal{})
	if _, err := e.Sync(context.Background(), Ask); !errors.Is(err, mon.ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if e.State().UserID != "" {
		t.Error("the account is still known")
	}
}

func TestStateLastsBetweenLaunches(t *testing.T) {
	f := &fakeMon{}
	l := &fakeLocal{s: snap("safari", entry("Safari", 1, true))}
	e := newEngine(t, f, l)
	e.Sync(context.Background(), Ask)
	again := New(e.Client, l, e.path)
	if st := again.State(); st.UserID != "u1" || st.Version != 1 || st.Base == nil || !st.Base.Equal(l.s) {
		t.Errorf("state = %+v", st)
	}
}

func TestShortcutsRoundTrip(t *testing.T) {
	safari := store.NewShortcut("/Applications/Safari.app", "com.apple.Safari", "Safari")
	safari.Combo = &keys.Combo{KeyCode: 1, Modifiers: keys.CarbonCmd}
	notes := store.NewShortcut("/System/Applications/Notes.app", "com.apple.Notes", "Notes")
	notes.IsEnabled = false
	s := Of([]store.Shortcut{safari, notes}, nil)
	if !slices.Equal(s.Order, []string{"com.apple.Safari", "com.apple.Notes"}) {
		t.Fatalf("order = %v", s.Order)
	}

	// On another Mac, which has Safari's shortcut, not Notes, and no Xcode.
	other := store.NewShortcut("/Applications/Safari.app", "com.apple.Safari", "Safari")
	s.Shortcuts["com.apple.dt.Xcode"] = entry("Xcode", 5, true)
	s.Order = append(s.Order, "com.apple.dt.Xcode")
	resolve := func(bundleID, name string) (string, bool) {
		if bundleID == "com.apple.Notes" {
			return "/System/Applications/Notes.app", true
		}
		return "", false
	}
	got := ShortcutsOf(s, []store.Shortcut{other}, resolve)
	if len(got) != 3 {
		t.Fatalf("got %d shortcuts", len(got))
	}
	if got[0].ID != other.ID || got[0].Combo == nil || *got[0].Combo != *safari.Combo {
		t.Errorf("Safari = %+v, want its ID kept and the keys taken", got[0])
	}
	if got[1].ID == notes.ID || got[1].Path() != "/System/Applications/Notes.app" || got[1].IsEnabled ||
		got[1].ShortcutName != "hotapp_"+got[1].ID {
		t.Errorf("Notes = %+v", got[1])
	}
	if got[2].Path() != "/Applications/Xcode.app" || got[2].BundleURL != "file:///Applications/Xcode.app/" {
		t.Errorf("Xcode = %+v", got[2])
	}
	if again := Of(got, nil); !maps.EqualFunc(again.Shortcuts, s.Shortcuts, sameJSON) {
		t.Errorf("snapshot again = %s, want %s", again.Shortcuts, s.Shortcuts)
	}
}
