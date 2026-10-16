// Package store keeps Shun's shortcuts, preferences and recipes on disk, in
// the formats the Swift versions of Shun wrote, so that existing users keep
// their setup and can still go back to a Swift version.
package store

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"

	"shun/internal/keys"
)

// Shortcut binds a key combo to an app.
//
// The JSON fields up to isEnabled are the Swift app's AppShortcut, which it
// requires; combo is new, and the Swift app ignores it.
type Shortcut struct {
	BundleIdentifier string `json:"bundleIdentifier"`
	// BundleURL is the app's file URL with a trailing slash, as Foundation
	// writes a directory URL: file:///Applications/Google%20Chrome.app/
	BundleURL   string      `json:"bundleURL"`
	Combo       *keys.Combo `json:"combo,omitempty"`
	DisplayName string      `json:"displayName"`
	ID          string      `json:"id"`
	IsEnabled   bool        `json:"isEnabled"`
	// ShortcutName is the KeyboardShortcuts name the Swift app stored the
	// combo under ("hotapp_<id>"); kept for the migration and for going back.
	ShortcutName string `json:"shortcutName"`

	// Stale is set when the app is no longer where BundleURL points.
	Stale bool `json:"-"`
}

// NewShortcut returns an enabled shortcut for the app at path, without keys.
func NewShortcut(path, bundleID, name string) Shortcut {
	id := NewID()
	return Shortcut{
		ID:               id,
		BundleURL:        FileURL(path),
		BundleIdentifier: bundleID,
		DisplayName:      name,
		ShortcutName:     "hotapp_" + id,
		IsEnabled:        true,
	}
}

// Path returns the app's path from BundleURL, without a trailing slash.
func (s Shortcut) Path() string { return PathFromURL(s.BundleURL) }

// FileURL returns the file URL of a directory, as Foundation writes it.
func FileURL(path string) string {
	path = strings.TrimSuffix(path, "/") + "/"
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// PathFromURL returns the path of a file URL, without a trailing slash; a
// plain path is returned as it is.
func PathFromURL(s string) string {
	if u, err := url.Parse(s); err == nil && u.Scheme == "file" {
		s = u.Path
	}
	if len(s) > 1 {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}

// NewID returns a random UUID in Foundation's uppercase form.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
