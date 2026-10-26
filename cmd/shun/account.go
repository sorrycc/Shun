package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"shun/internal/cloudsync"
	"shun/internal/mon"
	"shun/internal/store"
)

// monURL is Mon's, the account behind settings sync and crash reports, or
// SHUN_MON_URL's, as a Mon on localhost to try it with.
func monURL() string {
	return cmp.Or(os.Getenv("SHUN_MON_URL"), "https://mon.sorrycc.dev")
}

// monHost is Mon's host name, as the person sees it in the browser.
func monHost() string {
	if u, err := url.Parse(monURL()); err == nil && u.Host != "" {
		return u.Host
	}
	return monURL()
}

// tokenKey is the key of Mon's access token in the Keychain, and the name
// of the file that keeps it in development.
const tokenKey = "mon-token"

// pushDelay is how long settings sync waits after a change before saving
// it to the account, so that changes made together go together.
const pushDelay = 2 * time.Second

// pullEvery is how often, at most, settings sync asks the account for
// changes as Shun comes to the front.
const pullEvery = 30 * time.Second

// pullPeriod is how often settings sync asks the account for changes in
// the background: a menu bar app seldom comes to the front.
const pullPeriod = 15 * time.Minute

// quitPush is how long, at most, quitting waits to save a change made just
// before to the account.
const quitPush = 3 * time.Second

// syncRetryDelay is how long settings sync waits to try again after its
// n-th failure in a row, as the connection comes back.
func syncRetryDelay(n int) time.Duration {
	delays := []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute}
	if n < len(delays) {
		return delays[n]
	}
	return 5 * time.Minute
}

// errQuitting is what applying the account's shortcuts fails with as Shun
// quits: they apply at the next launch.
var errQuitting = errors.New("shun is quitting")

// cloud syncs the shortcuts and preferences with the person's Mon account.
// Its fields are the main thread's, but for the engine.
type cloud struct {
	a      *app
	client *mon.Client
	engine *cloudsync.Engine
	// signin is the sign-in waiting for its approval, nil for none.
	signin *signin
	// asked is set once a first sync asked whose shortcuts to keep, which
	// a sync of its own asks again only as the person syncs.
	asked    bool
	push     *time.Timer
	lastPull time.Time
	busy     int
	// failure says why the last sync failed, "" when it did not; retry
	// tries again at retryAt, after failures in a row.
	failure  string
	retry    *time.Timer
	retryAt  time.Time
	failures int
	// avatar is the person's picture, of avatarURL, once fetched.
	avatar    *ui.Bitmap
	avatarURL string
	// frozen is what a push as Shun quits reads, as the main thread waits
	// for it; quitting fails an Apply then.
	frozen   atomic.Pointer[cloudsync.Snapshot]
	quitting atomic.Bool
	// onMain runs fn on the main thread and waits for it: mygo.RunOnMain,
	// but in tests.
	onMain func(fn func())
	// tokenFile keeps the token in development, "" for the Keychain.
	tokenFile string
	// resolve finds an app of the account's on this Mac.
	resolve func(bundleID, name string) (string, bool)
}

// signin is a sign-in under way: the code to approve and where, once Mon
// gave them, and when the code expires.
type signin struct {
	code, url string
	expires   time.Time
	cancel    context.CancelFunc
}

// newCloud returns settings sync for a, with the account it last had.
func newCloud(a *app, dir string) *cloud {
	c := &cloud{a: a, client: mon.New(monURL()), onMain: mygo.RunOnMain, resolve: resolveApp}
	if mygo.IsDev() {
		// Each development build signs anew, ad hoc, and the Keychain asks
		// again whether it may read what the last one kept.
		c.tokenFile = filepath.Join(dir, tokenKey)
	}
	c.engine = cloudsync.New(c.client, cloudLocal{c}, filepath.Join(dir, "sync.json"))
	return c
}

// start loads the token of the account settings sync last had, and syncs
// with it, then as Shun comes to the front and every pullPeriod.
func (c *cloud) start() {
	mygo.App.OnDidBecomeActive(func() {
		if time.Since(c.lastPull) > pullEvery {
			c.sync(cloudsync.Ask, false)
		}
	})
	go func() {
		for range time.Tick(pullPeriod) {
			mygo.RunOnMain(func() {
				if time.Since(c.lastPull) > pullPeriod/2 {
					c.sync(cloudsync.Ask, false)
				}
			})
		}
	}()
	if c.engine.State().UserID == "" {
		return
	}
	go func() {
		token, err := c.loadToken()
		mygo.RunOnMain(func() {
			if errors.Is(err, mygo.ErrSecretNotFound) {
				// The token is gone: the next sign-in starts anew.
				c.engine.Forget()
				c.notify()
				return
			}
			if err != nil {
				log.Printf("sync: reading the token: %v", err)
				return
			}
			c.client.Token = string(token)
			c.sync(cloudsync.Ask, false)
			c.refreshProfile()
		})
	}()
}

// refreshProfile takes the person's name and picture as the account has
// them now, which they may have changed since signing in.
func (c *cloud) refreshProfile() {
	client := *c.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u, err := client.Me(ctx)
		if err != nil {
			return
		}
		if c.engine.Refreshed(u) == nil {
			mygo.RunOnMain(c.notify)
		}
	}()
}

// account is the name of the person settings sync syncs for, "" for no
// one.
func (c *cloud) account() string {
	if c == nil {
		return ""
	}
	st := c.engine.State()
	if st.UserID == "" {
		return ""
	}
	return cmp.Or(st.Name, st.Email, st.UserID)
}

// notify shows what changed of settings sync: in Settings, and in the
// menu bar menu.
func (c *cloud) notify() {
	c.a.invalidate()
	c.a.updateTrayMenu()
}

// changed saves this Mac's shortcuts to the account a moment after they
// change.
func (c *cloud) changed() {
	if c == nil || c.account() == "" || c.client.Token == "" {
		return
	}
	if c.push != nil {
		c.push.Stop()
	}
	c.push = time.AfterFunc(pushDelay, func() {
		mygo.RunOnMain(func() { c.run(false, c.engine.Push) })
	})
}

// sync merges the account's shortcuts with this Mac's, the first time as
// choice says. manual tells of a failure, as Sync Now does.
func (c *cloud) sync(choice cloudsync.Choice, manual bool) {
	if c.account() == "" || c.client.Token == "" {
		return
	}
	c.lastPull = time.Now()
	c.run(manual, func(ctx context.Context) (cloudsync.Result, error) { return c.engine.Sync(ctx, choice) })
}

// run runs a sync off the main thread, showing that it does.
func (c *cloud) run(manual bool, job func(context.Context) (cloudsync.Result, error)) {
	c.busy++
	if c.retry != nil {
		c.retry.Stop()
		c.retry = nil
	}
	c.notify()
	go func() {
		res, err := job(context.Background())
		mygo.RunOnMain(func() {
			c.busy--
			c.done(res, err, manual)
			c.notify()
		})
	}()
}

// done tells of a sync's outcome, and asks whose shortcuts to keep when a
// first sync needs to know.
func (c *cloud) done(_ cloudsync.Result, err error, manual bool) {
	if err == nil || errors.Is(err, cloudsync.ErrNeedsChoice) || errors.Is(err, cloudsync.ErrStale) {
		c.failure, c.failures, c.retryAt = "", 0, time.Time{}
	}
	switch {
	case err == nil, errors.Is(err, errQuitting):
	case errors.Is(err, cloudsync.ErrStale):
		// The shortcuts kept changing as it synced: the push of the last
		// change syncs them.
	case errors.Is(err, cloudsync.ErrNeedsChoice):
		if c.asked && !manual {
			return
		}
		c.asked = true
		c.a.ui.tab = tabAccount
		c.a.ui.choosing = true
		c.a.openSettings()
	case errors.Is(err, mon.ErrUnauthorized):
		// Signed out elsewhere, as from the account page.
		c.forget()
		c.failure = "You were signed out. Sign in again to sync."
	default:
		// Offline, most likely: nothing is lost, and the next sync merges
		// what this one did not.
		log.Printf("sync: %v", err)
		c.failure = syncError(err)
		c.retryAt = time.Now().Add(syncRetryDelay(c.failures))
		c.retry = time.AfterFunc(time.Until(c.retryAt), func() {
			mygo.RunOnMain(func() {
				c.retry = nil
				c.sync(cloudsync.Ask, false)
			})
		})
		c.failures++
		if manual {
			c.a.alert("Couldn't Sync", c.failure)
		}
	}
}

// syncError says why a sync failed, in words for the person rather than
// the network's.
func syncError(err error) string {
	var me *mon.Error
	var ne net.Error
	switch {
	case errors.As(err, &me) && me.Status >= 500:
		return monHost() + " is having trouble. Shun will try again."
	case errors.As(err, &ne), errors.Is(err, context.DeadlineExceeded), errors.Is(err, io.ErrUnexpectedEOF):
		return "Couldn't reach " + monHost() + ". Shun will try again."
	}
	return err.Error()
}

// flush saves a change waiting to go to the account, as Shun quits,
// waiting for it a moment at most.
func (c *cloud) flush() {
	if c == nil || c.push == nil || !c.push.Stop() || c.client.Token == "" {
		return
	}
	snap := c.snapshot()
	c.frozen.Store(&snap)
	c.quitting.Store(true)
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), quitPush)
		defer cancel()
		c.engine.Push(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(quitPush + 250*time.Millisecond):
	}
}

// deviceName is how Mon lists Shun signed in on this Mac: its name, as in
// Sharing settings. A development build signs in apart from the release,
// and says so.
func deviceName() string {
	name := ""
	if out, err := exec.Command("/usr/sbin/scutil", "--get", "ComputerName").Output(); err == nil {
		name = strings.TrimSpace(string(out))
	}
	if name == "" {
		host, _ := os.Hostname()
		name = cmp.Or(strings.TrimSuffix(host, ".local"), "Mac")
	}
	if mygo.IsDev() {
		name += " (dev)"
	}
	return name
}

// signIn signs in to Mon, showing the code to approve in the Account tab,
// and syncs once it is approved.
func (c *cloud) signIn() {
	if c.signin != nil {
		c.signin.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	si := &signin{cancel: cancel}
	c.signin = si
	c.failure = ""
	c.notify()
	// The countdown of the code.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				mygo.RunOnMain(c.a.invalidate)
			}
		}
	}()
	go func() {
		// A token still kept is signed out first, so Mon doesn't list it on.
		if old, err := c.loadToken(); err == nil && len(old) > 0 {
			stale := *c.client
			stale.Token = string(old)
			lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			stale.Logout(lctx)
			cancel()
		}
		dc, err := c.client.StartDeviceLogin(ctx, cloudsync.App, deviceName())
		if err != nil {
			mygo.RunOnMain(func() { c.signInFailed(si, ctx, err) })
			return
		}
		mygo.RunOnMain(func() {
			if ctx.Err() != nil || c.signin != si {
				return
			}
			si.code, si.url = dc.UserCode, dc.VerificationURIComplete
			if dc.ExpiresIn > 0 {
				si.expires = time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
			}
			mygo.Shell.OpenExternal(dc.VerificationURIComplete)
			c.notify()
		})
		token, err := c.client.WaitForDeviceToken(ctx, dc)
		var u *mon.User
		if err == nil {
			err = c.saveToken([]byte(token))
		}
		if err == nil {
			signedIn := *c.client
			signedIn.Token = token
			u, err = signedIn.Me(ctx)
		}
		mygo.RunOnMain(func() {
			if err == nil {
				c.client.Token = token
				err = c.engine.SignedIn(u)
			}
			if err != nil {
				c.signInFailed(si, ctx, err)
				return
			}
			c.dropSignin(si)
			c.asked = false
			c.sync(cloudsync.Ask, false)
			c.refreshProfile()
		})
	}()
}

// cancelSignIn stops the sign-in under way.
func (c *cloud) cancelSignIn() {
	if si := c.signin; si != nil {
		si.cancel()
		c.dropSignin(si)
	}
}

// dropSignin forgets the sign-in si, if it is still the one under way.
func (c *cloud) dropSignin(si *signin) {
	if c.signin == si {
		si.cancel()
		c.signin = nil
		c.notify()
	}
}

// signInFailed says why a sign-in did not happen, unless it was
// cancelled.
func (c *cloud) signInFailed(si *signin, ctx context.Context, err error) {
	cancelled := ctx.Err() != nil
	c.dropSignin(si)
	switch {
	case cancelled:
	case errors.Is(err, mon.ErrAccessDenied):
		c.failure = "The sign-in was declined."
	case errors.Is(err, mon.ErrExpired):
		c.failure = "The code expired. Sign in again to get a new one."
	case errors.Is(err, errors.ErrUnsupported):
		c.failure = "Shun can't keep the sign-in: the Keychain is out of reach."
	default:
		log.Printf("sync: signing in: %v", err)
		c.failure = "Couldn't sign in: " + syncError(err)
	}
	c.notify()
}

// signOut stops syncing, and revokes the token on Mon.
func (c *cloud) signOut() {
	client := *c.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client.Logout(ctx)
	}()
	c.forget()
}

// loadToken returns the token kept at sign-in, or mygo.ErrSecretNotFound.
func (c *cloud) loadToken() ([]byte, error) {
	if c.tokenFile == "" {
		return mygo.Secrets.Get(tokenKey)
	}
	b, err := os.ReadFile(c.tokenFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, mygo.ErrSecretNotFound
	}
	return b, err
}

// saveToken keeps the token for the next launches.
func (c *cloud) saveToken(token []byte) error {
	if c.tokenFile == "" {
		return mygo.Secrets.Set(tokenKey, token)
	}
	return os.WriteFile(c.tokenFile, token, 0o600)
}

// dropToken removes the token kept, if any.
func (c *cloud) dropToken() error {
	if c.tokenFile == "" {
		return mygo.Secrets.Delete(tokenKey)
	}
	if err := os.Remove(c.tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// forget forgets the account and its token, as signed out.
func (c *cloud) forget() {
	if c.push != nil {
		c.push.Stop()
	}
	if c.retry != nil {
		c.retry.Stop()
		c.retry = nil
	}
	c.failure, c.failures, c.retryAt = "", 0, time.Time{}
	c.avatar, c.avatarURL = nil, ""
	c.client.Token = ""
	c.engine.Forget()
	go c.dropToken()
	c.notify()
}

// picture returns the person's picture, nil until it is fetched, which it
// starts the first time it is asked for.
func (c *cloud) picture() *ui.Bitmap {
	url := c.engine.State().Avatar
	if url == "" || url == c.avatarURL {
		return c.avatar
	}
	c.avatarURL, c.avatar = url, nil
	go func() {
		bm := fetchAvatar(url)
		if bm == nil {
			return
		}
		mygo.RunOnMain(func() {
			if c.avatarURL == url {
				c.avatar = bm
				c.notify()
			}
		})
	}()
	return nil
}

// fetchAvatar fetches the picture at url, nil if it cannot.
func fetchAvatar(url string) *ui.Bitmap {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
}

// syncStatus says how settings sync is, for the Account tab.
func (c *cloud) syncStatus(now time.Time) string {
	st := c.engine.State()
	switch {
	case c.busy > 0:
		return "Syncing…"
	case c.failure != "" && !c.retryAt.IsZero():
		return fmt.Sprintf("%s Next try %s.", c.failure, untilText(c.retryAt.Sub(now)))
	case c.failure != "":
		return c.failure
	case st.SyncedAt.IsZero():
		return "Not synced yet."
	}
	return "Synced " + agoText(now.Sub(st.SyncedAt)) + "."
}

// agoText says how long ago d was, roughly.
func agoText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	}
	return plural(int(d/(24*time.Hour)), "day") + " ago"
}

// untilText says how long until d is over, roughly.
func untilText(d time.Duration) string {
	if d < time.Minute {
		return "in " + plural(max(1, int(d/time.Second)), "second")
	}
	return "in " + plural(int(d/time.Minute), "minute")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// snapshot is this Mac's shortcuts and preferences as they sync.
func (c *cloud) snapshot() cloudsync.Snapshot {
	return cloudsync.Of(c.a.store.Shortcuts, c.a.prefs.Synced())
}

// cloudLocal is this Mac's shortcuts and preferences, as settings sync
// reads and writes them, from the engine's goroutine.
type cloudLocal struct{ c *cloud }

func (l cloudLocal) Read() (cloudsync.Snapshot, error) {
	if f := l.c.frozen.Load(); f != nil {
		return *f, nil
	}
	var s cloudsync.Snapshot
	l.c.onMain(func() { s = l.c.snapshot() })
	return s, nil
}

// Apply checks and applies on the main thread, where the person changes
// the shortcuts, so that none of their changes falls between the two.
func (l cloudLocal) Apply(read, s cloudsync.Snapshot) error {
	if l.c.quitting.Load() {
		return errQuitting
	}
	var err error
	l.c.onMain(func() {
		c := l.c
		if !c.snapshot().Equal(read) {
			err = cloudsync.ErrStale
			return
		}
		a := c.a
		if !sameShortcuts(read, s) {
			a.store.Replace(cloudsync.ShortcutsOf(s, a.store.Shortcuts, c.resolve))
		}
		if a.prefs.SetSynced(s.Settings) {
			a.globalEnabledChanged()
		}
	})
	return err
}

func (l cloudLocal) IsDefault(s cloudsync.Snapshot) bool { return len(s.Shortcuts) == 0 }

func (l cloudLocal) Known(key string) bool { return slices.Contains(store.SyncedPrefs, key) }

// sameShortcuts reports whether a and b hold the same shortcuts, whatever
// their preferences.
func sameShortcuts(a, b cloudsync.Snapshot) bool {
	a.Settings, b.Settings = nil, nil
	return a.Equal(b)
}
