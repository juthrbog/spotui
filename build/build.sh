#!/usr/bin/env bash
# Builds a portable Linux binary of spotui in a Docker container.
#
# Usage: build/build.sh amd64|arm64
#
# The binary is written to dist/linux_<arch>/spotui. VERSION, COMMIT, and
# DATE override the version information. By default they come from git.
set -euo pipefail

arch="${1:-}"
case "$arch" in
  amd64) target=x86-64-linux-gnu triplet=x64-linux cc=gcc ;;
  arm64) target=aarch64-linux-gnu triplet=arm64-linux cc=aarch64-linux-gnu-gcc ;;
  *)
    echo "usage: $0 amd64|arm64" >&2
    exit 2
    ;;
esac

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${VERSION:-$(git -C "$root" describe --tags --always --dirty)}"
version="${version#v}"
commit="${COMMIT:-$(git -C "$root" rev-parse --short HEAD)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
image="spotui-build-$arch"

docker build \
  --build-arg "TARGET=$target" \
  --build-arg "TRIPLET=$triplet" \
  --build-arg "GOCC=$cc" \
  --build-arg "GOARCH=$arch" \
  -t "$image" \
  "$root/build"

docker run --rm \
  -u "$(id -u):$(id -g)" \
  -v "$root":/src \
  -e "VERSION=$version" \
  -e "COMMIT=$commit" \
  -e "DATE=$date" \
  "$image"

file "$root/dist/linux_$arch/spotui"
