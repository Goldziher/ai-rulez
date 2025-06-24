import os
import platform
import shutil
import sys
import tempfile
import urllib.request
from pathlib import Path
from setuptools import setup, find_packages
from setuptools.command.install import install

VERSION = os.environ.get("RELEASE_VERSION", "0.0.0-placeholder")
REPO_NAME = "Goldziher/airules"

def get_platform_info():
    """Get platform and architecture information."""
    system = platform.system().lower()
    machine = platform.machine().lower()
    
    # Map platform names
    platform_map = {
        'darwin': 'darwin',
        'linux': 'linux', 
        'windows': 'windows'
    }
    
    # Map architecture names
    arch_map = {
        'x86_64': 'amd64',
        'amd64': 'amd64',
        'arm64': 'arm64',
        'aarch64': 'arm64',
        'i386': '386',
        'i686': '386'
    }
    
    mapped_platform = platform_map.get(system)
    mapped_arch = arch_map.get(machine)
    
    if not mapped_platform or not mapped_arch:
        raise RuntimeError(f"Unsupported platform: {system} {machine}")
    
    # Windows ARM64 is not supported
    if mapped_platform == 'windows' and mapped_arch == 'arm64':
        raise RuntimeError("Windows ARM64 is not supported")
    
    return mapped_platform, mapped_arch

def get_binary_name(platform_name):
    """Get binary name based on platform."""
    return 'airules.exe' if platform_name == 'windows' else 'airules'

def download_and_extract_binary(platform_name, arch, version):
    """Download and extract the appropriate binary for the platform."""
    binary_name = get_binary_name(platform_name)
    archive_format = 'zip' if platform_name == 'windows' else 'tar.gz'
    archive_name = f"airules_{version}_{platform_name}_{arch}.{archive_format}"
    download_url = f"https://github.com/{REPO_NAME}/releases/download/v{version}/{archive_name}"
    
    print(f"Downloading airules binary for {platform_name}/{arch}...")
    print(f"URL: {download_url}")
    
    with tempfile.TemporaryDirectory() as temp_dir:
        archive_path = Path(temp_dir) / archive_name
        
        # Download archive
        try:
            urllib.request.urlretrieve(download_url, archive_path)
        except Exception as e:
            raise RuntimeError(f"Failed to download binary: {e}")
        
        # Extract archive
        extract_dir = Path(temp_dir) / "extracted"
        extract_dir.mkdir()
        
        if platform_name == 'windows':
            import zipfile
            with zipfile.ZipFile(archive_path, 'r') as zip_ref:
                zip_ref.extractall(extract_dir)
        else:
            import tarfile
            with tarfile.open(archive_path, 'r:gz') as tar_ref:
                tar_ref.extractall(extract_dir)
        
        # Find the binary
        binary_path = extract_dir / binary_name
        if not binary_path.exists():
            raise RuntimeError(f"Binary {binary_name} not found in extracted archive")
        
        return binary_path.read_bytes()

class PostInstallCommand(install):
    """Post-installation for downloading platform-specific binary."""
    
    def run(self):
        install.run(self)
        self.download_binary()
    
    def download_binary(self):
        try:
            platform_name, arch = get_platform_info()
            binary_name = get_binary_name(platform_name)
            
            # Download binary
            binary_data = download_and_extract_binary(platform_name, arch, VERSION)
            
            # Write binary to install location
            install_dir = Path(self.install_scripts)
            install_dir.mkdir(parents=True, exist_ok=True)
            target_path = install_dir / binary_name
            
            target_path.write_bytes(binary_data)
            
            # Make executable on Unix-like systems
            if platform_name != 'windows':
                os.chmod(target_path, 0o755)
            
            print(f"✅ airules v{VERSION} installed successfully for {platform_name}/{arch}!")
            
        except Exception as e:
            print(f"Failed to install airules binary: {e}", file=sys.stderr)
            print("You can manually download the binary from:", file=sys.stderr)
            print(f"https://github.com/{REPO_NAME}/releases/tag/v{VERSION}", file=sys.stderr)
            sys.exit(1)

setup(
    name="airules",
    version=VERSION,
    description="CLI tool for managing AI assistant rules - generate configuration files for Claude, Cursor, Windsurf and more",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    author="Na'aman Hirschfeld",
    author_email="nhirschfeld@gmail.com",
    url=f"https://github.com/{REPO_NAME}",
    project_urls={
        "Homepage": f"https://github.com/{REPO_NAME}",
        "Bug Reports": f"https://github.com/{REPO_NAME}/issues",
        "Source": f"https://github.com/{REPO_NAME}",
    },
    keywords=["ai", "rules", "configuration", "claude", "cursor", "windsurf", "cli", "assistant", "copilot", "generator"],
    packages=find_packages(),
    install_requires=[],
    python_requires=">=3.8",
    cmdclass={
        'install': PostInstallCommand,
    },
    entry_points={
        'console_scripts': [
            'airules=airules:main',
        ],
    },
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Code Generators",
        "Topic :: Software Development :: Libraries :: Python Modules",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Operating System :: OS Independent",
        "Environment :: Console",
    ],
)