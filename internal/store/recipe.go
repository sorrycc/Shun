package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"shun/internal/keys"
)

// RecipeVersion is the version of the recipes Shun writes and reads.
const RecipeVersion = 1

// Recipe is a set of shortcuts to share, in a .hotrecipe file. Fields are
// in the order the Swift app wrote them (sorted keys).
type Recipe struct {
	Entries []RecipeEntry `json:"entries"`
	Version int           `json:"version"`
}

// RecipeEntry is a shortcut of a recipe.
type RecipeEntry struct {
	BundleIdentifier string `json:"bundleIdentifier"`
	CarbonModifiers  int    `json:"carbonModifiers"`
	DisplayName      string `json:"displayName"`
	KeyCode          int    `json:"keyCode"`
}

// ParseRecipe decodes a recipe, refusing versions newer than this one.
func ParseRecipe(data []byte) (Recipe, error) {
	var r Recipe
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("this file is not a valid recipe: %w", err)
	}
	if r.Version > RecipeVersion {
		return r, fmt.Errorf("this recipe requires a newer version of Shun (recipe version %d)", r.Version)
	}
	return r, nil
}

// Export returns the shortcuts that have keys as an indented recipe.
func (s *Store) Export() ([]byte, error) {
	r := Recipe{Version: RecipeVersion, Entries: []RecipeEntry{}}
	for _, sc := range s.Shortcuts {
		if sc.Combo == nil {
			continue
		}
		r.Entries = append(r.Entries, RecipeEntry{
			BundleIdentifier: sc.BundleIdentifier,
			DisplayName:      sc.DisplayName,
			KeyCode:          sc.Combo.KeyCode,
			CarbonModifiers:  sc.Combo.Modifiers,
		})
	}
	return json.MarshalIndent(r, "", "  ")
}

// ImportResult counts what an import did. SkippedKeyConflict counts the
// shortcuts added without their keys, which others had: they are among Added.
type ImportResult struct {
	Added, SkippedDuplicate, SkippedNotFound, SkippedKeyConflict int
}

// Total is the number of entries of the recipe.
func (r ImportResult) Total() int {
	return r.Added + r.SkippedDuplicate + r.SkippedNotFound
}

// Summary tells the user what an import did.
func (r ImportResult) Summary() string {
	if r.Added == r.Total() && r.SkippedKeyConflict == 0 {
		return fmt.Sprintf("Imported all %d shortcuts.", r.Added)
	}
	var parts []string
	if r.SkippedDuplicate > 0 {
		parts = append(parts, fmt.Sprintf("%d duplicate", r.SkippedDuplicate))
	}
	if r.SkippedNotFound > 0 {
		parts = append(parts, fmt.Sprintf("%d not found", r.SkippedNotFound))
	}
	if r.SkippedKeyConflict > 0 {
		parts = append(parts, fmt.Sprintf("%d without keys, taken by other shortcuts", r.SkippedKeyConflict))
	}
	return fmt.Sprintf("Imported %d of %d shortcuts (%s).", r.Added, r.Total(), strings.Join(parts, ", "))
}

// Import adds the recipe's shortcuts. resolve finds an app's path by its
// bundle ID or name. Apps that have a shortcut are skipped; an entry whose
// keys another shortcut has is added without keys, counted as a conflict.
func (s *Store) Import(r Recipe, resolve func(bundleID, name string) (string, bool)) ImportResult {
	var res ImportResult
	taken := map[keys.Combo]bool{}
	for _, sc := range s.Shortcuts {
		if sc.Combo != nil {
			taken[*sc.Combo] = true
		}
	}
	for _, e := range r.Entries {
		if s.HasApp(e.BundleIdentifier) {
			res.SkippedDuplicate++
			continue
		}
		path, ok := resolve(e.BundleIdentifier, e.DisplayName)
		if !ok {
			res.SkippedNotFound++
			continue
		}
		sc := NewShortcut(path, e.BundleIdentifier, e.DisplayName)
		sc.Stale = !s.AppExists(path)
		combo := keys.Combo{KeyCode: e.KeyCode, Modifiers: e.CarbonModifiers}
		if taken[combo] {
			res.SkippedKeyConflict++
		} else {
			sc.Combo = &combo
			taken[combo] = true
		}
		s.Shortcuts = append(s.Shortcuts, sc)
		res.Added++
	}
	if res.Added > 0 {
		s.changed()
	}
	return res
}
