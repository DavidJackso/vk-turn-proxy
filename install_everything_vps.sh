#!/usr/bin/env bash
# ============================================================
# One-shot VPS installer (quick start, safe by default)
#
# Installs and starts:
# - wg-easy (WireGuard server + UI) on a dedicated UDP port
# - vk-turn-proxy (DTLS server) forwarding to WireGuard UDP port
#
# It does NOT remove/stop any existing VPN/proxy services by default.
#
# Usage:
#   sudo bash install_everything_vps.sh
#   sudo WG_ADMIN_PASS='...' bash install_everything_vps.sh
#   sudo VK_TURN_LISTEN='0.0.0.0:56000' WG_PORT=51820 WG_UI_PORT=51821 bash install_everything_vps.sh
#
# After finish:
# - Open wg-easy UI, create profile, download .conf
# - Use that .conf in fyne-client (WireGuard tab) OR generate clients any way you like
# ============================================================

set -euo pipefail

WG_ADMIN_PASS="${WG_ADMIN_PASS:-admin123}"
WG_PORT="${WG_PORT:-51820}"
WG_UI_PORT="${WG_UI_PORT:-51821}"
VK_TURN_LISTEN="${VK_TURN_LISTEN:-0.0.0.0:56000}"
INSTALL_DIR="${INSTALL_DIR:-/opt/vk-turn-stack}"
SRC_DIR="${SRC_DIR:-/opt/vk-turn-src}"
REPO_URL="${REPO_URL:-https://github.com/DavidJackso/vk-turn-proxy.git}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${CYAN}>>> $*${NC}"; }
ok()   { echo -e "${GREEN}✔  $*${NC}"; }
warn() { echo -e "${YELLOW}⚠  $*${NC}"; }
err()  { echo -e "${RED}✘  $*${NC}"; exit 1; }

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    err "Run as root: sudo bash $0"
  fi
}

have_cmd() { command -v "$1" >/dev/null 2>&1; }

detect_public_ip() {
  curl -fsS ifconfig.me 2>/dev/null || curl -fsS api.ipify.org 2>/dev/null || true
}

ensure_docker() {
  if have_cmd docker; then
    ok "Docker already installed."
    return
  fi
  log "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  ok "Docker installed."
}

ensure_compose() {
  if docker compose version >/dev/null 2>&1; then
    ok "Docker Compose plugin available."
    return
  fi
  if have_cmd docker-compose; then
    ok "docker-compose available."
    return
  fi
  log "Installing Docker Compose plugin..."
  apt-get update -y >/dev/null 2>&1 || true
  apt-get install -y docker-compose-plugin >/dev/null 2>&1 || true
  docker compose version >/dev/null 2>&1 || err "Docker Compose not available after install."
  ok "Docker Compose plugin installed."
}

ensure_git() {
  if have_cmd git; then
    ok "git already installed."
    return
  fi
  log "Installing git..."
  apt-get update -y >/dev/null 2>&1 || true
  apt-get install -y git >/dev/null 2>&1 || err "Failed to install git."
  ok "git installed."
}

compose_cmd() {
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
    return
  fi
  if have_cmd docker-compose; then
    echo "docker-compose"
    return
  fi
  err "Neither docker compose nor docker-compose is available."
}

wg_password_hash() {
  local pass="$1"
  # wg-easy v14+ requires bcrypt hash in PASSWORD_HASH
  # Use upstream helper to avoid format mismatches:
  #   docker run ghcr.io/wg-easy/wg-easy wgpw mypass
  # Output is usually: PASSWORD_HASH='$2y$10$...'
  docker run --rm ghcr.io/wg-easy/wg-easy wgpw "${pass}" 2>/dev/null \
    | tr -d '\r\n' \
    | sed -E "s/^PASSWORD_HASH='(.*)'$/\\1/"
}

open_firewall() {
  local turn_listen="$1"
  local wg_port="$2"
  local ui_port="$3"

  local turn_port
  turn_port="$(echo "${turn_listen}" | awk -F: '{print $NF}')"

  log "Opening firewall ports: ${turn_port}/udp, ${wg_port}/udp, ${ui_port}/tcp (best-effort)"
  if have_cmd ufw; then
    ufw allow "${turn_port}/udp" comment 'vk-turn-proxy' >/dev/null 2>&1 || true
    ufw allow "${wg_port}/udp" comment 'wireguard' >/dev/null 2>&1 || true
    ufw allow "${ui_port}/tcp" comment 'wg-easy ui' >/dev/null 2>&1 || true
    ok "ufw rules applied (or already exist)"
    return
  fi
  if have_cmd firewall-cmd; then
    firewall-cmd --permanent --add-port="${turn_port}/udp" >/dev/null 2>&1 || true
    firewall-cmd --permanent --add-port="${wg_port}/udp" >/dev/null 2>&1 || true
    firewall-cmd --permanent --add-port="${ui_port}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
    ok "firewalld rules applied (or already exist)"
    return
  fi
  warn "Firewall tool not detected. Ensure ports are open manually."
}

write_stack_compose() {
  local dir="$1"
  local public_ip="$2"
  local turn_listen="$3"
  local wg_port="$4"
  local ui_port="$5"
  local admin_pass_hash="$6"

  mkdir -p "${dir}/vpn-data"
  # Keep bcrypt hash in an env_file to avoid any interpolation pitfalls.
  # (Compose treats .env specially for variable substitution and still expands $2a$... as variables.)
  cat > "${dir}/wg-easy.env" <<EOF
PASSWORD_HASH=${admin_pass_hash}
EOF
  chmod 600 "${dir}/wg-easy.env"

  cat > "${dir}/docker-compose.yml" <<EOF
services:
  vk-turn-proxy:
    image: vk-turn-proxy-local:latest
    restart: always
    network_mode: host
    command: ["./server", "-listen", "${turn_listen}", "-connect", "127.0.0.1:${wg_port}"]
    cap_add:
      - NET_ADMIN

  wg-easy:
    image: ghcr.io/wg-easy/wg-easy
    container_name: wg-easy
    restart: always
    ports:
      - "${wg_port}:${wg_port}/udp"
      - "${ui_port}:51821/tcp"
    env_file:
      - ./wg-easy.env
    environment:
      - WG_HOST=${public_ip}
      - WG_PORT=${wg_port}
      - WG_DEFAULT_ADDRESS=10.8.0.x
      - WG_DEFAULT_DNS=1.1.1.1
      - WG_ALLOWED_IPS=0.0.0.0/0, ::/0
    volumes:
      - ./vpn-data:/etc/wireguard
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    sysctls:
      - net.ipv4.ip_forward=1
      - net.ipv4.conf.all.src_valid_mark=1
EOF
}

ensure_source() {
  local src_dir="$1"
  local repo_url="$2"
  if [[ -d "${src_dir}/.git" ]]; then
    log "Updating source in ${src_dir}..."
    git -C "${src_dir}" pull --ff-only >/dev/null 2>&1 || warn "git pull failed, using existing source."
    ok "Source ready."
    return
  fi
  log "Cloning source from ${repo_url} to ${src_dir}..."
  rm -rf "${src_dir}"
  git clone --depth=1 "${repo_url}" "${src_dir}" >/dev/null 2>&1 || err "Failed to clone source."
  ok "Source ready."
}

write_server_dockerfile() {
  local dir="$1"
  cat > "${dir}/Dockerfile.server" <<'EOF'
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o server ./server

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
ENTRYPOINT ["./server"]
EOF
}

build_server_image() {
  local src_dir="$1"
  local stack_dir="$2"
  log "Building local image vk-turn-proxy-local:latest..."
  docker build -f "${stack_dir}/Dockerfile.server" -t vk-turn-proxy-local:latest "${src_dir}"
  ok "Local image built."
}

main() {
  need_root
  ensure_docker
  ensure_compose
  ensure_git
  have_cmd curl || err "curl is required"

  local public_ip
  public_ip="$(detect_public_ip)"
  [[ -n "${public_ip}" ]] || err "Failed to detect public IP."
  ok "Public IP: ${public_ip}"

  log "Writing stack into ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}"
  ensure_source "${SRC_DIR}" "${REPO_URL}"
  write_server_dockerfile "${INSTALL_DIR}"
  build_server_image "${SRC_DIR}" "${INSTALL_DIR}"
  local WG_ADMIN_PASS_HASH
  WG_ADMIN_PASS_HASH="$(wg_password_hash "${WG_ADMIN_PASS}")"
  write_stack_compose "${INSTALL_DIR}" "${public_ip}" "${VK_TURN_LISTEN}" "${WG_PORT}" "${WG_UI_PORT}" "${WG_ADMIN_PASS_HASH}"

  log "Starting stack..."
  local COMPOSE
  COMPOSE="$(compose_cmd)"
  (cd "${INSTALL_DIR}" && ${COMPOSE} pull >/dev/null 2>&1 || true)
  (cd "${INSTALL_DIR}" && ${COMPOSE} up -d)

  open_firewall "${VK_TURN_LISTEN}" "${WG_PORT}" "${WG_UI_PORT}"

  echo ""
  echo -e "${GREEN}╔══════════════════════════════════════════════════╗${NC}"
  echo -e "${GREEN}║              Stack setup complete                ║${NC}"
  echo -e "${GREEN}╚══════════════════════════════════════════════════╝${NC}"
  echo ""
  echo -e "  ${CYAN}TURN/DTLS listen${NC} → UDP ${public_ip}:$(echo "${VK_TURN_LISTEN}" | awk -F: '{print $NF}')"
  echo -e "  ${CYAN}WireGuard (WG)${NC}  → UDP ${public_ip}:${WG_PORT}"
  echo -e "  ${CYAN}wg-easy UI${NC}      → http://${public_ip}:${WG_UI_PORT} (pass: ${WG_ADMIN_PASS})"
  echo ""
  echo -e "  ${YELLOW}Next:${NC}"
  echo -e "  1) Open wg-easy UI → create profile → download .conf"
  echo -e "  2) Use fyne-client (WireGuard tab) and paste .conf"
  echo ""
  echo -e "  ${YELLOW}Note:${NC} Existing services/containers were not touched."
  echo ""
}

main "$@"

