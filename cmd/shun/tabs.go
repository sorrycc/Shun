package main

import (
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"shun/internal/macos"
)

// generalView is the General tab: login, all shortcuts, the Accessibility
// permission, and updates.
func (a *app) generalView(c *ui.Context) {
	t := c.Theme()
	page(c, func() {
		section(c, "", func() {
			login := a.ui.loginItem
			if switchRow(c, iconPower, tileGray, "Launch at Login", "Start Shun in the menu bar when you log in.", &login) {
				if err := mygo.App.SetOpenAtLogin(login); err != nil {
					log.Printf("login item: %v", err)
					a.alert("Launch at Login", err.Error())
				}
				a.ui.loginItem = mygo.App.OpenAtLogin()
			}
			divider(c, rowIndent)
			if on := a.prefs.GlobalEnabled(); switchRow(c, iconKeyboard, tileBlue, "Enable All Shortcuts",
				"Turn off to pause every shortcut at once.", &on) {
				a.setGlobalEnabled(on)
			}
		})
		section(c, "Permissions", func() {
			trusted := macos.IsTrusted()
			description := "Lets Shun switch apps instantly, before other apps see the keys."
			if !trusted {
				description = "Without it, shortcuts work, but more slowly."
			}
			settingRow(c, iconHand, tileIndigo, "Accessibility", description, func() {
				if trusted {
					statusPill(c, "Granted", t.Success)
					return
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					statusPill(c, "Not granted", t.Warning)
					if ui.Button(c, "Open Settings…").Clicked() {
						mygo.Shell.OpenExternal(accessibilitySettings)
					}
				})
			})
		})
		if mygo.Updater.Enabled() {
			section(c, "Updates", func() {
				if on := updater.AutomaticChecks(); switchRow(c, iconRefresh, tileGreen,
					"Check for updates automatically", "", &on) {
					updater.SetAutomaticChecks(on)
				}
				divider(c, rowIndent)
				settingRow(c, iconBolt, tileOrange, "Current version", mygo.App.Version(), func() {
					if ui.Button(c, "Check Now…").Clicked() {
						a.checkForUpdates()
					}
				})
			})
		}
		about(c)
	})
}

// about signs the General tab: the app's name, version and what it does.
func about(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(3).AlignItems(ui.Center).Padding(8, 0, 0, 0).Children(func() {
		name := "Shun"
		if v := mygo.App.Version(); v != "" {
			name += " " + v
		}
		ui.Text(c, name).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
		ui.Text(c, "瞬 — switch to any app in an instant").FontSize(11).TextColor(t.TextMuted.Alpha(0.75))
	})
}

// grow returns a value of e going to target, from 0 the first time e
// shows, after delay: bars and numbers grow as their page comes in.
func grow(e ui.Element, key any, target float32, delay time.Duration) float32 {
	if seen := ui.Local(e, key, func() bool { return false }); !*seen {
		*seen = true
		e.Jump(key, 0)
	}
	return e.AnimateTo(key, target, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond}, Delay: delay})
}

// insightsView is the Insights tab: how often shortcuts brought apps
// forward, in the last 7 days.
func (a *app) insightsView(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	total := a.usage.Total()
	if total == 0 {
		emptyState(c, iconChart, "No usage yet",
			"Use your shortcuts for a while — how often you switch to each app shows up here.", nil)
		return
	}
	top := a.usage.Top(10)
	days := a.usage.Daily()
	page(c, func() {
		ui.Row(c).Gap(12).Children(func() {
			statTile(c, iconBolt, tileOrange, "Switches", total, "past 7 days", 0)
			statTile(c, iconActivity, tileBlue, "Today", days[len(days)-1], "switches", 60*time.Millisecond)
			statTile(c, iconLayers, tilePurple, "Apps", a.usage.Apps(), "switched to", 120*time.Millisecond)
		})
		section(c, "Last 7 Days", func() {
			dailyChart(c, days)
		})
		section(c, "Most Used", func() {
			most := float32(top[0].Count)
			for i, e := range top {
				name, path := e.BundleIdentifier, ""
				for _, sc := range a.store.Shortcuts {
					if sc.BundleIdentifier == e.BundleIdentifier {
						name, path = sc.DisplayName, sc.Path()
						break
					}
				}
				ui.Row(c.Key(e.BundleIdentifier)).Gap(10).AlignItems(ui.Center).Children(func() {
					rank := ui.Textf(c, "%d", i+1).FontSize(11).FontWeight(600).Width(14).TextAlign(ui.End)
					rank.TextColor(t.TextMuted)
					if i == 0 {
						rank.TextColor(tileOrange)
					}
					if path != "" {
						a.appIcon(c, path, name, 24)
					} else {
						ui.Avatar(c, name, nil).Size(24, 24)
					}
					ui.Text(c, name).Width(120).SingleLine()
					ui.Row(c).Grow(1).Height(8).Radius(4).Background(l.subtle).Children(func() {
						fill := ui.Box(c).Height(8).Radius(4)
						fill.WidthPercent(grow(fill, "w", float32(e.Count)*100/most, time.Duration(i)*35*time.Millisecond))
						fill.LinearGradient(ui.LinearGradient{From: t.Accent.Alpha(0.55), To: t.Accent, Angle: 90})
					})
					ui.Textf(c, "%d", e.Count).FontSize(11).FontWeight(500).TextColor(t.TextMuted).Width(32).TextAlign(ui.End)
				})
			}
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.Button(c, "Clear Usage Data…").TextColor(t.Danger).Clicked() {
				a.ui.confirmClear = true
			}
		})
	})
	if ui.AlertDialog(c, &a.ui.confirmClear, "Clear all usage data?",
		"The counts of the past 7 days are deleted. This cannot be undone.", "Cancel", "Clear") == 1 {
		a.usage.Clear()
	}
}

// statTile is a number of the Insights tab, which counts up as it shows.
func statTile(c *ui.Context, icon *ui.SVG, tint ui.Color, label string, n int, unit string, delay time.Duration) {
	t := c.Theme()
	l := lookOf(t)
	tile := ui.Column(c).Grow(1).Basis(0).Gap(6).Padding(12, 14).Radius(10).Background(l.card).
		Border(1, l.cardBorder).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
	v := grow(tile, "n", float32(n), delay)
	tile.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).FontSize(13).TextColor(tint)
			ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
		})
		ui.Textf(c, "%d", int(v+0.5)).FontSize(24).FontWeight(700).FontFeatures("tnum")
		ui.Text(c, unit).FontSize(11).TextColor(t.TextMuted)
	})
}

// dailyChart shows the switches of each of the last 7 days as bars, today
// at the right in the accent color.
func dailyChart(c *ui.Context, days [7]int) {
	t := c.Theme()
	l := lookOf(t)
	most := 1
	for _, n := range days {
		most = max(most, n)
	}
	const height = 96
	today := time.Now()
	ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
		for i, n := range days {
			day := today.AddDate(0, 0, i-len(days)+1)
			isToday := i == len(days)-1
			ui.Column(c.Key(i)).Grow(1).Basis(0).Gap(6).AlignItems(ui.Center).
				Tooltip(fmt.Sprintf("%s: %d", day.Format("Monday, Jan 2"), n)).Children(func() {
				count := ui.Text(c, fmt.Sprint(n)).FontSize(10).FontWeight(600).TextColor(t.TextMuted)
				if n == 0 {
					count.Invisible()
				}
				ui.Column(c).Height(height).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
					bar := ui.Box(c).Width(22).Radius(5)
					h := grow(bar, "h", float32(n)*height/float32(most), time.Duration(i)*40*time.Millisecond)
					bar.Height(max(h, 3))
					switch {
					case n == 0:
						bar.Background(l.subtle)
					case isToday:
						bar.LinearGradient(ui.LinearGradient{From: t.Accent.Mix(ui.RGB(255, 255, 255), 0.2), To: t.Accent, Angle: 180})
					default:
						bar.Background(t.Accent.Alpha(0.35))
					}
				})
				label := ui.Text(c, day.Format("Mon")).FontSize(10).TextColor(t.TextMuted)
				if isToday {
					label.FontWeight(700).TextColor(t.Text)
				}
			})
		}
	})
}

// privacyView is the Privacy tab: telemetry, which the user can turn off.
func (a *app) privacyView(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	page(c, func() {
		if _, ok := telemetryConfig(); !ok {
			section(c, "", func() {
				settingRow(c, iconShield, tileGray, "Telemetry is off",
					"Telemetry is only active in packaged release builds.", func() {})
			})
			return
		}
		section(c, "Telemetry", func() {
			if on := a.prefs.CrashReportsEnabled(); switchRow(c, iconBug, tileRed, "Send crash reports automatically",
				"When Shun crashes, macOS writes a .ips file under ~/Library/Logs/DiagnosticReports/. "+
					"We upload those on next launch so we can fix the bug.", &on) {
				a.prefs.SetCrashReportsEnabled(on)
			}
			divider(c, rowIndent)
			if on := a.prefs.UsageStatsEnabled(); switchRow(c, iconChart, tileBlue, "Send anonymous usage stats",
				"Sends one anonymous ping per day with your app version, macOS version, and locale. "+
					"Used to count active installs.", &on) {
				a.prefs.SetUsageStatsEnabled(on)
			}
		})
		ui.Row(c).Gap(10).Padding(10, 12).Radius(10).Background(l.subtle).Children(func() {
			ui.Icon(c, iconLock).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "We never send personal data, file contents, keystrokes, or hotkey definitions. "+
				"Data is stored on Cloudflare under our account and auto-deleted after one year.").
				Grow(1).Shrink(1).FontSize(11).TextColor(t.TextMuted).LineHeight(1.35)
		})
		section(c, "Identifier", func() {
			settingRow(c, iconHash, tileGray, "Anonymous ID", "Ties pings and crash reports together, and nothing else.", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					id := a.prefs.InstallID()
					ui.Text(c, id[:min(8, len(id))]+"…").Font("monospace").FontSize(11).TextColor(t.TextMuted).
						Padding(2, 6).Radius(5).Background(l.subtle)
					if ui.Button(c, "Reset…").Label("Reset Anonymous ID").Clicked() {
						a.ui.confirmReset = true
					}
				})
			})
		})
	})
	if ui.AlertDialog(c, &a.ui.confirmReset, "Reset Anonymous ID?",
		"Future pings and crashes will be attributed to a new UUID. This does not change the telemetry toggles above.",
		"Cancel", "Reset") == 1 {
		a.prefs.ResetInstallID()
	}
}
