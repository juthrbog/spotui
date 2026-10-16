# Static libraries for macOS on Apple Silicon. Go 1.27 needs macOS 12 or
# later, so the libraries target the same version.
set(VCPKG_TARGET_ARCHITECTURE arm64)
set(VCPKG_CRT_LINKAGE dynamic)
set(VCPKG_LIBRARY_LINKAGE static)
set(VCPKG_CMAKE_SYSTEM_NAME Darwin)
set(VCPKG_OSX_ARCHITECTURES arm64)
set(VCPKG_OSX_DEPLOYMENT_TARGET 12.0)
set(VCPKG_BUILD_TYPE release)

# Only the C decoder is used, not the C++ FLAC API.
if(PORT STREQUAL "libflac")
    set(VCPKG_CMAKE_CONFIGURE_OPTIONS -DBUILD_CXXLIBS=OFF)
endif()
