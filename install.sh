#!/bin/sh
set -e

# Forge installer — https://sultan1993.github.io/Forge/install.sh
# Detects current system state, skips what's already configured,
# and walks new users through the full setup.

FORGE_VERSION="0.1.0"
FORGE_REPO="Sultan1993/forge"
FORGE_BIN="/usr/local/bin/forge-host"
FORGE_TRAY_BIN="/usr/local/bin/forge-host-tray"
FORGE_PORT=8080

# --- Output helpers ---
BOLD="\033[1m"
GREEN="\033[0;32m"
YELLOW="\033[0;33m"
RED="\033[0;31m"
DIM="\033[2m"
RESET="\033[0m"

info()  { printf "  ${GREEN}✓${RESET} %s\n" "$1"; }
warn()  { printf "  ${YELLOW}!${RESET} %s\n" "$1"; }
fail()  { printf "  ${RED}✗${RESET} %s\n" "$1"; exit 1; }
step()  { printf "\n  %s\n" "$1"; }
dim()   { printf "  ${DIM}%s${RESET}\n" "$1"; }

# --- Detect OS and architecture ---
detect_os() {
  OS="$(uname -s)"
  ARCH="$(uname -m)"

  case "$OS" in
    Darwin) OS="darwin" ;;
    Linux)  OS="linux" ;;
    *)      fail "Unsupported OS: $OS" ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             fail "Unsupported architecture: $ARCH" ;;
  esac
}

detect_os_version() {
  if [ "$OS" = "darwin" ]; then
    OS_VERSION="$(sw_vers -productVersion)"
    info "macOS $OS_VERSION detected"
  else
    if [ -f /etc/os-release ]; then
      . /etc/os-release
      OS_VERSION="$PRETTY_NAME"
    else
      OS_VERSION="Linux"
    fi
    info "$OS_VERSION detected"
  fi
}

# --- Check existing install ---
check_existing() {
  if [ -f "$FORGE_BIN" ]; then
    warn "Forge is already installed at $FORGE_BIN"
    # If stdin is a terminal, ask. Otherwise (piped install), just reinstall.
    if [ -t 0 ]; then
      printf "  Reinstall? [Y/n] "
      read -r answer
      case "$answer" in
        [nN]*) info "Exiting."; exit 0 ;;
        *)     info "Reinstalling..." ;;
      esac
    else
      info "Reinstalling..."
    fi
  fi
}

# --- Enable SSH ---
enable_ssh() {
  step "Configuring SSH..."
  if [ "$OS" = "darwin" ]; then
    if systemsetup -getremotelogin 2>/dev/null | grep -qi "on"; then
      info "Remote Login already enabled"
    else
      sudo systemsetup -setremotelogin on
      info "Remote Login enabled"
    fi
  else
    # Check if SSH is already running
    for svc in sshd ssh; do
      if systemctl is-active "$svc" >/dev/null 2>&1; then
        info "SSH already running ($svc)"
        return
      fi
    done
    # Not running — install and enable
    if command -v apt-get >/dev/null 2>&1; then
      if ! dpkg -l openssh-server >/dev/null 2>&1; then
        sudo apt-get update -qq && sudo apt-get install -y -qq openssh-server
      fi
    elif command -v dnf >/dev/null 2>&1; then
      sudo dnf install -y -q openssh-server
    fi
    for svc in sshd ssh; do
      if sudo systemctl enable "$svc" 2>/dev/null && sudo systemctl start "$svc" 2>/dev/null; then
        info "SSH enabled ($svc)"
        return
      fi
    done
    warn "Could not enable SSH automatically"
  fi
}

# --- Disable sleep ---
disable_sleep() {
  step "Configuring sleep..."
  if [ "$OS" = "darwin" ]; then
    # Check if already disabled
    SLEEP_VAL=$(pmset -g custom 2>/dev/null | grep "^ sleep" | awk '{print $2}')
    if [ "$SLEEP_VAL" = "0" ]; then
      info "Sleep already disabled"
    else
      sudo pmset -a sleep 0 disksleep 0
      info "Sleep disabled"
    fi
  else
    # Check if already masked
    if systemctl is-enabled sleep.target 2>/dev/null | grep -q "masked"; then
      info "Sleep already disabled"
    else
      sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target >/dev/null 2>&1
      info "Sleep disabled"
    fi
  fi
}

# --- Auto-login (macOS only, opt-in) ---
configure_autologin() {
  if [ "$OS" != "darwin" ]; then
    return
  fi

  step "Configuring auto-login..."

  # Check FileVault
  if fdesetup status 2>/dev/null | grep -q "FileVault is On"; then
    warn "FileVault is enabled — auto-login is not compatible"
    dim "Skipping auto-login setup"
    return
  fi

  # Check if already enabled
  AUTOLOGIN_USER="$(defaults read /Library/Preferences/com.apple.loginwindow autoLoginUser 2>/dev/null || true)"
  if [ -n "$AUTOLOGIN_USER" ]; then
    info "Auto-login already enabled for $AUTOLOGIN_USER"
    return
  fi

  printf "  Enable auto-login for current user? [y/N] "
  read -r answer
  case "$answer" in
    [yY]*)
      CURRENT_USER="$(whoami)"
      sudo defaults write /Library/Preferences/com.apple.loginwindow autoLoginUser "$CURRENT_USER"
      info "Auto-login enabled for $CURRENT_USER"
      ;;
    *)
      dim "Auto-login skipped"
      ;;
  esac
}

# --- Install Tailscale ---
install_tailscale() {
  step "Checking Tailscale..."

  if command -v tailscale >/dev/null 2>&1; then
    info "Tailscale already installed"
    return
  fi

  warn "Tailscale is not installed"
  dim "Tailscale creates a secure private network so you can access this"
  dim "machine from anywhere without exposing it to the public internet."
  printf "  Install Tailscale now? [Y/n] "
  read -r answer
  case "$answer" in
    [nN]*)
      warn "Skipping Tailscale — dashboard will bind to 0.0.0.0"
      dim "Install later: https://tailscale.com/download"
      return
      ;;
  esac

  if [ "$OS" = "darwin" ]; then
    if command -v brew >/dev/null 2>&1; then
      brew install tailscale
    else
      fail "Homebrew not found. Install Tailscale manually: https://tailscale.com/download"
    fi
  else
    curl -fsSL https://tailscale.com/install.sh | sh
  fi

  info "Tailscale installed"
}

# --- Start Tailscale ---
start_tailscale() {
  if ! command -v tailscale >/dev/null 2>&1; then
    return
  fi

  step "Connecting to Tailscale..."

  # Start the Tailscale daemon if not running
  if [ "$OS" = "darwin" ]; then
    if ! pgrep -x tailscaled >/dev/null 2>&1; then
      sudo tailscaled >/dev/null 2>&1 &
      sleep 2
    fi
  else
    sudo systemctl enable --now tailscaled >/dev/null 2>&1 || true
  fi

  # Check if already authenticated
  if tailscale status >/dev/null 2>&1; then
    TS_IP="$(tailscale ip -4 2>/dev/null || true)"
    if [ -n "$TS_IP" ]; then
      info "Already connected: $TS_IP"
      return
    fi
  fi

  # Start authentication
  printf "\n"
  tailscale up 2>&1 | while IFS= read -r line; do
    case "$line" in
      *https://login.tailscale.com*)
        printf "\n  ${BOLD}→ Open this URL to connect your machine to Tailscale:${RESET}\n"
        printf "    %s\n\n" "$line"
        ;;
    esac
  done

  # Wait for connection
  printf "  Waiting for authorization..."
  for i in $(seq 1 60); do
    if tailscale status >/dev/null 2>&1; then
      TS_IP="$(tailscale ip -4 2>/dev/null || true)"
      if [ -n "$TS_IP" ]; then
        printf " ${GREEN}✓${RESET} Connected\n"
        info "Tailscale IP: $TS_IP"
        return
      fi
    fi
    sleep 2
  done

  fail "Tailscale authentication timed out"
}

# --- Install Forge binary ---
install_binary() {
  step "Installing Forge daemon..."

  DOWNLOAD_URL="https://github.com/${FORGE_REPO}/releases/download/v${FORGE_VERSION}/forge-host-${OS}-${ARCH}"

  if command -v curl >/dev/null 2>&1; then
    sudo curl -fsSL -o "$FORGE_BIN" "$DOWNLOAD_URL"
  elif command -v wget >/dev/null 2>&1; then
    sudo wget -q -O "$FORGE_BIN" "$DOWNLOAD_URL"
  else
    fail "Neither curl nor wget found"
  fi

  sudo chmod +x "$FORGE_BIN"
  info "Forge daemon installed"

  # Tray app
  TRAY_URL="https://github.com/${FORGE_REPO}/releases/download/v${FORGE_VERSION}/forge-host-tray-${OS}-${ARCH}"
  if command -v curl >/dev/null 2>&1; then
    sudo curl -fsSL -o "$FORGE_TRAY_BIN" "$TRAY_URL"
  elif command -v wget >/dev/null 2>&1; then
    sudo wget -q -O "$FORGE_TRAY_BIN" "$TRAY_URL"
  fi
  sudo chmod +x "$FORGE_TRAY_BIN"
  info "Forge tray app installed"
}

# --- Register system service ---
register_service() {
  step "Registering Forge service..."

  if [ "$OS" = "darwin" ]; then
    PLIST="/Library/LaunchDaemons/dev.forge.plist"
    sudo tee "$PLIST" >/dev/null <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>dev.forge</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/forge-host</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/var/log/forge.log</string>
  <key>StandardErrorPath</key>
  <string>/var/log/forge.log</string>
</dict>
</plist>
PLIST_EOF

    sudo launchctl bootout system/dev.forge 2>/dev/null || true
    sudo launchctl bootstrap system "$PLIST"
    info "launchd service registered"

  else
    UNIT="/etc/systemd/system/forge.service"
    sudo tee "$UNIT" >/dev/null <<UNIT_EOF
[Unit]
Description=Forge remote dev server daemon
After=network-online.target tailscaled.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/forge-host
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT_EOF

    sudo systemctl daemon-reload
    sudo systemctl enable forge.service
    sudo systemctl restart forge.service
    info "systemd service registered"
  fi
}

# --- Register tray app ---
register_tray() {
  step "Setting up menu bar app..."

  if [ "$OS" = "darwin" ]; then
    TRAY_PLIST="$HOME/Library/LaunchAgents/dev.forge.tray.plist"
    mkdir -p "$HOME/Library/LaunchAgents"
    cat > "$TRAY_PLIST" <<TRAY_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>dev.forge.tray</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/forge-host-tray</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
TRAY_EOF
    launchctl unload -w "$TRAY_PLIST" 2>/dev/null || true
    launchctl load -w "$TRAY_PLIST"
    info "Menu bar app registered and started"

  else
    AUTOSTART_DIR="$HOME/.config/autostart"
    mkdir -p "$AUTOSTART_DIR"
    cat > "$AUTOSTART_DIR/forge-host-tray.desktop" <<DESKTOP_EOF
[Desktop Entry]
Type=Application
Name=Forge Tray
Exec=/usr/local/bin/forge-host-tray
X-GNOME-Autostart-enabled=true
DESKTOP_EOF
    # Launch it now
    nohup "$FORGE_TRAY_BIN" >/dev/null 2>&1 &
    info "Tray app registered and started"
  fi
}

# --- Verify daemon ---
verify_daemon() {
  step "Verifying Forge daemon..."

  TS_IP="${TS_IP:-$(tailscale ip -4 2>/dev/null || echo '127.0.0.1')}"
  HEALTH_URL="http://${TS_IP}:${FORGE_PORT}/api/health"

  for i in $(seq 1 10); do
    if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
      info "Forge daemon is running"
      return
    fi
    sleep 2
  done

  warn "Forge daemon did not respond — check logs: /var/log/forge.log"
}

# --- Print summary ---
print_summary() {
  TS_IP="${TS_IP:-$(tailscale ip -4 2>/dev/null || echo '???')}"
  CURRENT_USER="$(whoami)"

  printf "\n  ${GREEN}✓ Forge is ready.${RESET}\n"
  printf "\n  Connect from any device on your Tailscale network:\n"
  printf "    ${BOLD}ssh %s@%s${RESET}\n" "$CURRENT_USER" "$TS_IP"
  printf "\n  Web dashboard:\n"
  printf "    ${BOLD}http://%s:%s${RESET}\n" "$TS_IP" "$FORGE_PORT"
  printf "\n  ${DIM}Manage SSH, sleep, screen sharing, and more from the dashboard.${RESET}\n\n"
}

# --- Main ---
acquire_sudo() {
  printf "\n  Forge needs admin privileges to configure system services,\n"
  printf "  install the daemon, and manage SSH and sleep settings.\n\n"
  if ! sudo -v 2>/dev/null; then
    fail "Could not obtain admin privileges"
  fi
  info "Admin access granted"
  # Keep sudo alive in the background for the duration of the install
  while true; do sudo -n true; sleep 50; done 2>/dev/null &
  SUDO_KEEPALIVE_PID=$!
}

cleanup() {
  # Kill the sudo keepalive process
  if [ -n "$SUDO_KEEPALIVE_PID" ]; then
    kill "$SUDO_KEEPALIVE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

main() {
  printf "\n  ${BOLD}⚒  Forge${RESET} — remote dev server setup\n"

  acquire_sudo

  step "Checking system..."
  detect_os
  detect_os_version
  check_existing

  enable_ssh
  disable_sleep
  configure_autologin
  install_tailscale
  start_tailscale
  install_binary
  register_service
  register_tray
  verify_daemon
  print_summary
}

main
