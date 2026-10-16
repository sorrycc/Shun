package keys

import "testing"

func TestString(t *testing.T) {
	cases := []struct {
		c    Combo
		want string
	}{
		{Combo{KeyCode: 8, Modifiers: CarbonOption}, "⌥C"},
		{Combo{KeyCode: 2, Modifiers: 6912}, "⌃⌥⇧⌘D"}, // the hyper key
		{Combo{KeyCode: 0x31, Modifiers: CarbonCmd | CarbonShift}, "⇧⌘Space"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestCGFlags(t *testing.T) {
	c := Combo{KeyCode: 2, Modifiers: CarbonCmd | CarbonShift | CarbonOption | CarbonControl}
	if got := c.CGFlags(); got != FlagMask {
		t.Errorf("hyper flags %#x, want %#x", got, FlagMask)
	}
	if got := (Combo{Modifiers: CarbonOption}).CGFlags(); got != FlagOption {
		t.Errorf("option flags %#x", got)
	}
}

func TestAccelerator(t *testing.T) {
	acc, ok := Combo{KeyCode: 0x28, Modifiers: CarbonOption | CarbonShift}.Accelerator()
	if !ok || acc != "Alt+Shift+K" {
		t.Errorf("got %q %v", acc, ok)
	}
	if _, ok := (Combo{KeyCode: 0x28}).Accelerator(); ok {
		t.Error("a combo without modifiers has no accelerator")
	}
}

func TestCodeForName(t *testing.T) {
	if code, ok := CodeForName("k"); !ok || code != 0x28 {
		t.Errorf("k: %d %v", code, ok)
	}
	if code, ok := CodeForName("Space"); !ok || code != 0x31 {
		t.Errorf("Space: %d %v", code, ok)
	}
}
