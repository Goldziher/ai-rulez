import os
import platform
import subprocess
import sys
import tarfile
import zipfile
from pathlib import Path

import requests
from setuptools import setup, find_packages
from setuptools.command.install import install

VERSION = "1.0.0rc1"
REPO_NAME = "Goldziher/airules"

class PostInstallCommand(install):
    """Post-installation for downloading the appropriate binary."""
    
    def run(self):
        install.run(self)
        self.download_binary()
    
    def get_platform_info(self):
        system = platform.system().lower()
        machine = platform.machine().lower()
        
        os_map = {
            'darwin': 'darwin',
            'linux': 'linux',
            'windows': 'windows'
        }
        
        arch_map = {
            'x86_64': 'amd64',
            'amd64': 'amd64',
            'aarch64': 'arm64',
            'arm64': 'arm64',
            'i386': '386',
            'i686': '386'
        }
        
        return {
            'os': os_map.get(system, system),
            'arch': arch_map.get(machine, machine)
        }
    
    def download_binary(self):
        platform_info = self.get_platform_info()
        binary_name = 'airules.exe' if platform_info['os'] == 'windows' else 'airules'
        
        # Determine where to install the binary
        install_dir = Path(self.install_scripts)
        binary_path = install_dir / binary_name
        
        # Download URL
        ext = 'zip' if platform_info['os'] == 'windows' else 'tar.gz'
        filename = f"airules_{VERSION}_{platform_info['os']}_{platform_info['arch']}.{ext}"
        url = f"https://github.com/{REPO_NAME}/releases/download/v{VERSION}/{filename}"
        
        print(f"Downloading airules for {platform_info['os']}/{platform_info['arch']}...")
        
        try:
            # Download the archive
            response = requests.get(url, stream=True)
            response.raise_for_status()
            
            archive_path = install_dir / f"archive.{ext}"
            with open(archive_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
            
            # Extract the binary
            if ext == 'zip':
                with zipfile.ZipFile(archive_path, 'r') as z:
                    z.extract('airules.exe', install_dir)
            else:
                with tarfile.open(archive_path, 'r:gz') as t:
                    t.extract('airules', install_dir)
            
            # Make executable on Unix-like systems
            if platform_info['os'] != 'windows':
                os.chmod(binary_path, 0o755)
            
            # Clean up
            archive_path.unlink()
            
            print("✅ airules installed successfully!")
            
        except Exception as e:
            print(f"Failed to download airules: {e}", file=sys.stderr)
            sys.exit(1)

setup(
    name="airules",
    version=VERSION,
    description="CLI tool for managing AI assistant rules - generate configuration files for Claude, Cursor, Windsurf and more",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    author="Goldziher",
    author_email="",
    url=f"https://github.com/{REPO_NAME}",
    project_urls={
        "Homepage": f"https://github.com/{REPO_NAME}",
        "Bug Reports": f"https://github.com/{REPO_NAME}/issues",
        "Source": f"https://github.com/{REPO_NAME}",
    },
    keywords=["ai", "rules", "configuration", "claude", "cursor", "windsurf", "cli", "assistant", "copilot", "generator"],
    packages=find_packages(),
    install_requires=["requests"],
    python_requires=">=3.7",
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
        "Programming Language :: Python :: 3.7",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Operating System :: OS Independent",
        "Environment :: Console",
    ],
)