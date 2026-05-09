#!/usr/bin/env bash
# Ding installer — Linux (Docker or native binary)
# Usage: sudo bash install.sh
# Tested on: Ubuntu 20.04+, Debian 11+, Raspberry Pi OS 64-bit
set -euo pipefail

# ── Colours ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()    { echo -e "${CYAN}▸ $*${RESET}"; }
success() { echo -e "${GREEN}✔ $*${RESET}"; }
warn()    { echo -e "${YELLOW}⚠ $*${RESET}"; }
die()     { echo -e "${RED}✖ $*${RESET}" >&2; exit 1; }
header()  { echo -e "\n${BOLD}$*${RESET}"; }
step()    { echo -e "${BOLD}  $*${RESET}"; }

# ── Config ─────────────────────────────────────────────────────────────────────
INSTALL_DIR="${INSTALL_DIR:-/opt/ding}"
DOCKER_IMAGE="${DOCKER_IMAGE:-hamed0406/ding:latest}"
REPO_RAW="${REPO_RAW:-https://raw.githubusercontent.com/hamed0406/ding/main}"
GITHUB_RELEASES="${GITHUB_RELEASES:-https://github.com/hamed0406/ding/releases/latest/download}"
SERVICE_USER="${SERVICE_USER:-ding}"

# ── Banner ─────────────────────────────────────────────────────────────────────
echo -e "${BOLD}"
echo "  ██████╗ ██╗███╗   ██╗ ██████╗ "
echo "  ██╔══██╗██║████╗  ██║██╔════╝ "
echo "  ██║  ██║██║██╔██╗ ██║██║  ███╗"
echo "  ██║  ██║██║██║╚██╗██║██║   ██║"
echo "  ██████╔╝██║██║ ╚████║╚██████╔╝"
echo "  ╚═════╝ ╚═╝╚═╝  ╚═══╝ ╚═════╝ "
echo -e "${RESET}"
echo "  Network Scanner — Linux Installer"
echo ""

# ── Pre-flight ─────────────────────────────────────────────────────────────────
[ "$(uname -s)" = "Linux" ] || die "This installer is Linux-only. For Windows use scripts/install.ps1."
[ "$(id -u)" -eq 0 ] || die "Please run as root: sudo bash install.sh"

# ── Detect architecture ────────────────────────────────────────────────────────
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64)
    ARCH_SUFFIX="linux-amd64"
    ;;
  aarch64|arm64)
    ARCH_SUFFIX="linux-arm64"
    ;;
  *)
    die "Unsupported architecture: $ARCH. Current releases support linux-amd64 and linux-arm64."
    ;;
esac

# ── curl / wget helpers ────────────────────────────────────────────────────────
if command -v curl &>/dev/null; then
  FETCH_FILE() { curl -fsSL "$1" -o "$2"; }
  FETCH_OUT()  { curl -fsSL "$1"; }
  HEALTH_CHECK() { curl -sf "$1" &>/dev/null; }
elif command -v wget &>/dev/null; then
  FETCH_FILE() { wget -q "$1" -O "$2"; }
  FETCH_OUT()  { wget -qO- "$1"; }
  HEALTH_CHECK() { wget -q --spider "$1" &>/dev/null; }
else
  die "curl or wget is required. Install one first: apt-get install curl"
fi

# ── Installation mode ─────────────────────────────────────────────────────────
header "Installation Method"
echo "  How would you like to install Ding?"
echo ""
step "1) Docker        — recommended; isolated, easy to update"
step "2) Native binary — runs as a systemd service; no Docker required"
echo ""
read -rp "  Choose [1/2] (default: 1): " MODE_CHOICE
MODE_CHOICE="${MODE_CHOICE:-1}"

case "$MODE_CHOICE" in
  1) INSTALL_MODE="docker" ;;
  2) INSTALL_MODE="native" ;;
  *) die "Invalid choice. Run the script again and enter 1 or 2." ;;
esac

# ══════════════════════════════════════════════════════════════════════════════
# Docker installation
# ══════════════════════════════════════════════════════════════════════════════
if [ "$INSTALL_MODE" = "docker" ]; then

  header "Checking prerequisites Docker"

  if command -v docker &>/dev/null; then
    DOCKER_VER=$(docker --version 2>/dev/null | grep -oP '[0-9.]+' | head -1 || true)
    success "Docker ${DOCKER_VER:-found}"
  else
    echo ""
    warn "Docker not found."
    echo "  Install it automatically? requires internet access"
    read -rp "  Install Docker now? [y/N]: " INSTALL_DOCKER

    if [[ "${INSTALL_DOCKER,,}" == "y" ]]; then
      info "Installing Docker via get.docker.com..."
      FETCH_OUT "https://get.docker.com" | bash
      systemctl enable --now docker
      success "Docker installed"
    else
      die "Docker is required. Install it first: https://docs.docker.com/engine/install/"
    fi
  fi

  docker info &>/dev/null || die "Docker daemon is not running. Start it with: systemctl start docker"

  if docker compose version &>/dev/null 2>&1; then
    COMPOSE="docker compose"
    success "Docker Compose v2 plugin"
  elif command -v docker-compose &>/dev/null; then
    COMPOSE="docker-compose"
    success "Docker Compose v1 standalone"
  else
    warn "Docker Compose not found."
    echo "  Attempting to install Docker Compose v2 plugin..."
    apt-get update
    apt-get install -y docker-compose-plugin 2>/dev/null \
      || die "Install failed. Install Compose manually: https://docs.docker.com/compose/install/"
    COMPOSE="docker compose"
    success "Docker Compose installed"
  fi

  header "Setting up $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR/data"
  cd "$INSTALL_DIR"

  if [ ! -f docker-compose.yml ]; then
    info "Downloading docker-compose.yml..."
    FETCH_FILE "$REPO_RAW/docker-compose.yml" docker-compose.yml
    success "docker-compose.yml downloaded"
  else
    info "docker-compose.yml already exists — keeping yours"
  fi

  header "Configuration"

  generate_env() {
    info "Downloading .env.example..."
    FETCH_FILE "$REPO_RAW/.env.example" .env.example

    if command -v openssl &>/dev/null; then
      SECRET_KEY=$(openssl rand -base64 32)
    else
      SECRET_KEY=$(
        python3 -c "import base64,os; print(base64.b64encode(os.urandom(32)).decode())" 2>/dev/null \
        || tr -dc 'A-Za-z0-9+/' </dev/urandom | head -c 44
      )
    fi

    cp .env.example .env

    if grep -q "^DING_SECRET_KEY=" .env; then
      sed -i "s|^DING_SECRET_KEY=.*|DING_SECRET_KEY=${SECRET_KEY}|" .env
    else
      echo "DING_SECRET_KEY=${SECRET_KEY}" >> .env
    fi

    # Docker uses mounted /data in many setups.
    for key in DING_DATA_PATH DING_DB_PATH DING_DATABASE_PATH; do
      if grep -q "^${key}=" .env; then
        sed -i "s|^${key}=.*|${key}=/data/ding.db|" .env
      else
        echo "${key}=/data/ding.db" >> .env
      fi
    done

    success "Secret key generated and saved to .env"
    echo ""
    warn "Back up this key — you need it if you ever move the database:"
    echo -e "  ${BOLD}DING_SECRET_KEY=${SECRET_KEY}${RESET}"
    echo ""
  }

  if [ -f .env ]; then
    info ".env already exists — keeping your settings"
    warn "If this is a re-install, review .env manually"
  else
    generate_env
  fi

  echo ""
  echo "Set up Telegram alerts now? you can do this later in Settings"
  read -rp "Set up Telegram? [y/N]: " SETUP_TG

  if [[ "${SETUP_TG,,}" == "y" ]]; then
    echo ""
    echo "  1. Message @BotFather on Telegram → /newbot"
    echo "  2. Copy your bot token"
    echo "  3. Start a chat with your bot, then open:"
    echo "     https://api.telegram.org/bot<TOKEN>/getUpdates"
    echo "     to find your chat_id"
    echo ""
    read -rp "  Bot token: " TG_TOKEN
    read -rp "  Chat ID:   " TG_CHAT_ID

    if [[ -n "${TG_TOKEN:-}" && -n "${TG_CHAT_ID:-}" ]]; then
      if grep -q "^DING_TELEGRAM_TOKEN=" .env; then
        sed -i "s|^DING_TELEGRAM_TOKEN=.*|DING_TELEGRAM_TOKEN=${TG_TOKEN}|" .env
      else
        echo "DING_TELEGRAM_TOKEN=${TG_TOKEN}" >> .env
      fi

      if grep -q "^DING_TELEGRAM_CHAT_ID=" .env; then
        sed -i "s|^DING_TELEGRAM_CHAT_ID=.*|DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}|" .env
      else
        echo "DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}" >> .env
      fi

      success "Telegram configured"
    else
      warn "Skipped — configure it later in Settings → Telegram"
    fi
  fi

  header "Starting Ding"
  info "Pulling Docker image $DOCKER_IMAGE..."
  docker pull "$DOCKER_IMAGE"

  info "Starting service..."
  $COMPOSE up -d

  info "Waiting for server to start..."
  for _ in $(seq 1 20); do
    HEALTH_CHECK "http://localhost:8081/api/auth/providers" && break
    sleep 1
  done

  LAN_IP=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+' || echo "localhost")

  echo ""
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${GREEN}${BOLD}  Ding is running!${RESET}"
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo ""
  echo -e "  Open in your browser: ${BOLD}http://${LAN_IP}:8081${RESET}"
  echo ""
  echo "  First visit: create your account email + password."
  echo "  Then Ding will scan your network automatically."
  echo ""
  echo -e "${BOLD}Useful commands run from $INSTALL_DIR:${RESET}"
  echo "  View logs:  $COMPOSE logs -f"
  echo "  Stop:       $COMPOSE down"
  echo "  Update:     docker pull $DOCKER_IMAGE && $COMPOSE up -d"
  echo ""
  echo -e "${YELLOW}${BOLD}Keep these safe:${RESET}"
  echo "  $INSTALL_DIR/.env          ← config and secret key"
  echo "  $INSTALL_DIR/data/ding.db  ← scan history database"
  echo ""

  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# Native binary installation
# ══════════════════════════════════════════════════════════════════════════════

header "Checking prerequisites native binary"

MISSING_PKGS=()
for pkg in tar systemctl sha256sum; do
  command -v "$pkg" &>/dev/null || MISSING_PKGS+=("$pkg")
done

if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
  die "Missing required tools: ${MISSING_PKGS[*]}. Install them with: apt-get install ${MISSING_PKGS[*]}"
fi

success "Required tools present"

if ! command -v setcap &>/dev/null; then
  warn "setcap not found. Installing libcap2-bin is recommended:"
  warn "apt-get install libcap2-bin"
fi

success "Kernel: $(uname -r)"
success "ARP/ICMP will use AF_PACKET on Linux"

systemctl --version &>/dev/null || die "systemd is required for native binary install."
success "systemd present"

# ── Download binaries ──────────────────────────────────────────────────────────
header "Downloading Ding ${ARCH_SUFFIX}"

TARBALL="ding-${ARCH_SUFFIX}.tar.gz"
DOWNLOAD_URL="${GITHUB_RELEASES}/${TARBALL}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}..."
if ! FETCH_FILE "$DOWNLOAD_URL" "$TMP_DIR/$TARBALL"; then
  die "Download failed. Check that a release for ${ARCH_SUFFIX} exists: https://github.com/hamed0406/ding/releases"
fi

# ── Verify checksum from sha256sums.txt ────────────────────────────────────────
info "Verifying checksum..."
CHECKSUM_URL="${GITHUB_RELEASES}/sha256sums.txt"

if FETCH_FILE "$CHECKSUM_URL" "$TMP_DIR/sha256sums.txt" 2>/dev/null; then
  EXPECTED=$(grep "ding-${ARCH_SUFFIX}.tar.gz" "$TMP_DIR/sha256sums.txt" | awk '{print $1}' || true)
  ACTUAL=$(sha256sum "$TMP_DIR/$TARBALL" | awk '{print $1}')

  if [ -z "$EXPECTED" ]; then
    warn "No checksum entry found for ${TARBALL} — skipping checksum verification"
  elif [ "$EXPECTED" != "$ACTUAL" ]; then
    die "Checksum mismatch! Expected: $EXPECTED Got: $ACTUAL"
  else
    success "Checksum OK"
  fi
else
  warn "sha256sums.txt not found — skipping checksum verification"
fi

info "Extracting..."
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

for bin in ding scanner; do
  if [ ! -f "$TMP_DIR/$bin" ]; then
    die "Expected '$bin' binary in archive — archive layout may have changed."
  fi
done

success "Binaries extracted"

# ── Install binaries ───────────────────────────────────────────────────────────
header "Installing binaries"

install -o root -g root -m 755 "$TMP_DIR/ding" /usr/local/bin/ding
install -o root -g root -m 755 "$TMP_DIR/scanner" /usr/local/bin/scanner

# Record installed version so update.sh can compare against GitHub releases.
INSTALLED_TAG=$(FETCH_OUT "https://api.github.com/repos/hamed0406/ding/releases/latest" 2>/dev/null \
  | grep '"tag_name"' | head -1 | cut -d'"' -f4 || true)
mkdir -p /usr/local/share/ding
echo "${INSTALLED_TAG:-unknown}" > /usr/local/share/ding/VERSION

if command -v setcap &>/dev/null; then
  setcap 'cap_net_raw,cap_net_admin+eip' /usr/local/bin/scanner
  success "Capabilities set on scanner binary cap_net_raw, cap_net_admin"
else
  warn "setcap not found — scanner may need root permissions."
fi

success "ding    → /usr/local/bin/ding"
success "scanner → /usr/local/bin/scanner"

# ── Create service user ────────────────────────────────────────────────────────
header "Creating service user"

if id "$SERVICE_USER" &>/dev/null; then
  info "User '$SERVICE_USER' already exists"
else
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
  success "User '$SERVICE_USER' created"
fi

# ── Create data directory ──────────────────────────────────────────────────────
header "Setting up data directory"

mkdir -p "$INSTALL_DIR/data"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
chmod 750 "$INSTALL_DIR"
chmod 750 "$INSTALL_DIR/data"

success "$INSTALL_DIR/data — owned by $SERVICE_USER"

# ── Generate .env ──────────────────────────────────────────────────────────────
header "Configuration"

cd "$INSTALL_DIR"

if [ -f .env ]; then
  info ".env already exists — keeping your settings"
  warn "If this is a re-install, review .env manually"

  # Make sure these important native values exist even on old .env files.
  for key in DING_DATA_PATH DING_DB_PATH DING_DATABASE_PATH; do
    if grep -q "^${key}=" .env; then
      sed -i "s|^${key}=.*|${key}=${INSTALL_DIR}/data/ding.db|" .env
    else
      echo "${key}=${INSTALL_DIR}/data/ding.db" >> .env
    fi
  done

  if grep -q "^DING_SCANNER_BIN=" .env; then
    sed -i "s|^DING_SCANNER_BIN=.*|DING_SCANNER_BIN=/usr/local/bin/scanner|" .env
  else
    echo "DING_SCANNER_BIN=/usr/local/bin/scanner" >> .env
  fi

  chmod 640 .env
  chown "root:$SERVICE_USER" .env
else
  info "Downloading .env.example..."
  FETCH_FILE "$REPO_RAW/.env.example" .env.example

  if command -v openssl &>/dev/null; then
    SECRET_KEY=$(openssl rand -base64 32)
  else
    SECRET_KEY=$(
      python3 -c "import base64,os; print(base64.b64encode(os.urandom(32)).decode())" 2>/dev/null \
      || tr -dc 'A-Za-z0-9+/' </dev/urandom | head -c 44
    )
  fi

  cp .env.example .env

  if grep -q "^DING_SECRET_KEY=" .env; then
    sed -i "s|^DING_SECRET_KEY=.*|DING_SECRET_KEY=${SECRET_KEY}|" .env
  else
    echo "DING_SECRET_KEY=${SECRET_KEY}" >> .env
  fi

  # Set all common DB env names to avoid SQLite unable-to-open errors.
  for key in DING_DATA_PATH DING_DB_PATH DING_DATABASE_PATH; do
    if grep -q "^${key}=" .env; then
      sed -i "s|^${key}=.*|${key}=${INSTALL_DIR}/data/ding.db|" .env
    else
      echo "${key}=${INSTALL_DIR}/data/ding.db" >> .env
    fi
  done

  if grep -q "^DING_SCANNER_BIN=" .env; then
    sed -i "s|^DING_SCANNER_BIN=.*|DING_SCANNER_BIN=/usr/local/bin/scanner|" .env
  else
    echo "DING_SCANNER_BIN=/usr/local/bin/scanner" >> .env
  fi

  chmod 640 .env
  chown "root:$SERVICE_USER" .env

  success "Secret key generated and saved to .env"
  echo ""
  warn "Back up this key — you need it if you ever move the database:"
  echo -e "  ${BOLD}DING_SECRET_KEY=${SECRET_KEY}${RESET}"
  echo ""
fi

# ── Optional Telegram ─────────────────────────────────────────────────────────
echo "Set up Telegram alerts now? you can do this later in Settings"
read -rp "Set up Telegram? [y/N]: " SETUP_TG

if [[ "${SETUP_TG,,}" == "y" ]]; then
  echo ""
  echo "  1. Message @BotFather on Telegram → /newbot"
  echo "  2. Copy your bot token"
  echo "  3. Start a chat with your bot, then open:"
  echo "     https://api.telegram.org/bot<TOKEN>/getUpdates"
  echo "     to find your chat_id"
  echo ""
  read -rp "  Bot token: " TG_TOKEN
  read -rp "  Chat ID:   " TG_CHAT_ID

  if [[ -n "${TG_TOKEN:-}" && -n "${TG_CHAT_ID:-}" ]]; then
    if grep -q "^DING_TELEGRAM_TOKEN=" .env; then
      sed -i "s|^DING_TELEGRAM_TOKEN=.*|DING_TELEGRAM_TOKEN=${TG_TOKEN}|" .env
    else
      echo "DING_TELEGRAM_TOKEN=${TG_TOKEN}" >> .env
    fi

    if grep -q "^DING_TELEGRAM_CHAT_ID=" .env; then
      sed -i "s|^DING_TELEGRAM_CHAT_ID=.*|DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}|" .env
    else
      echo "DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}" >> .env
    fi

    chmod 640 .env
    chown "root:$SERVICE_USER" .env

    success "Telegram configured"
  else
    warn "Skipped — configure it later in Settings → Telegram"
  fi
fi

# ── Install systemd service ────────────────────────────────────────────────────
header "Installing systemd service"

cat > /etc/systemd/system/ding.service <<EOF
[Unit]
Description=Ding Network Scanner
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/.env
Environment=DING_SCANNER_BIN=/usr/local/bin/scanner
Environment=DING_DATA_PATH=${INSTALL_DIR}/data/ding.db
Environment=DING_DB_PATH=${INSTALL_DIR}/data/ding.db
Environment=DING_DATABASE_PATH=${INSTALL_DIR}/data/ding.db
ExecStart=/usr/local/bin/ding
Restart=on-failure
RestartSec=5s

# Raw packet permissions for scanner.
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN

# Service hardening.
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=${INSTALL_DIR}/data
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now ding

success "systemd service 'ding' enabled and started"

# ── Optional auto-update timer ─────────────────────────────────────────────────
echo ""
echo "Enable automatic daily updates? Ding will check GitHub for new releases"
echo "every night and update itself automatically (restarts for ~2 s)."
read -rp "Enable auto-updates? [y/N]: " SETUP_AUTOUPDATE

if [[ "${SETUP_AUTOUPDATE,,}" == "y" ]]; then
  SCRIPT_PATH="$(realpath "$0" 2>/dev/null || echo "/opt/ding/scripts/install.sh")"
  UPDATE_SCRIPT="$(dirname "$SCRIPT_PATH")/update.sh"

  cat > /etc/systemd/system/ding-update.service <<EOF
[Unit]
Description=Ding auto-update
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash ${UPDATE_SCRIPT}
StandardOutput=journal
StandardError=journal
EOF

  cat > /etc/systemd/system/ding-update.timer <<EOF
[Unit]
Description=Ding daily auto-update check

[Timer]
OnCalendar=*-*-* 03:00:00
RandomizedDelaySec=3600
Persistent=true

[Install]
WantedBy=timers.target
EOF

  systemctl daemon-reload
  systemctl enable --now ding-update.timer
  success "Auto-update timer enabled — Ding will update automatically each night"
else
  info "Skipped. To enable later: sudo bash scripts/update.sh --install-timer"
  info "To update manually:       sudo bash scripts/update.sh"
fi

# ── Open firewall port optional ────────────────────────────────────────────────
if command -v ufw &>/dev/null && ufw status | grep -q "Status: active"; then
  ufw allow 8081/tcp comment "Ding web UI" &>/dev/null || true
  success "ufw rule added for port 8081"
elif command -v firewall-cmd &>/dev/null; then
  firewall-cmd --permanent --add-port=8081/tcp &>/dev/null || true
  firewall-cmd --reload &>/dev/null || true
  success "firewalld rule added for port 8081"
fi

# ── Wait for server to start ───────────────────────────────────────────────────
info "Waiting for Ding to start..."

STARTED="false"
for _ in $(seq 1 20); do
  if HEALTH_CHECK "http://localhost:8081/api/auth/providers"; then
    STARTED="true"
    break
  fi
  sleep 1
done

LAN_IP=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+' || echo "localhost")

echo ""

if [ "$STARTED" = "true" ]; then
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${GREEN}${BOLD}  Ding is running!${RESET}"
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo ""
  echo -e "  Open in your browser: ${BOLD}http://${LAN_IP}:8081${RESET}"
else
  warn "Ding service was installed, but the health check did not pass yet."
  echo ""
  echo "Check logs with:"
  echo "  journalctl -u ding -f"
fi

echo ""
echo "  First visit: create your account email + password."
echo "  Then Ding will scan your network automatically."
echo ""
echo -e "${BOLD}Useful commands:${RESET}"
echo "  View logs:       journalctl -u ding -f"
echo "  Stop:            systemctl stop ding"
echo "  Start:           systemctl start ding"
echo "  Restart:         systemctl restart ding"
echo "  Status:          systemctl status ding"
echo "  Update now:      sudo bash scripts/update.sh"
echo "  Check version:   sudo bash scripts/update.sh --check"
echo ""
echo -e "${YELLOW}${BOLD}Keep these safe:${RESET}"
echo "  $INSTALL_DIR/.env          ← config and secret key"
echo "  $INSTALL_DIR/data/ding.db  ← scan history database"
echo ""
