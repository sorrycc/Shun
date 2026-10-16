package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"shun/internal/store"
)

const key = "test-key"

type server struct {
	mu    sync.Mutex
	pings []map[string]string
	files []string
	fail  bool
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Header.Get("X-Telemetry-Sig") != Sign(body, key) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/v1/ping":
		var m map[string]string
		json.Unmarshal(body, &m)
		s.pings = append(s.pings, m)
	case "/v1/crash":
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, h, err := r.FormFile("file")
		if err != nil || r.FormValue("meta") == "" {
			http.Error(w, "missing parts", http.StatusBadRequest)
			return
		}
		f.Close()
		s.files = append(s.files, h.Filename)
	}
}

func setup(t *testing.T) (*server, Config, *store.Prefs) {
	srv := &server{}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	crashes := t.TempDir()
	for _, n := range []string{"Shun-2026-10-01.ips", "Shun-2026-10-02.ips", "Other-2026.ips", "Shun-notes.txt"} {
		os.WriteFile(filepath.Join(crashes, n), []byte("{}"), 0o644)
	}
	p, _ := store.OpenPrefs(t.TempDir())
	return srv, Config{Endpoint: ts.URL, IngestKey: key, AppVersion: "2.0.0", OSVersion: "26.3", Locale: "en_US",
		CrashDir: crashes, CrashPrefix: "Shun-"}, p
}

func TestPingOncePerDay(t *testing.T) {
	srv, c, p := setup(t)
	ctx := context.Background()
	for range 2 {
		if err := Ping(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	if len(srv.pings) != 1 {
		t.Fatalf("%d pings", len(srv.pings))
	}
	if m := srv.pings[0]; m["app"] != "shun" || m["installId"] != p.InstallID() || m["version"] != "2.0.0" {
		t.Errorf("ping %v", m)
	}
}

func TestUploadsCrashesOnce(t *testing.T) {
	srv, c, p := setup(t)
	ctx := context.Background()
	UploadCrashes(ctx, c, p)
	UploadCrashes(ctx, c, p)
	if len(srv.files) != 2 || len(p.UploadedCrashes()) != 2 {
		t.Errorf("uploaded %v, remembered %v", srv.files, p.UploadedCrashes())
	}
}

func TestFailuresRetry(t *testing.T) {
	srv, c, p := setup(t)
	srv.fail = true
	Run(context.Background(), c, p)
	if p.LastPingDate() != "" || len(p.UploadedCrashes()) != 0 {
		t.Error("failures are not remembered as done")
	}
}

func TestOptOut(t *testing.T) {
	srv, c, p := setup(t)
	p.SetUsageStatsEnabled(false)
	p.SetCrashReportsEnabled(false)
	Run(context.Background(), c, p)
	if len(srv.pings)+len(srv.files) != 0 {
		t.Error("sent despite the opt-out")
	}
}
