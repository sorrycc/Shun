package main

import (
	"time"

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
	c := keys.Combo{KeyCode: code, Modifiers: carbonMods(ev.Mods)}
	return c, c.Valid()
}

// carbonMods returns the Carbon modifiers of ui's.
func carbonMods(m ui.Modifiers) int {
	mods := 0
	if m&ui.Super != 0 {
		mods |= keys.CarbonCmd
	}
	if m&ui.Ctrl != 0 {
		mods |= keys.CarbonControl
	}
	if m&ui.Alt != 0 {
		mods |= keys.CarbonOption
	}
	if m&ui.Shift != 0 {
		mods |= keys.CarbonShift
	}
	return mods
}

// recorder shows a shortcut's keys and records new ones: click it, then
// press the keys, with a modifier. Escape cancels, Delete clears. While it
// records, it shows the modifiers held; a combo recorded pops.
func (a *app) recorder(c *ui.Context, id string, combo *keys.Combo) {
	t := c.Theme()
	l := lookOf(t)
	box := ui.Box(c.Key("recorder-"+id)).Focusable().Width(150).Height(28).Padding(0, 8).Radius(7).
		Cursor(ui.CursorPointer).FocusRing(false)
	recording := ui.Local(box, "recording", func() bool { return false })
	pops := ui.Local(box, "pops", func() uint64 { return 0 })
	if box.Clicked() {
		*recording = !*recording
	}
	if *recording && !box.Focused() {
		*recording = false // the user went elsewhere
	}
	if *recording {
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
				*pops++
				a.setCombo(id, &combo)
			}
			return true
		})
	}

	border, bg := l.cardBorder, l.card
	switch {
	case *recording:
		// The border breathes while it waits for the keys.
		pulse := box.Loop("pulse", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut))
		border, bg = t.Accent.Alpha(0.55+0.45*pulse), t.Accent.Alpha(0.07).Over(l.card)
	case box.Hovered():
		border = t.Text.Alpha(0.2)
	}
	box.Border(1.5, box.AnimateColor("border", border, quick)).Background(box.AnimateColor("bg", bg, quick))
	if combo == nil && !*recording {
		box.BorderStyle(ui.BorderDashed)
	}
	if *pops > 0 {
		s := box.Keyframes("pop", []ui.Keyframe{{At: 0, Value: 1}, {At: 90 * time.Millisecond, Value: 1.07, Ease: ui.EaseOut},
			{At: 320 * time.Millisecond, Value: 1, Ease: ui.EaseInOut}}, ui.KeyframeOptions{Run: *pops})
		box.Scale(s, s)
	}

	box.Label("Shortcut").Children(func() {
		ui.Row(c).Grow(1).Gap(4).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			switch {
			case *recording:
				if held := keys.ModifierSymbols(carbonMods(c.Modifiers())); len(held) > 0 {
					keycaps(c, held, t.Accent)
				} else {
					ui.Text(c, "Type Shortcut").FontSize(12).TextColor(t.Accent).SingleLine()
				}
			case combo != nil:
				keycaps(c, combo.Parts(), t.Text)
			default:
				ui.Text(c, "Record Shortcut").FontSize(12).TextColor(t.TextMuted).SingleLine()
			}
		})
		if combo != nil && !*recording {
			clearBtn := ui.ButtonBase(c.Key("clear-"+id)).Label("Clear Shortcut").Absolute().Right(6).Top(6).
				Size(16, 16).Radius(8).Center()
			show := float32(0)
			if box.Hovered() || clearBtn.Focused() {
				show = 1
			}
			clearBtn.Opacity(clearBtn.AnimateTo("show", show, quick))
			if clearBtn.Hovered() {
				clearBtn.Background(t.Text.Alpha(0.1))
			}
			clearBtn.Children(func() {
				ui.Icon(c, iconClose).FontSize(10).TextColor(t.TextMuted)
			})
			if clearBtn.Clicked() {
				a.setCombo(id, nil)
			}
		}
	})
}
