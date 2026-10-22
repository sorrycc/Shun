package main

import (
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/store"
)

// picker chooses installed apps to add: type to filter, Up and Down to
// choose, Enter or a click to add.
type picker struct {
	apps     []mygo.InstalledApp
	names    []string // the apps' names, in lower case, to match quickly
	loadedAt time.Time
	loading  bool
	query    string
	results  []mygo.InstalledApp
	chosen   int
	list     ui.ListState
	input    ui.Handle
}

// appsTTL is how long the list of installed apps is kept.
const appsTTL = time.Minute

func (a *app) openPicker() {
	p := &a.ui.picker
	p.query = ""
	a.ui.picking = true
	p.input.Focus()
	a.filterApps()
	if p.loading || time.Since(p.loadedAt) < appsTTL {
		return
	}
	p.loading = true
	go func() {
		apps, err := mygo.Shell.InstalledApps()
		if err != nil {
			log.Printf("listing apps: %v", err)
		}
		a.win.Update(func() {
			p.apps, p.names, p.loading, p.loadedAt = apps, nil, false, time.Now()
			a.filterApps()
		})
	}()
}

// filterApps finds the apps whose names have the query: those whose names
// start with it first, then those with a word starting with it, then the
// others, each in the order of the apps.
func (a *app) filterApps() {
	p := &a.ui.picker
	if len(p.names) != len(p.apps) {
		p.names = make([]string, len(p.apps))
		for i, app := range p.apps {
			p.names[i] = strings.ToLower(app.Name)
		}
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.results = p.results[:0]
	var later [2][]mygo.InstalledApp
	for i, app := range p.apps {
		name := p.names[i]
		switch {
		case q == "" || strings.HasPrefix(name, q):
			p.results = append(p.results, app)
		case strings.Contains(name, " "+q):
			later[0] = append(later[0], app)
		case strings.Contains(name, q):
			later[1] = append(later[1], app)
		}
	}
	p.results = append(append(p.results, later[0]...), later[1]...)
	a.choose(0)
}

func (a *app) choose(i int) {
	p := &a.ui.picker
	p.chosen = max(0, min(i, len(p.results)-1))
	if len(p.results) > 0 {
		p.list.ScrollIntoView(p.chosen)
	}
}

// addChosen adds the chosen app, unless it has a shortcut.
func (a *app) addChosen() {
	p := &a.ui.picker
	if p.chosen < 0 || p.chosen >= len(p.results) {
		return
	}
	app := p.results[p.chosen]
	if a.store.HasApp(app.ID) {
		return
	}
	a.added(store.NewShortcut(app.Path, app.ID, app.Name))
}

func (a *app) pickerView(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	p := &a.ui.picker
	if c.Shortcut(0, ui.KeyDown) {
		a.choose(p.chosen + 1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.choose(p.chosen - 1)
	}
	_, h := c.Size()
	ui.Column(c).Size(460, min(500, h-48)).Children(func() {
		ui.Row(c).Padding(12, 14, 12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSearch).FontSize(17).TextColor(t.TextMuted)
			search := ui.TextInputBase(c, &p.query).Bind(&p.input).Grow(1).FontSize(15).
				Label("Search applications").Placeholder("Search applications…")
			if search.Changed() {
				a.filterApps()
			}
			if search.Submitted() {
				a.addChosen()
			}
			if ui.Button(c, "Done").Clicked() {
				a.ui.picking = false
			}
		})
		ui.Box(c).Height(1).Background(l.cardBorder)
		switch {
		case p.loading && len(p.apps) == 0:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Scanning applications…").TextColor(t.TextMuted)
			})
		case len(p.results) == 0:
			ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
				ui.Icon(c, iconSearch).FontSize(28).TextColor(t.TextMuted.Alpha(0.6))
				ui.Text(c, "No apps found").FontWeight(600).TextColor(t.TextMuted)
				ui.Text(c, "Try another name, or drag the app into Settings.").FontSize(11).TextColor(t.TextMuted)
			})
		default:
			p.list.Key = func(i int) any { return p.results[i].Path }
			p.list.Label = func(i int) string { return p.results[i].Name }
			ui.List(c, &p.list, len(p.results), func(i int) { a.pickerRow(c, i) }).Grow(1).Padding(6, 8).Gap(1)
		}
		ui.Box(c).Height(1).Background(l.cardBorder)
		ui.Row(c).Padding(8, 14).Gap(14).AlignItems(ui.Center).Background(l.header).Radius(0, 0, 12, 12).Children(func() {
			hint := func(label string, caps ...string) {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					keycaps(c, caps, t.TextMuted)
					ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
				})
			}
			hint("Navigate", "↑", "↓")
			hint("Add", "↩")
			hint("Close", "esc")
			ui.Spacer(c)
			if !p.loading || len(p.apps) > 0 {
				ui.Textf(c, "%d apps", len(p.results)).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func (a *app) pickerRow(c *ui.Context, i int) {
	t := c.Theme()
	l := lookOf(t)
	p := &a.ui.picker
	app := p.results[i]
	added := a.store.HasApp(app.ID)
	row := ui.Row(c).Padding(6, 10).Gap(12).AlignItems(ui.Center).Radius(8)
	bg := ui.Color{}
	switch {
	case i == p.chosen:
		bg = t.Accent.Alpha(0.14)
	case row.Hovered():
		bg = l.hover
	}
	row.Background(row.AnimateColor("bg", bg, quick))
	if row.Clicked() {
		p.chosen = i
		a.addChosen()
	}
	fade := float32(1)
	if added {
		fade = 0.55
	}
	row.Children(func() {
		ui.Box(c).Opacity(fade).Children(func() {
			a.appIcon(c, app.Path, app.Name, 32)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Opacity(fade).Children(func() {
			ui.Text(c, app.Name).FontWeight(500).SingleLine()
			ui.Text(c, app.ID).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if added {
			ui.Row(c.Key("added")).Gap(4).AlignItems(ui.Center).
				Transition(ui.ElementTransition{Duration: 260 * time.Millisecond, Enter: &ui.Motion{Scale: 0.6}}).Children(func() {
				ui.Icon(c, iconCheck).FontSize(15).TextColor(t.Success)
				ui.Text(c, "Added").FontSize(11).FontWeight(600).TextColor(t.Success)
			})
		}
	})
}
