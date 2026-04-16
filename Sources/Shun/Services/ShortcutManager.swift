import AppKit
import Carbon.HIToolbox
@preconcurrency import CoreFoundation
import KeyboardShortcuts
import os

private let log = Logger(subsystem: "com.shun", category: "ShortcutManager")

// MARK: - Shortcut lookup types

private struct ShortcutEntry {
    let bundleIdentifier: String
    let bundleURL: URL
}

private struct ShortcutKey: Hashable {
    let keyCode: Int64
    let modifierMask: UInt64
}

// MARK: - Unified tap context (thread-safe)

/// Holds all state needed by the CGEvent tap callback: shortcut lookup table,
/// running app cache, frontmost app, pause flag, and tap port for re-enabling.
/// Single OSAllocatedUnfairLock — all operations are sub-microsecond dictionary lookups.
private final class TapContext: @unchecked Sendable {
    private let lock = OSAllocatedUnfairLock()

    // Shortcut lookup (updated on shortcut CRUD)
    private var table: [ShortcutKey: ShortcutEntry] = [:]
    private var _paused = false

    // Running app cache (updated via workspace notifications)
    private var _runningApps: [String: NSRunningApplication] = [:]
    private var _frontmostApp: NSRunningApplication?

    // Tap port for re-enabling on tapDisabledByTimeout
    private var _tapPort: CFMachPort?

    // Usage logging callback (dispatches to main → UsageStore.log)
    var logActivation: (@Sendable (String) -> Void)?

    // MARK: - Shortcut table

    var paused: Bool {
        get { lock.withLock { _paused } }
        set { lock.withLock { _paused = newValue } }
    }

    func updateShortcuts(_ entries: [(key: ShortcutKey, entry: ShortcutEntry)]) {
        lock.withLock {
            table = Dictionary(entries.map { ($0.key, $0.entry) }, uniquingKeysWith: { _, last in last })
        }
    }

    var shortcutsEmpty: Bool {
        lock.withLock { table.isEmpty }
    }

    // MARK: - Running app cache

    func updateRunningApp(bundleId: String, app: NSRunningApplication) {
        lock.withLock { _runningApps[bundleId] = app }
    }

    func removeRunningApp(bundleId: String) {
        lock.withLock { _ = _runningApps.removeValue(forKey: bundleId) }
    }

    func updateFrontmost(_ app: NSRunningApplication?) {
        lock.withLock { _frontmostApp = app }
    }

    // MARK: - Tap port

    var tapPort: CFMachPort? {
        get { lock.withLock { _tapPort } }
        set { lock.withLock { _tapPort = newValue } }
    }

    // MARK: - Hot path: match + resolve in one lock acquisition

    enum ToggleAction {
        case hide(NSRunningApplication)
        case activate(NSRunningApplication)
        case launch(URL)
    }

    func matchAndResolve(keyCode: Int64, flags: CGEventFlags) -> ToggleAction? {
        lock.withLock {
            guard !_paused else { return nil }
            guard let entry = table[ShortcutKey(keyCode: keyCode, modifierMask: flags.rawValue)] else {
                return nil
            }

            // Target is frontmost → hide (toggle)
            if let frontmost = _frontmostApp,
               frontmost.bundleIdentifier == entry.bundleIdentifier
            {
                return .hide(frontmost)
            }

            // Target is running → activate
            if let app = _runningApps[entry.bundleIdentifier], !app.isTerminated {
                return .activate(app)
            }

            // Target not running → cold launch
            return .launch(entry.bundleURL)
        }
    }
}

// MARK: - Modifier flag helpers

/// Mask to extract only control/option/shift/command from CGEventFlags.
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

// MARK: - SkyLight fast activation (bypasses Apple Event Manager)

/// SkyLight fast-activation symbols, loaded once via dlsym.
/// Both are deprecated/private but functional on macOS 14+.
private let _skyLightSymbols: (
    getProcessForPID: @convention(c) (pid_t, UnsafeMutablePointer<ProcessSerialNumber>) -> OSStatus,
    setFrontProcess: @convention(c) (UnsafeMutablePointer<ProcessSerialNumber>, UInt32, UInt32) -> OSStatus
)? = {
    // GetProcessForPID lives in HIServices (part of ApplicationServices), unavailable in Swift
    guard let hiServices = dlopen(
        "/System/Library/Frameworks/ApplicationServices.framework/Frameworks/HIServices.framework/HIServices",
        RTLD_LAZY
    ) else { return nil }
    guard let getPSN = dlsym(hiServices, "GetProcessForPID") else { return nil }
    guard let skylight = dlopen(
        "/System/Library/PrivateFrameworks/SkyLight.framework/SkyLight",
        RTLD_LAZY
    ) else { return nil }
    guard let setFront = dlsym(skylight, "_SLPSSetFrontProcessWithOptions") else { return nil }
    return (
        getProcessForPID: unsafeBitCast(getPSN, to: (@convention(c) (pid_t, UnsafeMutablePointer<ProcessSerialNumber>) -> OSStatus).self),
        setFrontProcess: unsafeBitCast(setFront, to: (@convention(c) (UnsafeMutablePointer<ProcessSerialNumber>, UInt32, UInt32) -> OSStatus).self)
    )
}()

/// Activates an app via direct window server IPC (~1-5ms) instead of Apple Events (~10-50ms).
/// Falls back to `NSRunningApplication.activate()` if the private SPI is unavailable or fails.
private func _fastActivate(_ app: NSRunningApplication) {
    if let syms = _skyLightSymbols {
        var psn = ProcessSerialNumber()
        if syms.getProcessForPID(app.processIdentifier, &psn) == noErr,
           syms.setFrontProcess(&psn, 0, 0) == noErr
        {
            return
        }
    }
    app.activate()
}

// MARK: - Execute toggle action (called from event tap thread)

/// Executes a resolved toggle action. Thread-safe: activate/hide use Apple Event
/// transport. Only dispatches to main for cold launches (openApplication).
private func _executeToggle(_ action: TapContext.ToggleAction, log: (@Sendable (String) -> Void)?) {
    switch action {
    case .hide(let app):
        app.hide()
    case .activate(let app):
        _fastActivate(app)
        if let bundleId = app.bundleIdentifier { log?(bundleId) }
    case .launch(let url):
        let bundleId = Bundle(url: url)?.bundleIdentifier
        DispatchQueue.main.async {
            let config = NSWorkspace.OpenConfiguration()
            config.activates = true
            NSWorkspace.shared.openApplication(at: url, configuration: config)
        }
        if let bundleId { log?(bundleId) }
    }
}

// MARK: - CGEvent tap factory (free function to avoid Swift 6.2.1 SendNonSendable crash)

private func _makeEventTap(
    context: TapContext
) -> (tap: CFMachPort, source: CFRunLoopSource, thread: Thread)? {
    let tap = CGEvent.tapCreate(
        tap: .cgSessionEventTap,
        place: .headInsertEventTap,
        options: .defaultTap,
        eventsOfInterest: CGEventMask(1 << CGEventType.keyDown.rawValue),
        callback: { _, type, event, userInfo -> Unmanaged<CGEvent>? in
            let ctx = Unmanaged<TapContext>.fromOpaque(userInfo!).takeUnretainedValue()

            // Re-enable tap if macOS disabled it (timeout or user input)
            if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
                if let port = ctx.tapPort {
                    CGEvent.tapEnable(tap: port, enable: true)
                }
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

            // Single lock acquisition: match shortcut + resolve running app state
            if let action = ctx.matchAndResolve(keyCode: keyCode, flags: flags) {
                _executeToggle(action, log: ctx.logActivation)
                return nil // consume event
            }

            return Unmanaged.passUnretained(event)
        },
        userInfo: Unmanaged.passUnretained(context).toOpaque()
    )

    guard let tap else { return nil }

    guard let source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0) else {
        return nil
    }

    let thread = Thread {
        CFRunLoopAddSource(CFRunLoopGetCurrent(), source, .commonModes)
        CFRunLoopRun()
    }
    thread.name = "Shun.EventTap"
    thread.qualityOfService = .userInteractive

    return (tap, source, thread)
}

// MARK: - ShortcutManager

@MainActor
final class ShortcutManager {
    private let store: ShortcutStore
    private let context = TapContext()

    private var eventTap: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private var tapThread: Thread?

    /// Falls back to Carbon hotkeys if CGEvent tap creation fails.
    private var usingCarbonFallback = false

    private var activationObservers: [NSObjectProtocol] = []
    private var workspaceObservers: [NSObjectProtocol] = []

    private let usageStore: UsageStore

    init(store: ShortcutStore, usageStore: UsageStore) {
        self.store = store
        self.usageStore = usageStore
        context.logActivation = { [weak usageStore] bundleId in
            DispatchQueue.main.async {
                usageStore?.log(bundleIdentifier: bundleId)
            }
        }
        seedRunningAppCache()
        observeAppActivation()
        observeWorkspace()
    }

    // MARK: - Public

    func refreshRegistrations() {
        if usingCarbonFallback {
            refreshCarbonRegistrations()
            return
        }

        // Build new lookup entries
        var entries: [(key: ShortcutKey, entry: ShortcutEntry)] = []
        if store.globalEnabled {
            for shortcut in store.shortcuts where shortcut.isEnabled {
                guard let ks = KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName) else {
                    log.debug("refresh: no combo stored for \(shortcut.displayName)")
                    continue
                }
                let flags = cgEventFlags(fromCarbon: ks.carbonModifiers)
                log.debug("refresh: \(shortcut.displayName) keyCode=\(ks.carbonKeyCode) carbonMod=\(UInt(bitPattern: ks.carbonModifiers), format: .hex) cgFlags=\(flags.rawValue, format: .hex)")
                let key = ShortcutKey(keyCode: Int64(ks.carbonKeyCode), modifierMask: flags.rawValue)
                let entry = ShortcutEntry(
                    bundleIdentifier: shortcut.bundleIdentifier,
                    bundleURL: shortcut.bundleURL
                )
                entries.append((key: key, entry: entry))
            }
        }
        log.debug("refresh: \(entries.count) entries registered (globalEnabled=\(self.store.globalEnabled))")
        context.updateShortcuts(entries)

        // Tear down Carbon hotkeys that KeyboardShortcuts.Recorder auto-registers
        for shortcut in store.shortcuts {
            KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
        }

        // Ensure tap exists (or re-enable if macOS disabled it)
        if let tap = eventTap {
            let shouldEnable = store.globalEnabled && !context.shortcutsEmpty
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

    // MARK: - Running app cache

    /// Seed the cache with all currently running apps on init.
    private func seedRunningAppCache() {
        for app in NSWorkspace.shared.runningApplications {
            if let bundleId = app.bundleIdentifier {
                context.updateRunningApp(bundleId: bundleId, app: app)
            }
        }
        context.updateFrontmost(NSWorkspace.shared.frontmostApplication)
    }

    /// Observe workspace notifications to keep running app cache fresh.
    private func observeWorkspace() {
        let center = NSWorkspace.shared.notificationCenter
        let ctx = self.context

        let didLaunch = center.addObserver(
            forName: NSWorkspace.didLaunchApplicationNotification,
            object: nil,
            queue: .main
        ) { notification in
            if let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
               let bundleId = app.bundleIdentifier
            {
                ctx.updateRunningApp(bundleId: bundleId, app: app)
            }
        }

        let didTerminate = center.addObserver(
            forName: NSWorkspace.didTerminateApplicationNotification,
            object: nil,
            queue: .main
        ) { notification in
            if let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
               let bundleId = app.bundleIdentifier
            {
                // Check if another instance of the same app is still running
                let remaining = NSRunningApplication.runningApplications(withBundleIdentifier: bundleId)
                if let next = remaining.first {
                    ctx.updateRunningApp(bundleId: bundleId, app: next)
                } else {
                    ctx.removeRunningApp(bundleId: bundleId)
                }
            }
        }

        let didActivate = center.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { notification in
            let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication
            ctx.updateFrontmost(app)
        }

        workspaceObservers = [didLaunch, didTerminate, didActivate]
    }

    // MARK: - Pause when our app is active (Recorder needs events)

    private func observeAppActivation() {
        let ctx = self.context

        let didActivate = NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification,
            object: nil,
            queue: .main
        ) { _ in
            ctx.paused = true
        }

        let didDeactivate = NotificationCenter.default.addObserver(
            forName: NSApplication.didResignActiveNotification,
            object: nil,
            queue: .main
        ) { _ in
            ctx.paused = false
        }

        activationObservers = [didActivate, didDeactivate]
    }

    // MARK: - CGEvent Tap lifecycle

    private func createTap() {
        guard let result = _makeEventTap(context: context) else {
            usingCarbonFallback = true
            refreshCarbonRegistrations()
            return
        }

        self.eventTap = result.tap
        self.runLoopSource = result.source
        // Set tap port on context so callback can re-enable on timeout
        context.tapPort = result.tap
        result.thread.start()
        self.tapThread = result.thread
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
        let usageStore = self.usageStore

        KeyboardShortcuts.onKeyUp(for: name) {
            let workspace = NSWorkspace.shared
            if let frontmost = workspace.frontmostApplication,
               frontmost.bundleIdentifier == bundleIdentifier
            {
                frontmost.hide()
            } else {
                let running = NSRunningApplication.runningApplications(withBundleIdentifier: bundleIdentifier)
                if let app = running.first {
                    app.activate()
                } else {
                    DispatchQueue.main.async {
                        let config = NSWorkspace.OpenConfiguration()
                        config.activates = true
                        workspace.openApplication(at: bundleURL, configuration: config)
                    }
                }
                Task { @MainActor in
                    usageStore.log(bundleIdentifier: bundleIdentifier)
                }
            }
        }
    }

    private func unregisterAllCarbon() {
        for shortcut in store.shortcuts {
            KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
        }
    }
}
