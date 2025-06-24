const fs = require('fs');
const path = require('path');
const https = require('https');
const { execSync } = require('child_process');

const PACKAGE_VERSION = require('./package.json').version;
const REPO_NAME = 'Goldziher/airules';

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
  
  return {
    os: platformMap[platform] || platform,
    arch: archMap[arch] || arch
  };
}

function getBinaryName(platform) {
  return platform.os === 'windows' ? 'airules.exe' : 'airules';
}

function getDownloadUrl(platform) {
  const ext = platform.os === 'windows' ? 'zip' : 'tar.gz';
  const filename = `airules_${PACKAGE_VERSION}_${platform.os}_${platform.arch}.${ext}`;
  return `https://github.com/${REPO_NAME}/releases/download/v${PACKAGE_VERSION}/${filename}`;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        https.get(response.headers.location, (response) => {
          response.pipe(file);
          file.on('finish', () => {
            file.close();
            resolve();
          });
        });
      } else if (response.statusCode === 200) {
        response.pipe(file);
        file.on('finish', () => {
          file.close();
          resolve();
        });
      } else {
        reject(new Error(`Download failed: ${response.statusCode}`));
      }
    }).on('error', reject);
  });
}

async function install() {
  const platform = getPlatform();
  const binaryName = getBinaryName(platform);
  const downloadUrl = getDownloadUrl(platform);
  const binDir = path.join(__dirname, 'bin');
  const binPath = path.join(binDir, binaryName);
  
  console.log(`Installing airules for ${platform.os}/${platform.arch}...`);
  
  // Create bin directory
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }
  
  // Download archive
  const archivePath = path.join(__dirname, 'archive' + (platform.os === 'windows' ? '.zip' : '.tar.gz'));
  
  try {
    console.log(`Downloading from ${downloadUrl}...`);
    await download(downloadUrl, archivePath);
    
    // Extract archive
    if (platform.os === 'windows') {
      // Use PowerShell on Windows
      execSync(`powershell -command "Expand-Archive -Path '${archivePath}' -DestinationPath '${binDir}' -Force"`);
    } else {
      // Use tar on Unix-like systems
      execSync(`tar -xzf "${archivePath}" -C "${binDir}"`);
    }
    
    // Make binary executable on Unix-like systems
    if (platform.os !== 'windows') {
      fs.chmodSync(binPath, 0o755);
    }
    
    // Clean up archive
    fs.unlinkSync(archivePath);
    
    console.log('✅ airules installed successfully!');
  } catch (error) {
    console.error('Failed to install airules:', error.message);
    process.exit(1);
  }
}

// Run installation
install().catch(console.error);