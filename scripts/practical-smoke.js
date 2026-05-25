#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const keep = process.argv.includes("--keep") || process.env.HARNESS_KEEP_SMOKE === "1";
const smokeRoot = fs.mkdtempSync(path.join(os.tmpdir(), "harness-practical-smoke-"));
const packDir = path.join(smokeRoot, "pack");
const passDir = path.join(smokeRoot, "pass");
const failDir = path.join(smokeRoot, "blocking");
const cacheDir = path.join(smokeRoot, "npm-cache");

fs.mkdirSync(packDir, { recursive: true });
fs.mkdirSync(passDir, { recursive: true });
fs.mkdirSync(failDir, { recursive: true });

try {
  const pack = run(binary("npm"), ["pack", "--pack-destination", packDir], root);
  const tarballName = lastNonEmptyLine(pack.stdout);
  if (!tarballName) {
    throw new Error("npm pack did not print a tarball name");
  }
  const tarball = path.join(packDir, tarballName);
  if (!fs.existsSync(tarball)) {
    throw new Error(`npm pack did not create ${tarball}`);
  }

  const passHarness = setupFromTarball(tarball, passDir);
  runHarness(passHarness, ["feature", "new", "package pass smoke"], passDir);
  writeFile(
    path.join(passDir, ".specs", "features", "sprint-001", "spec.md"),
    `# Sprint 001 - Package Pass Smoke

## Goal
Validate that the packaged Harness tarball installs and the project-local binary can complete agreement, full QA, and score.

## Size
small

## Requirements
- REQ-001: The packaged Harness command completes the full lifecycle.

## Deliverables
- \`README.md\` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion                                                          | Evidence      | Threshold |
|---|---------|--------------------------------------------------------------------|---------------|-----------|
| 1 | REQ-001 | WHEN README.md exists THEN the system SHALL pass contract validation | e2e:README.md | 8/10      |

## Edge Cases
- Fresh smoke workspace with no prior reports.

## Out of Scope
- Stack-specific adapters.

## Constraints
- coverage_min: 0
`,
  );
  runHarness(passHarness, ["feature", "propose"], passDir);
  runHarness(passHarness, ["feature", "approve", "--role", "planner"], passDir);
  runHarness(passHarness, ["feature", "approve", "--role", "tester"], passDir);
  writeFile(path.join(passDir, "README.md"), "# Package Pass Smoke\n");
  initGit(passDir);
  const passQA = runHarness(passHarness, ["feature", "qa", "--format", "json"], passDir);
  const passReport = JSON.parse(passQA.stdout);
  assert(passReport.verdict === "PASS", `expected PASS, got ${passReport.verdict}`);
  runHarness(passHarness, ["feature", "score"], passDir);
  assert(fs.existsSync(path.join(passDir, ".harness", "reports", "sprint-001.json")), "score report missing");

  const failHarness = setupFromTarball(tarball, failDir, "manual");
  runHarness(failHarness, ["feature", "new", "package blocking smoke"], failDir);
  writeFile(
    path.join(failDir, ".specs", "features", "sprint-001", "spec.md"),
    `# Sprint 001 - Package Blocking Smoke

## Goal
Validate that orphan SPEC_DEVIATION markers fail packaged Harness QA.

## Deliverables
- \`README.md\`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | README exists | 8/10 |
`,
  );
  runHarness(failHarness, ["feature", "propose"], failDir);
  runHarness(failHarness, ["feature", "approve", "--role", "planner"], failDir);
  runHarness(failHarness, ["feature", "approve", "--role", "tester"], failDir);
  writeFile(path.join(failDir, "README.md"), "# Package Blocking Smoke\n");
  initGit(failDir);
  writeFile(path.join(failDir, "main.go"), "package main\n\n// SPEC_DEVIATION\nfunc main() {}\n");
  const failQA = runHarness(failHarness, ["feature", "qa", "--format", "json"], failDir);
  const failReport = JSON.parse(failQA.stdout);
  assert(failReport.verdict === "FAIL", `expected FAIL, got ${failReport.verdict}`);
  assert(failReport.dimensions.contract.score === 0, "expected contract score 0");

  console.log("Practical package smoke passed");
  console.log(`  tarball: ${tarball}`);
  console.log(`  pass verdict: ${passReport.verdict} (${passReport.total_score})`);
  console.log(`  blocking verdict: ${failReport.verdict} (contract ${failReport.dimensions.contract.score})`);
  if (keep) {
    console.log(`  kept workspace: ${smokeRoot}`);
  }
} finally {
  if (!keep) {
    fs.rmSync(smokeRoot, { recursive: true, force: true });
  }
}

function setupFromTarball(tarball, dir, planning = "spec-driven") {
  run(binary("npm"), [
    "exec",
    "--yes",
    "--cache",
    cacheDir,
    "--package",
    tarball,
    "--",
    "harness",
    "setup",
    "--planning",
    planning,
    "--cli",
    "none",
    "--yes",
  ], dir);
  const local = path.join(dir, ".harness", "bin", executable("harness"));
  assert(fs.existsSync(local), `local harness binary missing: ${local}`);
  return local;
}

function runHarness(harness, args, cwd) {
  return run(harness, args, cwd);
}

function initGit(dir) {
  run("git", ["init"], dir);
  run("git", ["config", "user.email", "harness-smoke@example.local"], dir);
  run("git", ["config", "user.name", "Harness Smoke"], dir);
  run("git", ["add", "."], dir);
  run("git", ["commit", "-m", "smoke baseline"], dir);
}

function run(command, args, cwd) {
  let actualCommand = command;
  let actualArgs = args;
  if (process.platform === "win32" && command === "npm") {
    actualCommand = process.env.ComSpec || "cmd.exe";
    actualArgs = ["/d", "/s", "/c", ["npm", ...args].map(quoteWindowsArg).join(" ")];
  }
  const result = spawnSync(actualCommand, actualArgs, {
    cwd,
    encoding: "utf8",
    env: process.env,
  });
  if (result.error || result.status !== 0) {
    const details = [
      `${command} ${args.join(" ")}`,
      result.error ? result.error.message : `exit ${result.status}`,
      result.stdout,
      result.stderr,
    ].filter(Boolean).join("\n");
    throw new Error(details);
  }
  return result;
}

function writeFile(file, body) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body);
}

function executable(name) {
  return process.platform === "win32" ? `${name}.exe` : name;
}

function binary(name) {
  return name;
}

function quoteWindowsArg(value) {
  if (!/[\s"]/u.test(value)) {
    return value;
  }
  return `"${value.replace(/"/gu, '\\"')}"`;
}

function lastNonEmptyLine(value) {
  return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean).pop();
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}
