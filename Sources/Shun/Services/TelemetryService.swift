import CryptoKit
import Foundation
import os

final actor TelemetryService {
    private let log = Logger(subsystem: "com.shun", category: "telemetry")
    private let config: TelemetryConfig
    private let session: URLSession
    private let enabledKey = "telemetry.dauEnabled"
    private let lastPingKey = "telemetry.lastPingDate"

    init(config: TelemetryConfig, session: URLSession = .shared) {
        self.config = config
        self.session = session
    }

    func start() async {
        // Default on: registered-but-unset reads as false; we treat nil as "on".
        let defaults = UserDefaults.standard
        let enabled = defaults.object(forKey: enabledKey) as? Bool ?? true
        guard enabled else {
            log.debug("dau disabled by user")
            return
        }

        let today = Self.todayUTC()
        if defaults.string(forKey: lastPingKey) == today {
            log.debug("dau already pinged today")
            return
        }

        do {
            try await sendPing()
            defaults.set(today, forKey: lastPingKey)
            log.info("dau ping ok")
        } catch {
            log.error("dau ping failed: \(String(describing: error), privacy: .public)")
        }
    }

    private func sendPing() async throws {
        let payload: [String: String] = [
            "app": config.appID,
            "installId": InstallID.current(),
            "version": config.appVersion,
            "osVersion": config.osVersion,
            "locale": config.locale,
        ]
        let body = try JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys])
        let sig = Self.hmacHex(body: body, key: config.ingestKey)

        var req = URLRequest(url: config.endpointURL.appendingPathComponent("v1/ping"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue(sig, forHTTPHeaderField: "X-Telemetry-Sig")
        req.httpBody = body
        req.timeoutInterval = 15

        let (data, response) = try await session.data(for: req)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let snippet = String(data: data, encoding: .utf8)?.prefix(200) ?? ""
            throw NSError(domain: "Telemetry", code: code, userInfo: [NSLocalizedDescriptionKey: "bad status: \(snippet)"])
        }
    }

    static func hmacHex(body: Data, key: String) -> String {
        let keyData = Data(key.utf8)
        let mac = HMAC<SHA256>.authenticationCode(for: body, using: SymmetricKey(data: keyData))
        return Data(mac).map { String(format: "%02x", $0) }.joined()
    }

    static func todayUTC() -> String {
        let fmt = DateFormatter()
        fmt.calendar = Calendar(identifier: .gregorian)
        fmt.locale = Locale(identifier: "en_US_POSIX")
        fmt.timeZone = TimeZone(identifier: "UTC")
        fmt.dateFormat = "yyyy-MM-dd"
        return fmt.string(from: Date())
    }
}
