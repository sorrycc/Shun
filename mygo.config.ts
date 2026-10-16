// The MyGo configuration of Shun, computed:
//
// - the version is VERSION's;
// - builds are signed ad hoc unless MACOS_SIGNING_IDENTITY names a Developer
//   ID, and notarized when NOTARY_PROFILE names a notarytool profile
//   (NOTARY_KEYCHAIN the keychain holding it);
// - telemetry goes into Info.plist from TELEMETRY_ENDPOINT_URL and
//   TELEMETRY_INGEST_KEY, or Scripts/telemetry.config, else the app sends
//   none;
// - updates are signed with the key whose public half is mygo-update.pub
//   (go tool mygo keygen -o <dir>), and published as GitHub releases.
import { existsSync, readFileSync } from "node:fs";

const version = readFileSync("VERSION", "utf8").trim();

// telemetryConfig reads KEY="value" lines of Scripts/telemetry.config.
function telemetryConfig(): Record<string, string> {
  const vars: Record<string, string> = {};
  if (!existsSync("Scripts/telemetry.config")) return vars;
  for (const line of readFileSync("Scripts/telemetry.config", "utf8").split("\n")) {
    const m = line.match(/^\s*([A-Z_]+)=["']?(.*?)["']?\s*$/);
    if (m) vars[m[1]] = m[2];
  }
  return vars;
}

const file = telemetryConfig();
const endpoint = process.env.TELEMETRY_ENDPOINT_URL ?? file.TELEMETRY_ENDPOINT_URL;
const ingestKey = process.env.TELEMETRY_INGEST_KEY ?? file.TELEMETRY_INGEST_KEY;
const telemetry =
  endpoint && ingestKey ? { TelemetryEndpointURL: endpoint, TelemetryIngestKey: ingestKey } : {};

const publicKey = existsSync("mygo-update.pub") ? readFileSync("mygo-update.pub", "utf8").trim() : "";
const notaryProfile = process.env.NOTARY_PROFILE;

export default {
  name: "Shun",
  identifier: "com.shun.Shun",
  version,
  copyright: "© 2026 chen cheng",
  main: "./cmd/shun",
  out: "build",
  ...(publicKey ? { updates: { publicKey, github: "sorrycc/shun" } } : {}),
  macos: {
    minimumSystemVersion: "14.0",
    signingIdentity: process.env.MACOS_SIGNING_IDENTITY ?? "-",
    ...(notaryProfile
      ? {
          notarize: {
            keychainProfile: notaryProfile,
            ...(process.env.NOTARY_KEYCHAIN ? { keychain: process.env.NOTARY_KEYCHAIN } : {}),
          },
        }
      : {}),
    // No Dock icon: Shun lives in the menu bar.
    infoPlist: { LSUIElement: true, ...telemetry },
  },
};
