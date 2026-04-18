#!/usr/bin/env bun

import { existsSync, readFileSync, statSync } from "fs";
import { createHash } from "crypto";
import { resolve } from "path";
import { marked } from "marked";

interface Args {
  help: boolean;
  version?: string;
  dryRun: boolean;
  force: boolean;
}

interface SparkleConfig {
  publicKey: string;
  feedURL: string;
  publicBaseURL: string;
}

interface EdSignature {
  edSignature: string;
  length: string;
}

interface ArtifactPlan {
  localPath: string;
  key: string;
  publicURL: string;
  contentType: string;
  cacheControl: string;
  overwrite: boolean;
}

function parseArgs(): Args {
  const args: Args = { help: false, dryRun: false, force: false };
  const argv = Bun.argv.slice(2);
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "-h" || a === "--help") args.help = true;
    else if (a === "--dry-run") args.dryRun = true;
    else if (a === "--force") args.force = true;
    else if (a === "--version") args.version = argv[++i];
    else {
      console.error(`Unknown argument: ${a}`);
      process.exit(2);
    }
  }
  return args;
}

function showHelp(): void {
  console.log(`
Usage: ./Scripts/upload_r2.ts [--version X.Y.Z] [--dry-run] [--force]

Publishes a Shun release to Cloudflare R2:
  - EdDSA-signs the ZIP via Sparkle's sign_update
  - Fetches existing appcast.xml (404 => empty template for first release)
  - Composes a new <item> from CHANGELOG.md
  - Uploads DMG, zip, sha256, Shun-latest.dmg alias, appcast.xml
  - Verifies via S3 HEAD and public-URL HEAD smoke test

Options:
  --version X.Y.Z  Explicit version (default: read from VERSION file)
  --dry-run        Run preflight + sign + compose appcast; skip all R2 writes
  --force          Overwrite versioned artifacts (use with care)
  -h, --help       Show this help
`);
}

function loadSparkleConfig(projectRoot: string): SparkleConfig {
  const path = resolve(projectRoot, "Scripts/sparkle.config");
  if (!existsSync(path)) {
    throw new Error(`Missing Scripts/sparkle.config — see docs/designs for setup.`);
  }
  const raw = readFileSync(path, "utf8");
  const cfg: Partial<SparkleConfig> = {};
  const unquote = (s: string) => s.trim().replace(/^['"]|['"]$/g, "");
  for (const line of raw.split("\n")) {
    const m = line.match(/^([A-Z_]+)=(.*)$/);
    if (!m) continue;
    const [, k, v] = m;
    if (k === "SPARKLE_PUBLIC_KEY") cfg.publicKey = unquote(v);
    else if (k === "SPARKLE_FEED_URL") cfg.feedURL = unquote(v);
    else if (k === "SPARKLE_PUBLIC_BASE_URL") cfg.publicBaseURL = unquote(v);
  }
  if (!cfg.publicKey || !cfg.feedURL || !cfg.publicBaseURL) {
    throw new Error(
      "Scripts/sparkle.config is missing SPARKLE_PUBLIC_KEY, SPARKLE_FEED_URL, or SPARKLE_PUBLIC_BASE_URL.",
    );
  }
  return cfg as SparkleConfig;
}

function requireEnv(name: string): string {
  const v = Bun.env[name];
  if (!v) {
    throw new Error(
      `Missing ${name} — put R2 creds in ./.env at repo root (not Scripts/.env).`,
    );
  }
  return v;
}

function findSignUpdate(): string {
  // Prefer PATH (works if the user symlinked it themselves).
  const pathCheck = Bun.spawnSync(["which", "sign_update"]);
  if (pathCheck.exitCode === 0) {
    return new TextDecoder().decode(pathCheck.stdout).trim();
  }
  // Fall back to the Homebrew cask — tools live under Caskroom/sparkle/<ver>/bin.
  // Cask doesn't symlink binaries to PATH like formulas do.
  for (const prefix of ["/opt/homebrew/Caskroom/sparkle", "/usr/local/Caskroom/sparkle"]) {
    const ls = Bun.spawnSync(["ls", prefix]);
    if (ls.exitCode !== 0) continue;
    const versions = new TextDecoder().decode(ls.stdout).trim().split("\n");
    // Pick the latest by sort order (Caskroom versions sort lexically fine for semver-ish).
    versions.sort();
    for (let i = versions.length - 1; i >= 0; i--) {
      const candidate = `${prefix}/${versions[i]}/bin/sign_update`;
      if (existsSync(candidate)) return candidate;
    }
  }
  throw new Error(
    "sign_update not found. Install with: brew install --cask sparkle",
  );
}

async function signUpdate(zipPath: string, signUpdateBin: string): Promise<EdSignature> {
  const proc = Bun.spawn([signUpdateBin, zipPath], {
    stdout: "pipe",
    stderr: "pipe",
  });
  const exitCode = await proc.exited;
  const stdout = await new Response(proc.stdout).text();
  const stderr = await new Response(proc.stderr).text();
  if (exitCode !== 0) {
    throw new Error(
      `sign_update failed (exit ${exitCode}). Is 'brew install --cask sparkle' run and a key generated?\n${stderr}`,
    );
  }
  // Output format: sparkle:edSignature="xxx" length="12345"
  const sigMatch = stdout.match(/sparkle:edSignature="([^"]+)"/);
  const lenMatch = stdout.match(/length="([^"]+)"/);
  if (!sigMatch || !lenMatch) {
    throw new Error(`sign_update output unparsable:\n${stdout}`);
  }
  return { edSignature: sigMatch[1], length: lenMatch[1] };
}

function readMinimumSystemVersion(appBundle: string): string {
  const plist = `${appBundle}/Contents/Info.plist`;
  const proc = Bun.spawnSync([
    "plutil",
    "-extract",
    "LSMinimumSystemVersion",
    "raw",
    "-o",
    "-",
    plist,
  ]);
  if (proc.exitCode !== 0) {
    throw new Error(
      `plutil failed to read LSMinimumSystemVersion from ${plist}`,
    );
  }
  return new TextDecoder().decode(proc.stdout).trim();
}

function parseChangelogEntry(projectRoot: string, version: string): string {
  const path = resolve(projectRoot, "CHANGELOG.md");
  if (!existsSync(path)) {
    throw new Error(`CHANGELOG.md not found at ${path}`);
  }
  const md = readFileSync(path, "utf8");
  const lines = md.split("\n");
  let start = -1;
  let end = lines.length;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Match `## 1.0.2` or `## 1.0.2 - 2026-04-18` or `## [1.0.2]`
    if (
      start === -1
      && /^##\s+\[?\s*[vV]?[\d]/.test(line)
      && line.includes(version)
    ) {
      start = i + 1;
    } else if (start !== -1 && /^##\s/.test(line)) {
      end = i;
      break;
    }
  }
  if (start === -1) {
    throw new Error(
      `CHANGELOG.md has no heading for version ${version}. Expected "## ${version}" or "## ${version} - YYYY-MM-DD".`,
    );
  }
  const body = lines.slice(start, end).join("\n").trim();
  if (!body) {
    throw new Error(`CHANGELOG.md section for ${version} is empty.`);
  }
  return marked.parse(body, { async: false }) as string;
}

function fileMD5(path: string): string {
  const data = readFileSync(path);
  return createHash("md5").update(data).digest("hex");
}

function rfc822Now(): string {
  // Sparkle appcast <pubDate> expects RFC-822; JS toUTCString is close enough.
  return new Date().toUTCString();
}

function buildAppcastItem(opts: {
  version: string;
  pubDate: string;
  descriptionHTML: string;
  minSystemVersion: string;
  zipURL: string;
  zipLength: string;
  edSignature: string;
}): string {
  return `    <item>
      <title>Version ${opts.version}</title>
      <pubDate>${opts.pubDate}</pubDate>
      <description><![CDATA[${opts.descriptionHTML}]]></description>
      <sparkle:minimumSystemVersion>${opts.minSystemVersion}</sparkle:minimumSystemVersion>
      <enclosure url="${opts.zipURL}" sparkle:version="${opts.version}" sparkle:shortVersionString="${opts.version}" length="${opts.zipLength}" type="application/zip" sparkle:edSignature="${opts.edSignature}"/>
    </item>`;
}

function wrapAppcastXML(itemsXML: string): string {
  return `<?xml version="1.0" standalone="yes"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Shun</title>
    <link>https://github.com/sorrycc/shun</link>
    <description>Shun auto-update feed</description>
    <language>en</language>
${itemsXML}
  </channel>
</rss>
`;
}

function prependItemToAppcast(existing: string | null, newItemXML: string): string {
  if (!existing) {
    return wrapAppcastXML(newItemXML);
  }
  // Insert new <item> right after the first <channel> opening, before any existing <item>.
  const channelOpen = existing.indexOf("<channel>");
  if (channelOpen === -1) {
    // Existing file is malformed — rebuild.
    return wrapAppcastXML(newItemXML);
  }
  const insertAt = existing.indexOf(">", channelOpen) + 1;
  // Skip past any whitespace/intro tags (<title>, <link>, <description>, <language>)
  // to land just before the first <item>. If there's no <item>, insert before </channel>.
  const firstItem = existing.indexOf("<item>", insertAt);
  const channelClose = existing.indexOf("</channel>", insertAt);
  const anchor = firstItem !== -1 && firstItem < channelClose ? firstItem : channelClose;
  return existing.slice(0, anchor) + newItemXML + "\n" + existing.slice(anchor);
}

function log(msg: string): void {
  console.log(msg);
}

async function main(): Promise<void> {
  const args = parseArgs();
  if (args.help) {
    showHelp();
    process.exit(0);
  }

  const projectRoot = resolve(import.meta.dir, "..");
  process.chdir(projectRoot);

  const version = args.version ?? readFileSync("VERSION", "utf8").trim();
  if (!/^\d+\.\d+\.\d+(-[\w.]+)?$/.test(version)) {
    throw new Error(`Invalid version: "${version}"`);
  }

  log(`Publishing Shun ${version}${args.dryRun ? " (dry run)" : ""}...`);

  // ---- Preflight ----
  const r2AccountId = requireEnv("R2_ACCOUNT_ID");
  const r2AccessKey = requireEnv("R2_ACCESS_KEY_ID");
  const r2SecretKey = requireEnv("R2_SECRET_ACCESS_KEY");
  const r2Bucket = Bun.env.R2_BUCKET ?? "downloads";
  const appPrefix = Bun.env.APP_KEY_PREFIX ?? "shun";

  const sparkle = loadSparkleConfig(projectRoot);

  const dmgPath = `build/Shun-${version}.dmg`;
  const zipPath = `build/Shun-${version}.zip`;
  const shaPath = `build/Shun-${version}.sha256`;
  const appBundle = "Shun.app";
  for (const p of [dmgPath, zipPath, shaPath, appBundle]) {
    if (!existsSync(p)) {
      throw new Error(
        `Missing artifact: ${p}. Run ./Scripts/release.sh first.`,
      );
    }
  }

  // Locate sign_update (fail early with a clear message if missing).
  const signUpdateBin = findSignUpdate();

  // ---- Sign the ZIP (Sparkle convention — ZIP, not DMG) ----
  log(`Signing ZIP with EdDSA (via ${signUpdateBin})...`);
  const sig = await signUpdate(zipPath, signUpdateBin);

  // ---- Read min macOS from the built app's Info.plist ----
  const minMac = readMinimumSystemVersion("Shun.app");
  log(`  sparkle:minimumSystemVersion = ${minMac}`);

  // ---- S3 client ----
  const s3 = new Bun.S3Client({
    accessKeyId: r2AccessKey,
    secretAccessKey: r2SecretKey,
    endpoint: `https://${r2AccountId}.r2.cloudflarestorage.com`,
    bucket: r2Bucket,
    region: "auto",
  });

  // ---- Fetch existing appcast (404 => empty template) ----
  const appcastKey = `${appPrefix}/appcast.xml`;
  let existingAppcast: string | null = null;
  try {
    existingAppcast = await s3.file(appcastKey).text();
    log(`  fetched existing appcast.xml (${existingAppcast.length} bytes)`);
  } catch (e: any) {
    // Bun.S3Client throws on not-found. Treat as first release.
    log(`  no existing appcast.xml (first release) — starting from template`);
  }

  // ---- Compose new <item> ----
  const descriptionHTML = parseChangelogEntry(projectRoot, version);
  const zipURL = `${sparkle.publicBaseURL.replace(/\/$/, "")}/${appPrefix}/Shun-${version}.zip`;
  const newItem = buildAppcastItem({
    version,
    pubDate: rfc822Now(),
    descriptionHTML,
    minSystemVersion: minMac,
    zipURL,
    zipLength: sig.length,
    edSignature: sig.edSignature,
  });
  const newAppcastXML = prependItemToAppcast(existingAppcast, newItem);

  // ---- Build upload plan ----
  const baseURL = sparkle.publicBaseURL.replace(/\/$/, "");
  const plan: ArtifactPlan[] = [
    {
      localPath: dmgPath,
      key: `${appPrefix}/Shun-${version}.dmg`,
      publicURL: `${baseURL}/${appPrefix}/Shun-${version}.dmg`,
      contentType: "application/x-apple-diskimage",
      cacheControl: "public, max-age=31536000, immutable",
      overwrite: false,
    },
    {
      localPath: zipPath,
      key: `${appPrefix}/Shun-${version}.zip`,
      publicURL: `${baseURL}/${appPrefix}/Shun-${version}.zip`,
      contentType: "application/zip",
      cacheControl: "public, max-age=31536000, immutable",
      overwrite: false,
    },
    {
      localPath: shaPath,
      key: `${appPrefix}/Shun-${version}.sha256`,
      publicURL: `${baseURL}/${appPrefix}/Shun-${version}.sha256`,
      contentType: "text/plain",
      cacheControl: "public, max-age=31536000, immutable",
      overwrite: false,
    },
    {
      localPath: dmgPath,
      key: `${appPrefix}/Shun-latest.dmg`,
      publicURL: `${baseURL}/${appPrefix}/Shun-latest.dmg`,
      contentType: "application/x-apple-diskimage",
      cacheControl: "public, max-age=300",
      overwrite: true,
    },
  ];

  if (args.dryRun) {
    log("\n--- DRY RUN: composed appcast.xml ---");
    log(newAppcastXML);
    log("\n--- planned uploads ---");
    for (const p of plan) {
      const size = statSync(p.localPath).size;
      log(
        `  ${p.key}  (${size} bytes, ${p.contentType}, ${p.overwrite ? "overwrite" : "no-overwrite"}) -> ${p.publicURL}`,
      );
    }
    log(
      `  ${appcastKey}  (${newAppcastXML.length} bytes, application/xml, overwrite) -> ${sparkle.feedURL}`,
    );
    log("\nDry run complete. No writes to R2.");
    return;
  }

  // ---- Idempotent collision check ----
  log("Checking for existing versioned artifacts...");
  for (const p of plan) {
    if (p.overwrite) continue;
    let remote: { size: number; etag?: string } | null = null;
    try {
      const stat = await s3.file(p.key).stat();
      remote = { size: stat.size, etag: stat.etag };
    } catch {
      remote = null;
    }
    if (!remote) continue;
    const localSize = statSync(p.localPath).size;
    if (remote.size === localSize) {
      // Try ETag match (simple upload => ETag == md5 in hex). Skip idempotently.
      const md5 = fileMD5(p.localPath);
      const remoteEtag = (remote.etag ?? "").replace(/"/g, "");
      if (remoteEtag === md5 || remoteEtag === "") {
        log(`  skip ${p.key} — already uploaded with matching bytes`);
        (p as any).skip = true;
        continue;
      }
    }
    if (!args.force) {
      throw new Error(
        `${p.key} already exists on R2 with different bytes. Use --force to overwrite.`,
      );
    }
    log(`  will overwrite ${p.key} (--force)`);
  }

  // ---- Upload versioned artifacts in parallel ----
  // Bun.S3Client.write options pass through as S3 headers — `type` -> Content-Type,
  // `cacheControl` -> Cache-Control. If the current Bun version ignores cacheControl,
  // we fall back to r2.dev's default caching (still correct, just less efficient).
  log("Uploading artifacts...");
  await Promise.all(
    plan
      .filter((p) => !p.overwrite && !(p as any).skip)
      .map(async (p) => {
        const data = readFileSync(p.localPath);
        await s3.write(p.key, data, {
          type: p.contentType,
          cacheControl: p.cacheControl,
        } as any);
        log(`  uploaded ${p.key}  (${data.length} bytes)`);
      }),
  );

  // ---- Upload aliases + appcast last ----
  const latest = plan.find((p) => p.overwrite);
  if (latest) {
    const data = readFileSync(latest.localPath);
    await s3.write(latest.key, data, {
      type: latest.contentType,
      cacheControl: latest.cacheControl,
    } as any);
    log(`  uploaded ${latest.key}  (alias -> ${version})`);
  }

  await s3.write(appcastKey, newAppcastXML, {
    type: "application/xml",
    cacheControl: "public, max-age=300",
  } as any);
  log(`  uploaded ${appcastKey}  (${newAppcastXML.length} bytes)`);

  // ---- Verify via S3 HEAD ----
  log("Verifying uploads (S3 HEAD)...");
  for (const p of plan) {
    const stat = await s3.file(p.key).stat();
    const localSize = statSync(p.localPath).size;
    if (stat.size !== localSize) {
      throw new Error(
        `Size mismatch for ${p.key}: local=${localSize}, remote=${stat.size}`,
      );
    }
  }

  // ---- Public URL smoke test ----
  log("Verifying public URLs...");
  for (const url of [sparkle.feedURL, `${baseURL}/${appPrefix}/Shun-${version}.zip`]) {
    const res = await fetch(url, { method: "HEAD" });
    if (!res.ok) {
      throw new Error(
        `Public HEAD ${url} failed with ${res.status}. Is the bucket's r2.dev public access enabled?`,
      );
    }
  }

  log(`\nRelease ${version} published:`);
  log(`  Appcast:  ${sparkle.feedURL}`);
  log(`  ZIP:      ${baseURL}/${appPrefix}/Shun-${version}.zip`);
  log(`  DMG:      ${baseURL}/${appPrefix}/Shun-${version}.dmg`);
  log(`  latest:   ${baseURL}/${appPrefix}/Shun-latest.dmg`);
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
