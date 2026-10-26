package mon

import "context"

// The kinds of crash report.
const (
	// CrashFatal is a crash that ended the app.
	CrashFatal = "fatal"
	// CrashRecovered is a panic the app recovered from.
	CrashRecovered = "recovered"
	// CrashSystem is the operating system's report of a crash, such as a
	// macOS .ips file.
	CrashSystem = "ips"
)

// Crash is a crash report. App is the app's client id, Kind one of the
// kinds above, OccurredAt a Unix time and Log the crash's log, at most
// 256 KB, with the home folder's paths taken out.
type Crash struct {
	App        string `json:"app"`
	Version    string `json:"version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Runtime    string `json:"runtime"`
	Kind       string `json:"kind"`
	OccurredAt int64  `json:"occurred_at"`
	Log        string `json:"log"`
}

// SendCrash sends a crash report to Mon. It is anonymous: it never sends
// the token, so it works signed out, and Mon doesn't know whose it is.
func (c *Client) SendCrash(ctx context.Context, crash Crash) error {
	anon := *c
	anon.Token = ""
	return anon.do(ctx, "POST", "/api/v1/crashes", crash, nil)
}
