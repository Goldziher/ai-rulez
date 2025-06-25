const fs = require('fs');
const path = require('path');
const https = require('https');
const { exec } = require('child_process');
const { promisify } = require('util');

const execAsync = promisify(exec);

const REPO_NAME = 'Goldziher/ai-rulez';

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
  
  
  if (mappedPlatform === 'windows' && mappedArch === 'arm64') {
    throw new Error('Windows ARM64 is not supported');
  }
  
  return {
    os: mappedPlatform,
    arch: mappedArch
  };
}

function getBinaryName(platform) {
  return platform === 'windows' ? 'ai-rulez.exe' : 'ai-rulez';
}

async function downloadBinary(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        
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
    
    await execAsync(`powershell -command "Expand-Archive -Path '${archivePath}' -DestinationPath '${extractDir}' -Force"`);
  } else {
    
    await execAsync(`tar -xzf "${archivePath}" -C "${extractDir}"`);
  }
}

async function install() {
  try {
    const { os, arch } = getPlatform();
    const binaryName = getBinaryName(os);
    
    
    const packageJson = JSON.parse(fs.readFileSync(path.join(__dirname, 'package.json'), 'utf8'));
    const version = packageJson.version;
    
    
    const archiveExt = os === 'windows' ? 'zip' : 'tar.gz';
    const archiveName = `ai-rulez_${version}_${os}_${arch}.${archiveExt}`;
    const downloadUrl = `https://github.com/${REPO_NAME}/releases/download/v${version}/${archiveName}`;
    
    console.log(`Downloading ai-rulez ${version} for ${os}/${arch}...`);
    console.log(`URL: ${downloadUrl}`);
    
    
    const binDir = path.join(__dirname, 'bin');
    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }
    
    
    const archivePath = path.join(__dirname, archiveName);
    await downloadBinary(downloadUrl, archivePath);
    
    
    console.log('Extracting binary...');
    await extractArchive(archivePath, binDir, os);
    
    
    if (os !== 'windows') {
      const binaryPath = path.join(binDir, binaryName);
      fs.chmodSync(binaryPath, 0o755);
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


if (require.main === module) {
  install();
}