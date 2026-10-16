#!/usr/bin/env bash
# Builds a release binary of spotui.
#
# Usage: build/build.sh TARGET
#
#   linux-amd64, linux-arm64, windows-amd64
#       Built in a Docker container. Needs Docker.
#   darwin-arm64
#       Built on this computer. Needs a Mac with Go and pkg-config.
#
# The binary is written to dist/<os>_<arch>/. VERSION, COMMIT, and DATE
# override the version information. By default they come from git.
set -euo pipefail

usage() {
  echo "usage: $0 linux-amd64|linux-arm64|windows-amd64|darwin-arm64" >&2
  exit 2
}

target="${1:-}"
root="$(cd "$(dirname "$0")/.." && pwd)"
version="${VERSION:-$(git -C "$root" describe --tags --always --dirty)}"
version="${version#v}"
commit="${COMMIT:-$(git -C "$root" rev-parse --short HEAD)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
pkg=github.com/juthrbog/spotui/internal/version
ldflags="-s -w -X $pkg.Version=$version -X $pkg.Commit=$commit -X $pkg.Date=$date"

# in_docker builds an image from a Dockerfile in build/ and runs it. The
# container compiles spotui from the mounted source.
in_docker() {
  local dockerfile="$1" image="$2"
  shift 2
  docker build -f "$root/build/$dockerfile" -t "$image" "$@" "$root/build"
  docker run --rm -u "$(id -u):$(id -g)" -v "$root":/src -e "LDFLAGS=$ldflags" "$image"
}

# on_mac builds spotui for macOS with static audio libraries from vcpkg.
on_mac() {
  local arch="$1" triplet="$1-osx"
  if [[ "$(uname -s)" != Darwin ]]; then
    echo "$target must be built on a Mac" >&2
    exit 2
  fi
  if ! command -v pkg-config >/dev/null; then
    echo "pkg-config is missing. Install it with: brew install pkgconf" >&2
    exit 2
  fi
  # Keep the vcpkg version the same as in the Dockerfiles.
  local vcpkg="${VCPKG_ROOT:-$root/build/.vcpkg}"
  if [[ ! -x "$vcpkg/vcpkg" ]]; then
    git clone --depth 1 --branch 2025.09.17 https://github.com/microsoft/vcpkg.git "$vcpkg"
    "$vcpkg/bootstrap-vcpkg.sh" -disableMetrics
  fi
  (cd "$root/build" && "$vcpkg/vcpkg" install --triplet "$triplet")

  # PKG_CONFIG_LIBDIR keeps libraries from Homebrew out of the binary.
  local pc="$root/build/vcpkg_installed/$triplet/lib/pkgconfig"
  (cd "$root" && CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" MACOSX_DEPLOYMENT_TARGET=12.0 \
    PKG_CONFIG_PATH="$pc" PKG_CONFIG_LIBDIR="$pc" \
    go build -trimpath -o "dist/darwin_$arch/spotui" -ldflags "$ldflags" .)
}

case "$target" in
  linux-amd64)
    in_docker Dockerfile spotui-build-linux-amd64 \
      --build-arg TARGET=x86-64-linux-gnu --build-arg TRIPLET=x64-linux \
      --build-arg GOCC=gcc --build-arg GOARCH=amd64
    ;;
  linux-arm64)
    in_docker Dockerfile spotui-build-linux-arm64 \
      --build-arg TARGET=aarch64-linux-gnu --build-arg TRIPLET=arm64-linux \
      --build-arg GOCC=aarch64-linux-gnu-gcc --build-arg GOARCH=arm64
    ;;
  windows-amd64)
    in_docker Dockerfile.windows spotui-build-windows-amd64
    ;;
  darwin-arm64)
    on_mac arm64
    ;;
  *)
    usage
    ;;
esac

file "$root/dist/${target/-/_}"/spotui*
