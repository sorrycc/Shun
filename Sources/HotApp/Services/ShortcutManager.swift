import AppKit
import Carbon.HIToolbox
@preconcurrency import CoreFoundation
import KeyboardShortcuts
import os

private let log = Logger(subsystem: "com.hotapp", category: "ShortcutManager")

// MARK: - Lookup entry for CGEvent tap matching

private struct ShortcutEntry {
    let keyCode: Int64
    let modifierMask: CGEventFlags
    let bundleIdentifier: String
    let bundleURL: URL
}

// MARK: - Thread-safe lookup table + pause flag

private final class AtomicLookup: @unchecked Sendable {
    private let lock = OSAllocatedUnfairLock()
    private var entries: [ShortcutEntry] = []
    private var _paused = false

    var paused: Bool {
        get { lock.withLock { _paused } }
        set { lock.withLock { _paused = newValue } }
    }

    func update(_ newEntries: [ShortcutEntry]) {
        lock.withLock { entries = newEntries }
    }

    func match(keyCode: Int64, flags: CGEventFlags) -> ShortcutEntry? {
        lock.withLock {
            guard !_paused else { return nil }
            return entries.first { $0.keyCode == keyCode && $0.modifierMask == flags }
        }
    }
}

// MARK: - Modifier flag helpers

/// Mask to extract only ⌃⌥⇧⌘ from CGEventFlags, ignoring capsLock/numPad/fn/etc.
private let modifierOnlyMask: CGEventFlags = [
    .maskCommand, .maskShift, .maskControl, .maskAlternate,
]

private func cgEventFlags(fromCarbon carbon: Int) -> CGEventFlags {
    var flags = CGEventFlags()
    if carbon & cmdKey == cmdKey { flags.insert(.maskCommand) }
    if carbon & shiftKey == shiftKey { flags.insert(.maskShift) }
    if carbon & controlKey == controlKey { flags.insert(.maskControl) }
    if carbon & optionKey == optionKey { flags.insert(.maskAlternate) }
    return flags
}

// MARK: - CGEvent tap factory (free function to avoid Swift 6.2.1 SendNonSendable crash)

/// Extracted from ShortcutManager.createTap() because the compiler's SendNonSendable
/// region analysis crashes when a @MainActor method contains CGEvent.tapCreate callback
/// + Thread closure + Unmanaged pointer roundtrip in the same function body.
private func _makeEventTap(
    lookup: AtomicLookup
) -> (tap: CFMachPort, source: CFRunLoopSource, thread: Thread)? {
    let tap = CGEvent.tapCreate(
        tap: .cgSessionEventTap,
        place: .headInsertEventTap,
        options: .defaultTap,
        eventsOfInterest: CGEventMask(1 << CGEventType.keyDown.rawValue),
        callback: { _, type, event, userInfo -> Unmanaged<CGEvent>? in
            let lookup = Unmanaged<AtomicLookup>.fromOpaque(userInfo!).takeUnretainedValue()

            // If macOS disabled our tap, pass through
            if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
                return Unmanaged.passUnretained(event)
            }

            guard type == .keyDown else {
                return Unmanaged.passUnretained(event)
            }

            // Ignore key repeats
            if event.getIntegerValueField(.keyboardEventAutorepeat) != 0 {
                return Unmanaged.passUnretained(event)
            }

            let keyCode = Int64(event.getIntegerValueField(.keyboardEventKeycode))
            let rawFlags = event.flags
            let flags = rawFlags.intersection(modifierOnlyMask)

            guard !flags.isEmpty else {
                return Unmanaged.passUnretained(event)
            }

            log.debug("tap: keyCode=\(keyCode) rawFlags=\(rawFlags.rawValue, format: .hex) masked=\(flags.rawValue, format: .hex)")

            // match() returns nil when paused (app is active / Recorder in use)
            if let entry = lookup.match(keyCode: keyCode, flags: flags) {
                DispatchQueue.main.async {
                    ShortcutManager.toggleApp(
                        bundleIdentifier: entry.bundleIdentifier,
                        bundleURL: entry.bundleURL
                    )
                }
                return nil // consume event
            }

            return Unmanaged.passUnretained(event)
        },
        userInfo: Unmanaged.passUnretained(lookup).toOpaque()
    )

    guard let tap else { return nil }

    guard let source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0) else {
        return nil
    }

    let thread = Thread {
        CFRunLoopAddSource(CFRunLoopGetCurrent(), source, .commonModes)
        CFRunLoopRun()
    }
    thread.name = "HotApp.EventTap"
    thread.qualityOfService = .userInteractive

    return (tap, source, thread)
}

// MARK: - ShortcutManager

@MainActor
final class ShortcutManager {
    private let store: ShortcutStore
    private let lookup = AtomicLookup()

    private var eventTap: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private var tapThread: Thread?

    /// Falls back to Carbon hotkeys if CGEvent tap creation fails.
    private var usingCarbonFallback = false

    private var activationObservers: [NSObjectProtocol] = []

    init(store: ShortcutStore) {
        self.store = store
        observeAppActivation()
    }

    // No deinit needed — ShortcutManager lives for the app's entire lifetime (owned by AppDelegate).

    // MARK: - Public

    func refreshRegistrations() {
        if usingCarbonFallback {
            refreshCarbonRegistrations()
            return
        }

        // Build new lookup entries
        var entries: [ShortcutEntry] = []
        if store.globalEnabled {
            for shortcut in store.shortcuts where shortcut.isEnabled {
                guard let ks = KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName) else {
                    log.debug("refresh: no combo stored for \(shortcut.displayName)")
                    continue
                }
                let flags = cgEventFlags(fromCarbon: ks.carbonModifiers)
                log.debug("refresh: \(shortcut.displayName) keyCode=\(ks.carbonKeyCode) carbonMod=\(UInt(bitPattern: ks.carbonModifiers), format: .hex) cgFlags=\(flags.rawValue, format: .hex)")
                entries.append(ShortcutEntry(
                    keyCode: Int64(ks.carbonKeyCode),
                    modifierMask: flags,
                    bundleIdentifier: shortcut.bundleIdentifier,
                    bundleURL: shortcut.bundleURL
                ))
            }
        }
        log.debug("refresh: \(entries.count) entries registered (globalEnabled=\(self.store.globalEnabled))")
        lookup.update(entries)

        // Tear down Carbon hotkeys that KeyboardShortcuts.Recorder auto-registers
        // via RegisterEventHotKey. They can intercept events before our CGEvent tap,
        // breaking Karabiner hyper key (synthetic ⌃⌥⇧⌘).
        for shortcut in store.shortcuts {
            KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
        }

        // Ensure tap exists (or re-enable if macOS disabled it)
        if let tap = eventTap {
            let shouldEnable = store.globalEnabled && !entries.isEmpty
            if !CGEvent.tapIsEnabled(tap: tap) && shouldEnable {
                CGEvent.tapEnable(tap: tap, enable: true)
            } else if !shouldEnable {
                CGEvent.tapEnable(tap: tap, enable: false)
            }
        } else {
            createTap()
        }
    }

    func register(_ shortcut: AppShortcut) {
        refreshRegistrations()
    }

    func unregister(_ shortcut: AppShortcut) {
        refreshRegistrations()
    }

    // MARK: - Pause when our app is active (Recorder needs events)

    private func observeAppActivation() {
        let lookup = self.lookup

        let didActivate = NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification,
            object: nil,
            queue: .main
        ) { _ in
            lookup.paused = true
        }

        let didDeactivate = NotificationCenter.default.addObserver(
            forName: NSApplication.didResignActiveNotification,
            object: nil,
            queue: .main
        ) { _ in
            lookup.paused = false
        }

        activationObservers = [didActivate, didDeactivate]
    }

    // MARK: - CGEvent Tap lifecycle

    private func createTap() {
        guard let result = _makeEventTap(lookup: lookup) else {
            usingCarbonFallback = true
            refreshCarbonRegistrations()
            return
        }

        self.eventTap = result.tap
        self.runLoopSource = result.source
        result.thread.start()
        self.tapThread = result.thread
    }

    // MARK: - App toggling

    @MainActor
    fileprivate static func toggleApp(bundleIdentifier: String, bundleURL: URL) {
        let workspace = NSWorkspace.shared
        if let frontmost = workspace.frontmostApplication,
           frontmost.bundleIdentifier == bundleIdentifier
        {
            frontmost.hide()
        } else {
            let config = NSWorkspace.OpenConfiguration()
            config.activates = true
            workspace.openApplication(at: bundleURL, configuration: config)
        }
    }

    // MARK: - Carbon fallback (original KeyboardShortcuts-based dispatch)

    private func refreshCarbonRegistrations() {
        unregisterAllCarbon()
        guard store.globalEnabled else { return }
        for shortcut in store.shortcuts where shortcut.isEnabled {
            registerCarbon(shortcut)
        }
    }

    private func registerCarbon(_ shortcut: AppShortcut) {
        let name = shortcut.keyboardShortcutName
        let bundleIdentifier = shortcut.bundleIdentifier
        let bundleURL = shortcut.bundleURL

        KeyboardShortcuts.onKeyUp(for: name) {
            Self.toggleApp(bundleIdentifier: bundleIdentifier, bundleURL: bundleURL)
        }
    }

    private func unregisterAllCarbon() {
        for shortcut in store.shortcuts {
            KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
        }
    }
}
