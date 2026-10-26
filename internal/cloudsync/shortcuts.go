package cloudsync

import (
	"encoding/json"
	"strings"

	"shun/internal/keys"
	"shun/internal/store"
)

// Of returns the snapshot of shortcuts, in their order, and of the
// preferences that sync. Of two shortcuts of one app, the first counts.
func Of(shortcuts []store.Shortcut, settings map[string]json.RawMessage) Snapshot {
	s := Snapshot{Shortcuts: map[string]json.RawMessage{}, Settings: settings}
	for _, sc := range shortcuts {
		if _, ok := s.Shortcuts[sc.BundleIdentifier]; ok || sc.BundleIdentifier == "" {
			continue
		}
		e := Entry{Name: sc.DisplayName, Enabled: sc.IsEnabled}
		if sc.Combo != nil {
			e.Keys = &keys.Combo{KeyCode: sc.Combo.KeyCode, Modifiers: sc.Combo.Modifiers}
		}
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		s.Shortcuts[sc.BundleIdentifier] = data
		s.Order = append(s.Order, sc.BundleIdentifier)
	}
	return s
}

// ShortcutsOf returns the shortcuts of s, in its order: those of have, by
// bundle ID, keep their IDs and paths, and the others are new, at the path
// resolve finds for their app, or where it would be in /Applications when
// it is not on this Mac.
func ShortcutsOf(s Snapshot, have []store.Shortcut, resolve func(bundleID, name string) (string, bool)) []store.Shortcut {
	byApp := map[string]store.Shortcut{}
	for _, sc := range have {
		if _, ok := byApp[sc.BundleIdentifier]; !ok {
			byApp[sc.BundleIdentifier] = sc
		}
	}
	out := []store.Shortcut{}
	for _, id := range s.Order {
		var e Entry
		if data, ok := s.Shortcuts[id]; !ok || json.Unmarshal(data, &e) != nil {
			continue
		}
		sc, ok := byApp[id]
		if !ok {
			path, found := resolve(id, e.Name)
			if !found {
				path = "/Applications/" + strings.ReplaceAll(e.Name, "/", ":") + ".app"
			}
			sc = store.NewShortcut(path, id, e.Name)
		}
		sc.DisplayName, sc.IsEnabled, sc.Combo = e.Name, e.Enabled, e.Keys
		out = append(out, sc)
	}
	return out
}
