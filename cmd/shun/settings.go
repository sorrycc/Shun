package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	tabShortcuts = iota
	tabGeneral
	tabInsights
	tabPrivacy
)

// settingsState is the Settings window's own state.
type settingsState struct {
	tab int
	// shown is the tab whose page shows, and from which the page of a tab
	// chosen next slides in: from the right for a tab to the right.
	shown, slide int

	list     ui.ListState
	selected int // the row chosen, or -1
	// recording is the ID of the shortcut whose recorder takes keys.
	recording string

	// The app picker.
	picking bool
	picker  picker

	// fresh holds when shortcuts were just added, to show them for a moment.
	fresh map[string]time.Time

	// A message, as after importing a recipe.
	alertOpen  bool
	alertTitle string
	alertText  string

	// notices tell, under a shortcut's recorder, that its keys were taken
	// from another shortcut, until a time.
	notices map[string]notice

	loginItem    bool
	confirmClear bool

	// Whether Shun has the Accessibility permission, and when it was asked.
	isTrusted bool
	trustedAt time.Time
}

type notice struct {
	text  string
	until time.Time
}

func (s *settingsState) init() {
	s.selected = -1
	s.notices = map[string]notice{}
	s.fresh = map[string]time.Time{}
}

// reset forgets what was under way as the window hides.
func (s *settingsState) reset() {
	s.picking, s.alertOpen, s.recording = false, false, ""
	s.confirmClear = false
}

// trusted reports whether Shun has the Accessibility permission, asked at
// most once a second, though views ask on every frame.
func (a *app) trusted() bool {
	if now := time.Now(); now.Sub(a.ui.trustedAt) > time.Second {
		a.ui.isTrusted, a.ui.trustedAt = a.trust(), now
	}
	return a.ui.isTrusted
}

func (a *app) alert(title, text string) {
	a.ui.alertTitle, a.ui.alertText, a.ui.alertOpen = title, text, true
}

// invalidate draws the Settings window again, after a change from outside
// its view: not while it hides, as every shortcut pressed changes usage,
// and it draws afresh as it shows.
func (a *app) invalidate() {
	if a.win != nil && a.win.IsVisible() {
		a.win.Invalidate()
	}
}

func (a *app) hideSettings() {
	if a.win != nil {
		a.win.Hide()
	}
}

// view is the Settings window: the tabs in its title bar, over the page
// of the tab chosen, which comes in as the tab changes.
func (a *app) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyW, a.hideSettings)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	for i := range settingsTabs {
		c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.ui.tab = i })
	}
	t := c.Theme()
	l := lookOf(t)
	c.Root().Background(l.canvas)
	if s := &a.ui; s.tab != s.shown {
		s.slide = 1
		if s.tab < s.shown {
			s.slide = -1
		}
		s.shown = s.tab
	}
	ui.Column(c).Fill().Children(func() {
		header(c, &a.ui.tab)
		in := pageIn
		in.Enter = &ui.Motion{X: 16 * float32(a.ui.slide)}
		if a.ui.slide == 0 {
			in.Enter = &ui.Motion{Y: 6}
		}
		ui.Column(c.Key(a.ui.tab)).Grow(1).Transition(in).Children(func() {
			switch a.ui.tab {
			case tabShortcuts:
				a.shortcutsView(c)
			case tabGeneral:
				a.generalView(c)
			case tabInsights:
				a.insightsView(c)
			case tabPrivacy:
				a.privacyView(c)
			}
		})
	})
	ui.AlertDialog(c, &a.ui.alertOpen, a.ui.alertTitle, a.ui.alertText, "OK")
}

// page lays out a tab of settings that scrolls.
func page(c *ui.Context, fn func()) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(20, 24, 24, 24).Gap(20).Children(fn)
	})
}

// section is a titled card of a page, its rows apart by hairlines.
func section(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	l := lookOf(t)
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted).PaddingX(4)
		}
		ui.Column(c).Gap(10).Padding(12, 14).Radius(10).Background(l.card).Border(1, l.cardBorder).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).Children(fn)
	})
}

// divider parts the rows of a section, from under their text.
func divider(c *ui.Context, indent float32) {
	ui.Box(c).Height(1).MarginStart(indent).Background(lookOf(c.Theme()).subtle)
}

// settingRow is a setting with its icon and label at the left, a
// description below, and its control at the right.
func settingRow(c *ui.Context, icon *ui.SVG, tint ui.Color, label, description string, control func()) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).MinHeight(28).Children(func() {
		if icon != nil {
			iconTile(c, icon, tint)
		}
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, label)
			if description != "" {
				ui.Text(c, description).FontSize(11).TextColor(t.TextMuted).LineHeight(1.3)
			}
		})
		control()
	})
}

// switchRow is a setting turned on and off by a switch, which a click on
// its label turns over too; it reports a change.
func switchRow(c *ui.Context, icon *ui.SVG, tint ui.Color, label, description string, on *bool) bool {
	t := c.Theme()
	changed := false
	ui.Row(c).Gap(12).AlignItems(ui.Center).MinHeight(28).Children(func() {
		if icon != nil {
			iconTile(c, icon, tint)
		}
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			if ui.Text(c, label).Cursor(ui.CursorPointer).Clicked() {
				*on = !*on
				changed = true
			}
			if description != "" {
				ui.Text(c, description).FontSize(11).TextColor(t.TextMuted).LineHeight(1.3)
			}
		})
		if ui.Switch(c, on).Label(label).Changed() {
			changed = true
		}
	})
	return changed
}

// banner tells, over a page, of a state that keeps shortcuts from working
// as they should, with the action that fixes it. It slides in as the state
// comes, and fades as it goes.
func banner(c *ui.Context, key string, icon *ui.SVG, tint ui.Color, title, text, action string, fix func()) {
	t := c.Theme()
	l := lookOf(t)
	ui.Row(c.Key(key)).Gap(10).AlignItems(ui.Center).Padding(9, 10, 9, 10).Margin(0, 0, 4, 0).Radius(10).
		Background(tint.Alpha(0.1).Over(l.card)).Border(1, tint.Alpha(0.28)).
		Transition(ui.ElementTransition{Animation: &settle, Enter: &ui.Motion{Y: -8}, Exit: &ui.Motion{}}).
		Children(func() {
			iconTile(c, icon, tint)
			ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(12).FontWeight(600)
				ui.Text(c, text).FontSize(11).TextColor(t.TextMuted).LineHeight(1.3)
			})
			if ui.Button(c, action).Clicked() {
				fix()
			}
		})
}

// rowIndent is where the text of a row with an icon starts, for dividers.
const rowIndent = 38

// emptyState is a page's message when it has nothing to show, with an
// action under it.
func emptyState(c *ui.Context, icon *ui.SVG, title, subtitle string, action func()) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(8).Padding(24).Children(func() {
		badge := ui.Box(c).Size(64, 64).Radius(18).Center().Margin(0, 0, 6, 0).
			LinearGradient(ui.LinearGradient{From: t.Accent.Alpha(0.16), To: t.Accent.Alpha(0.06), Angle: 180}).
			Border(1, t.Accent.Alpha(0.15))
		badge.Transition(ui.ElementTransition{Duration: 320 * time.Millisecond, Enter: &ui.Motion{Scale: 0.85}})
		badge.Children(func() {
			ui.Icon(c, icon).FontSize(30).TextColor(t.Accent)
		})
		ui.Text(c, title).FontSize(15).FontWeight(600)
		ui.Text(c, subtitle).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(340).LineHeight(1.35)
		if action != nil {
			ui.Box(c).Height(6)
			action()
		}
	})
}
