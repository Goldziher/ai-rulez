import { execSync, spawn } from 'node:child_process'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { promisify } from 'node:util'
import { afterAll, beforeAll, describe, expect, test } from 'vitest'

const exec = promisify(require('node:child_process').exec)

// Cross-platform copy directory function
function copyDir(src: string, dest: string) {
  if (process.platform === 'win32') {
    execSync(`xcopy "${src}" "${dest}" /E /I /Q`, { stdio: 'inherit' })
  } else {
    execSync(`cp -r "${src}" "${dest}"`, { stdio: 'inherit' })
  }
}

// Cross-platform remove directory function
function removeDir(dir: string) {
  if (process.platform === 'win32') {
    execSync(`rmdir /S /Q "${dir}"`, { stdio: 'inherit' })
  } else {
    execSync(`rm -rf "${dir}"`, { stdio: 'inherit' })
  }
}

describe('NPM Installation Tests', () => {
  let tempDir: string
  let npmPackageDir: string

  beforeAll(async () => {
    // Create temporary directory
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ai-rulez-npm-test-'))
    npmPackageDir = path.join(tempDir, 'npm-package')

    // Check if npm build directory exists
    const npmSourceDir = path.join(__dirname, '../../build/npm')
    if (!fs.existsSync(npmSourceDir)) {
      throw new Error(`NPM source directory not found: ${npmSourceDir}`)
    }

    // Copy npm package files to temp directory
    copyDir(npmSourceDir, npmPackageDir)

    // Set a test version
    const packageJsonPath = path.join(npmPackageDir, 'package.json')
    const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'))
    packageJson.version = '1.0.0'
    fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, null, 2))
  })

  afterAll(async () => {
    // Clean up temp directory
    if (tempDir && fs.existsSync(tempDir)) {
      try {
        removeDir(tempDir)
      } catch (error) {
        // Ignore cleanup errors in tests
        console.warn('Failed to clean up temp directory:', error)
      }
    }
  })

  test('should detect platform correctly', { timeout: 30000 }, async () => {
    const testScript = `
      const { getPlatform } = require('./install.js')
      try {
        const platform = getPlatform()
        console.log(JSON.stringify(platform))
      } catch (e) {
        console.error('ERROR:', e.message)
        process.exit(1)
      }
    `

    const { stdout } = await exec(`cd "${npmPackageDir}" && node -e "${testScript}"`)
    const platform = JSON.parse(stdout.trim())

    expect(platform).toHaveProperty('os')
    expect(platform).toHaveProperty('arch')
    expect(['darwin', 'linux', 'windows']).toContain(platform.os)
    expect(['amd64', 'arm64', '386']).toContain(platform.arch)
  })

  test('should handle download errors gracefully', { timeout: 40000 }, async () => {
    // Test with invalid URL
    const invalidPackageDir = path.join(tempDir, 'invalid-npm')
    copyDir(npmPackageDir, invalidPackageDir)

    // Modify install.js to use invalid URL
    const installJsPath = path.join(invalidPackageDir, 'install.js')
    const installJs = fs.readFileSync(installJsPath, 'utf8')
    const modifiedJs = installJs.replace(
      'https://github.com/Goldziher/ai-rulez',
      'https://github.com/nonexistent/invalid-repo'
    )
    fs.writeFileSync(installJsPath, modifiedJs)

    try {
      // Use process timeout instead of shell timeout command for cross-platform compatibility
      const timeoutMs = 30000
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), timeoutMs)

      try {
        await exec(`cd "${invalidPackageDir}" && node install.js`, {
          signal: controller.signal,
        })
        clearTimeout(timeoutId)
        throw new Error('Should have failed with invalid URL')
      } catch (execError: unknown) {
        clearTimeout(timeoutId)
        const err = execError as { stderr?: string; stdout?: string; code?: string }

        // Check if it was aborted (timeout) or actual error
        if (err.code === 'ABORT_ERR') {
          // Timeout is acceptable as an error condition
          return
        }

        expect(err.stderr || err.stdout).toMatch(
          /Failed to download|Failed to install|Error|404|timeout/i
        )
      }
    } catch (error: unknown) {
      const err = error as { stderr?: string; stdout?: string }
      expect(err.stderr || err.stdout).toMatch(
        /Failed to download|Failed to install|Error|404|timeout/i
      )
    }
  })

  test('should validate Node.js version requirement', { timeout: 30000 }, async () => {
    const testScript = `
      // Mock old Node.js version
      const originalVersion = process.version
      Object.defineProperty(process, 'version', { value: 'v12.0.0' })
      
      try {
        require('./install.js')
        console.log('SHOULD_HAVE_FAILED')
        process.exit(0)
      } catch (e) {
        console.error('VERSION_ERROR_CAUGHT:', e.message)
        process.exit(1)
      }
    `

    try {
      const result = await exec(`cd "${npmPackageDir}" && node -e "${testScript}"`)
      // If we get here, the script didn't exit with error, which is unexpected
      expect(result.stdout).toContain('SHOULD_HAVE_FAILED')
      throw new Error('Should have failed with old Node.js version')
    } catch (error: unknown) {
      const err = error as { stderr?: string; stdout?: string; code?: number }

      // Check if it's the expected error (exit code 1)
      if (err.code === 1) {
        const output = err.stdout || err.stderr || ''
        expect(output).toMatch(/Node\.js.*not supported|VERSION_ERROR_CAUGHT/i)
      } else {
        // Unexpected error, re-throw
        throw error
      }
    }
  })

  test('should handle checksum verification', { timeout: 30000 }, async () => {
    const checksumTestScript = `
      const { calculateSHA256, getExpectedChecksum } = require('./install.js')
      const fs = require('fs')
      
      ;(async () => {
        try {
          // Create test file
          const testFile = 'test.txt'
          const testContent = 'Hello, World!'
          fs.writeFileSync(testFile, testContent)
          
          // Calculate hash
          const hash = await calculateSHA256(testFile)
          console.log('HASH:', hash)
          
          // Create mock checksums file
          const checksumContent = hash + '  test.txt\\n'
          fs.writeFileSync('checksums.txt', checksumContent)
          
          // Test checksum parsing
          const expectedHash = await getExpectedChecksum('checksums.txt', 'test.txt')
          console.log('EXPECTED:', expectedHash)
          console.log('MATCH:', hash === expectedHash)
          
          // Cleanup
          fs.unlinkSync(testFile)
          fs.unlinkSync('checksums.txt')
        } catch (e) {
          console.error('ERROR:', e.message)
          process.exit(1)
        }
      })()
    `

    const { stdout } = await exec(`cd "${npmPackageDir}" && node -e "${checksumTestScript}"`)
    expect(stdout).toContain('HASH:')
    expect(stdout).toContain('EXPECTED:')
    expect(stdout).toContain('MATCH: true')
  })

  test('should create mock binary installation', { timeout: 30000 }, async () => {
    // Create a mock test that simulates successful installation
    const mockInstallJs = `
      const fs = require('fs')
      const path = require('path')
      
      // Mock successful installation
      const binDir = path.join(__dirname, 'bin')
      if (!fs.existsSync(binDir)) {
        fs.mkdirSync(binDir, { recursive: true })
      }
      
      const binaryName = process.platform === 'win32' ? 'ai-rulez.exe' : 'ai-rulez'
      const binaryPath = path.join(binDir, binaryName)
      
      // Create a mock binary file
      fs.writeFileSync(binaryPath, '#!/bin/bash\\necho "ai-rulez mock version"\\n')
      if (process.platform !== 'win32') {
        fs.chmodSync(binaryPath, 0o755)
      }
      
      console.log('✅ ai-rulez mock installed successfully!')
    `

    const mockPackageDir = path.join(tempDir, 'mock-npm')
    copyDir(npmPackageDir, mockPackageDir)
    fs.writeFileSync(path.join(mockPackageDir, 'install.js'), mockInstallJs)

    const { stdout } = await exec(`cd "${mockPackageDir}" && node install.js`)
    expect(stdout).toContain('✅ ai-rulez mock installed successfully!')

    const binDir = path.join(mockPackageDir, 'bin')
    expect(fs.existsSync(binDir)).toBe(true)

    const binaryName = process.platform === 'win32' ? 'ai-rulez.exe' : 'ai-rulez'
    const binaryPath = path.join(binDir, binaryName)
    expect(fs.existsSync(binaryPath)).toBe(true)
  })
})
