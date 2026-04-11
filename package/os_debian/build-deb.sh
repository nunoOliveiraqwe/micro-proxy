#!/bin/bash
set -euo pipefail

usage() {
  echo "Usage: $0 --binary <path> --version <version> --arch <arch>"
  echo ""
  echo "Arguments:"
  echo "  --binary   Path to the compiled torii binary"
  echo "  --version  Package version (e.g. 1.0.0)"
  echo "  --arch     Target architecture (amd64, arm64, armhf)"
  exit 1
}

BINARY=""
VERSION=""
ARCH=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)  BINARY="$2";  shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --arch)    ARCH="$2";    shift 2 ;;
    *) echo "Unknown argument: $1"; usage ;;
  esac
done

if [[ -z "$BINARY" || -z "$VERSION" || -z "$ARCH" ]]; then
  echo "Error: --binary, --version, and --arch are all required."
  usage
fi

if [[ ! -f "$BINARY" ]]; then
  echo "Error: binary not found at '$BINARY'"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PKG_NAME="torii_${VERSION}_${ARCH}"
BUILD_DIR="${SCRIPT_DIR}/build/${PKG_NAME}"

echo "==> Building package: ${PKG_NAME}.deb"

# Clean previous build
rm -rf "${BUILD_DIR}"

# -- DEBIAN control files --
mkdir -p "${BUILD_DIR}/DEBIAN"

# Template the control file (replace placeholders)
sed \
  -e "s|{{torii.version}}|${VERSION}|g" \
  -e "s|{{torii.arch}}|${ARCH}|g" \
  "${SCRIPT_DIR}/DEBIAN/control" > "${BUILD_DIR}/DEBIAN/control"

# Copy maintainer scripts
for script in preinst postinst prerm; do
  cp "${SCRIPT_DIR}/DEBIAN/${script}" "${BUILD_DIR}/DEBIAN/${script}"
  chmod 0755 "${BUILD_DIR}/DEBIAN/${script}"
done

# Copy copyright and conffiles
cp "${SCRIPT_DIR}/DEBIAN/copyright" "${BUILD_DIR}/DEBIAN/copyright"
cp "${SCRIPT_DIR}/DEBIAN/conffiles" "${BUILD_DIR}/DEBIAN/conffiles"

# -- Binary --
mkdir -p "${BUILD_DIR}/usr/local/bin"
cp "$BINARY" "${BUILD_DIR}/usr/local/bin/torii"
chmod 0755 "${BUILD_DIR}/usr/local/bin/torii"

# -- Systemd service --
mkdir -p "${BUILD_DIR}/etc/systemd/system"
cp "${SCRIPT_DIR}/torii.service" "${BUILD_DIR}/etc/systemd/system/torii.service"
chmod 0644 "${BUILD_DIR}/etc/systemd/system/torii.service"

# -- Config directory --
mkdir -p "${BUILD_DIR}/etc/torii"
cp "${SCRIPT_DIR}/config.yaml" "${BUILD_DIR}/etc/torii/config.yaml"
cp "${SCRIPT_DIR}/config.yaml.example" "${BUILD_DIR}/etc/torii/config.yaml.example"
chmod 0644 "${BUILD_DIR}/etc/torii/config.yaml"
chmod 0644 "${BUILD_DIR}/etc/torii/config.yaml.example"

# -- Data directory --
mkdir -p "${BUILD_DIR}/var/lib/torii"

# -- Build the .deb --
OUTPUT_DIR="${SCRIPT_DIR}/build"
dpkg-deb --build --root-owner-group "${BUILD_DIR}" "${OUTPUT_DIR}/${PKG_NAME}.deb"

echo "==> Package built: ${OUTPUT_DIR}/${PKG_NAME}.deb"

# Clean up staging directory
rm -rf "${BUILD_DIR}"

