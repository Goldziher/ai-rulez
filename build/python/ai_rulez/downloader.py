import os
import platform
import sys
import tempfile
import tarfile
import zipfile
from pathlib import Path
from urllib.request import urlopen, Request
from urllib.error import URLError


def get_platform():
    """Determine the platform and architecture for binary selection."""
    system = platform.system().lower()
    machine = platform.machine().lower()

    
    platform_map = {
        'darwin': 'darwin',
        'linux': 'linux', 
        'windows': 'windows'
    }
    
    
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
    
    
    if mapped_platform == 'windows' and mapped_arch == 'arm64':
        raise RuntimeError("Windows ARM64 is not supported")
    
    return mapped_platform, mapped_arch


def get_binary_url(version):
    """Get the download URL for the binary."""
    platform_name, arch = get_platform()
    archive_format = 'zip' if platform_name == 'windows' else 'tar.gz'
    archive_name = f"ai-rulez_{version}_{platform_name}_{arch}.{archive_format}"
    return f"https://github.com/Goldziher/ai-rulez/releases/download/v{version}/{archive_name}"


def download_binary(url, dest_path):
    """Download and extract the binary from the given URL."""
    import time
    
    
    max_retries = 3
    retry_delay = 5  # ~keep seconds
    
    for attempt in range(max_retries):
        try:
            if attempt > 0:
                print(f"Retry attempt {attempt + 1}/{max_retries} after {retry_delay}s wait...", file=sys.stderr)
                time.sleep(retry_delay)
            
            with tempfile.NamedTemporaryFile(delete=False) as tmp_file:
                
                request = Request(url, headers={'User-Agent': 'ai-rulez-python-wrapper'})
                with urlopen(request, timeout=30) as response:
                    if response.status != 200:
                        raise RuntimeError(f"HTTP {response.status}")
                    
                    
                    while True:
                        chunk = response.read(8192)
                        if not chunk:
                            break
                        tmp_file.write(chunk)
                
                tmp_file.flush()
                
                
                if os.path.getsize(tmp_file.name) == 0:
                    raise RuntimeError("Downloaded file is empty")
                
                print(f"Successfully downloaded {os.path.getsize(tmp_file.name)} bytes", file=sys.stderr)
                
                
                platform_name, _ = get_platform()
                binary_name = 'ai-rulez.exe' if platform_name == 'windows' else 'ai-rulez'
                
                if url.endswith('.zip'):
                    with zipfile.ZipFile(tmp_file.name, 'r') as zip_ref:
                        
                        for member in zip_ref.namelist():
                            if member.endswith(binary_name):
                                with zip_ref.open(member) as binary_file:
                                    with open(dest_path, 'wb') as f:
                                        f.write(binary_file.read())
                                break
                        else:
                            raise RuntimeError(f"No binary found in archive from {url}")
                else:
                    with tarfile.open(tmp_file.name, 'r:gz') as tar:
                        
                        for member in tar.getmembers():
                            if member.name.endswith(binary_name):
                                with tar.extractfile(member) as binary_file:
                                    with open(dest_path, 'wb') as f:
                                        f.write(binary_file.read())
                                break
                        else:
                            raise RuntimeError(f"No binary found in archive from {url}")
                
                
                os.unlink(tmp_file.name)
                return
                
        except Exception as e:
            error_msg = f"Attempt {attempt + 1} failed: {e}"
            print(error_msg, file=sys.stderr)
            
            if attempt == max_retries - 1:
                raise RuntimeError(f"Failed to download binary after {max_retries} attempts: {e}")
            
            
            retry_delay = min(retry_delay * 2, 30)  # ~keep Cap at 30 seconds


def get_binary_path():
    """Get the path where the binary should be stored."""
    cache_dir = Path.home() / ".cache" / "ai-rulez"
    cache_dir.mkdir(parents=True, exist_ok=True)
    
    platform_name, _ = get_platform()
    ext = ".exe" if platform_name == 'windows' else ""
    return cache_dir / f"ai-rulez{ext}"


def ensure_binary():
    """Ensure the binary is available, downloading if necessary."""
    from . import __version__
    
    binary_path = get_binary_path()
    
    
    if binary_path.exists():
        if os.access(binary_path, os.X_OK):
            return str(binary_path)
    
    
    print(f"Downloading ai-rulez binary v{__version__}...", file=sys.stderr)
    url = get_binary_url(__version__)
    
    try:
        download_binary(url, binary_path)
        os.chmod(binary_path, 0o755)  # ~keep Make executable
        print("Binary downloaded successfully!", file=sys.stderr)
        return str(binary_path)
    except Exception as e:
        print(f"Failed to setup ai-rulez binary: {e}", file=sys.stderr)
        print("You can manually download the binary from:", file=sys.stderr)
        print(f"https://github.com/Goldziher/ai-rulez/releases/tag/v{__version__}", file=sys.stderr)
        sys.exit(1)