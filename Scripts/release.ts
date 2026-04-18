#!/usr/bin/env bun

import { existsSync, readFileSync, writeFileSync } from "fs";
import { resolve } from "path";

type BumpType = "patch" | "minor" | "major";

interface ParsedArgs {
  help: boolean;
  bump?: BumpType;
  push: boolean;
  upload: boolean;
}

function parseArgs(): ParsedArgs {
  const args = Bun.argv.slice(2);
  const out: ParsedArgs = { help: false, push: false, upload: false };
  for (const a of args) {
    if (a === "-h" || a === "--help") out.help = true;
    else if (a === "--push") out.push = true;
    else if (a === "--upload") out.upload = true;
    else if (a === "patch" || a === "minor" || a === "major") out.bump = a;
    else {
      console.error(`Unknown argument: ${a}`);
      process.exit(2);
    }
  }
  return out;
}

function showHelp(): void {
  console.log(`
Usage: ./Scripts/release.ts <patch|minor|major> [--push] [--upload]

Orchestrates a full Shun release:
  1. Verifies working tree is clean
  2. Verifies CHANGELOG.md has a non-empty "## Unreleased" body (if --upload)
  3. Bumps the VERSION file (semver)
  4. Promotes "## Unreleased" -> "## <version> - <UTC date>" in CHANGELOG.md
  5. Runs Scripts/release.sh (build + notarize + DMG + zip + sha256)
  6. Creates git commit (VERSION + CHANGELOG.md) and tag v<version>
  7. Optionally uploads to R2 via Scripts/upload_r2.ts (if --upload)
  8. Optionally pushes commit and tag to origin (if --push)

Examples:
  ./Scripts/release.ts patch                   # local only
  ./Scripts/release.ts patch --push            # commit + tag + push
  ./Scripts/release.ts patch --push --upload   # full publish

Options:
  patch|minor|major   Semver component to bump (required)
  --push              Push commit and tag to origin
  --upload            Publish artifacts + appcast to Cloudflare R2
  -h, --help          Show this help
`);
}

function bumpVersion(current: string, type: BumpType): string {
  const parts = current.trim().split(".");
  if (parts.length !== 3 || !parts.every((p) => /^\d+$/.test(p))) {
    throw new Error(`VERSION file is not plain semver "X.Y.Z": "${current}"`);
  }
  let [major, minor, patch] = parts.map(Number);
  if (type === "major") {
    major++;
    minor = 0;
    patch = 0;
  } else if (type === "minor") {
    minor++;
    patch = 0;
  } else {
    patch++;
  }
  return `${major}.${minor}.${patch}`;
}

async function run(cmd: string[], capture = false): Promise<string> {
  const proc = Bun.spawn(cmd, {
    stdout: capture ? "pipe" : "inherit",
    stderr: capture ? "pipe" : "inherit",
    stdin: "inherit",
  });
  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    throw new Error(`Command failed (exit ${exitCode}): ${cmd.join(" ")}`);
  }
  return capture ? await new Response(proc.stdout).text() : "";
}

async function tryRun(cmd: string[]): Promise<boolean> {
  const proc = Bun.spawn(cmd, { stdout: "pipe", stderr: "pipe" });
  return (await proc.exited) === 0;
}

const CHANGELOG_TEMPLATE = `# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

`;

function ensureChangelogExists(path: string): void {
  if (!existsSync(path)) {
    writeFileSync(path, CHANGELOG_TEMPLATE);
    console.log(`Created ${path} with empty Unreleased section.`);
  }
}

function readUnreleasedBody(md: string): string {
  const lines = md.split("\n");
  let start = -1;
  let end = lines.length;
  for (let i = 0; i < lines.length; i++) {
    if (start === -1 && /^##\s+Unreleased\s*$/i.test(lines[i])) {
      start = i + 1;
    } else if (start !== -1 && /^##\s/.test(lines[i])) {
      end = i;
      break;
    }
  }
  if (start === -1) return "";
  return lines.slice(start, end).join("\n").trim();
}

function promoteUnreleased(md: string, version: string, utcDate: string): string {
  const header = `## ${version} - ${utcDate}`;
  const newUnreleased = `## Unreleased\n\n\n`;
  return md.replace(/^##\s+Unreleased\s*$/m, `${newUnreleased}${header}`);
}

function utcToday(): string {
  const now = new Date();
  const y = now.getUTCFullYear();
  const m = String(now.getUTCMonth() + 1).padStart(2, "0");
  const d = String(now.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

async function main(): Promise<void> {
  const args = parseArgs();
  if (args.help) {
    showHelp();
    process.exit(0);
  }
  if (!args.bump) {
    console.error("Error: must specify patch, minor, or major\n");
    showHelp();
    process.exit(2);
  }

  const projectRoot = resolve(import.meta.dir, "..");
  process.chdir(projectRoot);

  const versionFile = resolve(projectRoot, "VERSION");
  if (!existsSync(versionFile)) {
    throw new Error(`VERSION file not found at ${versionFile}`);
  }

  // Preflight: clean working tree
  const dirty = (await run(["git", "status", "--porcelain"], true)).trim();
  if (dirty !== "") {
    console.error("Error: working tree is dirty. Commit or stash first.\n");
    console.error(dirty);
    process.exit(1);
  }

  const currentVersion = readFileSync(versionFile, "utf8").trim();
  const newVersion = bumpVersion(currentVersion, args.bump);
  const tag = `v${newVersion}`;

  if (await tryRun(["git", "rev-parse", "--verify", `refs/tags/${tag}`])) {
    console.error(`Error: tag ${tag} already exists.`);
    process.exit(1);
  }

  // Preflight: CHANGELOG.md (gate --upload on a filled-in Unreleased section).
  const changelogPath = resolve(projectRoot, "CHANGELOG.md");
  ensureChangelogExists(changelogPath);
  const changelogBefore = readFileSync(changelogPath, "utf8");
  const unreleasedBody = readUnreleasedBody(changelogBefore);
  if (args.upload && unreleasedBody === "") {
    console.error(
      "Error: CHANGELOG.md '## Unreleased' section is empty. Add release notes before publishing with --upload.",
    );
    process.exit(1);
  }

  const branch = (
    await run(["git", "symbolic-ref", "--short", "HEAD"], true)
  ).trim();

  console.log(`Releasing ${currentVersion} -> ${newVersion}`);
  console.log(`  branch: ${branch}`);
  console.log(`  tag:    ${tag}`);
  console.log(`  upload: ${args.upload ? "yes" : "no"}`);
  console.log(`  push:   ${args.push ? "yes" : "no (local only)"}`);
  console.log();

  writeFileSync(versionFile, `${newVersion}\n`);
  console.log(`Wrote VERSION = ${newVersion}`);

  // Promote `## Unreleased` -> `## <version> - <UTC date>`
  const utcDate = utcToday();
  const changelogAfter = promoteUnreleased(changelogBefore, newVersion, utcDate);
  writeFileSync(changelogPath, changelogAfter);
  console.log(`Promoted CHANGELOG.md: ## ${newVersion} - ${utcDate}`);

  try {
    await run(["./Scripts/release.sh"]);
  } catch (e) {
    // Revert both files on pipeline failure so the workspace stays clean.
    writeFileSync(versionFile, `${currentVersion}\n`);
    writeFileSync(changelogPath, changelogBefore);
    console.error(
      `\nRelease pipeline failed. Reverted VERSION and CHANGELOG.md.`,
    );
    throw e;
  }

  await run(["git", "add", "VERSION", "CHANGELOG.md"]);
  await run(["git", "commit", "-m", `release: ${newVersion}`]);
  await run(["git", "tag", tag]);
  console.log(`\nCreated commit and tag ${tag}.`);

  if (args.upload) {
    try {
      await run(["./Scripts/upload_r2.ts", "--version", newVersion]);
    } catch (e) {
      console.error(
        "\nUpload failed. Local commit + tag are intact. Fix and retry:\n" +
          `  ./Scripts/upload_r2.ts --version ${newVersion}\n` +
          `  git push origin ${branch} && git push origin ${tag}`,
      );
      throw e;
    }
  }

  if (args.push) {
    await run(["git", "push", "origin", branch]);
    await run(["git", "push", "origin", tag]);
    console.log(`Pushed ${branch} and ${tag} to origin.`);
  } else {
    console.log("\nNot pushed. When ready:");
    console.log(`  git push origin ${branch} && git push origin ${tag}`);
  }

  console.log(`\nRelease ${newVersion} complete.`);
  console.log(`  build/Shun-${newVersion}.zip`);
  console.log(`  build/Shun-${newVersion}.dmg`);
  console.log(`  build/Shun-${newVersion}.sha256`);
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
