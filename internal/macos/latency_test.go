//go:build darwin

package macos

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/ebitengine/purego"
)

// TestTapLatency posts key presses of a combo nobody uses (⌃⌥⇧⌘F19), which
// the tap takes, and measures how long they take to reach the handler.
// SHUN_TAP_LATENCY=1 runs it; it needs the Accessibility permission.
func TestTapLatency(t *testing.T) {
	if os.Getenv("SHUN_TAP_LATENCY") == "" {
		t.Skip("SHUN_TAP_LATENCY=1 to run")
	}
	const f19 = 0x50
	got := make(chan time.Time, 1)
	tap, err := StartEventTap(func(code int, flags uint64) bool {
		if code != f19 {
			return false
		}
		got <- time.Now()
		return true
	})
	if err != nil {
		t.Skip(err)
	}
	_ = tap
	create := mustDlsym(libCG, "CGEventCreateKeyboardEvent")
	setFlags := mustDlsym(libCG, "CGEventSetFlags")
	post := mustDlsym(libCG, "CGEventPost")
	var lat []time.Duration
	for range 50 {
		ev, _, _ := purego.SyscallN(create, 0, f19, 1)
		purego.SyscallN(setFlags, ev, 0x1E0000)
		start := time.Now()
		purego.SyscallN(post, 0 /* kCGHIDEventTap */, ev)
		select {
		case at := <-got:
			lat = append(lat, at.Sub(start))
		case <-time.After(time.Second):
			t.Fatal("the key press did not reach the tap")
		}
		release(id(ev))
		time.Sleep(10 * time.Millisecond)
	}
	slices.Sort(lat)
	t.Logf("tap latency over %d presses: p50 %v, p90 %v, max %v", len(lat), lat[len(lat)/2], lat[len(lat)*9/10], lat[len(lat)-1])
}
