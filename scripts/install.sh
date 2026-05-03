#!/usr/bin/env bash
# Ding installer — Linux (Docker or native binary)
# Usage: sudo bash install.sh
# Tested on: Ubuntu 20.04+, Debian 11+, Raspberry Pi OS (64-bit)
set -euo pipefail

# ── Colours ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}▸ $*${RESET}"; }
success() { echo -e "${GREEN}✔ $*${RESET}"; }
warn()    { echo -e "${YELLOW}⚠ $*${RESET}"; }
die()     { echo -e "${RED}✖ $*${RESET}" >&2; exit 1; }
header()  { echo -e "\n${BOLD}$*${RESET}"; }
step()    { echo -e "${BOLD}  $*${RESET}"; }

# ── Config ─────────────────────────────────────────────────────────────────────
INSTALL_DIR="${INSTALL_DIR:-/opt/ding}"
DOCKER_IMAGE="hamed0406/ding:latest"
REPO_RAW="https://raw.githubusercontent.com/hamed0406/ding/main"
GITHUB_RELEASES="https://github.com/hamed0406/ding/releases/latest/download"
SERVICE_USER="ding"

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

# ── Pre-flight: OS + root ──────────────────────────────────────────────────────
[ "$(uname -s)" = "Linux" ] || die "This installer is Linux-only. For Windows use scripts/install.ps1."
[ "$(id -u)" -eq 0 ]       || die "Please run as root:  sudo bash install.sh"

# ── Detect arch ───────────────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH_SUFFIX="linux-x86_64"  ;;
  aarch64) ARCH_SUFFIX="linux-aarch64" ;;
  armv7l)  ARCH_SUFFIX="linux-armv7"   ;;
  *) warn "Unknown architecture: $ARCH — native binary install may not work"; ARCH_SUFFIX="linux-${ARCH}" ;;
esac

# ── curl / wget ────────────────────────────────────────────────────────────────
if command -v curl &>/dev/null; then
  FETCH_FILE() { curl -fsSL "$1" -o "$2"; }
  FETCH_OUT()  { curl -fsSL "$1"; }
  HEALTH_CHECK() { curl -sf "$1" &>/dev/null; }
elif command -v wget &>/dev/null; then
  FETCH_FILE() { wget -q "$1" -O "$2"; }
  FETCH_OUT()  { wget -qO- "$1"; }
  HEALTH_CHECK() { wget -q --spider "$1" &>/dev/null; }
else
  die "curl or wget is required. Install one first:\n  apt-get install curl"
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
  1) INSTALL_MODE="docker"  ;;
  2) INSTALL_MODE="native"  ;;
  *) die "Invalid choice. Run the script again and enter 1 or 2." ;;
esac

# ══════════════════════════════════════════════════════════════════════════════
# DOCKER INSTALLATION
# ══════════════════════════════════════════════════════════════════════════════
if [ "$INSTALL_MODE" = "docker" ]; then

  header "Checking prerequisites (Docker)"

  # Docker engine
  if command -v docker &>/dev/null; then
    DOCKER_VER=$(docker --version 2>/dev/null | grep -oP '[\d.]+' | head -1)
    success "Docker $DOCKER_VER"
  else
    echo ""
    warn "Docker not found."
    echo "  Install it automatically? (requires internet access)"
    read -rp "  Install Docker now? [y/N]: " INSTALL_DOCKER
    if [[ "${INSTALL_DOCKER,,}" == "y" ]]; then
      info "Installing Docker via get.docker.com…"
      FETCH_OUT "https://get.docker.com" | bash
      systemctl enable --now docker
      success "Docker installed"
    else
      die "Docker is required. Install it first:\n  https://docs.docker.com/engine/install/"
    fi
  fi

  # Docker daemon running?
  docker info &>/dev/null || die "Docker daemon is not running. Start it with:\n  systemctl start docker"

  # Docker Compose (v2 plugin preferred, v1 standalone accepted)
  if docker compose version &>/dev/null 2>&1; then
    COMPOSE="docker compose"
    success "Docker Compose v2 (plugin)"
  elif command -v docker-compose &>/dev/null; then
    COMPOSE="docker-compose"
    success "Docker Compose v1 (standalone)"
  else
    warn "Docker Compose not found."
    echo "  Attempting to install the Compose v2 plugin…"
    apt-get install -y docker-compose-plugin 2>/dev/null \
      || die "Install failed. Install Compose manually:\n  https://docs.docker.com/compose/install/"
    COMPOSE="docker compose"
    success "Docker Compose installed"
  fi

  # ── Create install directory ─────────────────────────────────────────────────
  header "Setting up $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR/data"
  cd "$INSTALL_DIR"

  if [ ! -f docker-compose.yml ]; then
    info "Downloading docker-compose.yml…"
    FETCH_FILE "$REPO_RAW/docker-compose.yml" docker-compose.yml
    success "docker-compose.yml downloaded"
  else
    info "docker-compose.yml already exists — keeping yours"
  fi

  # ── Generate / preserve .env ─────────────────────────────────────────────────
  header "Configuration"
  _generate_env() {
    info "Downloading .env.example…"
    FETCH_FILE "$REPO_RAW/.env.example" .env.example

    if command -v openssl &>/dev/null; then
      SECRET_KEY=$(openssl rand -base64 32)
    else
      SECRET_KEY=$(python3 -c "import base64,os; print(base64.b64encode(os.urandom(32)).decode())" 2>/dev/null \
        || tr -dc 'A-Za-z0-9+/' </dev/urandom | head -c 44)
    fi

    sed "s|^DING_SECRET_KEY=.*|DING_SECRET_KEY=${SECRET_KEY}|" .env.example > .env
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
    _generate_env
  fi

  # ── Optional: Telegram ───────────────────────────────────────────────────────
  _ask_telegram() {
    echo ""
    echo "Set up Telegram alerts now? (you can do this later in Settings)"
    read -rp "Set up Telegram? [y/N]: " SETUP_TG
    if [[ "${SETUP_TG,,}" == "y" ]]; then
      echo ""
      echo "  1. Message @BotFather on Telegram → /newbot"
      echo "  2. Copy your bot token  (e.g. 123456:ABCdef…)"
      echo "  3. Start a chat with your bot, then open:"
      echo "     https://api.telegram.org/bot<TOKEN>/getUpdates"
      echo "     to find your chat_id"
      echo ""
      read -rp "  Bot token: " TG_TOKEN
      read -rp "  Chat ID:   " TG_CHAT_ID
      if [[ -n "${TG_TOKEN:-}" && -n "${TG_CHAT_ID:-}" ]]; then
        sed -i "s|^DING_TELEGRAM_TOKEN=.*|DING_TELEGRAM_TOKEN=${TG_TOKEN}|" .env
        sed -i "s|^DING_TELEGRAM_CHAT_ID=.*|DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}|" .env
        success "Telegram configured"
      else
        warn "Skipped — configure it later in Settings → Telegram"
      fi
    fi
  }
  _ask_telegram

  # ── Pull and start ───────────────────────────────────────────────────────────
  header "Starting Ding"
  info "Pulling Docker image ($DOCKER_IMAGE)…"
  docker pull "$DOCKER_IMAGE"

  info "Starting service…"
  $COMPOSE up -d

  info "Waiting for server to start…"
  for i in $(seq 1 20); do
    HEALTH_CHECK "http://localhost:8081/api/auth/providers" && break
    sleep 1
  done

  LAN_IP=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[\d.]+' || echo "localhost")

  echo ""
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${GREEN}${BOLD}  Ding is running!${RESET}"
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo ""
  echo -e "  Open in your browser: ${BOLD}http://${LAN_IP}:8081${RESET}"
  echo ""
  echo "  First visit: create your account (email + password)."
  echo "  Then Ding will scan your network automatically."
  echo ""
  echo -e "${BOLD}Useful commands (run from $INSTALL_DIR):${RESET}"
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
# NATIVE BINARY INSTALLATION
# ══════════════════════════════════════════════════════════════════════════════

header "Checking prerequisites (native binary)"

# Required tools
MISSING_PKGS=()
for pkg in tar systemctl; do
  command -v "$pkg" &>/dev/null || MISSING_PKGS+=("$pkg")
done
if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
  die "Missing required tools: ${MISSING_PKGS[*]}\nInstall them with: apt-get install ${MISSING_PKGS[*]}"
fi
success "Required tools present"

# CAP_NET_RAW — needed for ARP/ICMP
# The scanner binary will have the capability set, but we need kernel support
if ! grep -q 'cap_net_raw' /proc/1/status 2>/dev/null && [ "$(uname -r | cut -d. -f1)" -lt 4 ]; then
  warn "Kernel version is old ($(uname -r)) — CAP_NET_RAW may not work. Linux 4.0+ is recommended."
fi
success "Kernel: $(uname -r)"

# libpcap — pnet on Linux uses AF_PACKET (no pcap needed), but check anyway
success "ARP/ICMP will use AF_PACKET (no libpcap required on Linux)"

# systemd
systemctl --version &>/dev/null || die "systemd is required for the native binary install (to run Ding as a service)."
success "systemd present"

# ── Download binaries ──────────────────────────────────────────────────────────
header "Downloading Ding ($ARCH_SUFFIX)"

TARBALL="ding-${ARCH_SUFFIX}.tar.gz"
DOWNLOAD_URL="${GITHUB_RELEASES}/${TARBALL}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${TARBALL}…"
if ! FETCH_FILE "$DOWNLOAD_URL" "$TMP_DIR/$TARBALL"; then
  die "Download failed. Check that a release for ${ARCH_SUFFIX} exists:\n  https://github.com/hamed0406/ding/releases"
fi

info "Verifying checksum…"
CHECKSUM_URL="${GITHUB_RELEASES}/ding-${ARCH_SUFFIX}.tar.gz.sha256"
if FETCH_FILE "$CHECKSUM_URL" "$TMP_DIR/expected.sha256" 2>/dev/null; then
  EXPECTED=$(awk '{print $1}' "$TMP_DIR/expected.sha256")
  ACTUAL=$(sha256sum "$TMP_DIR/$TARBALL" | awk '{print $1}')
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    die "Checksum mismatch!\n  Expected: $EXPECTED\n  Got:      $ACTUAL\nDownload may be corrupt — try again."
  fi
  success "Checksum OK"
else
  warn "No .sha256 file found — skipping checksum verification"
fi

info "Extracting…"
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

# Expect: ding (Go controller) and scanner (Rust binary) in the archive
for bin in ding scanner; do
  if [ ! -f "$TMP_DIR/$bin" ]; then
    die "Expected '$bin' binary in archive — archive layout may have changed."
  fi
done
success "Binaries extracted"

# ── Install binaries ───────────────────────────────────────────────────────────
header "Installing binaries"

install -o root -g root -m 755 "$TMP_DIR/ding"    /usr/local/bin/ding
install -o root -g root -m 755 "$TMP_DIR/scanner" /usr/local/bin/scanner

# Grant CAP_NET_RAW + CAP_NET_ADMIN to scanner so it doesn't need sudo at runtime
if command -v setcap &>/dev/null; then
  setcap 'cap_net_raw,cap_net_admin+eip' /usr/local/bin/scanner
  success "Capabilities set on scanner binary (cap_net_raw, cap_net_admin)"
else
  warn "setcap not found — scanner will need to run as root or via sudo."
  warn "Install libcap2-bin:  apt-get install libcap2-bin"
  warn "Then run: setcap 'cap_net_raw,cap_net_admin+eip' /usr/local/bin/scanner"
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
success "$INSTALL_DIR/data — owned by $SERVICE_USER"

# ── Generate .env ──────────────────────────────────────────────────────────────
header "Configuration"
cd "$INSTALL_DIR"

if [ -f .env ]; then
  info ".env already exists — keeping your settings"
  warn "If this is a re-install, review .env manually"
else
  info "Downloading .env.example…"
  FETCH_FILE "$REPO_RAW/.env.example" .env.example

  if command -v openssl &>/dev/null; then
    SECRET_KEY=$(openssl rand -base64 32)
  else
    SECRET_KEY=$(python3 -c "import base64,os; print(base64.b64encode(os.urandom(32)).decode())" 2>/dev/null \
      || tr -dc 'A-Za-z0-9+/' </dev/urandom | head -c 44)
  fi

  sed "s|^DING_SECRET_KEY=.*|DING_SECRET_KEY=${SECRET_KEY}|
       s|^DING_DATA_PATH=.*|DING_DATA_PATH=${INSTALL_DIR}/data/ding.db|" \
    .env.example > .env

  chmod 640 .env
  chown "root:$SERVICE_USER" .env
  success "Secret key generated and saved to .env"
  echo ""
  warn "Back up this key — you need it if you ever move the database:"
  echo -e "  ${BOLD}DING_SECRET_KEY=${SECRET_KEY}${RESET}"
  echo ""
fi

# Optional Telegram
echo "Set up Telegram alerts now? (you can do this later in Settings)"
read -rp "Set up Telegram? [y/N]: " SETUP_TG
if [[ "${SETUP_TG,,}" == "y" ]]; then
  echo ""
  echo "  1. Message @BotFather on Telegram → /newbot"
  echo "  2. Copy your bot token  (e.g. 123456:ABCdef…)"
  echo "  3. Start a chat with your bot, then open:"
  echo "     https://api.telegram.org/bot<TOKEN>/getUpdates"
  echo "  to find your chat_id"
  echo ""
  read -rp "  Bot token: " TG_TOKEN
  read -rp "  Chat ID:   " TG_CHAT_ID
  if [[ -n "${TG_TOKEN:-}" && -n "${TG_CHAT_ID:-}" ]]; then
    sed -i "s|^DING_TELEGRAM_TOKEN=.*|DING_TELEGRAM_TOKEN=${TG_TOKEN}|" .env
    sed -i "s|^DING_TELEGRAM_CHAT_ID=.*|DING_TELEGRAM_CHAT_ID=${TG_CHAT_ID}|" .env
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
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
EnvironmentFile=${INSTALL_DIR}/.env
Environment=DING_SCANNER_BIN=/usr/local/bin/scanner
Environment=DING_DATA_PATH=${INSTALL_DIR}/data/ding.db
ExecStart=/usr/local/bin/ding
Restart=on-failure
RestartSec=5s

# Allow scanner to send raw packets via CAP_NET_RAW
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN

# Harden the service
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

# ── Open firewall port (optional) ─────────────────────────────────────────────
if command -v ufw &>/dev/null && ufw status | grep -q "Status: active"; then
  ufw allow 8081/tcp comment "Ding web UI" &>/dev/null
  success "ufw rule added for port 8081"
elif command -v firewall-cmd &>/dev/null; then
  firewall-cmd --permanent --add-port=8081/tcp &>/dev/null
  firewall-cmd --reload &>/dev/null
  success "firewalld rule added for port 8081"
fi

# ── Wait for server to start ───────────────────────────────────────────────────
info "Waiting for Ding to start…"
for i in $(seq 1 20); do
  HEALTH_CHECK "http://localhost:8081/api/auth/providers" && break
  sleep 1
done

LAN_IP=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[\d.]+' || echo "localhost")

echo ""
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${GREEN}${BOLD}  Ding is running!${RESET}"
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo ""
echo -e "  Open in your browser: ${BOLD}http://${LAN_IP}:8081${RESET}"
echo ""
echo "  First visit: create your account (email + password)."
echo "  Then Ding will scan your network automatically."
echo ""
echo -e "${BOLD}Useful commands:${RESET}"
echo "  View logs:   journalctl -u ding -f"
echo "  Stop:        systemctl stop ding"
echo "  Start:       systemctl start ding"
echo "  Update:      bash <(curl -fsSL https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.sh)"
echo ""
echo -e "${YELLOW}${BOLD}Keep these safe:${RESET}"
echo "  $INSTALL_DIR/.env          ← config and secret key"
echo "  $INSTALL_DIR/data/ding.db  ← scan history database"
echo ""
