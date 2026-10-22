package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"shun/internal/store"
)

// BenchmarkFrames draws each tab of Settings with 40 shortcuts and a week
// of usage, as a frame of an animation does.
func BenchmarkFrames(b *testing.B) {
	a := newTestApp(b)
	for i := range 37 {
		a.store.Add(store.NewShortcut(fmt.Sprintf("/Applications/App%d.app", i), fmt.Sprintf("com.example.app%d", i),
			fmt.Sprintf("App %d", i)))
	}
	for i := range 2000 {
		a.usage.Log(a.store.Shortcuts[i%len(a.store.Shortcuts)].BundleIdentifier)
	}
	for _, tab := range []string{"Shortcuts", "Insights"} {
		b.Run(tab, func(b *testing.B) {
			tt := ui.NewTester(a.view, 600, 520)
			tt.Click(tab)
			tt.Advance(2 * time.Second)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				tt.Move(300, float32(200+i%2*40)) // a hover, which draws again
				tt.Frame()
			}
		})
	}
}
