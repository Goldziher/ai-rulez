const fs = require('node:fs')
const path = require('node:path')
const { spawn, exec } = require('node:child_process')
const { promisify } = require('node:util')
const os = require('node:os')

const execAsync = promisify(exec)

describe('NPM Installation Integration Tests', () => {
  let tempDir
  let npmPackageDir

  beforeAll(async () => {
    // Create temp directory for testing
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ai-rulez-npm-test-'))
    npmPackageDir = path.join(tempDir, 'npm-package')

    // Copy npm package files to temp directory
    const npmSourceDir = path.join(__dirname, '../../build/npm')

    // Check if source directory exists
    if (!fs.existsSync(npmSourceDir)) {
      throw new Error(`NPM source directory not found: ${npmSourceDir}`)
    }

    await execAsync(`cp -r "${npmSourceDir}" "${npmPackageDir}"`)

    // Set a test version
    const packageJson = JSON.parse(
      fs.readFileSync(path.join(npmPackageDir, 'package.json'), 'utf8')
    )
    packageJson.version = '1.0.0' // Use a stable version for testing
    fs.writeFileSync(path.join(npmPackageDir, 'package.json'), JSON.stringify(packageJson, null, 2))
  })

  afterAll(async () => {
    // Clean up temp directory
    if (tempDir && fs.existsSync(tempDir)) {
      await execAsync(`rm -rf "${tempDir}"`)
    }
  })

  test('should detect platform correctly', async () => {
    const { stdout } = await execAsync(`cd ${npmPackageDir} && node -e "
      const { getPlatform } = require('./install.js');
      try {
        const platform = getPlatform();
        console.log(JSON.stringify(platform));
      } catch (e) {
        console.error('ERROR:', e.message);
        process.exit(1);
      }
    "`)

    const platform = JSON.parse(stdout.trim())
    expect(platform).toHaveProperty('os')
    expect(platform).toHaveProperty('arch')
    expect(['darwin', 'linux', 'windows']).toContain(platform.os)
    expect(['amd64', 'arm64', '386']).toContain(platform.arch)
  })

  test('should handle download errors gracefully', async () => {
    // Test with invalid URL
    const invalidPackageDir = path.join(tempDir, 'invalid-npm')
    await execAsync(`cp -r "${npmPackageDir}" "${invalidPackageDir}"`)

    // Modify install.js to use invalid URL
    const installJs = fs.readFileSync(path.join(invalidPackageDir, 'install.js'), 'utf8')
    const modifiedJs = installJs.replace(
      'https://github.com/Goldziher/ai-rulez',
      'https://github.com/nonexistent/invalid-repo'
    )
    fs.writeFileSync(path.join(invalidPackageDir, 'install.js'), modifiedJs)

    try {
      // Use a shorter timeout for faster tests
      await execAsync(`cd "${invalidPackageDir}" && timeout 30 node install.js`)
      throw new Error('Should have failed with invalid URL')
    } catch (error) {
      expect(error.stderr || error.stdout).toMatch(
        /Failed to download|Failed to install|Error|404|timeout/i
      )
    }
  }, 40000)

  test('should validate Node.js version requirement', async () => {
    const testScript = `
      const originalVersion = process.version;
      Object.defineProperty(process, 'version', { value: 'v12.0.0' });
      
      try {
        require('./install.js');
      } catch (e) {
        console.log('VERSION_ERROR');
        process.exit(1);
      }
    `

    try {
      await execAsync(`cd ${npmPackageDir} && node -e "${testScript}"`)
      fail('Should have failed with old Node.js version')
    } catch (error) {
      expect(error.stdout || error.stderr).toMatch(/Node\.js.*not supported|VERSION_ERROR/)
    }
  })

  test('should create bin directory and download binary (mock)', async () => {
    // Create a mock test that simulates successful download
    const mockInstallJs = `
      const fs = require('fs');
      const path = require('path');
      
      // Mock successful installation
      const binDir = path.join(__dirname, 'bin');
      if (!fs.existsSync(binDir)) {
        fs.mkdirSync(binDir, { recursive: true });
      }
      
      const binaryName = process.platform === 'win32' ? 'ai-rulez.exe' : 'ai-rulez';
      const binaryPath = path.join(binDir, binaryName);
      
      // Create a mock binary file
      fs.writeFileSync(binaryPath, '#!/bin/bash\\necho "ai-rulez mock version"\\n');
      if (process.platform !== 'win32') {
        fs.chmodSync(binaryPath, 0o755);
      }
      
      console.log('✅ ai-rulez mock installed successfully!');
    `

    const mockPackageDir = path.join(tempDir, 'mock-npm')
    await execAsync(`cp -r ${npmPackageDir} ${mockPackageDir}`)
    fs.writeFileSync(path.join(mockPackageDir, 'install.js'), mockInstallJs)

    const { stdout } = await execAsync(`cd ${mockPackageDir} && node install.js`)
    expect(stdout).toContain('✅ ai-rulez mock installed successfully!')

    const binDir = path.join(mockPackageDir, 'bin')
    expect(fs.existsSync(binDir)).toBe(true)

    const binaryName = process.platform === 'win32' ? 'ai-rulez.exe' : 'ai-rulez'
    const binaryPath = path.join(binDir, binaryName)
    expect(fs.existsSync(binaryPath)).toBe(true)
  })

  test('should handle checksum verification', async () => {
    // Test checksum functionality with mock data
    const checksumTestScript = `
      const { calculateSHA256, getExpectedChecksum } = require('./install.js');
      const fs = require('fs');
      
      (async () => {
        try {
          // Create test file
          const testFile = 'test.txt';
          const testContent = 'Hello, World!';
          fs.writeFileSync(testFile, testContent);
          
          // Calculate hash
          const hash = await calculateSHA256(testFile);
          console.log('HASH:', hash);
          
          // Create mock checksums file
          const checksumContent = hash + '  test.txt\\n';
          fs.writeFileSync('checksums.txt', checksumContent);
          
          // Test checksum parsing
          const expectedHash = await getExpectedChecksum('checksums.txt', 'test.txt');
          console.log('EXPECTED:', expectedHash);
          console.log('MATCH:', hash === expectedHash);
          
          // Cleanup
          fs.unlinkSync(testFile);
          fs.unlinkSync('checksums.txt');
        } catch (e) {
          console.error('ERROR:', e.message);
          process.exit(1);
        }
      })();
    `

    const { stdout } = await execAsync(`cd ${npmPackageDir} && node -e "${checksumTestScript}"`)
    expect(stdout).toContain('HASH:')
    expect(stdout).toContain('EXPECTED:')
    expect(stdout).toContain('MATCH: true')
  })
})
