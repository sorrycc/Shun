package main

import (
	"log"
	"math"
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
	// nudges counts the tries to add an app that has a shortcut already,
	// the one at nudged, whose badge shakes.
	nudges uint64
	nudged string
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
	word := " " + q // a word of a name starting with the query
	p.results = p.results[:0]
	var later [2][]mygo.InstalledApp
	for i, app := range p.apps {
		name := p.names[i]
		switch {
		case q == "" || strings.HasPrefix(name, q):
			p.results = append(p.results, app)
		case strings.Contains(name, word):
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
		p.nudges++
		p.nudged = app.Path
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
			glass := ui.Icon(c, iconSearch).FontSize(17)
			typed := p.query != ""
			if typed {
				glass.TextColor(glass.AnimateColor("fg", t.Accent, quick))
			} else {
				glass.TextColor(glass.AnimateColor("fg", t.TextMuted, quick))
			}
			search := ui.TextInputBase(c, &p.query).Bind(&p.input).Grow(1).FontSize(15).
				Label("Search applications").Placeholder("Search applications…")
			if search.Changed() {
				a.filterApps()
			}
			if search.Submitted() {
				a.addChosen()
			}
			if typed {
				x := ui.ButtonBase(c.Key("clear")).Label("Clear Search").Size(18, 18).Radius(9).Center().
					Transition(ui.ElementTransition{Duration: 140 * time.Millisecond, Enter: &ui.Motion{Scale: 0.6},
						Exit: &ui.Motion{Scale: 0.6}})
				bg := t.Text.Alpha(0.12)
				if x.Hovered() {
					bg = t.Text.Alpha(0.2)
				}
				x.Background(x.AnimateColor("bg", bg, quick)).Children(func() {
					ui.Icon(c, iconClose).FontSize(10).TextColor(t.TextMuted)
				})
				if x.Clicked() {
					p.query = ""
					a.filterApps()
					p.input.Focus()
				}
			}
			if ui.Button(c, "Done").Clicked() {
				a.ui.picking = false
			}
		})
		ui.Box(c).Height(1).Background(l.cardBorder)
		switch {
		case p.loading && len(p.apps) == 0:
			pickerSkeleton(c)
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

// pickerChoice is the identity of the highlight of the app chosen, which
// glides to the app chosen next.
type pickerChoice struct{}

func (a *app) pickerRow(c *ui.Context, i int) {
	t := c.Theme()
	l := lookOf(t)
	p := &a.ui.picker
	app := p.results[i]
	added := a.store.HasApp(app.ID)
	row := ui.Row(c).Padding(6, 10).Gap(12).AlignItems(ui.Center).Radius(8)
	bg := ui.Color{}
	if i != p.chosen && row.Hovered() {
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
		if i == p.chosen {
			// The highlight glides from the row chosen before.
			ui.Box(c.Key("chosen")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(8).PassThrough().
				Background(t.Accent.Alpha(0.14)).Border(1, t.Accent.Alpha(0.2)).
				Transition(ui.ElementTransition{Match: pickerChoice{}, Animation: &settle})
		}
		ui.Box(c).Opacity(fade).Children(func() {
			a.appIcon(c, app.Path, app.Name, 32)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Opacity(fade).Children(func() {
			ui.Text(c, app.Name).FontWeight(500).SingleLine()
			ui.Text(c, app.ID).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if added {
			badge := ui.Row(c.Key("added")).Gap(4).AlignItems(ui.Center).
				Transition(ui.ElementTransition{Duration: 260 * time.Millisecond, Enter: &ui.Motion{Scale: 0.6}})
			if p.nudges > 0 && p.nudged == app.Path {
				// Added before: the badge shakes, to tell why nothing happens.
				x := badge.Keyframes("nudge", []ui.Keyframe{{At: 0, Value: 0}, {At: 50 * time.Millisecond, Value: -4},
					{At: 110 * time.Millisecond, Value: 3}, {At: 170 * time.Millisecond, Value: -2},
					{At: 240 * time.Millisecond, Value: 0, Ease: ui.EaseOut}}, ui.KeyframeOptions{Run: p.nudges})
				badge.Translate(x, 0)
			}
			badge.Children(func() {
				ui.Icon(c, iconCheck).FontSize(15).TextColor(t.Success)
				ui.Text(c, "Added").FontSize(11).FontWeight(600).TextColor(t.Success)
			})
		}
	})
}

// pickerSkeleton stands for the list while the apps are scanned: rows of
// placeholders, which shimmer.
func pickerSkeleton(c *ui.Context) {
	l := lookOf(c.Theme())
	col := ui.Column(c).Grow(1).Padding(6, 8).Gap(1).Label("Scanning applications…")
	phase := loop(c, col, "shimmer", 1400*time.Millisecond, ui.Linear, 0)
	col.Children(func() {
		for i := range 7 {
			// Each row a little after the one above, as a wave down the list.
			k := 0.5 + 0.5*float32(math.Cos(2*math.Pi*float64(phase-float32(i)*0.1)))
			ui.Row(c.Key(i)).Padding(6, 10).Gap(12).AlignItems(ui.Center).Opacity(0.5 + 0.5*k).Children(func() {
				ui.Box(c).Size(32, 32).Radius(8).Background(l.subtle)
				ui.Column(c).Gap(5).Children(func() {
					ui.Box(c).Size(float32(110+(i*37)%90), 10).Radius(4).Background(l.subtle)
					ui.Box(c).Size(float32(150+(i*53)%70), 8).Radius(4).Background(l.subtle.Alpha(0.6))
				})
			})
		}
	})
}
