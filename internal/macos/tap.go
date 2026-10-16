//go:build darwin

package macos

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// KeyHandler decides about a key press: true takes it from the apps.
type KeyHandler func(keyCode int, flags uint64) bool

const (
	kCGSessionEventTap       = 1
	kCGHeadInsertEventTap    = 0
	kCGEventTapOptionDefault = 0
	kCGEventKeyDown          = 10
	tapDisabledByTimeout     = 0xFFFFFFFE
	tapDisabledByUserInput   = 0xFFFFFFFF

	kCGKeyboardEventAutorepeat = 8
	kCGKeyboardEventKeycode    = 9
)

// EventTap takes key presses before the apps get them, on a thread of its
// own, without the main thread.
type EventTap struct {
	port    uintptr
	handler KeyHandler
}

var (
	tapFns struct {
		create, enable, isEnabled, getField, getFlags uintptr
		createSource, getCurrent, addSource, run      uintptr
		commonModes                                   uintptr
	}
	tapFnsOnce sync.Once

	// theTap is the one event tap; the callback, made once, reads it.
	theTap   *EventTap
	callback uintptr
)

func loadTap() {
	tapFnsOnce.Do(func() {
		load()
		f := &tapFns
		f.create = mustDlsym(libCG, "CGEventTapCreate")
		f.enable = mustDlsym(libCG, "CGEventTapEnable")
		f.isEnabled = mustDlsym(libCG, "CGEventTapIsEnabled")
		f.getField = mustDlsym(libCG, "CGEventGetIntegerValueField")
		f.getFlags = mustDlsym(libCG, "CGEventGetFlags")
		f.createSource = mustDlsym(libCF, "CFMachPortCreateRunLoopSource")
		f.getCurrent = mustDlsym(libCF, "CFRunLoopGetCurrent")
		f.addSource = mustDlsym(libCF, "CFRunLoopAddSource")
		f.run = mustDlsym(libCF, "CFRunLoopRun")
		modes := mustDlsym(libCF, "kCFRunLoopCommonModes") // a CFStringRef variable
		f.commonModes = **(**uintptr)(unsafe.Pointer(&modes))
		callback = purego.NewCallback(tapCallback)
	})
}

// tapCallback is the CGEventTapCallBack. It runs on the tap's thread.
func tapCallback(proxy, typ, event, info uintptr) uintptr {
	t := theTap
	if t == nil {
		return event
	}
	switch uint32(typ) {
	case tapDisabledByTimeout, tapDisabledByUserInput:
		// macOS turns a slow tap off: turn it back on.
		purego.SyscallN(tapFns.enable, t.port, 1)
		return event
	case kCGEventKeyDown:
	default:
		return event
	}
	if r, _, _ := purego.SyscallN(tapFns.getField, event, kCGKeyboardEventAutorepeat); r != 0 {
		return event
	}
	code, _, _ := purego.SyscallN(tapFns.getField, event, kCGKeyboardEventKeycode)
	flags, _, _ := purego.SyscallN(tapFns.getFlags, event)
	pool, _, _ := purego.SyscallN(poolPush)
	take := t.handler(int(code), uint64(flags))
	purego.SyscallN(poolPop, pool)
	if take {
		return 0
	}
	return event
}

// StartEventTap starts taking key presses. It fails without the
// Accessibility permission. There is one event tap per process.
func StartEventTap(h KeyHandler) (*EventTap, error) {
	if theTap != nil {
		return nil, errors.New("shun: an event tap runs already")
	}
	loadTap()
	t := &EventTap{handler: h}
	theTap = t
	started := make(chan error, 1)
	go func() {
		// The tap's run loop is this thread's: the callback comes on it.
		runtime.LockOSThread()
		f := &tapFns
		port, _, _ := purego.SyscallN(f.create, kCGSessionEventTap, kCGHeadInsertEventTap,
			kCGEventTapOptionDefault, 1<<kCGEventKeyDown, callback, 0)
		if port == 0 {
			theTap = nil
			started <- errors.New("shun: cannot create the event tap; Shun needs the Accessibility permission")
			return
		}
		t.port = port
		source, _, _ := purego.SyscallN(f.createSource, 0, port, 0)
		loop, _, _ := purego.SyscallN(f.getCurrent)
		purego.SyscallN(f.addSource, loop, source, f.commonModes)
		started <- nil
		purego.SyscallN(f.run) // until the process exits
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return t, nil
}

// SetEnabled turns the tap on or off.
func (t *EventTap) SetEnabled(on bool) {
	var b uintptr
	if on {
		b = 1
	}
	purego.SyscallN(tapFns.enable, t.port, b)
}

// Enabled reports whether the tap is on.
func (t *EventTap) Enabled() bool {
	r, _, _ := purego.SyscallN(tapFns.isEnabled, t.port)
	return byte(r) != 0
}
