const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const http = require("node:http");
const crypto = require("node:crypto");
const { exec, spawn } = require("node:child_process");
const { promisify } = require("node:util");

const _execAsync = promisify(exec);

const REPO_NAME = "Goldziher/ai-rulez";
const DOWNLOAD_TIMEOUT = 30000;
const MAX_RETRIES = 3;
const RETRY_DELAY = 2000;

async function calculateSHA256(filePath) {
	return new Promise((resolve, reject) => {
		const hash = crypto.createHash("sha256");
		const stream = fs.createReadStream(filePath);

		stream.on("data", (data) => hash.update(data));
		stream.on("end", () => resolve(hash.digest("hex")));
		stream.on("error", reject);
	});
}

async function getExpectedChecksum(checksumPath, filename) {
	try {
		const checksumContent = fs.readFileSync(checksumPath, "utf8");
		const lines = checksumContent.split("\n");

		for (const line of lines) {
			const parts = line.trim().split(/\s+/);
			if (parts.length >= 2 && parts[1] === filename) {
				return parts[0];
			}
		}
		return null;
	} catch (error) {
		console.warn("Warning: Could not parse checksums file:", error.message);
		return null;
	}
}

function getPlatform() {
	const platform = process.platform;
	const arch = process.arch;

	const platformMap = {
		darwin: "darwin",
		linux: "linux",
		win32: "windows",
	};

	const archMap = {
		x64: "amd64",
		arm64: "arm64",
		ia32: "386",
		x32: "386",
	};

	const mappedPlatform = platformMap[platform];
	const mappedArch = archMap[arch];

	if (!mappedPlatform) {
		throw new Error(
			`Unsupported operating system: ${platform}. Supported platforms: darwin (macOS), linux, win32 (Windows)`,
		);
	}

	if (!mappedArch) {
		throw new Error(
			`Unsupported architecture: ${arch}. Supported architectures: x64, arm64, ia32`,
		);
	}

	if (mappedPlatform === "windows" && mappedArch === "arm64") {
		throw new Error(
			"Windows ARM64 is not currently supported. Please use x64 or ia32 version.",
		);
	}

	return {
		os: mappedPlatform,
		arch: mappedArch,
	};
}

function getBinaryName(platform) {
	return platform === "windows" ? "ai-rulez-bin.exe" : "ai-rulez-bin";
}

function getPackagedBinaryCandidates(os, arch) {
	const baseName = `ai-rulez-${os}-${arch}`;
	if (os === "windows") {
		return [`${baseName}.exe`, baseName];
	}
	return [baseName];
}

function usePackagedBinaryIfAvailable(os, arch, binaryName, binDir) {
	const packagedBinaryCandidates = getPackagedBinaryCandidates(os, arch);
	const binaryPath = path.join(binDir, binaryName);

	for (const candidateName of packagedBinaryCandidates) {
		const packagedBinaryPath = path.join(binDir, candidateName);
		if (!fs.existsSync(packagedBinaryPath)) {
			continue;
		}

		if (path.resolve(packagedBinaryPath) !== path.resolve(binaryPath)) {
			fs.copyFileSync(packagedBinaryPath, binaryPath);
		}

		if (os !== "windows") {
			fs.chmodSync(binaryPath, 0o755);
		}

		console.log(`Using packaged binary for ${os}/${arch} (${candidateName})`);
		return true;
	}

	return false;
}

async function downloadBinary(url, dest, retryCount = 0) {
	return new Promise((resolve, reject) => {
		const file = fs.createWriteStream(dest);
		const protocol = url.startsWith("https") ? https : http;

		const request = protocol.get(
			url,
			{ timeout: DOWNLOAD_TIMEOUT },
			(response) => {
				if (response.statusCode === 302 || response.statusCode === 301) {
					file.close();
					try {
						fs.unlinkSync(dest);
					} catch {}
					downloadBinary(response.headers.location, dest, retryCount)
						.then(resolve)
						.catch(reject);
					return;
				}

				if (response.statusCode !== 200) {
					file.close();
					try {
						fs.unlinkSync(dest);
					} catch {}
					const error = new Error(
						`HTTP ${response.statusCode}: ${response.statusMessage}`,
					);

					if (retryCount < MAX_RETRIES) {
						console.log(
							`Download failed, retrying in ${RETRY_DELAY / 1000}s... (${retryCount + 1}/${MAX_RETRIES})`,
						);
						setTimeout(() => {
							downloadBinary(url, dest, retryCount + 1)
								.then(resolve)
								.catch(reject);
						}, RETRY_DELAY);
						return;
					}

					reject(error);
					return;
				}

				let downloadedBytes = 0;
				response.on("data", (chunk) => {
					downloadedBytes += chunk.length;
				});

				response.pipe(file);

				file.on("finish", () => {
					file.close();
					if (downloadedBytes === 0) {
						try {
							fs.unlinkSync(dest);
						} catch {}
						reject(new Error("Downloaded file is empty"));
						return;
					}
					console.log(`Downloaded ${downloadedBytes} bytes`);
					resolve();
				});

				file.on("error", (err) => {
					file.close();
					try {
						fs.unlinkSync(dest);
					} catch {}
					reject(err);
				});
			},
		);

		request.on("timeout", () => {
			request.destroy();
			file.close();
			try {
				fs.unlinkSync(dest);
			} catch {}

			if (retryCount < MAX_RETRIES) {
				console.log(
					`Download timeout, retrying in ${RETRY_DELAY / 1000}s... (${retryCount + 1}/${MAX_RETRIES})`,
				);
				setTimeout(() => {
					downloadBinary(url, dest, retryCount + 1)
						.then(resolve)
						.catch(reject);
				}, RETRY_DELAY);
				return;
			}

			reject(new Error("Download timeout after multiple retries"));
		});

		request.on("error", (err) => {
			file.close();
			try {
				fs.unlinkSync(dest);
			} catch {}

			if (retryCount < MAX_RETRIES) {
				console.log(
					`Download error, retrying in ${RETRY_DELAY / 1000}s... (${retryCount + 1}/${MAX_RETRIES})`,
				);
				setTimeout(() => {
					downloadBinary(url, dest, retryCount + 1)
						.then(resolve)
						.catch(reject);
				}, RETRY_DELAY);
				return;
			}

			reject(err);
		});
	});
}

async function extractArchive(archivePath, extractDir, platform) {
	if (platform === "windows") {
		const escapedArchivePath = archivePath.replace(/'/g, "''");
		const escapedExtractDir = extractDir.replace(/'/g, "''");

		const powershellCommand = [
			"powershell.exe",
			"-NoProfile",
			"-ExecutionPolicy",
			"Bypass",
			"-Command",
			`Expand-Archive -LiteralPath '${escapedArchivePath}' -DestinationPath '${escapedExtractDir}' -Force`,
		];

		await new Promise((resolve, reject) => {
			const child = spawn(powershellCommand[0], powershellCommand.slice(1), {
				stdio: ["pipe", "pipe", "pipe"],
				windowsHide: true,
			});

			let stderr = "";
			child.stderr.on("data", (data) => {
				stderr += data.toString();
			});

			child.on("close", (code) => {
				if (code === 0) {
					resolve();
				} else {
					reject(
						new Error(
							`PowerShell extraction failed with code ${code}: ${stderr}`,
						),
					);
				}
			});

			child.on("error", reject);
		});
	} else {
		await new Promise((resolve, reject) => {
			const child = spawn("tar", ["-xzf", archivePath, "-C", extractDir], {
				stdio: ["pipe", "pipe", "pipe"],
			});

			let stderr = "";
			child.stderr.on("data", (data) => {
				stderr += data.toString();
			});

			child.on("close", (code) => {
				if (code === 0) {
					resolve();
				} else {
					reject(
						new Error(`tar extraction failed with code ${code}: ${stderr}`),
					);
				}
			});

			child.on("error", reject);
		});
	}
}

async function install(isPostInstall = false) {
	const DEBUG = process.env.AI_RULEZ_DEBUG === "1";

	try {
		if (DEBUG) console.error("[install.js] Starting installation");

		const nodeVersion = process.version;
		const majorVersion = parseInt(nodeVersion.slice(1).split(".")[0], 10);
		if (majorVersion < 20) {
			console.error(
				`Error: Node.js ${nodeVersion} is not supported. Please upgrade to Node.js 20 or later.`,
			);
			process.exit(1);
		}

		const { os, arch } = getPlatform();
		const binaryName = getBinaryName(os);
		const binDir = path.join(__dirname, "bin");

		if (DEBUG)
			console.error(
				`[install.js] Platform: ${os}/${arch}, Binary name: ${binaryName}`,
			);
		if (!fs.existsSync(binDir)) {
			fs.mkdirSync(binDir, { recursive: true });
		}

		if (usePackagedBinaryIfAvailable(os, arch, binaryName, binDir)) {
			if (DEBUG) {
				console.error("[install.js] Packaged binary detected, skipping download");
			}
			if (!isPostInstall) {
				process.exit(0);
			}
			return;
		}
		const packageJson = JSON.parse(
			fs.readFileSync(path.join(__dirname, "package.json"), "utf8"),
		);
		const version = packageJson.version;

		const archiveExt = os === "windows" ? "zip" : "tar.gz";
		const archiveName = `ai-rulez_${version}_${os}_${arch}.${archiveExt}`;
		const downloadUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/${archiveName}`;
		const checksumUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/checksums.txt`;

		console.log(`Downloading ai-rulez ${version} for ${os}/${arch}...`);
		console.log(`URL: ${downloadUrl}`);
		const archivePath = path.join(__dirname, archiveName);

		console.log("Downloading checksums...");
		const checksumPath = path.join(__dirname, "checksums.txt");
		try {
			await downloadBinary(checksumUrl, checksumPath);
		} catch (_checksumError) {
			console.warn(
				"Warning: Could not download checksums, skipping verification",
			);
		}

		await downloadBinary(downloadUrl, archivePath);

		if (fs.existsSync(checksumPath)) {
			console.log("Verifying checksum...");
			const expectedHash = await getExpectedChecksum(checksumPath, archiveName);
			if (expectedHash) {
				const actualHash = await calculateSHA256(archivePath);
				if (actualHash !== expectedHash) {
					throw new Error(
						`Checksum verification failed. Expected: ${expectedHash}, Got: ${actualHash}`,
					);
				}
				console.log("✓ Checksum verified");
			}
			fs.unlinkSync(checksumPath);
		}

		console.log("Extracting binary...");

		const tempExtractDir = path.join(__dirname, ".extract-temp");
		if (fs.existsSync(tempExtractDir)) {
			fs.rmSync(tempExtractDir, { recursive: true, force: true });
		}
		fs.mkdirSync(tempExtractDir, { recursive: true });

		await extractArchive(archivePath, tempExtractDir, os);

		const extractedName = os === "windows" ? "ai-rulez.exe" : "ai-rulez";
		const extractedPath = path.join(tempExtractDir, extractedName);
		const binaryPath = path.join(binDir, binaryName);

		if (fs.existsSync(extractedPath)) {
			if (fs.existsSync(binaryPath)) {
				fs.unlinkSync(binaryPath);
			}
			fs.renameSync(extractedPath, binaryPath);
		}

		fs.rmSync(tempExtractDir, { recursive: true, force: true });

		if (!fs.existsSync(binaryPath)) {
			throw new Error(`Binary not found after extraction: ${binaryPath}`);
		}

		if (os !== "windows") {
			fs.chmodSync(binaryPath, 0o755);
		}

		try {
			await new Promise((resolve, reject) => {
				const testCommand =
					os === "windows"
						? [binaryPath, "--version"]
						: [binaryPath, "--version"];
				const child = spawn(testCommand[0], testCommand.slice(1), {
					stdio: ["pipe", "pipe", "pipe"],
					timeout: 5000,
				});

				child.on("close", (_code) => {
					resolve();
				});

				child.on("error", (err) => {
					if (err.code === "ENOENT") {
						reject(new Error("Downloaded binary is not executable"));
					} else {
						resolve();
					}
				});
			});
		} catch (verifyError) {
			console.warn(
				"Warning: Could not verify binary execution:",
				verifyError.message,
			);
		}

		fs.unlinkSync(archivePath);

		console.log(
			`✅ ai-rulez ${version} installed successfully for ${os}/${arch}!`,
		);

		if (DEBUG) {
			console.error(`[install.js] Installation complete`);
			console.error(`[install.js] Binary location: ${binaryPath}`);
			console.error(`[install.js] Exiting with code 0`);
		}

		if (!isPostInstall) {
			process.exit(0);
		}
	} catch (error) {
		if (DEBUG)
			console.error(`[install.js] Installation failed: ${error.message}`);
		console.error("Failed to install ai-rulez binary:", error.message);
		console.error("You can manually download the binary from:");
		console.error(`https://github.com/${REPO_NAME}/releases`);
		if (!isPostInstall) {
			process.exit(1);
		} else {
			throw error;
		}
	}
}

if (typeof module !== "undefined" && module.exports) {
	module.exports = {
		getPlatform,
		getBinaryName,
		getPackagedBinaryCandidates,
		usePackagedBinaryIfAvailable,
		downloadBinary,
		extractArchive,
		calculateSHA256,
		getExpectedChecksum,
		install,
	};
}

if (require.main === module) {
	install(false);
}
