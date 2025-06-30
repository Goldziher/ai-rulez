const fs = require('fs');
const path = require('path');
const https = require('https');
const http = require('http');
const crypto = require('crypto');
const { exec, spawn } = require('child_process');
const { promisify } = require('util');

const execAsync = promisify(exec);

const REPO_NAME = 'Goldziher/ai-rulez';
const DOWNLOAD_TIMEOUT = 30000; // 30 seconds
const MAX_RETRIES = 3;
const RETRY_DELAY = 2000; // 2 seconds

async function calculateSHA256(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filePath);
    
    stream.on('data', (data) => hash.update(data));
    stream.on('end', () => resolve(hash.digest('hex')));
    stream.on('error', reject);
  });
}

async function getExpectedChecksum(checksumPath, filename) {
  try {
    const checksumContent = fs.readFileSync(checksumPath, 'utf8');
    const lines = checksumContent.split('\n');
    
    for (const line of lines) {
      const parts = line.trim().split(/\s+/);
      if (parts.length >= 2 && parts[1] === filename) {
        return parts[0];
      }
    }
    return null;
  } catch (error) {
    console.warn('Warning: Could not parse checksums file:', error.message);
    return null;
  }
}

function getPlatform() {
  const platform = process.platform;
  const arch = process.arch;
  
  const platformMap = {
    'darwin': 'darwin',
    'linux': 'linux',
    'win32': 'windows'
  };
  
  const archMap = {
    'x64': 'amd64',
    'arm64': 'arm64',
    'ia32': '386',
    'x32': '386'
  };
  
  const mappedPlatform = platformMap[platform];
  const mappedArch = archMap[arch];
  
  if (!mappedPlatform) {
    throw new Error(`Unsupported operating system: ${platform}. Supported platforms: darwin (macOS), linux, win32 (Windows)`);
  }
  
  if (!mappedArch) {
    throw new Error(`Unsupported architecture: ${arch}. Supported architectures: x64, arm64, ia32`);
  }
  
  
  if (mappedPlatform === 'windows' && mappedArch === 'arm64') {
    throw new Error('Windows ARM64 is not currently supported. Please use x64 or ia32 version.');
  }
  
  return {
    os: mappedPlatform,
    arch: mappedArch
  };
}

function getBinaryName(platform) {
  return platform === 'windows' ? 'ai-rulez.exe' : 'ai-rulez';
}

async function downloadBinary(url, dest, retryCount = 0) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const protocol = url.startsWith('https') ? https : http;
    
    const request = protocol.get(url, { timeout: DOWNLOAD_TIMEOUT }, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        file.close();
        try { fs.unlinkSync(dest); } catch {} // Clean up partial file
        downloadBinary(response.headers.location, dest, retryCount)
          .then(resolve)
          .catch(reject);
        return;
      }
      
      if (response.statusCode !== 200) {
        file.close();
        try { fs.unlinkSync(dest); } catch {} // Clean up partial file
        const error = new Error(`HTTP ${response.statusCode}: ${response.statusMessage}`);
        
        if (retryCount < MAX_RETRIES) {
          console.log(`Download failed, retrying in ${RETRY_DELAY/1000}s... (${retryCount + 1}/${MAX_RETRIES})`);
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
      response.on('data', (chunk) => {
        downloadedBytes += chunk.length;
      });
      
      response.pipe(file);
      
      file.on('finish', () => {
        file.close();
        if (downloadedBytes === 0) {
          try { fs.unlinkSync(dest); } catch {}
          reject(new Error('Downloaded file is empty'));
          return;
        }
        console.log(`Downloaded ${downloadedBytes} bytes`);
        resolve();
      });
      
      file.on('error', (err) => {
        file.close();
        try { fs.unlinkSync(dest); } catch {}
        reject(err);
      });
    });
    
    request.on('timeout', () => {
      request.destroy();
      file.close();
      try { fs.unlinkSync(dest); } catch {}
      
      if (retryCount < MAX_RETRIES) {
        console.log(`Download timeout, retrying in ${RETRY_DELAY/1000}s... (${retryCount + 1}/${MAX_RETRIES})`);
        setTimeout(() => {
          downloadBinary(url, dest, retryCount + 1)
            .then(resolve)
            .catch(reject);
        }, RETRY_DELAY);
        return;
      }
      
      reject(new Error('Download timeout after multiple retries'));
    });
    
    request.on('error', (err) => {
      file.close();
      try { fs.unlinkSync(dest); } catch {}
      
      if (retryCount < MAX_RETRIES) {
        console.log(`Download error, retrying in ${RETRY_DELAY/1000}s... (${retryCount + 1}/${MAX_RETRIES})`);
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
  if (platform === 'windows') {
    // Use safer PowerShell execution with proper escaping
    const escapedArchivePath = archivePath.replace(/'/g, "''");
    const escapedExtractDir = extractDir.replace(/'/g, "''");
    
    const powershellCommand = [
      'powershell.exe',
      '-NoProfile',
      '-ExecutionPolicy', 'Bypass',
      '-Command',
      `Expand-Archive -LiteralPath '${escapedArchivePath}' -DestinationPath '${escapedExtractDir}' -Force`
    ];
    
    await new Promise((resolve, reject) => {
      const child = spawn(powershellCommand[0], powershellCommand.slice(1), {
        stdio: ['pipe', 'pipe', 'pipe'],
        windowsHide: true
      });
      
      let stderr = '';
      child.stderr.on('data', (data) => {
        stderr += data.toString();
      });
      
      child.on('close', (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`PowerShell extraction failed with code ${code}: ${stderr}`));
        }
      });
      
      child.on('error', reject);
    });
  } else {
    // Use spawn instead of exec for better security and error handling
    await new Promise((resolve, reject) => {
      const child = spawn('tar', ['-xzf', archivePath, '-C', extractDir], {
        stdio: ['pipe', 'pipe', 'pipe']
      });
      
      let stderr = '';
      child.stderr.on('data', (data) => {
        stderr += data.toString();
      });
      
      child.on('close', (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`tar extraction failed with code ${code}: ${stderr}`));
        }
      });
      
      child.on('error', reject);
    });
  }
}

async function install() {
  try {
    // Check Node.js version compatibility
    const nodeVersion = process.version;
    const majorVersion = parseInt(nodeVersion.slice(1).split('.')[0]);
    if (majorVersion < 20) {
      console.error(`Error: Node.js ${nodeVersion} is not supported. Please upgrade to Node.js 20 or later.`);
      process.exit(1);
    }
    
    const { os, arch } = getPlatform();
    const binaryName = getBinaryName(os);
    
    
    const packageJson = JSON.parse(fs.readFileSync(path.join(__dirname, 'package.json'), 'utf8'));
    const version = packageJson.version;
    
    
    const archiveExt = os === 'windows' ? 'zip' : 'tar.gz';
    const archiveName = `ai-rulez_${version}_${os}_${arch}.${archiveExt}`;
    const downloadUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/${archiveName}`;
    const checksumUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/checksums.txt`;
    
    console.log(`Downloading ai-rulez ${version} for ${os}/${arch}...`);
    console.log(`URL: ${downloadUrl}`);
    
    
    const binDir = path.join(__dirname, 'bin');
    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }
    
    
    const archivePath = path.join(__dirname, archiveName);
    
    // Download checksums first for verification
    console.log('Downloading checksums...');
    const checksumPath = path.join(__dirname, 'checksums.txt');
    try {
      await downloadBinary(checksumUrl, checksumPath);
    } catch (checksumError) {
      console.warn('Warning: Could not download checksums, skipping verification');
    }
    
    await downloadBinary(downloadUrl, archivePath);
    
    // Verify checksum if available
    if (fs.existsSync(checksumPath)) {
      console.log('Verifying checksum...');
      const expectedHash = await getExpectedChecksum(checksumPath, archiveName);
      if (expectedHash) {
        const actualHash = await calculateSHA256(archivePath);
        if (actualHash !== expectedHash) {
          throw new Error(`Checksum verification failed. Expected: ${expectedHash}, Got: ${actualHash}`);
        }
        console.log('✓ Checksum verified');
      }
      fs.unlinkSync(checksumPath);
    }
    
    
    console.log('Extracting binary...');
    await extractArchive(archivePath, binDir, os);
    
    
    const binaryPath = path.join(binDir, binaryName);
    if (!fs.existsSync(binaryPath)) {
      throw new Error(`Binary not found after extraction: ${binaryPath}`);
    }
    
    
    if (os !== 'windows') {
      fs.chmodSync(binaryPath, 0o755);
    }
    
    // Verify binary is executable
    try {
      await new Promise((resolve, reject) => {
        const testCommand = os === 'windows' ? [binaryPath, '--version'] : [binaryPath, '--version'];
        const child = spawn(testCommand[0], testCommand.slice(1), {
          stdio: ['pipe', 'pipe', 'pipe'],
          timeout: 5000
        });
        
        child.on('close', (code) => {
          // Any exit code is fine, we just want to verify it can execute
          resolve();
        });
        
        child.on('error', (err) => {
          if (err.code === 'ENOENT') {
            reject(new Error('Downloaded binary is not executable'));
          } else {
            resolve(); // Other errors are OK for version check
          }
        });
      });
    } catch (verifyError) {
      console.warn('Warning: Could not verify binary execution:', verifyError.message);
    }
    
    
    fs.unlinkSync(archivePath);
    
    console.log(`✅ ai-rulez ${version} installed successfully for ${os}/${arch}!`);
    
  } catch (error) {
    console.error('Failed to install ai-rulez binary:', error.message);
    console.error('You can manually download the binary from:');
    console.error(`https://github.com/${REPO_NAME}/releases`);
    process.exit(1);
  }
}


// Export functions for testing
if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    getPlatform,
    getBinaryName,
    downloadBinary,
    extractArchive,
    calculateSHA256,
    getExpectedChecksum,
    install
  };
}

if (require.main === module) {
  install();
}