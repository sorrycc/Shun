package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadsSwiftUsageAndPrunes(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	data := `[{"timestamp":"` + old + `","bundleIdentifier":"a"},` +
		`{"timestamp":"` + recent + `","bundleIdentifier":"b"},` +
		`{"timestamp":"` + recent + `","bundleIdentifier":"b"}]`
	os.WriteFile(filepath.Join(dir, "usage.json"), []byte(data), 0o644)
	s := Open(dir)
	if s.Total() != 1 || s.Count("b") != 1 || s.Count("a") != 0 {
		t.Errorf("total %d, b %d, a %d", s.Total(), s.Count("b"), s.Count("a"))
	}
}

func TestLogDedupsAndSaves(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	clock := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	changes := 0
	s.OnChange(func() { changes++ })
	s.Log("a")
	clock = clock.Add(100 * time.Millisecond)
	s.Log("a") // a repeat
	clock = clock.Add(time.Second)
	s.Log("a")
	s.Log("b")
	if s.Total() != 3 || changes != 3 {
		t.Errorf("total %d, changes %d", s.Total(), changes)
	}
	top := s.Top(10)
	if len(top) != 2 || top[0] != (AppCount{"a", 2}) {
		t.Errorf("top %+v", top)
	}
	s.SaveNow()
	data, _ := os.ReadFile(filepath.Join(dir, "usage.json"))
	// Swift's ISO 8601 decoder takes no fractions of a second.
	if !strings.Contains(string(data), `"timestamp":"2026-10-16T12:00:00Z"`) || strings.Contains(string(data), ".1") {
		t.Errorf("saved %s", data)
	}
	s.Clear()
	if s.Total() != 0 {
		t.Error("cleared")
	}
	if _, err := os.Stat(filepath.Join(dir, "usage.json")); err == nil {
		t.Error("file kept")
	}
}

func TestDaily(t *testing.T) {
	s := Open(t.TempDir())
	clock := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)
	s.now = func() time.Time { return clock }
	s.Log("a")
	clock = clock.Add(2 * 24 * time.Hour) // two days later
	s.Log("a")
	s.Log("b")
	clock = clock.Add(5 * time.Hour)
	if days := s.Daily(); days != [7]int{0, 0, 0, 0, 1, 0, 2} {
		t.Errorf("days %v", days)
	}
	if s.Count("a") != 2 || s.Count("b") != 1 {
		t.Errorf("a %d, b %d", s.Count("a"), s.Count("b"))
	}
	s.Clear()
	if s.Count("a") != 0 || len(s.Top(10)) != 0 {
		t.Error("counts kept")
	}
}
