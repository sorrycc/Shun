package main

import (
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
	iconBolt     = svg(`<path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/>`)
)

// iconCache draws apps' icons the first time they show, and keeps them.
type iconCache struct {
	bitmaps map[string]*ui.Bitmap // by app path; nil where there is none
	draw    func(path string) *ui.Bitmap
}

func newIconCache() *iconCache {
	return &iconCache{bitmaps: map[string]*ui.Bitmap{}, draw: drawIcon}
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

func (ic *iconCache) get(path string) *ui.Bitmap {
	if b, ok := ic.bitmaps[path]; ok {
		return b
	}
	b := ic.draw(path)
	ic.bitmaps[path] = b
	return b
}

// clear forgets the icons, as the Settings window hides: apps may change.
func (ic *iconCache) clear() { clear(ic.bitmaps) }

// appIcon shows an app's icon, or its initial.
func (a *app) appIcon(c *ui.Context, path, name string, size float32) {
	if b := a.icons.get(path); b != nil {
		ui.Image(c, b).Size(size, size)
	} else {
		ui.Avatar(c, name, nil).Size(size, size)
	}
}
