package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"shun/internal/mon"
	"shun/internal/store"
)

// server is Mon's crash endpoint.
type server struct {
	mu      sync.Mutex
	crashes []mon.Crash
	fail    bool
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path != "/api/v1/crashes" || r.Header.Get("Authorization") != "" {
		http.Error(w, "unexpected", http.StatusBadRequest)
		return
	}
	if s.fail {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	var c mon.Crash
	json.NewDecoder(r.Body).Decode(&c)
	s.crashes = append(s.crashes, c)
	w.Write([]byte("{}"))
}

const ips = `{"app_name":"Shun","app_version":"1.1.0","bundleID":"com.shun.Shun"}
{"procPath":"/Users/ada/Applications/Shun.app/Contents/MacOS/Shun","shared":"/Users/Shared/x"}`

func setup(t *testing.T) (*server, Config, *store.Prefs) {
	srv := &server{}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	crashes := t.TempDir()
	for _, n := range []string{"Shun-2026-10-01.ips", "Shun-2026-10-02.ips", "Other-2026.ips", "Shun-notes.txt"} {
		os.WriteFile(filepath.Join(crashes, n), []byte(ips), 0o644)
	}
	p, _ := store.OpenPrefs(t.TempDir())
	client := mon.New(ts.URL)
	client.Token = "mon_secret" // never sent with a report
	return srv, Config{Client: client, AppVersion: "2.0.0", CrashDir: crashes, CrashPrefix: "Shun-", Home: "/Users/ada"}, p
}

func TestUploadsCrashesOnce(t *testing.T) {
	srv, c, p := setup(t)
	ctx := context.Background()
	UploadCrashes(ctx, c, p)
	UploadCrashes(ctx, c, p)
	if len(srv.crashes) != 2 || len(p.UploadedCrashes()) != 2 {
		t.Fatalf("sent %d, remembered %v", len(srv.crashes), p.UploadedCrashes())
	}
	cr := srv.crashes[0]
	if cr.App != "shun" || cr.Kind != mon.CrashSystem || cr.Version != "1.1.0" || cr.OccurredAt == 0 {
		t.Errorf("crash %+v", cr)
	}
	if strings.Contains(cr.Log, "ada") || !strings.Contains(cr.Log, "~/Applications/Shun.app") ||
		!strings.Contains(cr.Log, "/Users/Shared/x") {
		t.Errorf("log not redacted: %s", cr.Log)
	}
}

func TestFailuresRetry(t *testing.T) {
	srv, c, p := setup(t)
	srv.fail = true
	Run(context.Background(), c, p)
	if len(p.UploadedCrashes()) != 0 {
		t.Error("failures are not remembered as done")
	}
}

func TestOptOut(t *testing.T) {
	srv, c, p := setup(t)
	p.SetCrashReportsEnabled(false)
	Run(context.Background(), c, p)
	if len(srv.crashes) != 0 {
		t.Error("sent despite the opt-out")
	}
}
