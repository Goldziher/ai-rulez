#!/usr/bin/env node

const { spawn } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");
const { getPlatform, getBinaryName } = require("./install");

const binaryName = getBinaryName(getPlatform().os);
const binaryPath = path.join(__dirname, "bin", binaryName);

function runBinary(args) {
	const child = spawn(binaryPath, args, {
		stdio: "inherit",
	});

	child.on("close", (code) => {
		process.exit(code);
	});
}

async function main() {
	if (fs.existsSync(binaryPath)) {
		runBinary(process.argv.slice(2));
	} else {
		console.log(
			"🚀 First run detected - downloading ai-rulez binary...\n   This will only happen once and takes a few seconds.\n",
		);
		const { install } = require("./install");
		try {
			await install(true); // Pass a flag to indicate this is a post-install step
			runBinary(process.argv.slice(2));
		} catch (error) {
			console.error("Failed to install or run ai-rulez:", error);
			process.exit(1);
		}
	}
}

main();