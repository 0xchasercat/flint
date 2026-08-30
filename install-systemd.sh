#!/bin/bash
set -euo pipefail

# Flint Systemd Installation Script
# This script installs Flint as a systemd service

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root"
    exit 1
fi

# Check if Flint is installed
if ! command -v flint &> /dev/null; then
    log_error "Flint is not installed. Please install it first:"
    echo "curl -fsSL https://raw.githubusercontent.com/volantvm/flint/main/install.sh | sh"
    exit 1
fi

# Check if libvirtd is running
if ! systemctl is-active --quiet libvirtd; then
    log_error "libvirtd is not running. Please start it first:"
    echo "systemctl start libvirtd"
    exit 1
fi

log_info "Installing Flint as a systemd service..."

# Create flint user and group
if ! id -u flint &>/dev/null; then
    log_info "Creating flint user..."
    useradd -r -s /bin/false -d /var/lib/flint -m flint
fi

# Create necessary directories
log_info "Creating directories..."
mkdir -p /var/lib/flint
mkdir -p /var/lib/flint/images
mkdir -p /var/log/flint

# Set ownership
chown -R flint:flint /var/lib/flint
chown -R flint:flint /var/log/flint

# Add flint user to libvirt group for access to libvirt socket
usermod -a -G libvirt flint

# Copy systemd service file
log_info "Installing systemd service..."
cp flint.service /etc/systemd/system/
chmod 644 /etc/systemd/system/flint.service

# Reload systemd
systemctl daemon-reload

# Enable and start service
log_info "Enabling and starting Flint service..."
systemctl enable flint
systemctl start flint

# Check status
if systemctl is-active --quiet flint; then
    log_info "Flint service started successfully!"
    log_info "Service status: $(systemctl is-active flint)"
    log_info "Flint is now running at: http://localhost:5550"
    log_info "Retrieve the API key locally with: sudo -u flint HOME=/var/lib/flint flint api-key"
else
    log_error "Failed to start Flint service"
    log_info "Check service status: systemctl status flint"
    log_info "Check logs: journalctl -u flint -f"
    exit 1
fi

log_info "Installation complete!"
log_info ""
log_info "Useful commands:"
log_info "  Start service:   systemctl start flint"
log_info "  Stop service:    systemctl stop flint"
log_info "  Restart service: systemctl restart flint"
log_info "  View logs:       journalctl -u flint -f"
log_info "  Check status:    systemctl status flint"
