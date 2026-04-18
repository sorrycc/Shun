import Sparkle
import os

private let log = Logger(subsystem: "com.shun.Shun", category: "updater")

// Bare delegate — NSObject, no actor isolation, no @Observable.
// Keeps Sparkle protocol conformance separate from UI state.
final class SparkleDelegate: NSObject, SPUUpdaterDelegate {
    func updater(_ updater: SPUUpdater, didAbortWithError error: Error) {
        log.error("Update aborted: \(error.localizedDescription, privacy: .public)")
    }

    func updater(
        _ updater: SPUUpdater,
        didFinishUpdateCycleFor updateCheck: SPUUpdateCheck,
        error: Error?
    ) {
        if let error {
            log.error(
                "Update cycle error (\(updateCheck.rawValue, privacy: .public)): \(error.localizedDescription, privacy: .public)"
            )
        }
    }
}

@MainActor
final class UpdaterService {
    private let delegate = SparkleDelegate()
    private let controller: SPUStandardUpdaterController

    init() {
        controller = SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: delegate,
            userDriverDelegate: nil
        )
    }

    func checkForUpdates() {
        controller.checkForUpdates(nil)
    }
}
