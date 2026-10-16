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
			p.apps, p.loading, p.loadedAt = apps, false, time.Now()
			a.filterApps()
		})
	}()
}

func (a *app) filterApps() {
	p := &a.ui.picker
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.results = p.results[:0]
	for _, app := range p.apps {
		if q == "" || strings.Contains(strings.ToLower(app.Name), q) {
			p.results = append(p.results, app)
		}
	}
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
	a.store.Add(store.NewShortcut(app.Path, app.ID, app.Name))
}

func (a *app) pickerView(c *ui.Context) {
	t := c.Theme()
	p := &a.ui.picker
	if c.Shortcut(0, ui.KeyDown) {
		a.choose(p.chosen + 1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.choose(p.chosen - 1)
	}
	ui.Column(c).Size(420, 480).Children(func() {
		ui.Row(c).Padding(12).Gap(8).AlignItems(ui.Center).Children(func() {
			field := ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(8).
				Border(1, t.Border).Background(t.Background)
			field.Children(func() {
				ui.Icon(c, iconSearch).FontSize(14).TextColor(t.TextMuted)
				search := ui.TextInputBase(c, &p.query).Bind(&p.input).Grow(1).
					Label("Search applications").Placeholder("Search applications…")
				if search.Changed() {
					a.filterApps()
				}
				if search.Submitted() {
					a.addChosen()
				}
			})
			if ui.Button(c, "Done").Clicked() {
				a.ui.picking = false
			}
		})
		ui.Divider(c)
		if p.loading && len(p.apps) == 0 {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Scanning applications…").TextColor(t.TextMuted)
			})
			return
		}
		p.list.Key = func(i int) any { return p.results[i].Path }
		p.list.Label = func(i int) string { return p.results[i].Name }
		ui.List(c, &p.list, len(p.results), func(i int) { a.pickerRow(c, i) }).Grow(1).Padding(6).Children(func() {
			if len(p.results) == 0 {
				ui.Text(c, "No apps found").TextColor(t.TextMuted).Padding(16)
			}
		})
	})
}

func (a *app) pickerRow(c *ui.Context, i int) {
	t := c.Theme()
	p := &a.ui.picker
	app := p.results[i]
	added := a.store.HasApp(app.ID)
	row := ui.Row(c).Padding(6, 10).Gap(12).AlignItems(ui.Center).Radius(8)
	if i == p.chosen {
		row.Background(t.Accent.Alpha(0.2))
	}
	if added {
		row.Opacity(0.5)
	}
	if row.Clicked() {
		p.chosen = i
		a.addChosen()
	}
	row.Children(func() {
		a.appIcon(c, app.Path, app.Name, 32)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, app.Name).SingleLine()
			ui.Text(c, app.ID).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if added {
			ui.Icon(c, iconCheck).FontSize(16).TextColor(t.Success)
		}
	})
}
