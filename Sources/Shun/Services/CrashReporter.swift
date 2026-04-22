import Foundation
import os

final actor CrashReporter {
    private let log = Logger(subsystem: "com.shun", category: "crashReporter")
    private let config: TelemetryConfig
    private let session: URLSession
    private let enabledKey = "telemetry.crashReportsEnabled"
    private let uploadedKey = "telemetry.uploadedCrashes"
    private let filenamePrefix: String
    private let uploadedCap = 500

    init(config: TelemetryConfig, filenamePrefix: String = "Shun-", session: URLSession = .shared) {
        self.config = config
        self.filenamePrefix = filenamePrefix
        self.session = session
    }

    func start() async {
        let defaults = UserDefaults.standard
        let enabled = defaults.object(forKey: enabledKey) as? Bool ?? true
        guard enabled else {
            log.debug("crash uploads disabled by user")
            return
        }

        let reportDir = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/DiagnosticReports", isDirectory: true)

        let all: [URL]
        do {
            all = try FileManager.default.contentsOfDirectory(
                at: reportDir,
                includingPropertiesForKeys: [.contentModificationDateKey],
                options: [.skipsHiddenFiles]
            )
        } catch {
            log.debug("no DiagnosticReports dir (\(String(describing: error), privacy: .public))")
            return
        }

        let candidates = all
            .filter { $0.lastPathComponent.hasPrefix(filenamePrefix) && $0.pathExtension == "ips" }
            .sorted { ($0.modDate ?? .distantPast) < ($1.modDate ?? .distantPast) }

        var uploaded = Set(defaults.stringArray(forKey: uploadedKey) ?? [])
        let pending = candidates.filter { !uploaded.contains($0.lastPathComponent) }
        guard !pending.isEmpty else { return }

        log.info("uploading \(pending.count, privacy: .public) crash report(s)")
        for url in pending {
            do {
                try await uploadOne(url)
                uploaded.insert(url.lastPathComponent)
                log.info("uploaded \(url.lastPathComponent, privacy: .public)")
            } catch {
                log.error("upload failed for \(url.lastPathComponent, privacy: .public): \(String(describing: error), privacy: .public)")
                // keep trying next launch
            }
        }

        // Trim the remembered set so UserDefaults doesn't grow unbounded.
        var trimmed = Array(uploaded)
        if trimmed.count > uploadedCap {
            trimmed = Array(trimmed.suffix(uploadedCap))
        }
        defaults.set(trimmed, forKey: uploadedKey)
    }

    private func uploadOne(_ fileURL: URL) async throws {
        let data = try Data(contentsOf: fileURL)

        let meta: [String: String] = [
            "app": config.appID,
            "installId": InstallID.current(),
            "version": config.appVersion,
            "osVersion": config.osVersion,
            "fileName": fileURL.lastPathComponent,
        ]
        let metaJSON = try JSONSerialization.data(withJSONObject: meta, options: [.sortedKeys])

        let boundary = "----telemetry-\(UUID().uuidString)"
        var body = Data()
        body.appendString("--\(boundary)\r\n")
        body.appendString("Content-Disposition: form-data; name=\"meta\"\r\n\r\n")
        body.append(metaJSON)
        body.appendString("\r\n--\(boundary)\r\n")
        body.appendString("Content-Disposition: form-data; name=\"file\"; filename=\"\(fileURL.lastPathComponent)\"\r\n")
        body.appendString("Content-Type: application/octet-stream\r\n\r\n")
        body.append(data)
        body.appendString("\r\n--\(boundary)--\r\n")

        let sig = TelemetryService.hmacHex(body: body, key: config.ingestKey)

        var req = URLRequest(url: config.endpointURL.appendingPathComponent("v1/crash"))
        req.httpMethod = "POST"
        req.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        req.setValue(sig, forHTTPHeaderField: "X-Telemetry-Sig")
        req.httpBody = body
        req.timeoutInterval = 30

        let (respData, response) = try await session.data(for: req)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let snippet = String(data: respData, encoding: .utf8)?.prefix(200) ?? ""
            throw NSError(domain: "Telemetry", code: code, userInfo: [NSLocalizedDescriptionKey: "bad status: \(snippet)"])
        }
    }
}

private extension URL {
    var modDate: Date? {
        (try? resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate
    }
}

private extension Data {
    mutating func appendString(_ s: String) {
        self.append(Data(s.utf8))
    }
}
