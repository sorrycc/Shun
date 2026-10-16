package store

import (
	"log"
	"path/filepath"
	"slices"

	"shun/internal/keys"
)

const shortcutsFile = "shortcuts.json"

// Store holds the shortcuts, in their order. It is used on the main thread
// only; listeners of OnChange get a copy for other threads.
type Store struct {
	path        string
	Shortcuts   []Shortcut
	firstLaunch bool
	listeners   []func()

	// AppExists reports whether an app is at a path; os.Stat by default.
	AppExists func(path string) bool
}

// Open loads the shortcuts of dir.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, shortcutsFile), AppExists: func(p string) bool { return exists(p) }}
	found, err := readJSON(s.path, &s.Shortcuts)
	s.firstLaunch = !found
	if err != nil {
		log.Printf("store: reading %s: %v", s.path, err)
		s.Shortcuts = nil
	}
	s.RefreshStaleness()
	return s, nil
}

// FirstLaunch reports whether there was no shortcuts file when the store
// opened: Shun runs for the first time.
func (s *Store) FirstLaunch() bool { return s.firstLaunch }

// OnChange adds a function called after every change of the shortcuts.
func (s *Store) OnChange(fn func()) { s.listeners = append(s.listeners, fn) }

// Save writes the shortcuts.
func (s *Store) Save() error { return writeJSON(s.path, s.Shortcuts) }

// changed saves the shortcuts and tells the listeners.
func (s *Store) changed() {
	if err := s.Save(); err != nil {
		log.Printf("store: saving shortcuts: %v", err)
	}
	for _, fn := range s.listeners {
		fn()
	}
}

func (s *Store) index(id string) int {
	return slices.IndexFunc(s.Shortcuts, func(sc Shortcut) bool { return sc.ID == id })
}

// Get returns the shortcut of an ID.
func (s *Store) Get(id string) (Shortcut, bool) {
	if i := s.index(id); i >= 0 {
		return s.Shortcuts[i], true
	}
	return Shortcut{}, false
}

// HasApp reports whether an app has a shortcut.
func (s *Store) HasApp(bundleID string) bool {
	return slices.ContainsFunc(s.Shortcuts, func(sc Shortcut) bool { return sc.BundleIdentifier == bundleID })
}

// Add appends a shortcut, unless its app has one; false when it did not.
func (s *Store) Add(sc Shortcut) bool {
	if s.HasApp(sc.BundleIdentifier) {
		return false
	}
	sc.Stale = !s.AppExists(sc.Path())
	s.Shortcuts = append(s.Shortcuts, sc)
	s.changed()
	return true
}

// Remove deletes the shortcuts of the IDs.
func (s *Store) Remove(ids ...string) {
	n := len(s.Shortcuts)
	s.Shortcuts = slices.DeleteFunc(s.Shortcuts, func(sc Shortcut) bool { return slices.Contains(ids, sc.ID) })
	if len(s.Shortcuts) != n {
		s.changed()
	}
}

// Move moves the shortcuts at rows before the shortcut at index to (len
// for the end), keeping their order, as a list reorders its rows.
func (s *Store) Move(rows []int, to int) {
	if len(rows) == 0 {
		return
	}
	moving := make([]Shortcut, 0, len(rows))
	rest := make([]Shortcut, 0, len(s.Shortcuts))
	sorted := slices.Sorted(slices.Values(rows))
	insert := to
	for i, sc := range s.Shortcuts {
		if slices.Contains(sorted, i) {
			moving = append(moving, sc)
			if i < to {
				insert--
			}
		} else {
			rest = append(rest, sc)
		}
	}
	insert = max(0, min(insert, len(rest)))
	s.Shortcuts = slices.Concat(rest[:insert], moving, rest[insert:])
	s.changed()
}

// SetEnabled turns a shortcut on or off.
func (s *Store) SetEnabled(id string, on bool) {
	if i := s.index(id); i >= 0 && s.Shortcuts[i].IsEnabled != on {
		s.Shortcuts[i].IsEnabled = on
		s.changed()
	}
}

// SetCombo gives a shortcut its keys, or none for nil. Another shortcut that
// had the same keys loses them: its name returns, to tell the user.
func (s *Store) SetCombo(id string, c *keys.Combo) (reassignedFrom string) {
	i := s.index(id)
	if i < 0 {
		return ""
	}
	if c != nil {
		for j := range s.Shortcuts {
			if j != i && s.Shortcuts[j].Combo != nil && *s.Shortcuts[j].Combo == *c {
				s.Shortcuts[j].Combo = nil
				reassignedFrom = s.Shortcuts[j].DisplayName
				log.Printf("store: reassigned %s from %s to %s", c, reassignedFrom, s.Shortcuts[i].DisplayName)
			}
		}
		c = &keys.Combo{KeyCode: c.KeyCode, Modifiers: c.Modifiers}
	}
	s.Shortcuts[i].Combo = c
	s.changed()
	return reassignedFrom
}

// RefreshStaleness marks the shortcuts whose apps are gone.
func (s *Store) RefreshStaleness() bool {
	changed := false
	for i := range s.Shortcuts {
		stale := !s.AppExists(s.Shortcuts[i].Path())
		if stale != s.Shortcuts[i].Stale {
			s.Shortcuts[i].Stale = stale
			changed = true
		}
	}
	return changed
}
