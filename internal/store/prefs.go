package store

import (
	"log"
	"path/filepath"
	"sync"
)

const prefsFile = "settings.json"

// Prefs are Shun's preferences, which the Swift versions kept in
// UserDefaults. They are safe to use from any goroutine.
type Prefs struct {
	mu   sync.Mutex
	path string
	v    prefsData
}

type prefsData struct {
	GlobalEnabled       bool     `json:"globalEnabled"`
	CrashReportsEnabled bool     `json:"crashReportsEnabled"`
	UsageStatsEnabled   bool     `json:"usageStatsEnabled"`
	InstallID           string   `json:"installId,omitempty"`
	LastPingDate        string   `json:"lastPingDate,omitempty"`
	UploadedCrashes     []string `json:"uploadedCrashes,omitempty"`
	// Migrated is set once the Swift app's UserDefaults were imported.
	Migrated bool `json:"migratedFromUserDefaults"`
}

// OpenPrefs loads the preferences of dir, with everything on by default.
func OpenPrefs(dir string) (*Prefs, error) {
	p := &Prefs{path: filepath.Join(dir, prefsFile), v: prefsData{
		GlobalEnabled:       true,
		CrashReportsEnabled: true,
		UsageStatsEnabled:   true,
	}}
	if _, err := readJSON(p.path, &p.v); err != nil {
		log.Printf("prefs: reading %s: %v", p.path, err)
	}
	return p, nil
}

func (p *Prefs) get(fn func(v *prefsData)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn(&p.v)
}

// set changes the preferences and saves them.
func (p *Prefs) set(fn func(v *prefsData)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn(&p.v)
	if err := writeJSON(p.path, p.v); err != nil {
		log.Printf("prefs: saving: %v", err)
	}
}

func (p *Prefs) GlobalEnabled() (on bool) { p.get(func(v *prefsData) { on = v.GlobalEnabled }); return }
func (p *Prefs) SetGlobalEnabled(on bool) { p.set(func(v *prefsData) { v.GlobalEnabled = on }) }

func (p *Prefs) CrashReportsEnabled() (on bool) {
	p.get(func(v *prefsData) { on = v.CrashReportsEnabled })
	return
}
func (p *Prefs) SetCrashReportsEnabled(on bool) {
	p.set(func(v *prefsData) { v.CrashReportsEnabled = on })
}

func (p *Prefs) UsageStatsEnabled() (on bool) {
	p.get(func(v *prefsData) { on = v.UsageStatsEnabled })
	return
}
func (p *Prefs) SetUsageStatsEnabled(on bool) { p.set(func(v *prefsData) { v.UsageStatsEnabled = on }) }

// InstallID returns the anonymous ID of this installation, made the first
// time.
func (p *Prefs) InstallID() (id string) {
	p.get(func(v *prefsData) { id = v.InstallID })
	if id == "" {
		p.set(func(v *prefsData) {
			if v.InstallID == "" {
				v.InstallID = NewID()
			}
			id = v.InstallID
		})
	}
	return id
}

// ResetInstallID replaces the anonymous ID.
func (p *Prefs) ResetInstallID() { p.set(func(v *prefsData) { v.InstallID = NewID() }) }

func (p *Prefs) LastPingDate() (d string) { p.get(func(v *prefsData) { d = v.LastPingDate }); return }
func (p *Prefs) SetLastPingDate(d string) { p.set(func(v *prefsData) { v.LastPingDate = d }) }

func (p *Prefs) UploadedCrashes() (names []string) {
	p.get(func(v *prefsData) { names = append(names, v.UploadedCrashes...) })
	return
}
func (p *Prefs) SetUploadedCrashes(names []string) {
	p.set(func(v *prefsData) { v.UploadedCrashes = append([]string(nil), names...) })
}
