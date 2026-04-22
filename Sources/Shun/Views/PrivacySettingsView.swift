import SwiftUI

struct PrivacySettingsView: View {
    @AppStorage("telemetry.crashReportsEnabled") private var crashReportsEnabled = true
    @AppStorage("telemetry.dauEnabled") private var dauEnabled = true

    @State private var installID: String = InstallID.current()
    @State private var showResetConfirmation = false

    private var telemetryConfigured: Bool {
        TelemetryConfig.fromBundle() != nil
    }

    var body: some View {
        Form {
            if telemetryConfigured {
                Section {
                    Toggle("Send crash reports automatically", isOn: $crashReportsEnabled)
                        .help("When Shun crashes, macOS writes a .ips file under ~/Library/Logs/DiagnosticReports/. We upload those on next launch so we can fix the bug.")
                    Toggle("Send anonymous usage stats", isOn: $dauEnabled)
                        .help("Sends one anonymous ping per day with your app version, macOS version, and locale. Used to count active installs.")
                } header: {
                    Text("Telemetry")
                } footer: {
                    Text("We never send personal data, file contents, keystrokes, or hotkey definitions. Data is stored on Cloudflare under our account and auto-deleted after one year.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section {
                    HStack {
                        Text("Anonymous ID")
                        Spacer()
                        Text(String(installID.prefix(8)) + "…")
                            .font(.system(.body, design: .monospaced))
                            .foregroundStyle(.secondary)
                    }
                    HStack {
                        Spacer()
                        Button("Reset Anonymous ID") {
                            showResetConfirmation = true
                        }
                        .confirmationDialog(
                            "Reset Anonymous ID?",
                            isPresented: $showResetConfirmation,
                            titleVisibility: .visible
                        ) {
                            Button("Reset", role: .destructive) {
                                InstallID.reset()
                                installID = InstallID.current()
                            }
                            Button("Cancel", role: .cancel) {}
                        } message: {
                            Text("Future pings and crashes will be attributed to a new UUID. This does not change the telemetry toggles above.")
                        }
                    }
                } header: {
                    Text("Identifier")
                }
            } else {
                Section {
                    Text("Telemetry is only active in packaged release builds.")
                        .foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .padding()
    }
}
