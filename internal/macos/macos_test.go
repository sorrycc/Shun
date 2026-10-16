//go:build darwin

package macos

import (
	"os"
	"testing"
)

func TestRunningApps(t *testing.T) {
	apps := RunningApps()
	found := false
	for _, a := range apps {
		if a.BundleID == "com.apple.finder" && a.PID > 0 {
			found = true
			if !IsRunning(a.PID) {
				t.Error("Finder runs")
			}
			if inst, ok := InstanceOf("com.apple.finder"); !ok || inst.PID != a.PID {
				t.Errorf("instance %+v %v", inst, ok)
			}
		}
	}
	if !found {
		t.Errorf("no Finder among %d apps", len(apps))
	}
	if IsRunning(999999) {
		t.Error("no such process")
	}
}

func TestBundles(t *testing.T) {
	id, name, ok := BundleInfo("/System/Applications/Calculator.app")
	if !ok || id != "com.apple.calculator" || name == "" {
		t.Errorf("Calculator: %q %q %v", id, name, ok)
	}
	if _, _, ok := BundleInfo("/nonexistent.app"); ok {
		t.Error("no bundle")
	}
	if p, ok := AppPath("com.apple.calculator"); !ok || p != "/System/Applications/Calculator.app" {
		t.Errorf("path %q", p)
	}
	if LocaleIdentifier() == "" {
		t.Error("locale")
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults{AppID: "com.apple.finder"}
	if _, ok := d.String("no such key, surely"); ok {
		t.Error("missing key")
	}
	if os.Getenv("SHUN_TEST_DEFAULTS") == "" {
		return
	}
	// With the Swift app's preferences on this Mac.
	s := Defaults{AppID: "com.shun.Shun"}
	id, ok := s.String("telemetry.installId")
	t.Logf("installId %q %v", id, ok)
}

func TestEventTap(t *testing.T) {
	tap, err := StartEventTap(func(code int, flags uint64) bool { return false })
	if err != nil {
		t.Skip(err) // no Accessibility permission for the test binary
	}
	if !tap.Enabled() {
		t.Error("the tap is on")
	}
	tap.SetEnabled(false)
	if tap.Enabled() {
		t.Error("the tap is off")
	}
	tap.SetEnabled(true)
}
