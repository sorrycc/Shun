// Package usage logs the apps that shortcuts bring forward, for the last 7
// days, in usage.json as the Swift versions of Shun wrote it.
package usage

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	keep = 7 * 24 * time.Hour
	// dedupWindow collapses autorepeat bursts: the same app within it is
	// one activation.
	dedupWindow = 500 * time.Millisecond
	saveDelay   = 300 * time.Millisecond
)

// Event is an activation of an app by a shortcut.
type Event struct {
	Timestamp        time.Time
	BundleIdentifier string
}

// eventJSON is an event as the Swift app encoded it: an ISO 8601 date
// without fractions of a second, which its decoder requires.
type eventJSON struct {
	Timestamp        string `json:"timestamp"`
	BundleIdentifier string `json:"bundleIdentifier"`
}

// Store holds the events. It is safe to use from any goroutine.
type Store struct {
	mu     sync.Mutex
	path   string
	events []Event
	counts map[string]int // the events of each app
	// rev counts the changes of the events, which the Insights tab's
	// numbers, asked on every frame it draws, are kept for.
	rev    uint64
	top    []AppCount // all apps, most first, as of topRev
	topRev uint64
	daily  struct {
		rev   uint64
		today time.Time
		days  [7]int
	}
	timer    *time.Timer
	onChange func()
	now      func() time.Time
}

// Open loads the events of dir, without those older than 7 days and the
// repeats of autorepeat bursts.
func Open(dir string) *Store {
	s := &Store{path: filepath.Join(dir, "usage.json"), now: time.Now}
	s.events = load(s.path)
	s.prune()
	s.events = dedup(s.events)
	s.recount()
	return s
}

func load(path string) []Event {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var raw []eventJSON
	if err == nil {
		err = json.Unmarshal(data, &raw)
	}
	if err != nil {
		log.Printf("usage: reading %s: %v", path, err)
		return nil
	}
	events := make([]Event, 0, len(raw))
	for _, e := range raw {
		t, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		events = append(events, Event{Timestamp: t, BundleIdentifier: e.BundleIdentifier})
	}
	return events
}

// OnChange sets a function called, on any goroutine, after an event is
// logged or the events are cleared.
func (s *Store) OnChange(fn func()) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// Log records an activation of an app now, unless the same app was the
// last one, within half a second.
func (s *Store) Log(bundleID string) {
	s.mu.Lock()
	now := s.now()
	if n := len(s.events); n > 0 {
		last := s.events[n-1]
		if last.BundleIdentifier == bundleID && now.Sub(last.Timestamp) < dedupWindow {
			s.mu.Unlock()
			return
		}
	}
	s.events = append(s.events, Event{Timestamp: now, BundleIdentifier: bundleID})
	s.counts[bundleID]++
	s.rev++
	s.scheduleSave()
	fn := s.onChange
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Count returns the activations of an app.
func (s *Store) Count(bundleID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[bundleID]
}

// Total returns the number of activations.
func (s *Store) Total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// Apps returns the number of apps activated.
func (s *Store) Apps() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.counts)
}

// AppCount is the number of activations of an app.
type AppCount struct {
	BundleIdentifier string
	Count            int
}

// Top returns the apps activated most, most first.
func (s *Store) Top(limit int) []AppCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.top == nil || s.topRev != s.rev {
		s.top = make([]AppCount, 0, len(s.counts))
		for id, n := range s.counts {
			s.top = append(s.top, AppCount{id, n})
		}
		slices.SortFunc(s.top, func(a, b AppCount) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.BundleIdentifier, b.BundleIdentifier))
		})
		s.topRev = s.rev
	}
	return slices.Clone(s.top[:min(limit, len(s.top))])
}

// Daily returns the activations of each of the last 7 days, by the local
// calendar, today last. They are counted again only after a change, or
// on a new day.
func (s *Store) Daily() [7]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	d := &s.daily
	if d.rev == s.rev && d.today.Equal(today) {
		return d.days
	}
	var days [7]int
	for _, e := range s.events {
		t := e.Timestamp.In(now.Location())
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
		// Rounded, for the days that daylight saving time makes 23 or 25 hours.
		ago := int((today.Sub(day) + 12*time.Hour) / (24 * time.Hour))
		if ago >= 0 && ago < len(days) {
			days[len(days)-1-ago]++
		}
	}
	d.rev, d.today, d.days = s.rev, today, days
	return days
}

// Clear deletes every event, and the file.
func (s *Store) Clear() {
	s.mu.Lock()
	s.events = nil
	clear(s.counts)
	s.rev++
	if s.timer != nil {
		s.timer.Stop()
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("usage: %v", err)
	}
	fn := s.onChange
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// SaveNow writes the events at once, as the app quits.
func (s *Store) SaveNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.save()
}

func (s *Store) scheduleSave() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(saveDelay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.save()
	})
}

// save writes the events; s.mu is held.
func (s *Store) save() {
	s.prune()
	raw := make([]eventJSON, len(s.events))
	for i, e := range s.events {
		raw[i] = eventJSON{e.Timestamp.UTC().Format(time.RFC3339), e.BundleIdentifier}
	}
	data, err := json.Marshal(raw)
	if err == nil {
		tmp := s.path + ".tmp"
		if err = os.WriteFile(tmp, data, 0o644); err == nil {
			err = os.Rename(tmp, s.path)
		}
	}
	if err != nil {
		log.Printf("usage: saving: %v", err)
	}
}

func (s *Store) prune() {
	cutoff := s.now().Add(-keep)
	n := len(s.events)
	s.events = slices.DeleteFunc(s.events, func(e Event) bool { return e.Timestamp.Before(cutoff) })
	if len(s.events) != n {
		s.recount()
	}
}

// recount counts the events of each app again; s.mu is held.
func (s *Store) recount() {
	s.counts = make(map[string]int)
	for _, e := range s.events {
		s.counts[e.BundleIdentifier]++
	}
	s.rev++
}

// dedup removes the activations of an app within half a second of its
// previous one, from before Log collapsed them.
func dedup(events []Event) []Event {
	out := events[:0]
	for _, e := range events {
		if n := len(out); n > 0 && out[n-1].BundleIdentifier == e.BundleIdentifier &&
			e.Timestamp.Sub(out[n-1].Timestamp) < dedupWindow {
			continue
		}
		out = append(out, e)
	}
	return out
}
