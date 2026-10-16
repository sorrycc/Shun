package store

import (
	"encoding/json"
	"log"

	"shun/internal/keys"
)

// Defaults reads the UserDefaults of the Swift app (com.shun.Shun).
type Defaults interface {
	String(key string) (string, bool)
	Bool(key string) (bool, bool)
	Strings(key string) ([]string, bool)
}

// Migrate imports what the Swift app kept in UserDefaults, once: the keys
// of each shortcut (KeyboardShortcuts_hotapp_<id>), whether shortcuts are
// on, and the telemetry preferences. UserDefaults are left as they are, so
// that a Swift version still finds them. It reports whether it ran.
func Migrate(s *Store, p *Prefs, d Defaults) bool {
	var done bool
	p.get(func(v *prefsData) { done = v.Migrated })
	if done {
		return false
	}
	imported := 0
	for i, sc := range s.Shortcuts {
		if sc.Combo != nil || sc.ShortcutName == "" {
			continue
		}
		raw, ok := d.String("KeyboardShortcuts_" + sc.ShortcutName)
		if !ok {
			continue
		}
		var c keys.Combo
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			log.Printf("migrate: keys of %s: %v", sc.DisplayName, err)
			continue
		}
		s.Shortcuts[i].Combo = &c
		imported++
	}
	if imported > 0 {
		if err := s.Save(); err != nil {
			log.Printf("migrate: saving shortcuts: %v", err)
		}
	}
	p.set(func(v *prefsData) {
		if on, ok := d.Bool("globalEnabled"); ok {
			v.GlobalEnabled = on
		}
		if on, ok := d.Bool("telemetry.dauEnabled"); ok {
			v.UsageStatsEnabled = on
		}
		if on, ok := d.Bool("telemetry.crashReportsEnabled"); ok {
			v.CrashReportsEnabled = on
		}
		if id, ok := d.String("telemetry.installId"); ok && id != "" {
			v.InstallID = id
		}
		if day, ok := d.String("telemetry.lastPingDate"); ok {
			v.LastPingDate = day
		}
		if names, ok := d.Strings("telemetry.uploadedCrashes"); ok {
			v.UploadedCrashes = names
		}
		v.Migrated = true
	})
	log.Printf("migrate: imported the keys of %d shortcuts from UserDefaults", imported)
	return true
}
