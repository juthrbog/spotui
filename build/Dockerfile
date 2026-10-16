# Builds a portable Linux binary of spotui. The audio decoders (Ogg, Vorbis,
# FLAC, mpg123) are linked statically, so the binary needs only glibc 2.28 or
# later and libasound.so.2 at run time.
#
# Adapted from Dockerfile.build in go-librespot (GPL-3.0):
# https://github.com/devgianlu/go-librespot
#
# Use build/build.sh instead of calling docker directly.
FROM --platform=linux/amd64 debian:buster-slim

# Debian 10 (buster) gives an old glibc, so the binary runs on old and new
# distributions. Its packages moved to the Debian archive.
RUN sed -i 's/deb.debian.org/archive.debian.org/' /etc/apt/sources.list && \
    apt-get update && \
    apt-get install -y wget curl git zip gcc g++ make autoconf autopoint pkg-config libtool file

# Keep GO_VERSION the same as the go line in go.mod. The checksum is on
# https://go.dev/dl/.
ARG GO_VERSION=1.27.2
ARG GO_SHA256=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5
RUN wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" && \
    echo "${GO_SHA256}  go${GO_VERSION}.linux-amd64.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz" && \
    rm "go${GO_VERSION}.linux-amd64.tar.gz"
ENV PATH="/usr/local/go/bin:$PATH" GOTOOLCHAIN=local

# C cross compiler for the target, for example aarch64-linux-gnu.
ARG TARGET
RUN apt-get install -y "gcc-$TARGET"

# vcpkg builds the audio libraries for the target. Plain git and no cache
# mounts, so the file also works with the classic Docker builder.
WORKDIR /build
RUN git clone --depth 1 --branch 2025.09.17 https://github.com/microsoft/vcpkg.git vcpkg && \
    cd vcpkg && ./bootstrap-vcpkg.sh
ENV VCPKG_ROOT=/build/vcpkg
ENV PATH="$VCPKG_ROOT:$PATH"

ARG TRIPLET
COPY vcpkg.json vcpkg-configuration.json ./
COPY vcpkg-triplets ./vcpkg-triplets
RUN vcpkg install --triplet "$TRIPLET"

# The source is mounted at /src when the container runs. VERSION, COMMIT,
# and DATE come from docker run.
ARG GOCC
ARG GOARCH
WORKDIR /src
ENV CGO_ENABLED=1 CC="$GOCC" GOARCH="$GOARCH" GOAMD64=v1 \
    PKG_CONFIG_PATH="/build/vcpkg_installed/$TRIPLET/lib/pkgconfig" \
    HOME=/tmp GOCACHE=/src/.gocache/build GOMODCACHE=/src/.gocache/mod

CMD [ "/bin/sh", "-c", "go build -trimpath -buildvcs=false -o \"dist/linux_${GOARCH}/spotui\" -ldflags \"-s -w -X github.com/juthrbog/spotui/internal/version.Version=${VERSION} -X github.com/juthrbog/spotui/internal/version.Commit=${COMMIT} -X github.com/juthrbog/spotui/internal/version.Date=${DATE}\" ." ]
