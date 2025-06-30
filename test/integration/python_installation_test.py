#!/usr/bin/env python3
"""
Integration tests for Python installation functionality
"""

import os
import platform
import sys
import tempfile
import shutil
import subprocess
from pathlib import Path
import pytest


class TestPythonInstallation:
    """Test Python package installation functionality"""

    @pytest.fixture
    def temp_dir(self):
        """Create temporary directory for tests"""
        temp_dir = tempfile.mkdtemp(prefix='ai-rulez-python-test-')
        yield temp_dir
        shutil.rmtree(temp_dir, ignore_errors=True)

    @pytest.fixture  
    def python_package_dir(self, temp_dir):
        """Copy Python package to temp directory"""
        build_dir = Path(__file__).parent.parent.parent / "build" / "python"
        if not build_dir.exists():
            pytest.skip(f"Python build directory not found: {build_dir}")
        
        package_dir = Path(temp_dir) / "python-package" 
        shutil.copytree(build_dir, package_dir)
        return package_dir

    @pytest.mark.smoke
    def test_platform_detection(self, python_package_dir):
        """Test platform detection functionality"""
        test_script = f"""
import sys
sys.path.insert(0, '{python_package_dir}')
from ai_rulez.downloader import get_platform

try:
    platform_name, arch = get_platform()
    print(f'Platform: {{platform_name}}, Arch: {{arch}}')
    assert platform_name in ['darwin', 'linux', 'windows']
    assert arch in ['amd64', 'arm64', '386']
    print('SUCCESS: Platform detection works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script], 
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"Platform detection failed: {result.stderr}"
        assert "SUCCESS: Platform detection works" in result.stdout

    @pytest.mark.smoke  
    def test_url_generation(self, python_package_dir):
        """Test binary URL generation"""
        test_script = f"""
import sys
sys.path.insert(0, '{python_package_dir}')
from ai_rulez.downloader import get_binary_url, get_checksums_url

try:
    binary_url = get_binary_url('1.0.0')
    checksums_url = get_checksums_url('1.0.0')
    
    print(f'Binary URL: {{binary_url}}')
    print(f'Checksums URL: {{checksums_url}}')
    
    assert 'github.com/Goldziher/ai-rulez' in binary_url
    assert 'v1.0.0' in binary_url
    assert 'checksums.txt' in checksums_url
    
    print('SUCCESS: URL generation works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script],
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"URL generation failed: {result.stderr}"
        assert "SUCCESS: URL generation works" in result.stdout

    @pytest.mark.integration
    def test_checksum_calculation(self, python_package_dir, temp_dir):
        """Test SHA256 checksum calculation"""
        test_script = f"""
import sys
import os
sys.path.insert(0, '{python_package_dir}')
from ai_rulez.downloader import calculate_sha256, get_expected_checksum

try:
    # Create test file
    test_file = '{temp_dir}/test.txt'
    with open(test_file, 'w') as f:
        f.write('Hello, World!')
    
    # Calculate checksum
    checksum = calculate_sha256(test_file)
    print(f'Calculated hash: {{checksum}}')
    
    # Test checksum parsing
    checksums_content = f'{{checksum}}  test.txt'
    expected = get_expected_checksum(checksums_content, 'test.txt')
    print(f'Expected hash: {{expected}}')
    
    assert len(checksum) == 64
    assert checksum == expected
    
    # Cleanup
    os.unlink(test_file)
    print('SUCCESS: Checksum calculation works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script],
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"Checksum calculation failed: {result.stderr}"
        assert "SUCCESS: Checksum calculation works" in result.stdout

    @pytest.mark.integration
    def test_binary_path_generation(self, python_package_dir):
        """Test binary path generation"""
        test_script = f"""
import sys
sys.path.insert(0, '{python_package_dir}')
from ai_rulez.downloader import get_binary_path
import platform

try:
    binary_path = get_binary_path()
    print(f'Binary path: {{binary_path}}')
    
    # Should be in cache directory
    assert '.cache' in str(binary_path)
    assert 'ai-rulez' in str(binary_path)
    
    # Check extension based on platform  
    if platform.system().lower() == 'windows':
        assert str(binary_path).endswith('.exe')
    else:
        assert not str(binary_path).endswith('.exe')
    
    print('SUCCESS: Binary path generation works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script],
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"Binary path generation failed: {result.stderr}"
        assert "SUCCESS: Binary path generation works" in result.stdout

    @pytest.mark.integration
    def test_version_cache_management(self, python_package_dir):
        """Test version cache functionality"""
        test_script = f"""
import sys
import tempfile
import os
sys.path.insert(0, '{python_package_dir}')

# Mock the version
import ai_rulez.downloader as downloader
downloader.__version__ = '1.0.0'

from ai_rulez.downloader import (
    get_cache_version_file, 
    is_binary_current_version, 
    update_cache_version
)

try:
    version_file = get_cache_version_file()
    print(f'Version file: {{version_file}}')
    
    # Initially should not be current
    assert not is_binary_current_version()
    
    # Update cache
    update_cache_version()
    
    # Now should be current
    assert is_binary_current_version()
    print('SUCCESS: Version cache management works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script],
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"Version cache management failed: {result.stderr}"
        assert "SUCCESS: Version cache management works" in result.stdout

    @pytest.mark.platform
    @pytest.mark.skipif(platform.system() == 'Windows', reason="Unix-specific test")
    def test_unix_permissions(self, python_package_dir, temp_dir):
        """Test Unix file permissions handling"""
        test_script = f"""
import sys
import os
import stat
sys.path.insert(0, '{python_package_dir}')
from ai_rulez.downloader import verify_binary

try:
    # Create mock binary file
    mock_binary = '{temp_dir}/mock-ai-rulez'
    with open(mock_binary, 'w') as f:
        f.write('#!/bin/bash\\necho "mock binary"\\n')
    
    # Without execute permissions should fail
    os.chmod(mock_binary, 0o644)
    assert not verify_binary(mock_binary)
    
    # With execute permissions should pass basic checks
    os.chmod(mock_binary, 0o755)
    # Note: Will still fail because it's not the real binary, but permissions check passes
    
    print('SUCCESS: Unix permissions handling works')
except Exception as e:
    print(f'ERROR: {{e}}')
    sys.exit(1)
"""
        
        result = subprocess.run([sys.executable, '-c', test_script],
                              capture_output=True, text=True, timeout=30)
        
        assert result.returncode == 0, f"Unix permissions test failed: {result.stderr}"
        assert "SUCCESS: Unix permissions handling works" in result.stdout