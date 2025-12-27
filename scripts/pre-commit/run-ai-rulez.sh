#!/usr/bin/env bash

# run-ai-rulez.sh ensures an ai-rulez binary is available without requiring Go,
# downloads a released binary if needed, and then executes it with the provided arguments.

set -euo pipefail

AI_RULEZ_VERSION="${AI_RULEZ_VERSION:-v3.0.0}"
AI_RULEZ_BINARY="${AI_RULEZ_BINARY:-}"
AI_RULEZ_CACHE_DIR="${AI_RULEZ_CACHE_DIR:-$HOME/.cache/ai-rulez/pre-commit}"

if [[ -n "$AI_RULEZ_BINARY" && -x "$AI_RULEZ_BINARY" ]]; then
	BINARY_PATH="$AI_RULEZ_BINARY"
elif command -v ai-rulez >/dev/null 2>&1; then
	BINARY_PATH="$(command -v ai-rulez)"
else
	TAG="${AI_RULEZ_VERSION}"
	VERSION="${TAG#v}"

	uname_s="$(uname -s)"
	case "$uname_s" in
	Linux*) PLATFORM="linux" ;;
	Darwin*) PLATFORM="darwin" ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT) PLATFORM="windows" ;;
	*) echo "Unsupported platform: $uname_s" >&2; exit 1 ;;
	esac

	uname_m="$(uname -m)"
	case "$uname_m" in
	x86_64 | amd64) ARCH="amd64" ;;
	arm64 | aarch64) ARCH="arm64" ;;
	*) echo "Unsupported architecture: $uname_m" >&2; exit 1 ;;
	esac

	ext="tar.gz"
	if [[ "$PLATFORM" == "windows" ]]; then
		ext="zip"
	fi

	archive_name="ai-rulez_${VERSION}_${PLATFORM}_${ARCH}.${ext}"
	download_url="https://github.com/Goldziher/ai-rulez/releases/download/${TAG}/${archive_name}"
	checksum_url="https://github.com/Goldziher/ai-rulez/releases/download/${TAG}/checksums.txt"

	cache_dir="${AI_RULEZ_CACHE_DIR}/${TAG}/${PLATFORM}-${ARCH}"
	mkdir -p "$cache_dir"

	exe_suffix=""
	if [[ "$PLATFORM" == "windows" ]]; then
		exe_suffix=".exe"
	fi

	BINARY_PATH="${cache_dir}/ai-rulez${exe_suffix}"

	if [[ ! -x "$BINARY_PATH" ]]; then
		tmp_dir="$(mktemp -d)"
		cleanup() {
			rm -rf "$tmp_dir"
		}
		trap cleanup EXIT

		archive_path="${tmp_dir}/${archive_name}"
		echo "Downloading ai-rulez ${TAG} for ${PLATFORM}/${ARCH}..."
		curl -fsSL "$download_url" -o "$archive_path"

		if curl -fsSL "$checksum_url" -o "${tmp_dir}/checksums.txt" 2>/dev/null; then
			expected_sum="$(grep " ${archive_name}$" "${tmp_dir}/checksums.txt" | awk '{print $1}')"
			if [[ -n "$expected_sum" ]]; then
				if command -v shasum >/dev/null 2>&1; then
					actual_sum="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
				elif command -v sha256sum >/dev/null 2>&1; then
					actual_sum="$(sha256sum "$archive_path" | awk '{print $1}')"
				else
					echo "Warning: Neither shasum nor sha256sum is available, skipping checksum verification" >&2
					actual_sum="$expected_sum"
				fi
				if [[ "$expected_sum" != "$actual_sum" ]]; then
					echo "Checksum verification failed for ${archive_name}" >&2
					exit 1
				fi
			fi
		fi

		if [[ "$ext" == "zip" ]]; then
			command -v unzip >/dev/null 2>&1 || { echo "unzip is required to extract ai-rulez" >&2; exit 1; }
			unzip -q "$archive_path" -d "$tmp_dir"
		else
			tar -xzf "$archive_path" -C "$tmp_dir"
		fi

		mv "${tmp_dir}/ai-rulez${exe_suffix}" "$BINARY_PATH"
		chmod +x "$BINARY_PATH"
		trap - EXIT
		cleanup
	fi
fi

exec "$BINARY_PATH" "$@"
