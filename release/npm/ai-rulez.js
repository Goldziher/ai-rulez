#!/usr/bin/env node

const { spawn } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");
const { getPlatform, getBinaryName, resolveBundledBinary, isMusl } = require("./install");

function runBinary(binaryPath, args) {
  const child = spawn(binaryPath, args, { stdio: "inherit" });

  child.on("error", (error) => {
    console.error(`Failed to run ai-rulez: ${error.message}`);
    process.exit(1);
  });

  child.on("close", (code, signal) => {
    if (signal) {
      const num = require("node:os").constants.signals[signal];
      process.exit(num ? 128 + num : 1);
    }
    process.exit(code ?? 1);
  });
}

async function main() {
  const args = process.argv.slice(2);

  if (isMusl()) {
    console.error(
      "ai-rulez ships glibc-only Linux binaries; no musl (Alpine) build exists. " +
        "Use a glibc-based image or build from source: https://github.com/Goldziher/ai-rulez",
    );
    process.exit(1);
  }

  const bundled = resolveBundledBinary();
  if (bundled) {
    runBinary(bundled, args);
    return;
  }

  let platform;
  try {
    platform = getPlatform();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }

  const binaryPath = path.join(__dirname, "bin", getBinaryName(platform.os));
  if (fs.existsSync(binaryPath)) {
    runBinary(binaryPath, args);
    return;
  }

  console.log(
    "First run detected - downloading ai-rulez binary...\n   This will only happen once and takes a few seconds.\n",
  );
  const { install } = require("./install");
  try {
    await install(true);
    runBinary(binaryPath, args);
  } catch (error) {
    console.error("Failed to install or run ai-rulez:", error);
    process.exit(1);
  }
}

main();
