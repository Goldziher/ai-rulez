const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
	getBinaryName,
	getPackagedBinaryCandidates,
	usePackagedBinaryIfAvailable,
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
	assert.deepEqual(getPackagedBinaryCandidates("linux", "amd64"), [
		"ai-rulez-linux-amd64",
	]);
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
