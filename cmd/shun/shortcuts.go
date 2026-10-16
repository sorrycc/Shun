package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	t := c.Theme()
	s := &a.ui
	root := ui.Column(c).Grow(1)
	if files := root.DroppedFiles(); files != nil {
		a.dropped(files)
	}
	if root.FileDragOver() {
		root.Border(2, t.Accent)
	}
	if s.selected >= len(a.store.Shortcuts) {
		s.selected = -1
	}
	root.Children(func() {
		if len(a.store.Shortcuts) == 0 {
			emptyState(c, iconKeyboard, "No shortcuts configured", "Click + or drag apps here to get started")
		} else {
			a.shortcutList(c)
		}
		ui.Divider(c)
		a.toolbar(c)
	})

	ui.Modal(c, &s.picking, func() { a.pickerView(c) })
	if ui.AlertDialog(c, &s.confirmRemove, a.removeTitle(), "", "Cancel", "Remove") == 1 {
		a.store.Remove(s.removing...)
		s.selected = -1
		s.removing = nil
	}
}

func (a *app) shortcutList(c *ui.Context) {
	s := &a.ui
	shortcuts := a.store.Shortcuts
	s.list.Selected = &s.selected
	s.list.Key = func(i int) any { return a.store.Shortcuts[i].ID }
	s.list.Label = func(i int) string { return a.store.Shortcuts[i].DisplayName }
	s.list.Reorder = func(rows []int, to int) { a.store.Move(rows, to) }
	list := ui.List(c, &s.list, len(shortcuts), func(i int) { a.shortcutRow(c, i) }).Grow(1).Padding(6, 8)
	if s.selected >= 0 && (list.Shortcut(0, ui.KeyBackspace) || list.Shortcut(0, ui.KeyDelete)) {
		a.askRemove(shortcuts[s.selected].ID)
	}
}

func (a *app) shortcutRow(c *ui.Context, i int) {
	t := c.Theme()
	sc := a.store.Shortcuts[i]
	row := ui.Row(c).Padding(6, 10).Gap(12).AlignItems(ui.Center).Radius(8)
	if i == a.ui.selected {
		row.Background(t.Accent.Alpha(0.14))
	}
	row.Children(func() {
		ui.Box(c).Opacity(staleOpacity(sc)).Children(func() {
			a.appIcon(c, sc.Path(), sc.DisplayName, 32)
		})
		ui.Column(c).Grow(1).Gap(2).Opacity(staleOpacity(sc)).Children(func() {
			ui.Text(c, sc.DisplayName).SingleLine()
			if sc.Stale {
				ui.Text(c, "App not found").FontSize(11).TextColor(t.TextMuted)
			} else if n := a.usage.Count(sc.BundleIdentifier); n > 0 {
				ui.Textf(c, "%d× past 7 days", n).FontSize(11).TextColor(t.TextMuted)
			}
		})
		if sc.Stale {
			ui.Icon(c, iconWarning).FontSize(14).TextColor(t.Warning).Tooltip("App not found at " + sc.Path())
		}
		ui.Column(c).Gap(2).AlignItems(ui.End).Children(func() {
			a.recorder(c, sc.ID, sc.Combo)
			if n, ok := a.ui.notices[sc.ID]; ok {
				if left := time.Until(n.until); left > 0 {
					ui.Text(c, n.text).FontSize(10).TextColor(t.Warning)
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

func staleOpacity(sc store.Shortcut) float32 {
	if sc.Stale {
		return 0.5
	}
	return 1
}

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
	s := &a.ui
	button := func(icon *ui.SVG, label string, disabled bool) bool {
		b := ui.Button(c, "").Label(label).Tooltip(label).Disabled(disabled).Padding(4, 8).Children(func() {
			ui.Icon(c, icon).FontSize(15).TextColor(t.Text)
		})
		return b.Clicked()
	}
	ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		if button(iconPlus, "Add App", false) {
			a.openPicker()
		}
		if button(iconMinus, "Remove Shortcut", s.selected < 0) {
			a.askRemove(a.store.Shortcuts[s.selected].ID)
		}
		ui.Spacer(c)
		if button(iconImport, "Import Recipe", false) {
			a.importRecipe()
		}
		if button(iconExport, "Export Recipe", len(a.store.Shortcuts) == 0) {
			a.exportRecipe()
		}
	})
}

func (a *app) askRemove(ids ...string) {
	a.ui.removing = ids
	a.ui.confirmRemove = true
}

func (a *app) removeTitle() string {
	if len(a.ui.removing) == 1 {
		if sc, ok := a.store.Get(a.ui.removing[0]); ok {
			return fmt.Sprintf("Remove shortcut for %s?", sc.DisplayName)
		}
	}
	return fmt.Sprintf("Remove %d shortcuts?", len(a.ui.removing))
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
	a.store.Add(store.NewShortcut(path, id, name))
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
