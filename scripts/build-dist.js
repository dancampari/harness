#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const pkg = require(path.join(root, "package.json"));
const dist = path.join(root, "dist");
const ext = process.platform === "win32" ? ".exe" : "";
const platformBinary = `harness-${process.platform}-${process.arch}${ext}`;
const genericBinary = `harness${ext}`;

fs.mkdirSync(dist, { recursive: true });

const output = path.join(dist, platformBinary);
const ldflags = `-s -w -X main.version=${pkg.version}`;
const build = spawnSync("go", ["build", "-ldflags", ldflags, "-o", output, "."], {
  cwd: root,
  stdio: "inherit",
});

if (build.error || build.status !== 0) {
  if (build.error) {
    console.error(build.error.message);
  }
  process.exit(build.status || 1);
}

const generic = path.join(dist, genericBinary);
fs.copyFileSync(output, generic);
if (process.platform !== "win32") {
  fs.chmodSync(output, 0o755);
  fs.chmodSync(generic, 0o755);
}

console.log(`Built ${path.relative(root, output)}`);
console.log(`Built ${path.relative(root, generic)}`);
