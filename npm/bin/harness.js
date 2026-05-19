#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..", "..");
const pkg = require(path.join(root, "package.json"));
const args = process.argv.slice(2);
const env = {
  ...process.env,
  HARNESS_INVOKE: process.env.HARNESS_INVOKE || `npx ${pkg.name}`,
};

function binaryNames() {
  const ext = process.platform === "win32" ? ".exe" : "";
  return [
    `harness-${process.platform}-${process.arch}${ext}`,
    `harness${ext}`,
  ];
}

function run(command, commandArgs, options = {}) {
  const result = spawnSync(command, commandArgs, {
    stdio: "inherit",
    env,
    ...options,
  });
  if (result.error) {
    return { ok: false, error: result.error };
  }
  process.exit(result.status === null ? 1 : result.status);
}

const configured = process.env.HARNESS_BINARY;
if (configured && fs.existsSync(configured)) {
  run(configured, args);
}

const distBinary = findDistBinary();
if (distBinary) {
  run(distBinary, args);
}

const sourceMain = path.join(root, "main.go");
if (fs.existsSync(sourceMain)) {
  const go = findGo();
  if (go) {
    const buildBinary = path.join(root, "dist", binaryNames()[1]);
    fs.mkdirSync(path.dirname(buildBinary), { recursive: true });
    const build = spawnSync(
      go,
      ["build", "-ldflags", `-X main.version=${pkg.version}`, "-o", buildBinary, "."],
      {
        cwd: root,
        stdio: "inherit",
      },
    );
    if (!build.error && build.status === 0 && fs.existsSync(buildBinary)) {
      run(buildBinary, args);
    }
  }
}

console.error("Harness binary was not found.");
console.error("Expected one of:", binaryNames().map((name) => path.join(root, "dist", name)).join(", "));
console.error("Install Go and rerun, or set HARNESS_BINARY to a built harness executable.");
process.exit(1);

function findDistBinary() {
  for (const name of binaryNames()) {
    const candidate = path.join(root, "dist", name);
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  return null;
}

function findGo() {
  const candidates = [
    process.env.GO_BINARY,
    process.env.GOROOT && path.join(process.env.GOROOT, "bin", binary("go")),
    process.platform === "win32" && "C:\\Program Files\\Go\\bin\\go.exe",
    "/usr/local/go/bin/go",
    "/opt/homebrew/bin/go",
    "/usr/bin/go",
    "go",
  ].filter(Boolean);

  for (const candidate of candidates) {
    const result = spawnSync(candidate, ["version"], { stdio: "ignore" });
    if (!result.error && result.status === 0) {
      return candidate;
    }
  }
  return null;
}

function binary(name) {
  return process.platform === "win32" ? `${name}.exe` : name;
}
