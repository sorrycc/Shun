package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// PrepareDir moves the files of the legacy HotApp directory, a sibling of
// dir, into dir when dir has no shortcuts yet, so that users of HotApp keep
// their shortcuts and usage history. dir is created when missing.
func PrepareDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if exists(filepath.Join(dir, shortcutsFile)) {
		return nil
	}
	legacy := filepath.Join(filepath.Dir(dir), "HotApp")
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return nil // no legacy directory
	}
	for _, e := range entries {
		if exists(filepath.Join(dir, e.Name())) {
			continue
		}
		if err := os.Rename(filepath.Join(legacy, e.Name()), filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(legacy)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readJSON decodes path into v; a missing file leaves v as it is and
// returns false.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}

// writeJSON writes v as indented JSON, replacing path atomically.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // CreateTemp makes it 0600
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
