#!/usr/bin/env bash
# update.sh — update Ding to the latest GitHub release.
#
# Detects whether Ding is running as a native binary (systemd) or Docker,
# and updates accordingly. Safe to run while Ding is running — downtime is
# only the few seconds it takes to restart the service.
#
# Usage:
#   sudo bash scripts/update.sh                  # update to latest
#   sudo bash scripts/update.sh --version v1.2.3 # update to a specific tag
#   sudo bash scripts/update.sh --check          # print versions, exit without updating
#
# For automatic updates, install the systemd timer (native only):
#   sudo bash scripts/update.sh --install-timer

set -euo pipefail

# ── Colours ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
info()    { echo -e "${CYAN}▸ $*${RESET}"; }
success() { echo -e "${GREEN}✔ $*${RESET}"; }
warn()    { echo -e "${YELLOW}⚠ $*${RESET}"; }
die()     { echo -e "${RED}✖ $*${RESET}" >&2; exit 1; }

# ── Config ─────────────────────────────────────────────────────────────────────
REPO="hamed0406/ding"
GITHUB_API="https://api.github.com/repos/${REPO}/releases"
GITHUB_RELEASES="https://github.com/${REPO}/releases/download"
DOCKER_IMAGE="hamed0406/ding"
VERSION_FILE="/usr/local/share/ding/VERSION"
INSTALL_DIR="${INSTALL_DIR:-/opt/ding}"

# ── Arg parsing ────────────────────────────────────────────────────────────────
WANTED_TAG=""
CHECK_ONLY=false
INSTALL_TIMER=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) WANTED_TAG="$2"; shift 2 ;;
    --check)   CHECK_ONLY=true; shift ;;
    --install-timer) INSTALL_TIMER=true; shift ;;
    *) die "Unknown option: $1" ;;
  esac
done

# ── Helpers ────────────────────────────────────────────────────────────────────
if command -v curl &>/dev/null; then
  fetch_out()  { curl -fsSL "$1"; }
  fetch_file() { curl -fsSL "$1" -o "$2"; }
elif command -v wget &>/dev/null; then
  fetch_out()  { wget -qO- "$1"; }
  fetch_file() { wget -q "$1" -O "$2"; }
else
  die "curl or wget required"
fi

github_latest_tag() {
  fetch_out "${GITHUB_API}/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4
}

current_native_version() {
  [ -f "$VERSION_FILE" ] && cat "$VERSION_FILE" || echo "(unknown)"
}

# ── Detect installation mode ───────────────────────────────────────────────────
detect_mode() {
  if systemctl is-active --quiet ding 2>/dev/null && [ -f /usr/local/bin/ding ]; then
    echo "native"
  elif [ -f "${INSTALL_DIR}/docker-compose.yml" ] && command -v docker &>/dev/null; then
    echo "docker"
  else
    die "Cannot detect installation mode. Is Ding installed?"
  fi
}

MODE=$(detect_mode)

# ── Install systemd timer (native only) ───────────────────────────────────────
if $INSTALL_TIMER; then
  [ "$MODE" = "native" ] || die "--install-timer is only for native binary installations"
  [ "$(id -u)" -eq 0 ] || die "Run as root: sudo bash scripts/update.sh --install-timer"

  SCRIPT_PATH="$(realpath "$0")"

  cat > /etc/systemd/system/ding-update.service <<EOF
[Unit]
Description=Ding auto-update
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash ${SCRIPT_PATH}
StandardOutput=journal
StandardError=journal
EOF

  cat > /etc/systemd/system/ding-update.timer <<EOF
[Unit]
Description=Ding daily auto-update check

[Timer]
# Run once a day at a random minute in the 03:xx hour to spread load.
OnCalendar=*-*-* 03:00:00
RandomizedDelaySec=3600
Persistent=true

[Install]
WantedBy=timers.target
EOF

  systemctl daemon-reload
  systemctl enable --now ding-update.timer
  success "Systemd timer installed — Ding will auto-update daily around 3 AM"
  echo ""
  echo "  Check timer status:    systemctl status ding-update.timer"
  echo "  See next trigger:      systemctl list-timers ding-update.timer"
  echo "  Run now (dry-test):    systemctl start ding-update.service"
  echo "  View update logs:      journalctl -u ding-update -n 50"
  echo "  Disable auto-updates:  systemctl disable --now ding-update.timer"
  echo ""
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# Docker update
# ══════════════════════════════════════════════════════════════════════════════
if [ "$MODE" = "docker" ]; then
  cd "$INSTALL_DIR"

  # Determine compose command.
  if docker compose version &>/dev/null 2>&1; then
    COMPOSE="docker compose"
  else
    COMPOSE="docker-compose"
  fi

  TAG="${WANTED_TAG:-latest}"
  CURRENT=$(docker inspect --format '{{index .RepoDigests 0}}' "${DOCKER_IMAGE}:${TAG}" 2>/dev/null || echo "(not pulled)")

  info "Pulling ${DOCKER_IMAGE}:${TAG} ..."
  docker pull "${DOCKER_IMAGE}:${TAG}"

  NEW=$(docker inspect --format '{{index .RepoDigests 0}}' "${DOCKER_IMAGE}:${TAG}" 2>/dev/null || echo "")

  if $CHECK_ONLY; then
    echo "  Current digest: ${CURRENT}"
    echo "  Remote digest:  ${NEW}"
    exit 0
  fi

  if [ "$CURRENT" = "$NEW" ] && [ -n "$NEW" ]; then
    success "Already up to date (${TAG})"
    exit 0
  fi

  info "Restarting ding with new image..."
  $COMPOSE up -d --no-build ding
  success "Updated to ${TAG}"
  echo ""
  docker compose images ding
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# Native binary update
# ══════════════════════════════════════════════════════════════════════════════
[ "$(id -u)" -eq 0 ] || die "Native update requires root: sudo bash scripts/update.sh"

# Resolve target version.
if [ -n "$WANTED_TAG" ]; then
  LATEST_TAG="$WANTED_TAG"
else
  info "Fetching latest release from GitHub..."
  LATEST_TAG=$(github_latest_tag) || die "Could not reach GitHub API. Check your internet connection."
fi

CURRENT_VERSION=$(current_native_version)

echo ""
echo -e "  Installed : ${BOLD}${CURRENT_VERSION}${RESET}"
echo -e "  Available : ${BOLD}${LATEST_TAG}${RESET}"
echo ""

if $CHECK_ONLY; then
  if [ "$CURRENT_VERSION" = "$LATEST_TAG" ]; then
    success "Already up to date"
  else
    warn "Update available: ${CURRENT_VERSION} → ${LATEST_TAG}"
  fi
  exit 0
fi

if [ "$CURRENT_VERSION" = "$LATEST_TAG" ]; then
  success "Already up to date (${LATEST_TAG})"
  exit 0
fi

# Detect architecture.
case "$(uname -m)" in
  x86_64|amd64)   ARCH_SUFFIX="linux-amd64" ;;
  aarch64|arm64)  ARCH_SUFFIX="linux-arm64" ;;
  *) die "Unsupported architecture: $(uname -m)" ;;
esac

TARBALL="ding-${ARCH_SUFFIX}.tar.gz"
DOWNLOAD_URL="${GITHUB_RELEASES}/${LATEST_TAG}/${TARBALL}"
CHECKSUM_URL="${GITHUB_RELEASES}/${LATEST_TAG}/sha256sums.txt"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL} (${LATEST_TAG})..."
fetch_file "$DOWNLOAD_URL" "$TMP_DIR/$TARBALL" \
  || die "Download failed. Does release ${LATEST_TAG} have a ${ARCH_SUFFIX} binary?"

# Verify checksum.
if fetch_file "$CHECKSUM_URL" "$TMP_DIR/sha256sums.txt" 2>/dev/null; then
  EXPECTED=$(grep "${TARBALL}" "$TMP_DIR/sha256sums.txt" | awk '{print $1}')
  ACTUAL=$(sha256sum "$TMP_DIR/$TARBALL" | awk '{print $1}')
  if [ -n "$EXPECTED" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
    die "Checksum mismatch — aborting. Expected: ${EXPECTED}  Got: ${ACTUAL}"
  fi
  success "Checksum verified"
else
  warn "sha256sums.txt not found — skipping checksum verification"
fi

info "Extracting..."
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

for bin in ding scanner; do
  [ -f "$TMP_DIR/$bin" ] || die "Binary '$bin' missing from archive"
done

info "Stopping ding service..."
systemctl stop ding

info "Installing new binaries..."
install -o root -g root -m 755 "$TMP_DIR/ding"    /usr/local/bin/ding
install -o root -g root -m 755 "$TMP_DIR/scanner" /usr/local/bin/scanner

# Re-apply network capabilities to scanner.
if command -v setcap &>/dev/null; then
  setcap 'cap_net_raw,cap_net_admin+eip' /usr/local/bin/scanner
fi

# Record the installed version.
mkdir -p "$(dirname "$VERSION_FILE")"
echo "$LATEST_TAG" > "$VERSION_FILE"

info "Starting ding service..."
systemctl start ding

success "Updated ${CURRENT_VERSION} → ${LATEST_TAG}"
echo ""
echo "  View logs:  journalctl -u ding -f"
echo ""
