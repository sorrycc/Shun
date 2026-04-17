#!/usr/bin/env bun

import { existsSync, readFileSync, writeFileSync } from "fs";
import { resolve } from "path";

type BumpType = "patch" | "minor" | "major";

interface ParsedArgs {
  help: boolean;
  bump?: BumpType;
  push: boolean;
}

function parseArgs(): ParsedArgs {
  const args = Bun.argv.slice(2);
  const out: ParsedArgs = { help: false, push: false };
  for (const a of args) {
    if (a === "-h" || a === "--help") out.help = true;
    else if (a === "--push") out.push = true;
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
Usage: ./Scripts/release.ts <patch|minor|major> [--push]

Orchestrates a full Shun release:
  1. Verifies working tree is clean
  2. Bumps the VERSION file (semver)
  3. Runs Scripts/release.sh (build + notarize + DMG + zip + sha256)
  4. Creates git commit and tag v<version>
  5. Optionally pushes commit and tag to origin

Examples:
  ./Scripts/release.ts patch          # 1.0.0 -> 1.0.1, commit + tag (no push)
  ./Scripts/release.ts minor --push   # 1.0.1 -> 1.1.0, commit + tag + push
  ./Scripts/release.ts major          # 1.1.0 -> 2.0.0

Options:
  patch|minor|major   Semver component to bump (required)
  --push              Push commit and tag to origin after success
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

  const branch = (
    await run(["git", "symbolic-ref", "--short", "HEAD"], true)
  ).trim();

  console.log(`Releasing ${currentVersion} -> ${newVersion}`);
  console.log(`  branch: ${branch}`);
  console.log(`  tag:    ${tag}`);
  console.log(`  push:   ${args.push ? "yes" : "no (local only)"}`);
  console.log();

  writeFileSync(versionFile, `${newVersion}\n`);
  console.log(`Wrote VERSION = ${newVersion}`);

  try {
    await run(["./Scripts/release.sh"]);
  } catch (e) {
    writeFileSync(versionFile, `${currentVersion}\n`);
    console.error(
      `\nRelease pipeline failed. Reverted VERSION to ${currentVersion}.`,
    );
    throw e;
  }

  await run(["git", "add", "VERSION"]);
  await run(["git", "commit", "-m", `release: ${newVersion}`]);
  await run(["git", "tag", tag]);
  console.log(`\nCreated commit and tag ${tag}.`);

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
