#!/bin/bash
set -e

REPO="mamounhisham1/vibecode"
BINARY="vibecode"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${GREEN}[info]${RESET} $*"; }
error() { echo -e "${RED}[error]${RESET} $*" >&2; exit 1; }

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      error "Unsupported OS: $OS" ;;
esac

# Detect arch
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             error "Unsupported architecture: $ARCH" ;;
esac

info "Detected: $OS/$ARCH"

# Get latest version
VERSION=$(curl -sfL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
  error "Could not determine latest version. Check your internet connection."
fi
info "Latest version: $VERSION"

# Download
ARCHIVE="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading $URL..."
curl -sfL "$URL" -o "$TMPDIR/$ARCHIVE" || error "Download failed"

tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

# Install
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

mv "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
chmod +x "$INSTALL_DIR/$BINARY"

info "Installed ${BOLD}$BINARY${RESET} to ${BOLD}$INSTALL_DIR/$BINARY${RESET}"

# Check PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo -e "\n${YELLOW}warning${RESET} $INSTALL_DIR is not in your PATH. Add it with:" ;;
esac

if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
  echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
fi

echo -e "\n${GREEN}${BOLD}Done!${RESET} Run ${BOLD}vibecode${RESET} to get started."
