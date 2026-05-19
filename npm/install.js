const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const https = require("node:https");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const dist = path.join(root, "dist");
const pkg = require(path.join(root, "package.json"));
const binary = path.join(dist, binaryNames()[0]);

main().catch((error) => {
  console.warn(`Harness postinstall failed: ${error.message}`);
  process.exit(0);
});

async function main() {
  if (process.env.HARNESS_SKIP_POSTINSTALL === "1") {
    return;
  }

  if (findDistBinary()) {
    return;
  }

  fs.mkdirSync(dist, { recursive: true });

  if (await downloadReleaseBinary(binary)) {
    chmodExecutable(binary);
    return;
  }

  const go = findGo();
  if (!go) {
    console.warn("Harness postinstall could not find a prebuilt binary or Go.");
    console.warn("Set HARNESS_BINARY to a harness executable, or install Go and rerun.");
    return;
  }

  const build = spawnSync(
    go,
    ["build", "-ldflags", `-X main.version=${pkg.version}`, "-o", path.join(dist, binaryNames()[1]), "."],
    {
      cwd: root,
      stdio: "inherit",
    },
  );

  if (build.error || build.status !== 0) {
    console.warn("Harness postinstall could not build the Go binary.");
  }
}

function downloadReleaseBinary(destination) {
  const asset = binaryNames()[0];
  const url = `https://github.com/dancampari/harness/releases/download/v${pkg.version}/${asset}`;
  return new Promise((resolve) => {
    download(url, destination, 0, (error) => {
      if (error) {
        resolve(false);
        return;
      }
      resolve(true);
    });
  });
}

function download(url, destination, redirects, done) {
  const request = https.get(
    url,
    {
      headers: {
        "User-Agent": `@dancampari/harness/${pkg.version}`,
      },
    },
    (response) => {
      if ([301, 302, 303, 307, 308].includes(response.statusCode) && response.headers.location) {
        response.resume();
        if (redirects >= 5) {
          done(new Error("too many redirects"));
          return;
        }
        download(response.headers.location, destination, redirects + 1, done);
        return;
      }

      if (response.statusCode !== 200) {
        response.resume();
        done(new Error(`HTTP ${response.statusCode}`));
        return;
      }

      const tmp = `${destination}.tmp`;
      const file = fs.createWriteStream(tmp);
      response.pipe(file);
      file.on("finish", () => {
        file.close(() => {
          fs.rename(tmp, destination, done);
        });
      });
      file.on("error", (error) => {
        fs.rm(tmp, { force: true }, () => done(error));
      });
    },
  );
  request.on("error", done);
  request.setTimeout(30_000, () => request.destroy(new Error("download timeout")));
}

function findGo() {
  const candidates = [
    process.env.GO_BINARY,
    process.env.GOROOT && path.join(process.env.GOROOT, "bin", bin("go")),
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

function bin(name) {
  return process.platform === "win32" ? `${name}.exe` : name;
}

function binaryNames() {
  const ext = process.platform === "win32" ? ".exe" : "";
  return [
    `harness-${process.platform}-${process.arch}${ext}`,
    `harness${ext}`,
  ];
}

function findDistBinary() {
  for (const name of binaryNames()) {
    const candidate = path.join(dist, name);
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  return null;
}

function chmodExecutable(file) {
  if (process.platform !== "win32") {
    fs.chmodSync(file, 0o755);
  }
}
