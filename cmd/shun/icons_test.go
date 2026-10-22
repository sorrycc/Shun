package main

import (
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestIconCacheKeepsShortcutIcons(t *testing.T) {
	ic := newIconCache()
	var mu sync.Mutex
	draws := map[string]int{}
	icon := &ui.Bitmap{}
	ic.draw = func(p string) *ui.Bitmap {
		mu.Lock()
		draws[p]++
		mu.Unlock()
		return icon
	}
	posted := make(chan func(), 8)
	ic.post = func(fn func()) { posted <- fn }
	// drain runs what the icons drawn post, as the main thread would.
	drain := func() {
		for len(ic.loading) > 0 {
			(<-posted)()
		}
	}

	for _, p := range []string{"/A.app", "/B.app"} {
		if _, known := ic.get(p); known {
			t.Errorf("%s known before it is drawn", p)
		}
	}
	drain()
	ic.clear([]string{"/A.app"})
	if b, known := ic.get("/A.app"); !known || b != icon {
		t.Error("the shortcut's icon is kept")
	}
	if _, known := ic.get("/B.app"); known {
		t.Error("other icons are forgotten")
	}
	drain()
	if draws["/A.app"] != 2 {
		t.Errorf("the kept icon is drawn again once: %d draws", draws["/A.app"])
	}
	ic.get("/A.app")
	if len(ic.loading) != 0 {
		t.Error("drawn again only once")
	}
}
