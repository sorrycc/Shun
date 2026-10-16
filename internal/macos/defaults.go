//go:build darwin

package macos

import "github.com/ebitengine/purego"

// Defaults reads the UserDefaults of an app domain, such as com.shun.Shun,
// as CFPreferences keeps them.
type Defaults struct{ AppID string }

// value calls fn with the value of key, or 0, in an autorelease pool.
func (d Defaults) value(key string, fn func(v id)) {
	withPool(func() {
		copyValue := mustDlsym(libCF, "CFPreferencesCopyAppValue")
		r, _, _ := purego.SyscallN(copyValue, uintptr(nsString(key)), uintptr(nsString(d.AppID)))
		v := id(r)
		defer release(v)
		fn(v)
	})
}

// String returns a string value.
func (d Defaults) String(key string) (s string, ok bool) {
	d.value(key, func(v id) {
		if isKind(v, "NSString") {
			s, ok = goString(v), true
		}
	})
	return
}

// Bool returns a boolean value.
func (d Defaults) Bool(key string) (b, ok bool) {
	d.value(key, func(v id) {
		if isKind(v, "NSNumber") {
			b, ok = sendBool(v, "boolValue"), true
		}
	})
	return
}

// Strings returns an array of strings.
func (d Defaults) Strings(key string) (list []string, ok bool) {
	d.value(key, func(v id) {
		if !isKind(v, "NSArray") {
			return
		}
		ok = true
		n := int(send(v, "count"))
		for i := range n {
			if s := goString(send(v, "objectAtIndex:", uintptr(i))); s != "" {
				list = append(list, s)
			}
		}
	})
	return
}
