package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"shun/internal/cloudsync"
	"shun/internal/store"
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
			trusted := a.trusted()
			description := "Lets Shun switch apps instantly, before other apps see the keys."
			if !trusted {
				description = "Without it, shortcuts work, but more slowly."
			}
			settingRow(c, iconHand, tileIndigo, "Accessibility", description, func() {
				// The status pops as it changes, as when the permission comes.
				in := ui.ElementTransition{Duration: 260 * time.Millisecond, Enter: &ui.Motion{Scale: 0.8}}
				if trusted {
					ui.Box(c.Key("granted")).Transition(in).Children(func() { statusPill(c, "Granted", t.Success) })
					return
				}
				ui.Row(c.Key("not-granted")).Gap(8).AlignItems(ui.Center).Transition(in).Children(func() {
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

// about signs the General tab: the app's mark, name, version and what it
// does.
func about(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(4).AlignItems(ui.Center).Padding(10, 0, 4, 0).Children(func() {
		mark := ui.Box(c).Size(40, 40).Radius(10).Center().Margin(0, 0, 4, 0).
			LinearGradient(ui.LinearGradient{From: t.Accent.Mix(ui.RGB(255, 255, 255), 0.25), To: t.Accent, Angle: 180}).
			Shadow(0, 2, 6, 0, t.Accent.Alpha(0.3))
		pose := ui.Pose{}
		if mark.Hovered() {
			pose = ui.Pose{Scale: 1.08, Rotate: -4}
		}
		mark.AnimatePose(pose, ui.Bouncy)
		mark.Children(func() {
			ui.Text(c, "瞬").FontSize(20).FontWeight(700).TextColor(ui.RGB(255, 255, 255))
		})
		name := "Shun"
		if v := mygo.App.Version(); v != "" {
			name += " " + v
		}
		ui.Text(c, name).FontSize(12).FontWeight(600)
		ui.Text(c, "Switch to any app in an instant").FontSize(11).TextColor(t.TextMuted)
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
			statTile(c, iconActivity, tileBlue, "Today", days[len(days)-1],
				againstAverage(float32(days[len(days)-1]), float32(total)/float32(len(days))), 60*time.Millisecond)
			statTile(c, iconLayers, tilePurple, "Apps", a.usage.Apps(), "switched to", 120*time.Millisecond)
		})
		section(c, "Last 7 Days", func() {
			dailyChart(c, days)
		})
		section(c, "Most Used", func() {
			most := float32(top[0].Count)
			byID := make(map[string]*store.Shortcut, len(a.store.Shortcuts))
			for i := range a.store.Shortcuts {
				sc := &a.store.Shortcuts[i]
				if _, ok := byID[sc.BundleIdentifier]; !ok {
					byID[sc.BundleIdentifier] = sc
				}
			}
			for i, e := range top {
				name, path := e.BundleIdentifier, ""
				if sc, ok := byID[e.BundleIdentifier]; ok {
					name, path = sc.DisplayName, sc.Path()
				}
				row := ui.Row(c.Key(e.BundleIdentifier)).Gap(10).AlignItems(ui.Center).Padding(3, 6).Margin(-3, -6).Radius(6)
				hover := ui.Color{}
				if row.Hovered() {
					hover = l.hover
				}
				row.Background(row.AnimateColor("hover", hover, quick)).Children(func() {
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
					ui.Textf(c, "%d", e.Count).FontSize(11).FontWeight(600).FontFeatures("tnum").Width(32).TextAlign(ui.End)
					ui.Textf(c, "%.0f%%", float32(e.Count)*100/float32(total)).FontSize(11).FontFeatures("tnum").
						TextColor(t.TextMuted).Width(34).TextAlign(ui.End)
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

// perDay says an average, with a decimal while it is small.
func perDay(n float32) string {
	if n >= 10 {
		return fmt.Sprintf("%.0f", n)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", n), ".0")
}

// againstAverage says how today compares with the daily average.
func againstAverage(today, avg float32) string {
	switch d := today - avg; {
	case d >= 0.05:
		return perDay(d) + " above average"
	case d <= -0.05:
		return perDay(-d) + " below average"
	}
	return "on average"
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
	total := 0
	for _, n := range days {
		total += n
	}
	avg := float32(total) / float32(len(days)) * height / float32(most)
	today := time.Now()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		ui.Box(c).Width(12).Height(1).Background(t.Text.Alpha(0.3))
		ui.Text(c, "Daily average "+perDay(float32(total)/float32(len(days)))).FontSize(10).TextColor(t.TextMuted)
		ui.Box(c).Size(8, 8).Radius(2).Margin(0, 0, 0, 6).Background(t.Accent)
		ui.Text(c, "Today").FontSize(10).TextColor(t.TextMuted)
	})
	// The columns touch, which keeps the average line through them whole.
	ui.Row(c).AlignItems(ui.End).Children(func() {
		for i, n := range days {
			day := today.AddDate(0, 0, i-len(days)+1)
			isToday := i == len(days)-1
			col := ui.Column(c.Key(i)).Grow(1).Basis(0).Gap(6).AlignItems(ui.Center).
				Tooltip(fmt.Sprintf("%s: %d", day.Format("Monday, Jan 2"), n))
			hovered := col.Hovered()
			col.Children(func() {
				countColor := t.TextMuted
				if hovered {
					countColor = t.Text
				}
				count := ui.Text(c, fmt.Sprint(n)).FontSize(10).FontWeight(600).FontFeatures("tnum").
					TextColor(col.AnimateColor("count", countColor, quick))
				if n == 0 {
					count.Invisible()
				}
				ui.Column(c).Height(height).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
					line := ui.Box(c.Key("avg")).Absolute().Left(0).Right(0).Height(1).PassThrough().
						Background(t.Text.Alpha(0.14))
					line.Bottom(grow(line, "y", avg, 0))
					// Rounded at the top, as it rises from the axis; a bar the
					// pointer is over widens a little.
					bar := ui.Box(c).Radius(5, 5, 2, 2)
					width := float32(22)
					if hovered && n > 0 {
						width = 26
					}
					bar.Width(bar.AnimateTo("w", width, press))
					h := grow(bar, "h", float32(n)*height/float32(most), time.Duration(i)*40*time.Millisecond)
					bar.Height(max(h, 3))
					switch {
					case n == 0:
						bar.Background(l.subtle)
					case isToday:
						bar.LinearGradient(ui.LinearGradient{From: t.Accent.Mix(ui.RGB(255, 255, 255), 0.2), To: t.Accent, Angle: 180})
					case hovered:
						bar.Background(bar.AnimateColor("bg", t.Accent.Alpha(0.6), quick))
					default:
						bar.Background(bar.AnimateColor("bg", t.Accent.Alpha(0.35), quick))
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

// privacyView is the Privacy tab: crash reports, which the user can turn off.
func (a *app) privacyView(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	promise := func() {
		ui.Row(c).Gap(10).Padding(10, 12).Radius(10).Background(l.subtle).Children(func() {
			ui.Icon(c, iconLock).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "Crash reports are anonymous: they never carry your account, personal data, file contents, "+
				"keystrokes, or shortcuts, and your home folder is taken out of them.").
				Grow(1).Shrink(1).FontSize(11).TextColor(t.TextMuted).LineHeight(1.35)
		})
	}
	page(c, func() {
		if _, ok := telemetryConfig(); !ok {
			section(c, "Telemetry", func() {
				settingRow(c, iconShield, tileGray, "Telemetry is off",
					"Telemetry is only active in packaged release builds.", func() {
						statusPill(c, "Off", t.TextMuted)
					})
			})
			section(c, "On This Mac", func() {
				settingRow(c, iconLock, tileGreen, "Your shortcuts stay local",
					"Shortcuts, settings and usage counts are kept in ~/Library/Application Support/Shun, "+
						"and shortcuts sync only once you sign in.", func() {})
			})
			promise()
			return
		}
		section(c, "Telemetry", func() {
			if on := a.prefs.CrashReportsEnabled(); switchRow(c, iconBug, tileRed, "Send crash reports automatically",
				"When Shun crashes, macOS writes a .ips file under ~/Library/Logs/DiagnosticReports/. "+
					"Shun sends those to "+monHost()+" on next launch so we can fix the bug.", &on) {
				a.prefs.SetCrashReportsEnabled(on)
			}
		})
		promise()
	})
}

// accountView is the Account tab: signing in to Mon, and how settings sync
// goes once signed in.
func (a *app) accountView(c *ui.Context) {
	t := c.Theme()
	cl := a.cloud
	page(c, func() {
		switch {
		case cl.signin != nil:
			a.signinView(c)
		case cl.account() == "":
			section(c, "", func() {
				settingRow(c, iconCloud, tileBlue, "Sync Your Shortcuts",
					"Sign in to your account at "+monHost()+" to keep your shortcuts the same on every Mac.", func() {
						if ui.Button(c, "Sign In…").Clicked() {
							cl.signIn()
						}
					})
			})
			if cl.failure != "" {
				ui.Text(c, cl.failure).FontSize(12).TextColor(t.Warning).PaddingX(4)
			}
		default:
			a.signedInView(c)
		}
		ui.Text(c, "Your shortcuts and Enable All Shortcuts sync. Usage counts and the crash report choice stay "+
			"on this Mac. A shortcut of an app not installed here shows as missing.").
			FontSize(11).TextColor(t.TextMuted).LineHeight(1.35).PaddingX(4)
	})
}

// signinView shows the code to approve in the browser while a sign-in
// waits for it.
func (a *app) signinView(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	cl := a.cloud
	si := cl.signin
	section(c, "Sign In", func() {
		ui.Column(c).Gap(10).AlignItems(ui.Center).Padding(8, 0).Children(func() {
			if si.code == "" {
				ui.Spinner(c)
				ui.Text(c, "Getting a code…").FontSize(12).TextColor(t.TextMuted)
				return
			}
			ui.Text(c, "Check that your browser shows this code, then approve it.").FontSize(12).TextColor(t.TextMuted)
			ui.Box(c).Padding(8, 16).Radius(10).Background(l.subtle).Children(func() {
				ui.Text(c, si.code).FontSize(26).FontWeight(700).Selectable()
			})
			if !si.expires.IsZero() {
				left := max(0, time.Until(si.expires).Round(time.Second))
				ui.Text(c, fmt.Sprintf("The code expires in %d:%02d.", int(left.Minutes()), int(left.Seconds())%60)).
					FontSize(11).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				cl.cancelSignIn()
			}
			if si.url != "" && ui.Button(c, "Open Browser Again").Clicked() {
				mygo.Shell.OpenExternal(si.url)
			}
		})
	})
}

// signedInView shows whose account Shun syncs with, and how it goes.
func (a *app) signedInView(c *ui.Context) {
	t := c.Theme()
	cl := a.cloud
	st := cl.engine.State()
	section(c, "Account", func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if pic := cl.picture(); pic != nil {
				ui.Image(c, pic).Size(36, 36).Radius(18)
			} else {
				ui.Box(c).Size(36, 36).Radius(18).Center().Shrink(0).Background(t.Accent.Alpha(0.14)).Children(func() {
					ui.Icon(c, iconUser).FontSize(18).TextColor(t.Accent)
				})
			}
			ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
				ui.Text(c, cl.account()).FontWeight(600).SingleLine()
				if st.Email != "" && st.Email != cl.account() {
					ui.Text(c, st.Email).FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
			if ui.Button(c, "Sign Out").Clicked() {
				cl.signOut()
			}
		})
	})
	section(c, "Settings Sync", func() {
		icon, tint := iconCloud, tileGreen
		if cl.failure != "" {
			icon, tint = iconCloudOff, tileOrange
		}
		settingRow(c, icon, tint, "Shortcuts", cl.syncStatus(time.Now()), func() {
			if ui.Button(c, "Sync Now").Disabled(cl.busy > 0).Clicked() {
				cl.sync(cloudsync.Ask, true)
			}
		})
		divider(c, rowIndent)
		settingRow(c, iconUser, tileGray, "Manage Account", "Your Macs signed in, at "+monHost()+".", func() {
			if ui.Button(c, "Open…").Clicked() {
				mygo.Shell.OpenExternal(monURL())
			}
		})
	})
}
