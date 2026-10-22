package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func svg(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Icons, after Lucide's.
var (
	iconPlus     = svg(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconMinus    = svg(`<path d="M5 12h14"/>`)
	iconImport   = svg(`<path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/>`)
	iconExport   = svg(`<path d="M12 3v12"/><path d="m17 8-5-5-5 5"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>`)
	iconKeyboard = svg(`<path d="M10 8h.01"/><path d="M12 12h.01"/><path d="M14 8h.01"/><path d="M16 12h.01"/><path d="M18 8h.01"/><path d="M6 8h.01"/><path d="M7 16h10"/><path d="M8 12h.01"/><rect width="20" height="16" x="2" y="4" rx="2"/>`)
	iconChart    = svg(`<path d="M3 3v16a2 2 0 0 0 2 2h16"/><path d="M18 17V9"/><path d="M13 17V5"/><path d="M8 17v-3"/>`)
	iconWarning  = svg(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	iconCheck    = svg(`<circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/>`)
	iconClose    = svg(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconSearch   = svg(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	iconSliders  = svg(`<path d="M21 4h-7M10 4H3M21 12h-9M8 12H3M21 20h-5M12 20H3M14 2v4M8 10v4M16 18v4"/>`)
	iconShield   = svg(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)
	iconPower    = svg(`<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.77.04"/>`)
	iconRefresh  = svg(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	iconHand     = svg(`<circle cx="16" cy="4" r="1"/><path d="m18 19 1-7-6 1"/><path d="m5 8 3-3 5.5 3-2.36 3.5"/><path d="M4.24 14.5a5 5 0 0 0 6.88 6"/><path d="M13.76 17.5a5 5 0 0 0-6.88-6"/>`)
	iconCommand  = svg(`<path d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3"/>`)
	iconActivity = svg(`<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2"/>`)
	iconLayers   = svg(`<path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z"/><path d="m22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65"/><path d="m22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65"/>`)
	iconBug      = svg(`<path d="m8 2 1.88 1.88"/><path d="M14.12 3.88 16 2"/><path d="M9 7.13v-1a3 3 0 1 1 6 0v1"/><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"/><path d="M12 20v-9"/><path d="M6.53 9C4.6 8.8 3 7.1 3 5"/><path d="M6 13H2"/><path d="M3 21c0-2.1 1.7-3.9 3.8-4"/><path d="M20.97 5c0 2.1-1.6 3.8-3.5 4"/><path d="M22 13h-4"/><path d="M17.2 17c2.1.1 3.8 1.9 3.8 4"/>`)
	iconHash     = svg(`<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18"/>`)
	iconLock     = svg(`<rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	iconGrip     = svg(`<circle cx="9" cy="12" r="1"/><circle cx="9" cy="5" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="15" cy="19" r="1"/>`)
	iconPause    = svg(`<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>`)
	iconBolt     = svg(`<path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/>`)
)

// iconCache draws apps' icons the first time they show, and keeps them.
// With post set, it draws them in the background, which keeps the main
// thread free while a list of apps scrolls: post runs a function on it
// and redraws.
type iconCache struct {
	bitmaps map[string]*ui.Bitmap // by app path; nil where there is none
	loading map[string]bool
	stale   map[string]bool // kept from before, to draw again
	draw    func(path string) *ui.Bitmap
	post    func(fn func())
	slots   chan struct{} // icons drawn at once
}

func newIconCache() *iconCache {
	return &iconCache{bitmaps: map[string]*ui.Bitmap{}, loading: map[string]bool{}, stale: map[string]bool{},
		draw: drawIcon, slots: make(chan struct{}, 4)}
}

const iconPoints = 32

// drawIcon draws an app's icon for the sharpest display.
func drawIcon(path string) *ui.Bitmap {
	scale := 2.0
	for _, d := range mygo.Screen.Displays() {
		scale = max(scale, d.ScaleFactor)
	}
	data, err := mygo.Shell.FileIcon(path, int(iconPoints*scale))
	if err != nil {
		return nil
	}
	b, _ := ui.DecodeBitmap(data)
	return b
}

// get returns the icon of the app at path, and whether it is known yet.
// An icon kept from before shows while it is drawn again.
func (ic *iconCache) get(path string) (*ui.Bitmap, bool) {
	if b, ok := ic.bitmaps[path]; ok {
		if ic.stale[path] && ic.post != nil {
			delete(ic.stale, path)
			ic.load(path)
		}
		return b, true
	}
	if ic.post == nil {
		b := ic.draw(path)
		ic.bitmaps[path] = b
		return b, true
	}
	ic.load(path)
	return nil, false
}

// load draws the icon of the app at path in the background.
func (ic *iconCache) load(path string) {
	if ic.loading[path] {
		return
	}
	ic.loading[path] = true
	go func() {
		ic.slots <- struct{}{}
		b := ic.draw(path)
		<-ic.slots
		ic.post(func() {
			delete(ic.loading, path)
			if old := ic.bitmaps[path]; b != nil || old == nil {
				ic.bitmaps[path] = b
			}
		})
	}()
}

// clear forgets the icons as the Settings window hides, but those of the
// apps at keep, the shortcuts', which show at once as it opens again, and
// are drawn again then, in case their apps changed.
func (ic *iconCache) clear(keep []string) {
	kept := map[string]*ui.Bitmap{}
	for _, p := range keep {
		if b := ic.bitmaps[p]; b != nil {
			kept[p] = b
		}
	}
	clear(ic.bitmaps)
	clear(ic.stale)
	for p, b := range kept {
		ic.bitmaps[p], ic.stale[p] = b, true
	}
}

// appIcon shows an app's icon, which fades in once drawn: a tile until
// then, and its initial if it has none.
func (a *app) appIcon(c *ui.Context, path, name string, size float32) {
	switch b, known := a.icons.get(path); {
	case b != nil:
		ui.Image(c.Key("icon"), b).Size(size, size).
			Transition(ui.ElementTransition{Duration: 180 * time.Millisecond, Enter: &ui.Motion{Scale: 0.9}})
	case !known:
		ui.Box(c.Key("icon")).Size(size, size).Radius(size * 0.22).Background(lookOf(c.Theme()).subtle)
	default:
		ui.Avatar(c.Key("icon"), name, nil).Size(size, size)
	}
}
