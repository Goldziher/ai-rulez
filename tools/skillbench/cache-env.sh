# shellcheck shell=bash
# Sourced by bench.sh and iterate.sh: what the verdict cache key says about the environment. Anything
# that can change a tool's answer belongs here or in the key's tool, version and adapter digest.
cache_dir="${CACHE_DIR:-$here/../../bench/skills/cache}"
case "$(uname -m)" in
arm64 | aarch64) cache_arch=arm64 ;;
x86_64 | amd64) cache_arch=amd64 ;;
*) cache_arch="$(uname -m)" ;;
esac
cache_env="$cache_arch-${NETWORK:-none}-$(cat "$here/Dockerfile" "$here/scanners/uv.lock" | shasum -a 256 | cut -c1-12)"
protocol_release() {
  awk -v t="[scanners.$1]" '$0==t{f=1;next} f&&/^release/{gsub(/"/,"",$3);print $3;exit}' "$here/protocol.toml"
}
