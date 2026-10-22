// Package keys describes the key combos of shortcuts: a macOS virtual key
// code and Carbon modifier flags, the format KeyboardShortcuts stored them in
// for the Swift versions of Shun, and recipes still use.
package keys

import "strings"

// Carbon modifier flags (Carbon.HIToolbox).
const (
	CarbonCmd     = 1 << 8
	CarbonShift   = 1 << 9
	CarbonOption  = 1 << 11
	CarbonControl = 1 << 12

	carbonMask = CarbonCmd | CarbonShift | CarbonOption | CarbonControl
)

// CGEventFlags of the modifiers, as an event tap sees them.
const (
	FlagShift   uint64 = 0x20000
	FlagControl uint64 = 0x40000
	FlagOption  uint64 = 0x80000
	FlagCommand uint64 = 0x100000

	// FlagMask keeps only Control, Option, Shift and Command of an event's flags.
	FlagMask = FlagShift | FlagControl | FlagOption | FlagCommand
)

// Combo is a key with its modifiers. The JSON names are KeyboardShortcuts'.
type Combo struct {
	KeyCode   int `json:"carbonKeyCode"`
	Modifiers int `json:"carbonModifiers"`
}

// Valid reports whether the combo has a key and at least one modifier,
// which a global shortcut needs.
func (c Combo) Valid() bool {
	return c.KeyCode >= 0 && c.Modifiers&carbonMask != 0
}

// CGFlags returns the event tap flags of the combo's modifiers.
func (c Combo) CGFlags() uint64 {
	var f uint64
	if c.Modifiers&CarbonCmd != 0 {
		f |= FlagCommand
	}
	if c.Modifiers&CarbonShift != 0 {
		f |= FlagShift
	}
	if c.Modifiers&CarbonControl != 0 {
		f |= FlagControl
	}
	if c.Modifiers&CarbonOption != 0 {
		f |= FlagOption
	}
	return f
}

// String spells the combo as macOS menus do, such as ⌃⌥⇧⌘K.
func (c Combo) String() string { return strings.Join(c.Parts(), "") }

// Parts returns the symbols of the combo's modifiers, in the order of
// macOS menus, then its key's label: ⌃ ⌥ ⇧ ⌘ K.
func (c Combo) Parts() []string {
	parts := ModifierSymbols(c.Modifiers)
	if k, ok := byCode[c.KeyCode]; ok {
		return append(parts, k.label)
	}
	return append(parts, "?")
}

// ModifierSymbols returns the symbols of Carbon modifiers, as menus show
// them: ⌃ ⌥ ⇧ ⌘.
func ModifierSymbols(mods int) []string {
	var s []string
	for _, m := range []struct {
		flag   int
		symbol string
	}{{CarbonControl, "⌃"}, {CarbonOption, "⌥"}, {CarbonShift, "⇧"}, {CarbonCmd, "⌘"}} {
		if mods&m.flag != 0 {
			s = append(s, m.symbol)
		}
	}
	return s
}

// Accelerator returns the combo as a MyGo accelerator, such as
// "Alt+Shift+K", for mygo.GlobalShortcut; false when MyGo cannot register
// the key.
func (c Combo) Accelerator() (string, bool) {
	k, ok := byCode[c.KeyCode]
	if !ok || k.name == "" || !c.Valid() {
		return "", false
	}
	var parts []string
	if c.Modifiers&CarbonCmd != 0 {
		parts = append(parts, "Cmd")
	}
	if c.Modifiers&CarbonControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Modifiers&CarbonOption != 0 {
		parts = append(parts, "Alt")
	}
	if c.Modifiers&CarbonShift != 0 {
		parts = append(parts, "Shift")
	}
	name := k.name
	if len(name) == 1 {
		name = strings.ToUpper(name)
	}
	return strings.Join(append(parts, name), "+"), true
}

// CodeForName returns the virtual key code of a key named as in
// accelerators: "a", "1", "Space", "F5", "Left", ";".
func CodeForName(name string) (int, bool) {
	k, ok := byName[name]
	return k.code, ok
}

type key struct {
	code  int
	name  string // accelerator name
	label string // what menus show
}

// table lists the keys of an ANSI keyboard by their virtual key codes.
var table = []key{
	{0x00, "a", "A"}, {0x01, "s", "S"}, {0x02, "d", "D"}, {0x03, "f", "F"}, {0x04, "h", "H"},
	{0x05, "g", "G"}, {0x06, "z", "Z"}, {0x07, "x", "X"}, {0x08, "c", "C"}, {0x09, "v", "V"},
	{0x0B, "b", "B"}, {0x0C, "q", "Q"}, {0x0D, "w", "W"}, {0x0E, "e", "E"}, {0x0F, "r", "R"},
	{0x10, "y", "Y"}, {0x11, "t", "T"}, {0x12, "1", "1"}, {0x13, "2", "2"}, {0x14, "3", "3"},
	{0x15, "4", "4"}, {0x16, "6", "6"}, {0x17, "5", "5"}, {0x18, "=", "="}, {0x19, "9", "9"},
	{0x1A, "7", "7"}, {0x1B, "-", "-"}, {0x1C, "8", "8"}, {0x1D, "0", "0"}, {0x1E, "]", "]"},
	{0x1F, "o", "O"}, {0x20, "u", "U"}, {0x21, "[", "["}, {0x22, "i", "I"}, {0x23, "p", "P"},
	{0x25, "l", "L"}, {0x26, "j", "J"}, {0x27, "'", "'"}, {0x28, "k", "K"}, {0x29, ";", ";"},
	{0x2A, "\\", "\\"}, {0x2B, ",", ","}, {0x2C, "/", "/"}, {0x2D, "n", "N"}, {0x2E, "m", "M"},
	{0x2F, ".", "."}, {0x32, "`", "`"},
	{0x24, "Enter", "↩"}, {0x30, "Tab", "⇥"}, {0x31, "Space", "Space"}, {0x33, "Backspace", "⌫"},
	{0x35, "Escape", "⎋"}, {0x75, "Delete", "⌦"}, {0x73, "Home", "↖"}, {0x77, "End", "↘"},
	{0x74, "PageUp", "⇞"}, {0x79, "PageDown", "⇟"},
	{0x7B, "Left", "←"}, {0x7C, "Right", "→"}, {0x7D, "Down", "↓"}, {0x7E, "Up", "↑"},
	{0x7A, "F1", "F1"}, {0x78, "F2", "F2"}, {0x63, "F3", "F3"}, {0x76, "F4", "F4"},
	{0x60, "F5", "F5"}, {0x61, "F6", "F6"}, {0x62, "F7", "F7"}, {0x64, "F8", "F8"},
	{0x65, "F9", "F9"}, {0x6D, "F10", "F10"}, {0x67, "F11", "F11"}, {0x6F, "F12", "F12"},
	{0x69, "F13", "F13"}, {0x6B, "F14", "F14"}, {0x71, "F15", "F15"}, {0x6A, "F16", "F16"},
	{0x40, "F17", "F17"}, {0x4F, "F18", "F18"}, {0x50, "F19", "F19"}, {0x5A, "F20", "F20"},
	{0x52, "Num0", "Num0"}, {0x53, "Num1", "Num1"}, {0x54, "Num2", "Num2"}, {0x55, "Num3", "Num3"},
	{0x56, "Num4", "Num4"}, {0x57, "Num5", "Num5"}, {0x58, "Num6", "Num6"}, {0x59, "Num7", "Num7"},
	{0x5B, "Num8", "Num8"}, {0x5C, "Num9", "Num9"}, {0x41, "NumDec", "Num."}, {0x43, "NumMult", "Num*"},
	{0x45, "NumAdd", "Num+"}, {0x4B, "NumDiv", "Num/"}, {0x4E, "NumSub", "Num-"},
	{0x4C, "", "⌤"}, {0x51, "", "Num="}, {0x47, "", "⌧"},
}

var (
	byCode = map[int]key{}
	byName = map[string]key{}
)

func init() {
	for _, k := range table {
		byCode[k.code] = k
		if k.name != "" {
			byName[k.name] = k
		}
	}
}
