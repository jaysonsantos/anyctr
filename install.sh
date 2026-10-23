#!/bin/sh
# Installs anyctr from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/pedrospdc/anyctr/main/install.sh | sh
#
# Environment:
#   VERSION=v1.2.3   install a specific release (default: latest)
#   PREFIX=/dir      install directory (default: /usr/local/bin if writable, else ~/.local/bin)
#   ALIAS=yes|no     create a `docker` command that runs anyctr (default: ask when docker is missing)
set -eu

REPO="pedrospdc/anyctr"
BIN="anyctr"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null || die "curl is required."

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  *) die "unsupported OS: $(uname -s). On Windows, download the .zip from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

VERSION="${VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's|.*/tag/||')
  [ -n "$VERSION" ] && [ "${VERSION#v}" != "$VERSION" ] || die "could not determine the latest release."
fi

if [ -z "${PREFIX:-}" ]; then
  if [ -w /usr/local/bin ]; then PREFIX=/usr/local/bin; else PREFIX="$HOME/.local/bin"; fi
fi

ARCHIVE="${BIN}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

say "Downloading $BIN $VERSION ($OS/$ARCH)..."
curl -fsSL -o "$TMP/$ARCHIVE" "$BASE/$ARCHIVE" || die "download failed: $BASE/$ARCHIVE"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt" || die "download failed: checksums.txt"

# Verify before installing: this script runs as root often enough to care.
if command -v sha256sum >/dev/null; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c - >/dev/null) || die "checksum mismatch for $ARCHIVE"
elif command -v shasum >/dev/null; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - >/dev/null) || die "checksum mismatch for $ARCHIVE"
else
  say "Warning: no sha256sum or shasum found; skipping checksum verification."
fi

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BIN"
mkdir -p "$PREFIX"
install -m 0755 "$TMP/$BIN" "$PREFIX/$BIN"
say "Installed $PREFIX/$BIN"

case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) say "Note: $PREFIX is not on your PATH. Add it, e.g.: export PATH=\"$PREFIX:\$PATH\"" ;;
esac

# anyctr is a gateway: without a runtime it has nothing to drive, so say so now
# rather than at the first command.
if ! command -v docker >/dev/null 2>&1 && \
   ! command -v podman >/dev/null 2>&1 && \
   ! command -v container >/dev/null 2>&1; then
  say ""
  say "No container runtime found on PATH. anyctr drives one of:"
  say "  docker     https://docs.docker.com/get-started/get-docker/"
  say "  podman     https://podman.io/docs/installation"
  if [ "$OS" = "darwin" ]; then
    say "  container  brew install container   (Apple silicon, macOS 26)"
  fi
else
  say ""
  "$PREFIX/$BIN" runtime list || true

  # Apple's container has no Compose of its own; anyctr routes to this provider.
  if command -v container >/dev/null 2>&1 && ! command -v container-compose >/dev/null 2>&1; then
    say ""
    say "For \`anyctr compose\` on Apple container, install the provider:"
    say "  brew install pedrospdc/tap/container-compose"
  fi
fi

# anyctr speaks Docker's convention, so a machine without Docker can still have a
# `docker` command. Offered, never assumed: it is skipped outright when a real
# Docker is installed.
case "${ALIAS:-ask}" in
  yes) PATH="$PREFIX:$PATH" "$PREFIX/$BIN" alias --yes ;;
  no) ;;
  *) PATH="$PREFIX:$PATH" "$PREFIX/$BIN" alias || true ;;
esac
