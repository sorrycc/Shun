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

	list     ui.ListState
	selected int // the row chosen, or -1

	// The app picker.
	picking bool
	picker  picker

	// Removing shortcuts, after asking.
	confirmRemove bool
	removing      []string

	// A message, as after importing a recipe.
	alertOpen  bool
	alertTitle string
	alertText  string

	// notices tell, under a shortcut's recorder, that its keys were taken
	// from another shortcut, until a time.
	notices map[string]notice

	loginItem    bool
	confirmClear bool
	confirmReset bool
}

type notice struct {
	text  string
	until time.Time
}

func (s *settingsState) init() {
	s.selected = -1
	s.notices = map[string]notice{}
}

// reset forgets what was under way as the window hides.
func (s *settingsState) reset() {
	s.picking, s.confirmRemove, s.alertOpen = false, false, false
	s.confirmClear, s.confirmReset = false, false
}

func (a *app) alert(title, text string) {
	a.ui.alertTitle, a.ui.alertText, a.ui.alertOpen = title, text, true
}

// invalidate draws the Settings window again, after a change from outside
// its view.
func (a *app) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) hideSettings() {
	if a.win != nil {
		a.win.Hide()
	}
}

// view is the Settings window.
func (a *app) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyW, a.hideSettings)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	t := c.Theme()
	c.Root().Background(t.Background)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(12, 16, 0, 16).Justify(ui.Center).Children(func() {
			ui.Tabs(c, &a.ui.tab, "Shortcuts", "General", "Insights", "Privacy")
		})
		ui.Column(c).Grow(1).Children(func() {
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
		ui.Column(c).Padding(20, 28).Gap(18).Children(fn)
	})
}

// section is a titled group of a page.
func section(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	ui.Column(c).Gap(10).Children(func() {
		if title != "" {
			ui.Text(c, title).Bold().TextColor(t.TextMuted).FontSize(12)
		}
		ui.Column(c).Gap(12).Padding(14, 16).Radius(10).Background(t.Surface).Border(1, t.Border).Children(fn)
	})
}

// settingRow is a setting with its label at the left and its control at
// the right, and a description below.
func settingRow(c *ui.Context, label, description string, control func()) {
	t := c.Theme()
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, label)
			if description != "" {
				ui.Text(c, description).FontSize(11).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

// switchRow is a setting turned on and off by a switch, which a click on
// its label turns over too; it reports a change.
func switchRow(c *ui.Context, label, description string, on *bool) bool {
	t := c.Theme()
	changed := false
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			if ui.Text(c, label).Cursor(ui.CursorPointer).Clicked() {
				*on = !*on
				changed = true
			}
			if description != "" {
				ui.Text(c, description).FontSize(11).TextColor(t.TextMuted)
			}
		})
		if ui.Switch(c, on).Label(label).Changed() {
			changed = true
		}
	})
	return changed
}

// emptyState is a page's message when it has nothing to show.
func emptyState(c *ui.Context, icon *ui.SVG, title, subtitle string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
		ui.Icon(c, icon).FontSize(44).TextColor(t.TextMuted)
		ui.Text(c, title).Bold().TextColor(t.TextMuted)
		ui.Text(c, subtitle).FontSize(12).TextColor(t.TextMuted.Alpha(0.75)).TextAlign(ui.Center)
	})
}
