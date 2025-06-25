const fs = require('fs');
const path = require('path');
const https = require('https');
const { exec } = require('child_process');
const { promisify } = require('util');

const execAsync = promisify(exec);

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
  
  const mappedPlatform = platformMap[platform];
  const mappedArch = archMap[arch];
  
  if (!mappedPlatform || !mappedArch) {
    throw new Error(`Unsupported platform: ${platform} ${arch}`);
  }
  
  // Windows ARM64 is not supported in our builds
  if (mappedPlatform === 'windows' && mappedArch === 'arm64') {
    throw new Error('Windows ARM64 is not supported');
  }
  
  return {
    os: mappedPlatform,
    arch: mappedArch
  };
}

function getBinaryName(platform) {
  return platform === 'windows' ? 'airules.exe' : 'airules';
}

async function downloadBinary(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        // Handle redirect
        https.get(response.headers.location, (redirectResponse) => {
          redirectResponse.pipe(file);
          file.on('finish', () => {
            file.close();
            resolve();
          });
        }).on('error', reject);
      } else if (response.statusCode === 200) {
        response.pipe(file);
        file.on('finish', () => {
          file.close();
          resolve();
        });
      } else {
        reject(new Error(`Failed to download: ${response.statusCode}`));
      }
    }).on('error', reject);
  });
}

async function extractArchive(archivePath, extractDir, platform) {
  if (platform === 'windows') {
    // Use PowerShell to extract zip on Windows
    await execAsync(`powershell -command "Expand-Archive -Path '${archivePath}' -DestinationPath '${extractDir}' -Force"`);
  } else {
    // Use tar for Unix-like systems
    await execAsync(`tar -xzf "${archivePath}" -C "${extractDir}"`);
  }
}

async function install() {
  try {
    const { os, arch } = getPlatform();
    const binaryName = getBinaryName(os);
    
    // Get version from package.json
    const packageJson = JSON.parse(fs.readFileSync(path.join(__dirname, 'package.json'), 'utf8'));
    const version = packageJson.version;
    
    // Construct download URL
    const archiveExt = os === 'windows' ? 'zip' : 'tar.gz';
    const archiveName = `airules_${version}_${os}_${arch}.${archiveExt}`;
    const downloadUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/${archiveName}`;
    
    console.log(`Downloading airules ${version} for ${os}/${arch}...`);
    console.log(`URL: ${downloadUrl}`);
    
    // Create bin directory
    const binDir = path.join(__dirname, 'bin');
    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }
    
    // Download archive
    const archivePath = path.join(__dirname, archiveName);
    await downloadBinary(downloadUrl, archivePath);
    
    // Extract archive
    console.log('Extracting binary...');
    await extractArchive(archivePath, binDir, os);
    
    // Make binary executable on Unix-like systems
    if (os !== 'windows') {
      const binaryPath = path.join(binDir, binaryName);
      fs.chmodSync(binaryPath, 0o755);
    }
    
    // Clean up archive
    fs.unlinkSync(archivePath);
    
    console.log(`✅ airules ${version} installed successfully for ${os}/${arch}!`);
    
  } catch (error) {
    console.error('Failed to install airules binary:', error.message);
    console.error('You can manually download the binary from:');
    console.error(`https://github.com/${REPO_NAME}/releases`);
    process.exit(1);
  }
}

// Only run install during postinstall
if (require.main === module) {
  install();
}