// Package telemetry sends a daily anonymous ping and uploads Shun's crash
// reports to the telemetry worker, both of which the user can turn off in
// Settings → Privacy. Requests are signed with an HMAC of their body.
package telemetry

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"shun/internal/store"
)

// Config says where to send, and about the app.
type Config struct {
	Endpoint   string // e.g. https://telemetry.sorrycc.dev
	IngestKey  string
	AppVersion string
	OSVersion  string
	Locale     string
	// CrashDir holds the crash reports, ~/Library/Logs/DiagnosticReports.
	CrashDir string
	// CrashPrefix starts the names of Shun's reports: "Shun-".
	CrashPrefix string
	Client      *http.Client
}

const (
	appID       = "shun"
	uploadedCap = 500
)

// Run pings and uploads the crash reports the user allows.
func Run(ctx context.Context, c Config, p *store.Prefs) {
	if c.Client == nil {
		c.Client = http.DefaultClient
	}
	if p.CrashReportsEnabled() {
		UploadCrashes(ctx, c, p)
	}
	if p.UsageStatsEnabled() {
		if err := Ping(ctx, c, p); err != nil {
			log.Printf("telemetry: ping: %v", err)
		}
	}
}

// Sign returns the hex HMAC-SHA256 of a body.
func Sign(body []byte, key string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

// Ping sends the daily ping, once a day.
func Ping(ctx context.Context, c Config, p *store.Prefs) error {
	day := today()
	if p.LastPingDate() == day {
		return nil
	}
	// encoding/json sorts map keys, as the Swift app's .sortedKeys did.
	body, err := json.Marshal(map[string]string{
		"app":       appID,
		"installId": p.InstallID(),
		"version":   c.AppVersion,
		"osVersion": c.OSVersion,
		"locale":    c.Locale,
	})
	if err != nil {
		return err
	}
	if err := c.post(ctx, "v1/ping", "application/json", body, 15*time.Second); err != nil {
		return err
	}
	p.SetLastPingDate(day)
	return nil
}

// UploadCrashes uploads the reports not uploaded yet, oldest first.
func UploadCrashes(ctx context.Context, c Config, p *store.Prefs) {
	entries, err := os.ReadDir(c.CrashDir)
	if err != nil {
		return
	}
	type report struct {
		name string
		mod  time.Time
	}
	uploaded := p.UploadedCrashes()
	var pending []report
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, c.CrashPrefix) || filepath.Ext(name) != ".ips" || slices.Contains(uploaded, name) {
			continue
		}
		if info, err := e.Info(); err == nil {
			pending = append(pending, report{name, info.ModTime()})
		}
	}
	if len(pending) == 0 {
		return
	}
	slices.SortFunc(pending, func(a, b report) int { return a.mod.Compare(b.mod) })
	log.Printf("telemetry: uploading %d crash reports", len(pending))
	for _, r := range pending {
		if err := c.uploadCrash(ctx, p, r.name); err != nil {
			log.Printf("telemetry: uploading %s: %v", r.name, err) // again at the next launch
			continue
		}
		uploaded = append(uploaded, r.name)
	}
	if len(uploaded) > uploadedCap {
		uploaded = uploaded[len(uploaded)-uploadedCap:]
	}
	p.SetUploadedCrashes(uploaded)
}

func (c Config) uploadCrash(ctx context.Context, p *store.Prefs, name string) error {
	data, err := os.ReadFile(filepath.Join(c.CrashDir, name))
	if err != nil {
		return err
	}
	meta, err := json.Marshal(map[string]string{
		"app":       appID,
		"installId": p.InstallID(),
		"version":   c.AppVersion,
		"osVersion": c.OSVersion,
		"fileName":  name,
	})
	if err != nil {
		return err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("meta", string(meta)); err != nil {
		return err
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", "application/octet-stream")
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	part.Write(data)
	if err := w.Close(); err != nil {
		return err
	}
	return c.post(ctx, "v1/crash", w.FormDataContentType(), body.Bytes(), 30*time.Second)
}

func (c Config) post(ctx context.Context, path, contentType string, body []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.Endpoint, "/")+"/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Telemetry-Sig", Sign(body, c.IngestKey))
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("status %d: %s", resp.StatusCode, snippet)
	}
	return nil
}
