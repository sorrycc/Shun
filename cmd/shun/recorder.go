package main

import (
	"github.com/egoist/mygo/ui"

	"shun/internal/keys"
)

// keyNames names the keys a shortcut can use, as in accelerators, by
// their place on a US keyboard.
var keyNames = map[ui.Key]string{
	ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeySpace: "Space", ui.KeyBackspace: "Backspace",
	ui.KeyEscape: "Escape", ui.KeyDelete: "Delete", ui.KeyHome: "Home", ui.KeyEnd: "End",
	ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
	ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	ui.KeyF1: "F1", ui.KeyF2: "F2", ui.KeyF3: "F3", ui.KeyF4: "F4", ui.KeyF5: "F5", ui.KeyF6: "F6",
	ui.KeyF7: "F7", ui.KeyF8: "F8", ui.KeyF9: "F9", ui.KeyF10: "F10", ui.KeyF11: "F11", ui.KeyF12: "F12",
	ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/",
	ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
	ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
}

func init() {
	for k := ui.KeyA; k <= ui.KeyZ; k++ {
		keyNames[k] = string(rune('a' + int(k-ui.KeyA)))
	}
	for k := ui.Key0; k <= ui.Key9; k++ {
		keyNames[k] = string(rune('0' + int(k-ui.Key0)))
	}
}

// comboOf returns the combo of a key press: the key by its place (Option
// changes what letters type), with at least one modifier.
func comboOf(ev ui.InputEvent) (keys.Combo, bool) {
	key := ev.PhysicalKey
	if key == ui.KeyUnknown {
		key = ev.Key
	}
	name, ok := keyNames[key]
	if !ok {
		return keys.Combo{}, false
	}
	code, ok := keys.CodeForName(name)
	if !ok {
		return keys.Combo{}, false
	}
	mods := 0
	if ev.Mods&ui.Super != 0 {
		mods |= keys.CarbonCmd
	}
	if ev.Mods&ui.Ctrl != 0 {
		mods |= keys.CarbonControl
	}
	if ev.Mods&ui.Alt != 0 {
		mods |= keys.CarbonOption
	}
	if ev.Mods&ui.Shift != 0 {
		mods |= keys.CarbonShift
	}
	c := keys.Combo{KeyCode: code, Modifiers: mods}
	return c, c.Valid()
}

// recorder shows a shortcut's keys and records new ones: click it, then
// press the keys, with a modifier. Escape cancels, Delete clears.
func (a *app) recorder(c *ui.Context, id string, combo *keys.Combo) {
	t := c.Theme()
	box := ui.Box(c.Key("recorder-"+id)).Focusable().Width(136).Padding(5, 10).Radius(6).
		Border(1, t.Border).Background(t.Background).Cursor(ui.CursorPointer)
	recording := ui.Local(box, "recording", func() bool { return false })
	if box.Clicked() {
		*recording = !*recording
	}
	if *recording && !box.Focused() {
		*recording = false // the user went elsewhere
	}
	if *recording {
		box.Border(2, t.Accent)
		box.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown {
				return false
			}
			switch {
			case ev.Mods == 0 && ev.Key == ui.KeyEscape:
				*recording = false
			case ev.Mods == 0 && (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete):
				*recording = false
				a.setCombo(id, nil)
			default:
				combo, ok := comboOf(ev)
				if !ok {
					return true // waits for a key with a modifier
				}
				*recording = false
				a.setCombo(id, &combo)
			}
			return true
		})
	}
	label := "Record Shortcut"
	color := t.TextMuted
	switch {
	case *recording:
		label, color = "Type Shortcut", t.Accent
	case combo != nil:
		label, color = combo.String(), t.Text
	}
	box.Label("Shortcut").Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).TextColor(color).Grow(1).TextAlign(ui.Center).SingleLine()
			if combo != nil && !*recording {
				clearBtn := ui.ButtonBase(c.Key("clear-" + id)).Label("Clear Shortcut").Children(func() {
					ui.Icon(c, iconClose).FontSize(11).TextColor(t.TextMuted)
				})
				if clearBtn.Clicked() {
					a.setCombo(id, nil)
				}
			}
		})
	})
}
