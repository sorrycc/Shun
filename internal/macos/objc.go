//go:build darwin

// Package macos calls the macOS APIs that Shun needs and MyGo does not
// offer: the event tap that takes the keys of shortcuts, running apps and
// their activation, the Accessibility permission, app bundles, and the
// UserDefaults of the Swift versions of Shun. It calls them with purego,
// without cgo, as MyGo does.
package macos

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type id uintptr

var (
	libObjC, libCF, libCG, libAppKit, libAS uintptr

	msgSendAddr       uintptr
	poolPush, poolPop uintptr
	cfRelease         uintptr
	loadOnce          sync.Once
)

func mustDlopen(path string) uintptr {
	h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		panic(fmt.Sprintf("shun: cannot load %s: %v", path, err))
	}
	return h
}

func mustDlsym(lib uintptr, name string) uintptr {
	p, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(fmt.Sprintf("shun: %v", err))
	}
	return p
}

// load opens the frameworks, once.
func load() {
	loadOnce.Do(func() {
		libObjC = mustDlopen("/usr/lib/libobjc.A.dylib")
		mustDlopen("/System/Library/Frameworks/Foundation.framework/Foundation")
		libCF = mustDlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		libCG = mustDlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		libAppKit = mustDlopen("/System/Library/Frameworks/AppKit.framework/AppKit")
		libAS = mustDlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
		msgSendAddr = mustDlsym(libObjC, "objc_msgSend")
		poolPush = mustDlsym(libObjC, "objc_autoreleasePoolPush")
		poolPop = mustDlsym(libObjC, "objc_autoreleasePoolPop")
		cfRelease = mustDlsym(libCF, "CFRelease")
	})
}

var (
	selMu    sync.RWMutex
	selCache = map[string]objc.SEL{}
	clsCache = map[string]id{}
)

func sel(name string) objc.SEL {
	selMu.RLock()
	s, ok := selCache[name]
	selMu.RUnlock()
	if ok {
		return s
	}
	s = objc.RegisterName(name)
	selMu.Lock()
	selCache[name] = s
	selMu.Unlock()
	return s
}

func class(name string) id {
	selMu.RLock()
	c, ok := clsCache[name]
	selMu.RUnlock()
	if ok {
		return c
	}
	load()
	c = id(objc.GetClass(name))
	if c == 0 {
		panic("shun: Objective-C class " + name + " not found")
	}
	selMu.Lock()
	clsCache[name] = c
	selMu.Unlock()
	return c
}

// send sends a message whose arguments are integers or pointers. Go
// pointers converted in the call are moved to the heap and kept alive
// until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func send(obj id, selector string, args ...uintptr) id {
	var a [8]uintptr
	a[0], a[1] = uintptr(obj), uintptr(sel(selector))
	n := copy(a[2:], args)
	r, _, _ := purego.SyscallN(msgSendAddr, a[:n+2]...)
	return id(r)
}

//go:uintptrescapes
func sendBool(obj id, selector string, args ...uintptr) bool {
	return byte(send(obj, selector, args...)) != 0
}

// isKind reports whether obj is an instance of a class or of a subclass.
func isKind(obj id, className string) bool {
	return obj != 0 && sendBool(obj, "isKindOfClass:", uintptr(class(className)))
}

// withPool runs fn in an autorelease pool, on one thread.
func withPool(fn func()) {
	load()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool, _, _ := purego.SyscallN(poolPush)
	defer purego.SyscallN(poolPop, pool)
	fn()
}

const nsUTF8StringEncoding = 4

// nsString returns an autoreleased NSString.
func nsString(s string) id {
	b := unsafe.StringData(s)
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:",
		uintptr(unsafe.Pointer(b)), uintptr(len(s)), nsUTF8StringEncoding)
	runtime.KeepAlive(s)
	return send(str, "autorelease")
}

// goString copies an NSString.
func goString(str id) string {
	if str == 0 || !isKind(str, "NSString") {
		return ""
	}
	p := send(str, "UTF8String")
	if p == 0 {
		return ""
	}
	n := int(send(str, "lengthOfBytesUsingEncoding:", nsUTF8StringEncoding))
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n))
}

func release(obj id) {
	if obj != 0 {
		purego.SyscallN(cfRelease, uintptr(obj))
	}
}
