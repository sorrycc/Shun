package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/keys"
	"shun/internal/macos"
	"shun/internal/store"
)

const recipeExt = "hotrecipe"

// shortcutsView is the Shortcuts tab: the shortcuts, which drag reorders,
// and a toolbar to add, remove, import and export them. Apps and recipes
// dropped from Finder are added.
func (a *app) shortcutsView(c *ui.Context) {
	s := &a.ui
	root := ui.Column(c).Grow(1)
	if files := root.DroppedFiles(); files != nil {
		a.dropped(files)
	}
	over := root.FileDragOver()
	if s.selected >= len(a.store.Shortcuts) {
		s.selected = -1
	}
	root.Children(func() {
		if len(a.store.Shortcuts) == 0 {
			emptyState(c, iconKeyboard, "No shortcuts yet",
				"Give an app a shortcut to bring it forward from anywhere, and press it again to hide it. "+
					"Add an app, or drag apps here from Finder.",
				func() {
					if ui.PrimaryButton(c, "Add App…").Clicked() {
						a.openPicker()
					}
				})
		} else {
			a.shortcutCard(c)
		}
		a.toolbar(c)
		if over {
			dropOverlay(c)
		}
	})

	ui.Modal(c, &s.picking, func() { a.pickerView(c) }).Padding(0).Gap(0).Radius(12).
		Transition(ui.ElementTransition{Duration: 180 * time.Millisecond, Enter: &ui.Motion{Scale: 0.96}})
}

// dropOverlay tells, over the tab, that apps dragged over it are added.
func dropOverlay(c *ui.Context) {
	t := c.Theme()
	ui.Box(c.Key("drop")).Absolute().Left(8).Top(8).Right(8).Bottom(8).Radius(12).Center().
		Background(t.Accent.Alpha(0.08)).Border(2, t.Accent.Alpha(0.7)).BorderStyle(ui.BorderDashed).
		Transition(ui.ElementTransition{Duration: 150 * time.Millisecond, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).
		PassThrough().Children(func() {
		ui.Column(c).Gap(8).AlignItems(ui.Center).Padding(16, 24).Radius(12).Background(t.Background).
			Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.12)).Children(func() {
			ui.Icon(c, iconImport).FontSize(26).TextColor(t.Accent)
			ui.Text(c, "Drop to add apps or recipes").FontWeight(600)
		})
	})
}

// shortcutCard holds the list in a card, as the other tabs their settings,
// under a caption that tells how to use it, and a banner while shortcuts
// are paused or slow.
func (a *app) shortcutCard(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	ui.Column(c).Grow(1).Gap(8).Padding(16, 20, 14, 20).Children(func() {
		switch {
		case !a.prefs.GlobalEnabled():
			banner(c, "paused", iconPause, tileOrange, "Shortcuts are paused",
				"None of them works until you turn them back on.", "Resume", func() { a.setGlobalEnabled(true) })
		case !a.trusted():
			banner(c, "untrusted", iconHand, tileIndigo, "Switch apps instantly",
				"Grant Shun the Accessibility permission; until then, shortcuts are slower.", "Open Settings…",
				func() { mygo.Shell.OpenExternal(accessibilitySettings) })
		}
		ui.Row(c).PaddingX(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Apps").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "Click a shortcut to record keys · drag to reorder").FontSize(11).
				TextColor(t.TextMuted.Alpha(0.8)).SingleLine()
		})
		card := ui.Column(c).Grow(1).Radius(10).Background(l.card).Border(1, l.cardBorder).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).Clip()
		// Paused, the list dims, as its shortcuts each do turned off.
		dim := float32(1)
		if !a.prefs.GlobalEnabled() {
			dim = 0.6
		}
		card.Opacity(card.AnimateTo("dim", dim, quick)).Children(func() {
			a.shortcutList(c)
			// Under a few shortcuts, the room left tells how to add more.
			if len(a.store.Shortcuts) < 6 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Justify(ui.Center).Padding(10).PassThrough().Children(func() {
					ui.Icon(c, iconImport).FontSize(12).TextColor(t.TextMuted.Alpha(0.6))
					ui.Text(c, "Drag apps here from Finder, or press + to add one").FontSize(11).
						TextColor(t.TextMuted.Alpha(0.75)).SingleLine()
				})
			}
		})
	})
}

func (a *app) shortcutList(c *ui.Context) {
	s := &a.ui
	shortcuts := a.store.Shortcuts
	s.list.Selected = &s.selected
	s.list.Key = func(i int) any { return a.store.Shortcuts[i].ID }
	s.list.Label = func(i int) string { return a.store.Shortcuts[i].DisplayName }
	s.list.Reorder = func(rows []int, to int) { a.store.Move(rows, to) }
	list := ui.List(c, &s.list, len(shortcuts), func(i int) { a.shortcutRow(c, i) }).Grow(1).Padding(6).Gap(2)
	if s.selected >= 0 && (list.Shortcut(0, ui.KeyBackspace) || list.Shortcut(0, ui.KeyDelete)) {
		a.remove(c, shortcuts[s.selected].ID)
	}
}

// shortcutRow is a shortcut of the list, which paints its hover and its
// choice itself, over the list's: chosen, the row takes a tint of the
// accent and a ring of it, which keep its recorder and switch legible.
func (a *app) shortcutRow(c *ui.Context, i int) {
	t := c.Theme()
	l := lookOf(t)
	sc := a.store.Shortcuts[i]
	chosen := i == a.ui.selected
	row := ui.Row(c).Padding(8, 10, 8, 18).Gap(12).AlignItems(ui.Center).Radius(t.Radius).TextColor(t.Text)
	hovered := row.Hovered()
	bg, ring := l.card, l.card.Alpha(0)
	switch {
	case chosen:
		bg, ring = t.Accent.Alpha(0.12).Over(l.card), t.Accent.Alpha(0.45)
	case hovered:
		bg = l.hover.Over(l.card)
	}
	bg = row.AnimateColor("bg", bg, quick)
	if at, ok := a.ui.fresh[sc.ID]; ok {
		// Just added: it lights up, then fades.
		if left := freshTime - time.Since(at); left > 0 {
			k := row.Keyframes("fresh", []ui.Keyframe{{At: 0, Value: 0.3}, {At: 300 * time.Millisecond, Value: 0.3},
				{At: freshTime, Value: 0, Ease: ui.EaseInOut}})
			bg = t.Accent.Alpha(k).Over(bg)
			c.After(left)
		} else {
			delete(a.ui.fresh, sc.ID)
		}
	}
	row.Background(bg).Border(1, row.AnimateColor("ring", ring, quick))
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("Show in Finder").Disabled(sc.Stale).Chosen() {
			mygo.Shell.ShowItemInFolder(sc.Path())
		}
		m.Separator()
		if m.Item("Remove").Chosen() {
			a.remove(c, sc.ID)
		}
	})
	// A shortcut turned off, or whose app is gone, shows faded.
	fade := float32(1)
	if sc.Stale || !sc.IsEnabled {
		fade = 0.5
	}
	fade = row.AnimateTo("fade", fade, quick)
	name, muted, warning := t.Text, t.TextMuted, t.Warning
	row.Children(func() {
		// A grip, as the pointer is over the row, tells that it drags.
		grip := float32(0)
		if hovered {
			grip = 0.55
		}
		ui.Box(c.Key("grip")).Absolute().Left(3).Top(0).Bottom(0).Center().PassThrough().
			Opacity(row.AnimateTo("grip", grip, quick)).Children(func() {
			ui.Icon(c, iconGrip).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Box(c).Opacity(fade).Children(func() {
			a.appIcon(c, sc.Path(), sc.DisplayName, 32)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Opacity(max(fade, 0.7)).Children(func() {
			ui.Text(c, sc.DisplayName).FontWeight(500).TextColor(name).SingleLine()
			switch n := a.usage.Count(sc.BundleIdentifier); {
			case sc.Stale:
				ui.Row(c).Gap(4).AlignItems(ui.Center).Tooltip("App not found at " + sc.Path()).Children(func() {
					ui.Icon(c, iconWarning).FontSize(11).TextColor(warning)
					ui.Text(c, "App not found").FontSize(11).TextColor(warning)
				})
			case n > 0:
				ui.Textf(c, "%d× past 7 days", n).FontSize(11).TextColor(muted)
			case sc.Combo == nil:
				ui.Text(c, "No shortcut").FontSize(11).TextColor(muted)
			}
		})
		ui.Column(c).Gap(3).AlignItems(ui.End).Children(func() {
			a.recorder(c, sc.ID, sc.Combo)
			if n, ok := a.ui.notices[sc.ID]; ok {
				if left := time.Until(n.until); left > 0 {
					ui.Row(c.Key("notice")).Gap(4).AlignItems(ui.Center).
						Transition(ui.ElementTransition{Enter: &ui.Motion{Y: -4}}).Children(func() {
						ui.Icon(c, iconRefresh).FontSize(10).TextColor(warning)
						ui.Text(c, n.text).FontSize(10).TextColor(warning)
					})
					c.After(left)
				} else {
					delete(a.ui.notices, sc.ID)
				}
			}
		})
		on := sc.IsEnabled
		if ui.Switch(c.Key("enabled-"+sc.ID), &on).Label("Enable " + sc.DisplayName).Changed() {
			a.store.SetEnabled(sc.ID, on)
		}
	})
}

// freshTime is how long a shortcut just added stands out.
const freshTime = 1600 * time.Millisecond

// setCombo gives a shortcut its keys, telling for 3 seconds which shortcut
// had them before.
func (a *app) setCombo(id string, combo *keys.Combo) {
	if from := a.store.SetCombo(id, combo); from != "" {
		a.ui.notices[id] = notice{"Reassigned from " + from, time.Now().Add(3 * time.Second)}
	} else {
		delete(a.ui.notices, id)
	}
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	l := lookOf(t)
	s := &a.ui
	ui.Box(c).Height(1).Background(l.cardBorder)
	ui.Toolbar(c, func() {
		toolGroup(c, func() {
			if toolButton(c, iconPlus, "Add App", false) {
				a.openPicker()
			}
			ui.Box(c).Width(1).Height(14).Background(l.cardBorder)
			if toolButton(c, iconMinus, "Remove Shortcut", s.selected < 0) {
				a.remove(c, a.store.Shortcuts[s.selected].ID)
			}
		})
		ui.Spacer(c)
		if n := len(a.store.Shortcuts); n > 0 {
			on := 0
			for _, sc := range a.store.Shortcuts {
				if sc.IsEnabled && sc.Combo != nil && !sc.Stale {
					on++
				}
			}
			color, status := t.TextMuted, fmt.Sprintf("%d of %d active", on, n)
			switch {
			case !a.prefs.GlobalEnabled():
				color, status = tileOrange, "Paused"
			case on > 0:
				color = t.Success
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				dot := ui.Box(c).Size(6, 6).Radius(3)
				dot.Background(dot.AnimateColor("dot", color, quick))
				ui.Text(c, status).FontSize(11).TextColor(t.TextMuted).FontFeatures("tnum")
			})
		}
		ui.Spacer(c)
		toolGroup(c, func() {
			if toolButton(c, iconImport, "Import Recipe", false) {
				a.importRecipe()
			}
			ui.Box(c).Width(1).Height(14).Background(l.cardBorder)
			if toolButton(c, iconExport, "Export Recipe", len(a.store.Shortcuts) == 0) {
				a.exportRecipe()
			}
		})
	}).Label("Shortcuts").Padding(7, 12).Background(l.header)
}

// remove removes shortcuts at once, with a toast to undo it.
func (a *app) remove(c *ui.Context, ids ...string) {
	type removed struct {
		at int
		sc store.Shortcut
	}
	var gone []removed
	for i, sc := range a.store.Shortcuts {
		if slices.Contains(ids, sc.ID) {
			gone = append(gone, removed{i, sc})
		}
	}
	if len(gone) == 0 {
		return
	}
	a.store.Remove(ids...)
	a.ui.selected = -1
	msg := fmt.Sprintf("Removed %d shortcuts", len(gone))
	if len(gone) == 1 {
		msg = "Removed " + gone[0].sc.DisplayName
	}
	c.ToastAction(msg, "Undo", func() {
		for _, r := range gone { // in order, so that each goes back where it was
			a.store.Insert(r.at, r.sc)
		}
	})
}

// dropped adds the apps and imports the recipes dropped on the tab.
func (a *app) dropped(paths []string) {
	for _, p := range paths {
		switch strings.ToLower(filepath.Ext(p)) {
		case ".app":
			a.addApp(p)
		case "." + recipeExt:
			a.importRecipeFile(p)
		}
	}
}

// addApp adds the app at path, which has no keys yet.
func (a *app) addApp(path string) {
	id, name, ok := macos.BundleInfo(path)
	if !ok {
		log.Printf("adding %s: not an app bundle", path)
		return
	}
	a.added(store.NewShortcut(path, id, name))
}

// added adds a shortcut, which stands out for a moment in the list.
func (a *app) added(sc store.Shortcut) {
	if a.store.Add(sc) {
		a.ui.fresh[sc.ID] = time.Now()
		a.ui.list.ScrollIntoView(len(a.store.Shortcuts) - 1)
	}
}

func (a *app) importRecipe() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  a.win,
			Title:   "Import Recipe",
			Filters: []mygo.FileFilter{{Name: "Shun Recipe", Extensions: []string{recipeExt}}},
		})
		if err == nil && len(paths) > 0 {
			a.win.Update(func() { a.importRecipeFile(paths[0]) })
		}
	}()
}

func (a *app) importRecipeFile(path string) {
	data, err := os.ReadFile(path)
	if err == nil {
		var r store.Recipe
		if r, err = store.ParseRecipe(data); err == nil {
			a.alert("Recipe Import", a.store.Import(r, resolveApp).Summary())
			return
		}
	}
	a.alert("Recipe Import", err.Error())
}

// resolveApp finds an app of a recipe: by its bundle ID, else by its name
// in the application folders.
func resolveApp(bundleID, name string) (string, bool) {
	if p, ok := macos.AppPath(bundleID); ok {
		return p, true
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/Applications", "/System/Applications", filepath.Join(home, "Applications")} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if n, ok := strings.CutSuffix(e.Name(), ".app"); ok && strings.EqualFold(n, name) {
				return filepath.Join(dir, e.Name()), true
			}
		}
	}
	return "", false
}

func (a *app) exportRecipe() {
	data, err := a.store.Export()
	if err != nil {
		a.alert("Export Failed", err.Error())
		return
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "Export Recipe",
			DefaultPath: "Shortcuts." + recipeExt,
			Filters:     []mygo.FileFilter{{Name: "Shun Recipe", Extensions: []string{recipeExt}}},
		})
		if err != nil || path == "" {
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			a.win.Update(func() { a.alert("Export Failed", err.Error()) })
		}
	}()
}
