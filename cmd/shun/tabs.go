package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

// generalView is the General tab: login, all shortcuts, and updates.
func (a *app) generalView(c *ui.Context) {
	t := c.Theme()
	page(c, func() {
		section(c, "", func() {
			login := a.ui.loginItem
			if switchRow(c, "Launch at Login", "", &login) {
				if err := mygo.App.SetOpenAtLogin(login); err != nil {
					log.Printf("login item: %v", err)
					a.alert("Launch at Login", err.Error())
				}
				a.ui.loginItem = mygo.App.OpenAtLogin()
			}
			ui.Divider(c)
			if on := a.prefs.GlobalEnabled(); switchRow(c, "Enable All Shortcuts", "", &on) {
				a.setGlobalEnabled(on)
			}
		})
		if !mygo.Updater.Enabled() {
			return
		}
		section(c, "Updates", func() {
			settingRow(c, "Current version", "", func() {
				ui.Text(c, mygo.App.Version()).TextColor(t.TextMuted)
			})
			ui.Divider(c)
			if on := updater.AutomaticChecks(); switchRow(c, "Automatically check for updates", "", &on) {
				updater.SetAutomaticChecks(on)
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "Check for Updates…").Clicked() {
					a.checkForUpdates()
				}
			})
		})
	})
}

// insightsView is the Insights tab: how often shortcuts brought apps
// forward, in the last 7 days.
func (a *app) insightsView(c *ui.Context) {
	t := c.Theme()
	total := a.usage.Total()
	if total == 0 {
		emptyState(c, iconChart, "No usage data yet", "Start using your shortcuts — insights will appear here.")
		return
	}
	page(c, func() {
		section(c, "", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBolt).FontSize(16).TextColor(t.Accent)
				ui.RichText(c,
					ui.Span{Text: "You've used shortcuts "},
					ui.Span{Text: fmt.Sprintf("%d times", total), Weight: 700},
					ui.Span{Text: " in the past 7 days"},
				).Grow(1)
			})
		})
		section(c, "Most Used", func() {
			top := a.usage.Top(10)
			most := top[0].Count
			for _, e := range top {
				name, path := e.BundleIdentifier, ""
				for _, sc := range a.store.Shortcuts {
					if sc.BundleIdentifier == e.BundleIdentifier {
						name, path = sc.DisplayName, sc.Path()
						break
					}
				}
				ui.Row(c.Key(e.BundleIdentifier)).Gap(10).AlignItems(ui.Center).Children(func() {
					if path != "" {
						a.appIcon(c, path, name, 24)
					} else {
						ui.Avatar(c, name, nil).Size(24, 24)
					}
					ui.Text(c, name).Width(120).SingleLine()
					ui.Row(c).Grow(1).Height(14).Children(func() {
						ui.Box(c).WidthPercent(float32(e.Count) * 100 / float32(most)).Height(14).Radius(3).
							Background(t.Accent.Alpha(0.6))
					})
					ui.Textf(c, "%d", e.Count).FontSize(11).TextColor(t.TextMuted).Width(36).TextAlign(ui.End)
				})
			}
		})
		ui.Row(c).Children(func() {
			if ui.Button(c, "Clear Usage Data").TextColor(t.Danger).Clicked() {
				a.ui.confirmClear = true
			}
		})
	})
	if ui.AlertDialog(c, &a.ui.confirmClear, "Clear all usage data?", "", "Cancel", "Clear") == 1 {
		a.usage.Clear()
	}
}

// privacyView is the Privacy tab: telemetry, which the user can turn off.
func (a *app) privacyView(c *ui.Context) {
	t := c.Theme()
	page(c, func() {
		if _, ok := telemetryConfig(); !ok {
			section(c, "", func() {
				ui.Text(c, "Telemetry is only active in packaged release builds.").TextColor(t.TextMuted)
			})
			return
		}
		section(c, "Telemetry", func() {
			if on := a.prefs.CrashReportsEnabled(); switchRow(c, "Send crash reports automatically",
				"When Shun crashes, macOS writes a .ips file under ~/Library/Logs/DiagnosticReports/. "+
					"We upload those on next launch so we can fix the bug.", &on) {
				a.prefs.SetCrashReportsEnabled(on)
			}
			ui.Divider(c)
			if on := a.prefs.UsageStatsEnabled(); switchRow(c, "Send anonymous usage stats",
				"Sends one anonymous ping per day with your app version, macOS version, and locale. "+
					"Used to count active installs.", &on) {
				a.prefs.SetUsageStatsEnabled(on)
			}
		})
		ui.Text(c, "We never send personal data, file contents, keystrokes, or hotkey definitions. "+
			"Data is stored on Cloudflare under our account and auto-deleted after one year.").
			FontSize(11).TextColor(t.TextMuted)
		section(c, "Identifier", func() {
			settingRow(c, "Anonymous ID", "", func() {
				id := a.prefs.InstallID()
				ui.Text(c, id[:min(8, len(id))]+"…").Font("monospace").TextColor(t.TextMuted)
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "Reset Anonymous ID").Clicked() {
					a.ui.confirmReset = true
				}
			})
		})
	})
	if ui.AlertDialog(c, &a.ui.confirmReset, "Reset Anonymous ID?",
		"Future pings and crashes will be attributed to a new UUID. This does not change the telemetry toggles above.",
		"Cancel", "Reset") == 1 {
		a.prefs.ResetInstallID()
	}
}
