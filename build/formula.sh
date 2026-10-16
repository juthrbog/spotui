#!/usr/bin/env bash
# Writes the Homebrew formula for a release to standard output.
#
# Usage: build/formula.sh VERSION CHECKSUMS
#
# VERSION is the version without "v", for example 0.1.0. CHECKSUMS is the
# checksums.txt file of the release.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 VERSION CHECKSUMS" >&2
  exit 2
fi
version="$1"
sums="$2"
base="https://github.com/juthrbog/spotui/releases/download/v${version}"

# sha prints the checksum of the archive for a platform, such as linux_amd64.
sha() {
  local file="spotui_${version}_$1.tar.gz" sum
  sum="$(awk -v f="$file" '$2 == f { print $1 }' "$sums")"
  if [[ ! "$sum" =~ ^[0-9a-f]{64}$ ]]; then
    echo "no checksum for $file in $sums" >&2
    exit 1
  fi
  echo "$sum"
}

darwin_arm64="$(sha darwin_arm64)"
linux_arm64="$(sha linux_arm64)"
linux_amd64="$(sha linux_amd64)"

cat <<FORMULA
# Generated from the release's SHA-256 checksums. Do not edit manually.
class Spotui < Formula
  desc "Terminal Spotify player that plays through your own speakers"
  homepage "https://github.com/juthrbog/spotui"
  version "${version}"
  license "GPL-3.0-only"

  on_macos do
    depends_on arch: :arm64
    url "${base}/spotui_${version}_darwin_arm64.tar.gz"
    sha256 "${darwin_arm64}"
  end

  on_linux do
    on_arm do
      url "${base}/spotui_${version}_linux_arm64.tar.gz"
      sha256 "${linux_arm64}"
    end
    on_intel do
      url "${base}/spotui_${version}_linux_amd64.tar.gz"
      sha256 "${linux_amd64}"
    end
  end

  def install
    bin.install "spotui"
  end

  def caveats
    <<~EOS
      spotui needs Spotify Premium. On the first start, it opens a Spotify
      login page in your browser.

      Search and saved albums need your own Spotify developer app:
        https://github.com/juthrbog/spotui#set-up-search-and-saved-albums
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/spotui -version")
  end
end
FORMULA
