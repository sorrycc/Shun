// Package telemetry sends Shun's crash reports to Mon, which the user can
// turn off in Settings → Privacy. A report is anonymous: it goes without
// the account's token, and with the home folder taken out.
package telemetry

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"shun/internal/mon"
	"shun/internal/store"
)

// Config says where to send, and about the app.
type Config struct {
	Client     *mon.Client
	AppVersion string
	// CrashDir holds the crash reports, ~/Library/Logs/DiagnosticReports.
	CrashDir string
	// CrashPrefix starts the names of Shun's reports: "Shun-".
	CrashPrefix string
	// Home is the home folder, which reports leave out.
	Home string
}

const (
	appID       = "shun"
	uploadedCap = 500
	// maxReport is the most of a report Mon takes, in bytes.
	maxReport = 256 << 10
)

// Run sends the crash reports, if the user allows it.
func Run(ctx context.Context, c Config, p *store.Prefs) {
	if p.CrashReportsEnabled() {
		UploadCrashes(ctx, c, p)
	}
}

// UploadCrashes sends the reports not sent yet, oldest first.
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
	log.Printf("telemetry: sending %d crash reports", len(pending))
	for _, r := range pending {
		data, err := os.ReadFile(filepath.Join(c.CrashDir, r.name))
		if err == nil {
			sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = c.Client.SendCrash(sctx, c.crash(string(data), r.mod))
			cancel()
		}
		if err != nil {
			log.Printf("telemetry: sending %s: %v", r.name, err) // again at the next launch
			continue
		}
		uploaded = append(uploaded, r.name)
	}
	if len(uploaded) > uploadedCap {
		uploaded = uploaded[len(uploaded)-uploadedCap:]
	}
	p.SetUploadedCrashes(uploaded)
}

// crash is the report of an .ips file's text, written at.
func (c Config) crash(text string, at time.Time) mon.Crash {
	cr := mon.Crash{
		App: appID, Version: c.AppVersion, OS: runtime.GOOS, Arch: runtime.GOARCH, Runtime: runtime.Version(),
		Kind: mon.CrashSystem, OccurredAt: at.Unix(), Log: redactHome(text, c.Home),
	}
	// An .ips report's first line is JSON about it, with the version of
	// Shun that crashed, maybe older than this one.
	first, _, _ := strings.Cut(text, "\n")
	var ips struct {
		AppVersion string `json:"app_version"`
	}
	if json.Unmarshal([]byte(first), &ips) == nil && ips.AppVersion != "" {
		cr.Version = ips.AppVersion
	}
	if cr.Version == "" {
		cr.Version = "unknown" // Mon wants one
	}
	if len(cr.Log) > maxReport {
		cr.Log = strings.ToValidUTF8(cr.Log[:maxReport], "")
	}
	return cr
}

var homePath = regexp.MustCompile(`/Users/[^/\s"':]+`)

// redactHome takes the home folders out of a report, as ~, so that it
// says nothing of whose it is. /Users/Shared is no one's.
func redactHome(text, home string) string {
	if len(home) > 1 {
		text = strings.ReplaceAll(text, home, "~")
	}
	return homePath.ReplaceAllStringFunc(text, func(m string) string {
		if m == "/Users/Shared" {
			return m
		}
		return "~"
	})
}
