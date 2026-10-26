// Package cloudsync syncs Shun's shortcuts and preferences with the
// person's Mon account, as one settings document of Mon's:
//
//	{"shortcuts": {"com.apple.Safari": {"name": "Safari", "keys": {…}, "enabled": true}, …},
//	 "order": ["com.apple.Safari", …], "settings": {"globalEnabled": true}}
//
// Shortcuts are keyed by their apps' bundle IDs, which are the same on
// every Mac, unlike the apps' paths and the shortcuts' IDs. They and the
// preferences merge one by one against the snapshot of the last sync, the
// base: one changed on one side only takes that side's, and one changed on
// both the side that syncs now. The order merges as one value.
package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"shun/internal/keys"
	"shun/internal/mon"
)

// App is Shun's client id with Mon, and its settings document's name.
const App = "shun"

// Snapshot is what syncs: the shortcuts by bundle ID, as Entry in JSON,
// their order, and the preferences by their keys in settings.json.
type Snapshot struct {
	Shortcuts map[string]json.RawMessage `json:"shortcuts,omitempty"`
	Order     []string                   `json:"order,omitempty"`
	Settings  map[string]json.RawMessage `json:"settings,omitempty"`
}

// Entry is a shortcut as it syncs: no path, which is the Mac's own.
type Entry struct {
	Name    string      `json:"name"`
	Keys    *keys.Combo `json:"keys,omitempty"`
	Enabled bool        `json:"enabled"`
}

// Equal reports whether s and o hold the same shortcuts and preferences.
func (s Snapshot) Equal(o Snapshot) bool {
	return slices.Equal(s.Order, o.Order) &&
		maps.EqualFunc(s.Shortcuts, o.Shortcuts, sameJSON) && maps.EqualFunc(s.Settings, o.Settings, sameJSON)
}

func (s Snapshot) clone() Snapshot {
	s.Shortcuts = maps.Clone(s.Shortcuts)
	s.Order = slices.Clone(s.Order)
	s.Settings = maps.Clone(s.Settings)
	return s
}

func sameJSON(a, b json.RawMessage) bool {
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

// Merge merges what changed here since base, local, with what changed in
// the account, remote. Where both changed a shortcut or a preference,
// local's stays. Two apps left with the same keys keep them for the one
// changed here, else the one first in the order.
func Merge(base, local, remote Snapshot) Snapshot {
	merged := Snapshot{
		Shortcuts: mergeMap(base.Shortcuts, local.Shortcuts, remote.Shortcuts),
		Settings:  mergeMap(base.Settings, local.Settings, remote.Settings),
	}
	order := remote.Order
	if !slices.Equal(local.Order, base.Order) {
		order = local.Order
	}
	// Every shortcut once, those the order lacks after it as the other
	// side has them.
	for _, id := range slices.Concat(order, local.Order, remote.Order, slices.Sorted(maps.Keys(merged.Shortcuts))) {
		if _, ok := merged.Shortcuts[id]; ok && !slices.Contains(merged.Order, id) {
			merged.Order = append(merged.Order, id)
		}
	}
	dedupeKeys(merged, func(id string) bool { return changed(base.Shortcuts, local.Shortcuts, id) })
	return merged
}

func mergeMap(base, local, remote map[string]json.RawMessage) map[string]json.RawMessage {
	merged := map[string]json.RawMessage{}
	ids := map[string]bool{}
	for _, m := range []map[string]json.RawMessage{base, local, remote} {
		for k := range m {
			ids[k] = true
		}
	}
	for k := range ids {
		side := remote
		if changed(base, local, k) {
			side = local
		}
		if v, ok := side[k]; ok {
			merged[k] = v
		}
	}
	return merged
}

// dedupeKeys takes the keys off the shortcuts of s whose keys another has,
// the shortcuts mine reports keeping theirs first.
func dedupeKeys(s Snapshot, mine func(id string) bool) {
	taken := map[keys.Combo]bool{}
	for _, first := range []bool{true, false} {
		for _, id := range s.Order {
			if mine(id) != first {
				continue
			}
			var e Entry
			if json.Unmarshal(s.Shortcuts[id], &e) != nil || e.Keys == nil {
				continue
			}
			if !taken[*e.Keys] {
				taken[*e.Keys] = true
				continue
			}
			e.Keys = nil
			if data, err := json.Marshal(e); err == nil {
				s.Shortcuts[id] = data
			}
		}
	}
}

// changed reports whether the key of to differs from that of from, as
// being set in one only.
func changed(from, to map[string]json.RawMessage, k string) bool {
	a, aok := from[k]
	b, bok := to[k]
	return aok != bok || aok && !sameJSON(a, b)
}

// Local is this Mac's shortcuts and preferences. The methods are called
// on the engine's goroutine.
type Local interface {
	// Read returns the shortcuts and preferences as they are now.
	Read() (Snapshot, error)
	// Apply makes s the shortcuts and preferences. Where they are no
	// longer read, as Read returned them, it changes nothing and fails
	// with ErrStale.
	Apply(read, s Snapshot) error
	// IsDefault reports whether s is what a first launch has, which a
	// first sync replaces without asking.
	IsDefault(s Snapshot) bool
	// Known reports whether this version of Shun has the preference key:
	// one it does not have, of a newer Shun's, it keeps as the account
	// has it.
	Known(key string) bool
}

// State is what the engine keeps between launches, in sync.json: the
// account, and the version of the account's document and the snapshot of
// the last sync. Base is nil until the first sync.
type State struct {
	UserID string `json:"userId"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	// Avatar is the URL of the person's picture.
	Avatar  string    `json:"avatar,omitempty"`
	Version int64     `json:"version"`
	Base    *Snapshot `json:"base,omitempty"`
	// SyncedAt is when this Mac and the account last agreed.
	SyncedAt time.Time `json:"syncedAt,omitzero"`
}

// Choice is how a first sync goes, where both this Mac and the account
// have shortcuts of their own.
type Choice int

const (
	// Ask fails a first sync with ErrNeedsChoice when both sides have
	// shortcuts of their own.
	Ask Choice = iota
	// UseAccount replaces this Mac's with the account's.
	UseAccount
	// UseLocal replaces the account's with this Mac's.
	UseLocal
)

// ErrNeedsChoice is what a first sync fails with when this Mac and the
// account both have shortcuts of their own: sync again with UseAccount or
// UseLocal.
var ErrNeedsChoice = errors.New("cloudsync: this Mac and the account both have shortcuts")

// ErrStale is what Apply fails with when this Mac's shortcuts changed
// since they were read, as the person changed one as a sync ran. A sync
// reads them again and merges anew, staleTries times at most, and fails
// with it then: the change syncs as it is pushed.
var ErrStale = errors.New("cloudsync: this Mac's shortcuts changed as they synced")

const staleTries = 3

// Result says what a sync did.
type Result struct {
	// Applied is set when the account's changes applied here, Pushed when
	// this Mac's were saved to the account.
	Applied, Pushed bool
}

// Engine syncs Local with the account. Its methods may be called from any
// goroutine; one sync runs at a time.
type Engine struct {
	Client *mon.Client
	Local  Local
	path   string

	mu    sync.Mutex // held through a sync
	smu   sync.Mutex // guards state
	state State
}

// New returns an engine whose state is kept at path, as it last was.
func New(client *mon.Client, local Local, path string) *Engine {
	e := &Engine{Client: client, Local: local, path: path}
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &e.state)
	}
	return e
}

// State returns the engine's state.
func (e *Engine) State() State {
	e.smu.Lock()
	defer e.smu.Unlock()
	return e.state
}

// SignedIn starts syncing for the person u, whose token the client has,
// with a first sync to come.
func (e *Engine) SignedIn(u *mon.User) error {
	return e.setState(State{UserID: u.ID, Name: u.Name, Email: u.Email, Avatar: u.AvatarURL})
}

// Refreshed takes the person's name, email and picture as the account
// has them now, keeping the rest.
func (e *Engine) Refreshed(u *mon.User) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.State()
	if st.UserID != u.ID || st.Name == u.Name && st.Email == u.Email && st.Avatar == u.AvatarURL {
		return nil
	}
	st.Name, st.Email, st.Avatar = u.Name, u.Email, u.AvatarURL
	return e.setState(st)
}

// Forget forgets the account and the last sync, as the person signs out.
func (e *Engine) Forget() {
	e.smu.Lock()
	e.state = State{}
	e.smu.Unlock()
	os.Remove(e.path)
}

func (e *Engine) setState(st State) error {
	e.smu.Lock()
	e.state = st
	e.smu.Unlock()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(e.path), 0o755)
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, e.path)
}

// Push saves this Mac's changes to the account, unless there are none
// since the last sync: then it does not ask the account at all.
func (e *Engine) Push(ctx context.Context) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.State()
	if st.Base == nil {
		return Result{}, nil
	}
	local, err := e.Local.Read()
	if err != nil {
		return Result{}, err
	}
	if e.withUnknown(*st.Base, local).Equal(*st.Base) {
		return Result{}, nil
	}
	return e.sync(ctx, Ask)
}

// Sync fetches the account's shortcuts, merges them with this Mac's and
// saves the merge on both sides. A first sync goes as choice says.
// mon.ErrUnauthorized means the person signed out elsewhere, as from the
// account page: the engine has forgotten the account.
func (e *Engine) Sync(ctx context.Context, choice Choice) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sync(ctx, choice)
}

func (e *Engine) sync(ctx context.Context, choice Choice) (Result, error) {
	st := e.State()
	if st.UserID == "" {
		return Result{}, mon.ErrUnauthorized
	}
	remote, version, err := e.fetch(ctx)
	if err != nil {
		return Result{}, err
	}
	local, err := e.Local.Read()
	if err != nil {
		return Result{}, err
	}
	var base Snapshot
	if st.Base != nil {
		base = *st.Base
	} else {
		// The first sync: the side that has nothing of its own takes the
		// other's, else the one chosen wins.
		switch {
		case version == 0 || choice == UseLocal:
			base = remote
		case e.Local.IsDefault(local) || choice == UseAccount:
			base = local
		default:
			return Result{}, ErrNeedsChoice
		}
	}
	// This Mac's changes are those of local since base, which merge with
	// each version of the account's this sync gets.
	mine := e.withUnknown(base, local)
	var res Result
	var applied *Snapshot
	stale := 0
	for {
		merged := Merge(base, mine, remote)
		now := mine
		if applied != nil {
			now = *applied
		}
		if !merged.Equal(now) {
			err := e.Local.Apply(local, merged.clone())
			if errors.Is(err, ErrStale) && stale < staleTries {
				// The person changed a shortcut since it was read: merge
				// that change too, rather than undo it.
				stale++
				fresh, err := e.Local.Read()
				if err != nil {
					return res, err
				}
				mine, local = since(mine, local, fresh), fresh
				continue
			}
			if err != nil {
				return res, err
			}
			res.Applied = true
			applied = &merged
			// As applied, which a later Apply of this sync checks against.
			if local, err = e.Local.Read(); err != nil {
				return res, err
			}
		}
		if merged.Equal(remote) && version > 0 {
			st.Version, st.Base, st.SyncedAt = version, &merged, time.Now()
			return res, e.setState(st)
		}
		saved, err := e.Client.PutSettings(ctx, App, version, merged)
		var conflictErr *mon.ConflictError
		if errors.As(err, &conflictErr) {
			// Another Mac saved since: merge this Mac's changes again,
			// with what it saved.
			if remote, err = decode(conflictErr.Current.Data); err != nil {
				return res, err
			}
			version = conflictErr.Current.Version
			continue
		}
		if err != nil {
			return res, e.signedOut(err)
		}
		res.Pushed = true
		st.Version, st.Base, st.SyncedAt = saved.Version, &merged, time.Now()
		return res, e.setState(st)
	}
}

// since returns mine with what changed from was to now, read as a sync
// ran: the person's changes of then, over this Mac's of before.
func since(mine, was, now Snapshot) Snapshot {
	out := mine.clone()
	out.Shortcuts = sinceMap(out.Shortcuts, was.Shortcuts, now.Shortcuts)
	out.Settings = sinceMap(out.Settings, was.Settings, now.Settings)
	if !slices.Equal(now.Order, was.Order) {
		out.Order = slices.Clone(now.Order)
	}
	return out
}

func sinceMap(out, was, now map[string]json.RawMessage) map[string]json.RawMessage {
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	for _, m := range []map[string]json.RawMessage{was, now} {
		for k := range m {
			if !changed(was, now, k) {
				continue
			}
			if v, ok := now[k]; ok {
				out[k] = v
			} else {
				delete(out, k)
			}
		}
	}
	return out
}

// withUnknown returns local with the preferences of base this version of
// Shun does not know, as unchanged, rather than as removed.
func (e *Engine) withUnknown(base, local Snapshot) Snapshot {
	out := local
	out.Settings = maps.Clone(local.Settings)
	if out.Settings == nil {
		out.Settings = map[string]json.RawMessage{}
	}
	for k, v := range base.Settings {
		if _, ok := out.Settings[k]; !ok && !e.Local.Known(k) {
			out.Settings[k] = v
		}
	}
	return out
}

// fetch returns the account's shortcuts and their version, 0 for none.
func (e *Engine) fetch(ctx context.Context) (Snapshot, int64, error) {
	st, err := e.Client.GetSettings(ctx, App)
	if err != nil {
		return Snapshot{}, 0, e.signedOut(err)
	}
	s, err := decode(st.Data)
	return s, st.Version, err
}

func decode(data json.RawMessage) (Snapshot, error) {
	var s Snapshot
	if len(data) == 0 || string(data) == "null" {
		return s, nil
	}
	err := json.Unmarshal(data, &s)
	return s, err
}

// signedOut forgets the account when err says its token is no more.
func (e *Engine) signedOut(err error) error {
	if errors.Is(err, mon.ErrUnauthorized) {
		e.Forget()
	}
	return err
}
