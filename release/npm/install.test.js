const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  getBinaryName,
  bundledPackageName,
  resolveBundledBinary,
  isMusl,
  getPackagedBinaryCandidates,
  downloadBinary,
  proxyFor,
  usePackagedBinaryIfAvailable,
  verifyArchiveChecksum,
} = require("./install");

function withTempBinDir(fn) {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), "ai-rulez-npm-test-"));
  const binDir = path.join(tmpRoot, "bin");
  fs.mkdirSync(binDir, { recursive: true });

  try {
    fn(binDir);
  } finally {
    fs.rmSync(tmpRoot, { recursive: true, force: true });
  }
}

test("getPackagedBinaryCandidates returns expected windows and non-windows names", () => {
  assert.deepEqual(getPackagedBinaryCandidates("linux", "amd64"), ["ai-rulez-linux-amd64"]);
  assert.deepEqual(getPackagedBinaryCandidates("windows", "amd64"), [
    "ai-rulez-windows-amd64.exe",
    "ai-rulez-windows-amd64",
  ]);
});

test("usePackagedBinaryIfAvailable copies packaged binary to runtime binary name", () => {
  withTempBinDir((binDir) => {
    const packagedBinaryPath = path.join(binDir, "ai-rulez-linux-amd64");
    const runtimeBinaryPath = path.join(binDir, getBinaryName("linux"));

    fs.writeFileSync(packagedBinaryPath, "linux-binary");

    const usedPackaged = usePackagedBinaryIfAvailable(
      "linux",
      "amd64",
      getBinaryName("linux"),
      binDir,
    );

    assert.equal(usedPackaged, true);
    assert.equal(fs.existsSync(runtimeBinaryPath), true);
    assert.equal(fs.readFileSync(runtimeBinaryPath, "utf8"), "linux-binary");
  });
});

test("usePackagedBinaryIfAvailable supports windows exe packaged binary", () => {
  withTempBinDir((binDir) => {
    const packagedBinaryPath = path.join(binDir, "ai-rulez-windows-amd64.exe");
    const runtimeBinaryPath = path.join(binDir, getBinaryName("windows"));

    fs.writeFileSync(packagedBinaryPath, "windows-binary");

    const usedPackaged = usePackagedBinaryIfAvailable(
      "windows",
      "amd64",
      getBinaryName("windows"),
      binDir,
    );

    assert.equal(usedPackaged, true);
    assert.equal(fs.existsSync(runtimeBinaryPath), true);
    assert.equal(fs.readFileSync(runtimeBinaryPath, "utf8"), "windows-binary");
  });
});

test("usePackagedBinaryIfAvailable returns false when no packaged binary exists", () => {
  withTempBinDir((binDir) => {
    const usedPackaged = usePackagedBinaryIfAvailable(
      "linux",
      "amd64",
      getBinaryName("linux"),
      binDir,
    );

    assert.equal(usedPackaged, false);
    assert.equal(fs.existsSync(path.join(binDir, getBinaryName("linux"))), false);
  });
});

async function withArchive(checksumsFor, fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ai-rulez-npm-sum-"));
  try {
    const archive = path.join(dir, "ai-rulez_1.0.0_linux_amd64.tar.gz");
    fs.writeFileSync(archive, "archive-bytes");
    const sums = path.join(dir, "checksums.txt");
    if (checksumsFor !== null) {
      fs.writeFileSync(sums, checksumsFor);
    }
    await fn(archive, sums);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

const archiveName = "ai-rulez_1.0.0_linux_amd64.tar.gz";
const archiveSha = require("node:crypto").createHash("sha256").update("archive-bytes").digest("hex");

test("verifyArchiveChecksum accepts a matching entry", async () => {
  await withArchive(`${archiveSha}  ${archiveName}\n`, async (archive, sums) => {
    await verifyArchiveChecksum(archive, sums, archiveName);
  });
});

test("verifyArchiveChecksum fails closed when checksums.txt is missing", async () => {
  await withArchive(null, async (archive, sums) => {
    await assert.rejects(verifyArchiveChecksum(archive, sums, archiveName), /checksums\.txt is unavailable/);
  });
});

test("verifyArchiveChecksum fails closed when the archive has no entry", async () => {
  await withArchive(`${archiveSha}  other.tar.gz\n`, async (archive, sums) => {
    await assert.rejects(verifyArchiveChecksum(archive, sums, archiveName), /no checksum for/);
  });
});

test("verifyArchiveChecksum rejects a mismatching digest", async () => {
  await withArchive(`${"0".repeat(64)}  ${archiveName}\n`, async (archive, sums) => {
    await assert.rejects(verifyArchiveChecksum(archive, sums, archiveName), /Checksum verification failed/);
  });
});

test("proxyFor honours HTTPS_PROXY and NO_PROXY", () => {
  const url = "https://github.com/Goldziher/ai-rulez/releases/download/v1/checksums.txt";
  assert.equal(proxyFor(url, {}), null);
  assert.equal(proxyFor(url, { HTTPS_PROXY: "http://proxy.local:3128" }).host, "proxy.local:3128");
  assert.equal(proxyFor(url, { https_proxy: "proxy.local:3128" }).host, "proxy.local:3128");
  assert.equal(proxyFor(url, { HTTPS_PROXY: "http://p:1", NO_PROXY: "foo.com,.github.com" }), null);
  assert.equal(proxyFor(url, { HTTPS_PROXY: "http://p:1", NO_PROXY: "*" }), null);
});

test("downloadBinary tunnels through HTTPS_PROXY with CONNECT", async () => {
  const http = require("node:http");
  const seen = [];
  const proxy = http.createServer();
  proxy.on("connect", (req, socket) => {
    seen.push(req.url);
    socket.end("HTTP/1.1 403 Forbidden\r\n\r\n");
  });
  await new Promise((r) => proxy.listen(0, "127.0.0.1", r));
  const saved = process.env.HTTPS_PROXY;
  process.env.HTTPS_PROXY = `http://127.0.0.1:${proxy.address().port}`;
  const dest = path.join(os.tmpdir(), `ai-rulez-proxy-${process.pid}`);
  try {
    await assert.rejects(downloadBinary("https://example.invalid/x.txt", dest, 99), /Proxy CONNECT .* HTTP 403/);
    assert.deepEqual(seen, ["example.invalid:443"]);
  } finally {
    if (saved === undefined) delete process.env.HTTPS_PROXY;
    else process.env.HTTPS_PROXY = saved;
    proxy.close();
    fs.rmSync(dest, { force: true });
  }
});

function fakeBundledPackage(binaryName) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "ai-rulez-bundled-"));
  fs.mkdirSync(path.join(root, "bin"), { recursive: true });
  fs.writeFileSync(path.join(root, "package.json"), "{}");
  if (binaryName) fs.writeFileSync(path.join(root, "bin", binaryName), "");
  return root;
}

test("resolveBundledBinary returns the binary of the resolved platform package", () => {
  const root = fakeBundledPackage("ai-rulez");
  try {
    let requested;
    const result = resolveBundledBinary({
      platform: "linux",
      arch: "x64",
      resolve: (id) => {
        requested = id;
        return path.join(root, "package.json");
      },
    });
    assert.equal(requested, "@ai-rulez/linux-x64/package.json");
    assert.equal(result, path.join(root, "bin", "ai-rulez"));
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("resolveBundledBinary uses .exe on win32", () => {
  const root = fakeBundledPackage("ai-rulez.exe");
  try {
    const result = resolveBundledBinary({
      platform: "win32",
      arch: "x64",
      resolve: () => path.join(root, "package.json"),
    });
    assert.equal(result, path.join(root, "bin", "ai-rulez.exe"));
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("resolveBundledBinary returns null when the package is not installed", () => {
  const result = resolveBundledBinary({
    platform: "linux",
    arch: "x64",
    resolve: () => {
      const err = new Error("Cannot find module");
      err.code = "MODULE_NOT_FOUND";
      throw err;
    },
  });
  assert.equal(result, null);
});

test("resolveBundledBinary returns null when the package lacks the binary", () => {
  const root = fakeBundledPackage(null);
  try {
    const result = resolveBundledBinary({
      platform: "darwin",
      arch: "arm64",
      resolve: () => path.join(root, "package.json"),
    });
    assert.equal(result, null);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("resolveBundledBinary returns null for unsupported platform or arch", () => {
  const resolve = () => {
    throw new Error("resolve must not be called");
  };
  for (const [platform, arch] of [
    ["freebsd", "x64"],
    ["linux", "ia32"],
    ["win32", "arm64"],
    ["linux", "riscv64"],
  ]) {
    assert.equal(resolveBundledBinary({ platform, arch, resolve }), null, `${platform}-${arch}`);
  }
});

test("bundledPackageName maps every supported platform and arch", () => {
  assert.equal(bundledPackageName("linux", "x64"), "@ai-rulez/linux-x64");
  assert.equal(bundledPackageName("linux", "arm64"), "@ai-rulez/linux-arm64");
  assert.equal(bundledPackageName("darwin", "x64"), "@ai-rulez/darwin-x64");
  assert.equal(bundledPackageName("darwin", "arm64"), "@ai-rulez/darwin-arm64");
  assert.equal(bundledPackageName("win32", "x64"), "@ai-rulez/win32-x64");
});

test("isMusl detects musl through the node report", () => {
  assert.equal(isMusl({ platform: "linux", report: () => ({ header: {} }) }), true);
  assert.equal(
    isMusl({ platform: "linux", report: () => ({ header: { glibcVersionRuntime: "2.35" } }) }),
    false,
  );
});

test("isMusl falls back to the Alpine marker when no report is available", () => {
  const noReport = () => undefined;
  assert.equal(isMusl({ platform: "linux", report: noReport, exists: () => true }), true);
  assert.equal(isMusl({ platform: "linux", report: noReport, exists: () => false }), false);
});

test("isMusl is false off linux", () => {
  assert.equal(isMusl({ platform: "darwin", report: () => ({ header: {} }), exists: () => true }), false);
});
