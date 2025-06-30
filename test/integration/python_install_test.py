#!/usr/bin/env python3
"""
Integration tests for Python installation script
"""

import os
import sys
import tempfile
import shutil
import subprocess
import unittest
import platform
from unittest.mock import patch, MagicMock
from pathlib import Path


class PythonInstallationTest(unittest.TestCase):
    """Test Python installation functionality"""

    def setUp(self):
        """Set up test environment"""
        self.temp_dir = tempfile.mkdtemp(prefix='ai-rulez-python-test-')
        self.python_package_dir = os.path.join(self.temp_dir, 'python-package')
        
        # Copy Python package to temp directory
        build_dir = os.path.join(os.path.dirname(__file__), '../../build/python')
        shutil.copytree(build_dir, self.python_package_dir)
        
        # Add to Python path
        sys.path.insert(0, self.python_package_dir)

    def tearDown(self):
        """Clean up test environment"""
        if sys.path[0] == self.python_package_dir:
            sys.path.pop(0)
        if os.path.exists(self.temp_dir):
            shutil.rmtree(self.temp_dir, ignore_errors=True)

    def test_platform_detection(self):
        """Test platform detection functionality"""
        from ai_rulez.downloader import get_platform
        
        platform_name, arch = get_platform()
        
        # Should return valid platform and architecture
        self.assertIn(platform_name, ['darwin', 'linux', 'windows'])
        self.assertIn(arch, ['amd64', 'arm64', '386'])
        
        # Should match current platform
        current_system = platform.system().lower()
        if current_system == 'darwin':
            self.assertEqual(platform_name, 'darwin')
        elif current_system == 'linux':
            self.assertEqual(platform_name, 'linux')
        elif current_system == 'windows':
            self.assertEqual(platform_name, 'windows')

    def test_binary_url_generation(self):
        """Test binary URL generation"""
        from ai_rulez.downloader import get_binary_url, get_checksums_url
        
        version = "1.0.0"
        binary_url = get_binary_url(version)
        checksums_url = get_checksums_url(version)
        
        # URLs should be properly formatted
        self.assertIn("github.com/Goldziher/ai-rulez", binary_url)
        self.assertIn(f"v{version}", binary_url)
        self.assertIn("github.com/Goldziher/ai-rulez", checksums_url)
        self.assertIn("checksums.txt", checksums_url)

    def test_checksum_calculation(self):
        """Test SHA256 checksum calculation"""
        from ai_rulez.downloader import calculate_sha256
        
        # Create test file with known content
        test_file = os.path.join(self.temp_dir, 'test.txt')
        test_content = b'Hello, World!'
        
        with open(test_file, 'wb') as f:
            f.write(test_content)
        
        # Calculate checksum
        checksum = calculate_sha256(test_file)
        
        # Verify it's a valid SHA256 hash
        self.assertEqual(len(checksum), 64)
        self.assertTrue(all(c in '0123456789abcdef' for c in checksum))
        
        # Should be consistent
        checksum2 = calculate_sha256(test_file)
        self.assertEqual(checksum, checksum2)

    def test_checksum_parsing(self):
        """Test checksum file parsing"""
        from ai_rulez.downloader import get_expected_checksum
        
        # Create mock checksums content
        checksums_content = """
abc123  ai-rulez_1.0.0_linux_amd64.tar.gz
def456  ai-rulez_1.0.0_windows_amd64.zip
789ghi  ai-rulez_1.0.0_darwin_amd64.tar.gz
        """.strip()
        
        # Test parsing
        linux_hash = get_expected_checksum(checksums_content, 'ai-rulez_1.0.0_linux_amd64.tar.gz')
        windows_hash = get_expected_checksum(checksums_content, 'ai-rulez_1.0.0_windows_amd64.zip')
        nonexistent_hash = get_expected_checksum(checksums_content, 'nonexistent.tar.gz')
        
        self.assertEqual(linux_hash, 'abc123')
        self.assertEqual(windows_hash, 'def456')
        self.assertIsNone(nonexistent_hash)

    def test_binary_path_generation(self):
        """Test binary path generation"""
        from ai_rulez.downloader import get_binary_path
        
        binary_path = get_binary_path()
        
        # Should be in user's cache directory
        self.assertIn('.cache', str(binary_path))
        self.assertIn('ai-rulez', str(binary_path))
        
        # Should have correct extension on Windows
        if platform.system().lower() == 'windows':
            self.assertTrue(str(binary_path).endswith('.exe'))
        else:
            self.assertFalse(str(binary_path).endswith('.exe'))

    def test_binary_verification(self):
        """Test binary verification functionality"""
        from ai_rulez.downloader import verify_binary
        
        # Test with non-existent file
        nonexistent_path = os.path.join(self.temp_dir, 'nonexistent')
        self.assertFalse(verify_binary(nonexistent_path))
        
        # Test with empty file
        empty_file = os.path.join(self.temp_dir, 'empty')
        Path(empty_file).touch()
        self.assertFalse(verify_binary(empty_file))
        
        # Test with non-executable file
        non_exec_file = os.path.join(self.temp_dir, 'non_exec')
        with open(non_exec_file, 'w') as f:
            f.write('not executable')
        self.assertFalse(verify_binary(non_exec_file))

    def test_version_cache_management(self):
        """Test version cache functionality"""
        from ai_rulez.downloader import (
            get_cache_version_file, 
            is_binary_current_version, 
            update_cache_version
        )
        
        # Mock version
        with patch('ai_rulez.downloader.__version__', '1.0.0'):
            version_file = get_cache_version_file()
            
            # Initially should not be current
            self.assertFalse(is_binary_current_version())
            
            # Update cache
            update_cache_version()
            
            # Now should be current
            self.assertTrue(is_binary_current_version())
            
            # Test with different version
            with patch('ai_rulez.downloader.__version__', '2.0.0'):
                self.assertFalse(is_binary_current_version())

    def test_download_error_handling(self):
        """Test download error handling"""
        from ai_rulez.downloader import download_file_with_retries
        
        # Test with invalid URL
        invalid_url = 'https://nonexistent.domain/file.txt'
        dest_path = os.path.join(self.temp_dir, 'download_test')
        
        with self.assertRaises(RuntimeError):
            download_file_with_retries(invalid_url, dest_path, "test file")

    @patch('ai_rulez.downloader.download_and_verify_binary')
    @patch('ai_rulez.downloader.verify_binary')
    @patch('ai_rulez.downloader.is_binary_current_version')
    def test_ensure_binary_flow(self, mock_is_current, mock_verify, mock_download):
        """Test the main ensure_binary flow"""
        from ai_rulez.downloader import ensure_binary
        
        # Test case: binary exists and is current
        mock_is_current.return_value = True
        mock_verify.return_value = True
        
        with patch('ai_rulez.downloader.get_binary_path') as mock_path:
            mock_path.return_value = Path('/mock/path/ai-rulez')
            mock_path.return_value.exists.return_value = True
            
            result = ensure_binary()
            self.assertEqual(result, '/mock/path/ai-rulez')
            mock_download.assert_not_called()

    def test_cli_integration(self):
        """Test CLI integration"""
        # Create a mock binary for testing
        mock_binary = os.path.join(self.temp_dir, 'ai-rulez')
        if platform.system().lower() == 'windows':
            mock_binary += '.exe'
        
        # Create mock binary that returns version
        mock_script = f'''#!/bin/bash
if [ "$1" = "--version" ]; then
    echo "ai-rulez 1.0.0"
    exit 0
fi
echo "Mock ai-rulez called with: $@"
'''
        
        with open(mock_binary, 'w') as f:
            f.write(mock_script)
        os.chmod(mock_binary, 0o755)
        
        # Test CLI with mock binary
        from ai_rulez.cli import main
        
        with patch('ai_rulez.downloader.ensure_binary', return_value=mock_binary):
            with patch('sys.argv', ['ai-rulez', '--version']):
                try:
                    main()
                except SystemExit as e:
                    # CLI should exit with code 0 for --version
                    pass


def run_integration_tests():
    """Run all integration tests"""
    # Set up test environment
    unittest.main(verbosity=2, exit=False)


if __name__ == '__main__':
    run_integration_tests()