import os
import platform
import sys
import tempfile
import tarfile
import zipfile
import subprocess
import hashlib
from pathlib import Path
from urllib.request import urlopen, Request
from urllib.error import HTTPError


def get_platform():
    system = platform.system().lower()
    machine = platform.machine().lower()

    platform_map = {"darwin": "darwin", "linux": "linux", "windows": "windows"}

    arch_map = {
        "x86_64": "amd64",
        "amd64": "amd64",
        "arm64": "arm64",
        "aarch64": "arm64",
        "i386": "386",
        "i686": "386",
    }

    mapped_platform = platform_map.get(system)
    mapped_arch = arch_map.get(machine)

    if not mapped_platform or not mapped_arch:
        raise RuntimeError(f"Unsupported platform: {system} {machine}")

    if mapped_platform == "windows" and mapped_arch == "arm64":
        raise RuntimeError("Windows ARM64 is not supported")

    return mapped_platform, mapped_arch


def get_binary_url(version):
    platform_name, arch = get_platform()
    archive_format = "zip" if platform_name == "windows" else "tar.gz"
    archive_name = f"ai-rulez_{version}_{platform_name}_{arch}.{archive_format}"
    return f"https://github.com/Goldziher/ai-rulez/releases/download/v{version}/{archive_name}"


def get_checksums_url(version):
    return f"https://github.com/Goldziher/ai-rulez/releases/download/v{version}/checksums.txt"


def calculate_sha256(file_path):
    hash_sha256 = hashlib.sha256()
    try:
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                hash_sha256.update(chunk)
        return hash_sha256.hexdigest()
    except IOError as e:
        raise RuntimeError(f"Failed to calculate checksum: {e}")


def get_expected_checksum(checksums_content, archive_name):
    lines = checksums_content.strip().split("\n")
    for line in lines:
        parts = line.strip().split()
        if len(parts) >= 2 and parts[1] == archive_name:
            return parts[0]
    return None


def download_file_with_retries(url, dest_path, description="file"):
    import time

    max_retries = 3
    retry_delay = 2

    for attempt in range(max_retries):
        try:
            if attempt > 0:
                print(
                    f"Retry {description} download attempt {attempt + 1}/{max_retries} after {retry_delay}s...",
                    file=sys.stderr,
                )
                time.sleep(retry_delay)

            request = Request(
                url,
                headers={
                    "User-Agent": "ai-rulez-python-wrapper/1.0",
                    "Accept": "application/octet-stream, text/plain, */*",
                },
            )

            with urlopen(request, timeout=60) as response:
                if response.status != 200:
                    raise HTTPError(
                        url,
                        response.status,
                        f"HTTP {response.status}",
                        response.headers,
                        None,
                    )

                os.makedirs(os.path.dirname(dest_path), exist_ok=True)

                with open(dest_path, "wb") as f:
                    while True:
                        chunk = response.read(8192)
                        if not chunk:
                            break
                        f.write(chunk)

            if os.path.getsize(dest_path) == 0:
                raise RuntimeError(f"Downloaded {description} is empty")

            return

        except Exception as e:
            if os.path.exists(dest_path):
                try:
                    os.unlink(dest_path)
                except OSError:
                    pass

            if attempt == max_retries - 1:
                raise RuntimeError(
                    f"Failed to download {description} after {max_retries} attempts: {e}"
                )

            retry_delay = min(retry_delay * 2, 30)


def download_and_verify_binary(url, dest_path, version):
    import time

    max_retries = 3
    retry_delay = 2

    for attempt in range(max_retries):
        tmp_file_path = None
        try:
            if attempt > 0:
                print(
                    f"Retry attempt {attempt + 1}/{max_retries} after {retry_delay}s wait...",
                    file=sys.stderr,
                )
                time.sleep(retry_delay)

            with tempfile.NamedTemporaryFile(delete=False) as tmp_file:
                tmp_file_path = tmp_file.name

                request = Request(
                    url,
                    headers={
                        "User-Agent": "ai-rulez-python-wrapper/1.0",
                        "Accept": "application/octet-stream, */*",
                    },
                )

                checksums_url = get_checksums_url(version)
                checksums_content = None

                try:
                    print("Downloading checksums for verification...", file=sys.stderr)
                    with tempfile.NamedTemporaryFile(
                        mode="w+", delete=False
                    ) as checksums_file:
                        checksums_tmp_path = checksums_file.name

                    download_file_with_retries(
                        checksums_url, checksums_tmp_path, "checksums"
                    )
                    with open(checksums_tmp_path, "r") as f:
                        checksums_content = f.read()
                    os.unlink(checksums_tmp_path)
                    print("Checksums downloaded successfully", file=sys.stderr)
                except Exception as e:
                    print(
                        f"Warning: Could not download checksums, skipping verification: {e}",
                        file=sys.stderr,
                    )

                print(f"Downloading binary from {url}...", file=sys.stderr)

                with urlopen(request, timeout=60) as response:
                    if response.status != 200:
                        raise HTTPError(
                            url,
                            response.status,
                            f"HTTP {response.status}",
                            response.headers,
                            None,
                        )

                    content_length = response.headers.get("content-length")
                    if content_length:
                        total_size = int(content_length)
                        print(f"Download size: {total_size} bytes", file=sys.stderr)

                    downloaded = 0
                    while True:
                        chunk = response.read(8192)
                        if not chunk:
                            break
                        tmp_file.write(chunk)
                        downloaded += len(chunk)

                tmp_file.flush()

                actual_size = os.path.getsize(tmp_file_path)
                if actual_size == 0:
                    raise RuntimeError("Downloaded file is empty")

                print(f"Successfully downloaded {actual_size} bytes", file=sys.stderr)

                platform_name, _ = get_platform()
                binary_name = (
                    "ai-rulez.exe" if platform_name == "windows" else "ai-rulez"
                )

                os.makedirs(os.path.dirname(dest_path), exist_ok=True)

                if url.endswith(".zip"):
                    with zipfile.ZipFile(tmp_file_path, "r") as zip_ref:
                        binary_found = False
                        for member in zip_ref.namelist():
                            if member.endswith(binary_name) or member.endswith(
                                binary_name.replace(".exe", "")
                            ):
                                with zip_ref.open(member) as binary_file:
                                    with open(dest_path, "wb") as f:
                                        f.write(binary_file.read())
                                binary_found = True
                                break

                        if not binary_found:
                            raise RuntimeError(
                                f"Binary '{binary_name}' not found in archive. Available files: {zip_ref.namelist()}"
                            )
                else:
                    with tarfile.open(tmp_file_path, "r:gz") as tar:
                        binary_found = False
                        for member in tar.getmembers():
                            if member.name.endswith(
                                binary_name
                            ) or member.name.endswith(binary_name.replace(".exe", "")):
                                with tar.extractfile(member) as binary_file:
                                    if binary_file is None:
                                        continue
                                    with open(dest_path, "wb") as f:
                                        f.write(binary_file.read())
                                binary_found = True
                                break

                        if not binary_found:
                            member_names = [m.name for m in tar.getmembers()]
                            raise RuntimeError(
                                f"Binary '{binary_name}' not found in archive. Available files: {member_names}"
                            )

                if not os.path.exists(dest_path):
                    raise RuntimeError(
                        f"Binary extraction failed: {dest_path} not created"
                    )

                if os.path.getsize(dest_path) == 0:
                    raise RuntimeError("Extracted binary is empty")

                if checksums_content:
                    platform_name, arch = get_platform()
                    archive_format = "zip" if platform_name == "windows" else "tar.gz"
                    archive_name = (
                        f"ai-rulez_{version}_{platform_name}_{arch}.{archive_format}"
                    )

                    expected_hash = get_expected_checksum(
                        checksums_content, archive_name
                    )
                    if expected_hash:
                        print("Verifying archive checksum...", file=sys.stderr)
                        actual_hash = calculate_sha256(tmp_file_path)
                        if actual_hash != expected_hash:
                            raise RuntimeError(
                                f"Checksum verification failed. Expected: {expected_hash}, Got: {actual_hash}"
                            )
                        print("✓ Checksum verified", file=sys.stderr)
                    else:
                        print(
                            "Warning: Could not find checksum for archive in checksums file",
                            file=sys.stderr,
                        )

                if platform_name != "windows":
                    os.chmod(dest_path, 0o755)

                print(f"Binary extracted to {dest_path}", file=sys.stderr)
                return

        except Exception as e:
            error_msg = f"Attempt {attempt + 1} failed: {e}"
            print(error_msg, file=sys.stderr)

            if tmp_file_path and os.path.exists(tmp_file_path):
                try:
                    os.unlink(tmp_file_path)
                except OSError:
                    pass

            if os.path.exists(dest_path):
                try:
                    os.unlink(dest_path)
                except OSError:
                    pass

            if attempt == max_retries - 1:
                raise RuntimeError(
                    f"Failed to download binary after {max_retries} attempts: {e}"
                )

            retry_delay = min(retry_delay * 2, 30)


def get_binary_path():
    cache_dir = Path.home() / ".cache" / "ai-rulez"
    cache_dir.mkdir(parents=True, exist_ok=True)

    platform_name, _ = get_platform()
    ext = ".exe" if platform_name == "windows" else ""
    return cache_dir / f"ai-rulez{ext}"


def verify_binary(binary_path):
    if not os.path.exists(binary_path):
        return False

    if os.path.getsize(binary_path) == 0:
        return False

    if not os.access(binary_path, os.X_OK):
        return False

    try:
        subprocess.run(
            [str(binary_path), "--version"], capture_output=True, timeout=10, text=True
        )
        return True
    except (subprocess.TimeoutExpired, subprocess.SubprocessError, OSError):
        return False


def get_cache_version_file():
    cache_dir = Path.home() / ".cache" / "ai-rulez"
    return cache_dir / "version.txt"


def is_binary_current_version():
    from . import __version__

    version_file = get_cache_version_file()
    if not version_file.exists():
        return False

    try:
        cached_version = version_file.read_text().strip()
        return cached_version == __version__
    except (OSError, IOError):
        return False


def update_cache_version():
    from . import __version__

    version_file = get_cache_version_file()
    version_file.parent.mkdir(parents=True, exist_ok=True)

    try:
        version_file.write_text(__version__)
    except (OSError, IOError):
        pass


def ensure_binary():
    from . import __version__

    binary_path = get_binary_path()

    if (
        binary_path.exists()
        and is_binary_current_version()
        and verify_binary(binary_path)
    ):
        return str(binary_path)

    if binary_path.exists():
        try:
            binary_path.unlink()
        except OSError:
            pass

    print(f"Downloading ai-rulez binary v{__version__}...", file=sys.stderr)
    url = get_binary_url(__version__)

    try:
        download_and_verify_binary(url, str(binary_path), __version__)

        if not verify_binary(binary_path):
            raise RuntimeError("Downloaded binary failed verification")

        update_cache_version()

        print("Binary downloaded and verified successfully!", file=sys.stderr)
        return str(binary_path)

    except Exception as e:
        print(f"Failed to setup ai-rulez binary: {e}", file=sys.stderr)
        print("You can manually download the binary from:", file=sys.stderr)
        print(
            f"https://github.com/Goldziher/ai-rulez/releases/tag/v{__version__}",
            file=sys.stderr,
        )
        sys.exit(1)
