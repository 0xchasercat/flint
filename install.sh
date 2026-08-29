#!/usr/bin/env bash
set -euo pipefail

REPO="volantvm/flint"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="flint"

# Colors
green() { printf "\033[32m%s\033[0m\n" "$1"; }
red()   { printf "\033[31m%s\033[0m\n" "$1"; }
yellow(){ printf "\033[33m%s\033[0m\n" "$1"; }

# Detect latest version from GitHub API
get_latest_release() {
    curl -s "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name":' \
    | sed -E 's/.*"([^"]+)".*/\1/'
}

# Detect OS and Arch
detect_platform() {
    local os="$(uname | tr '[:upper:]' '[:lower:]')"
    local arch="$(uname -m)"

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *) red "❌ Unsupported architecture: $arch" && exit 1 ;;
    esac

    case "$os" in
        linux)   platform="linux" ;;
        darwin)  platform="darwin" ;;
        *) red "❌ Unsupported OS: $os" && exit 1 ;;
    esac

    echo "${platform}-${arch}"
}

main() {
    green "🚀 Installing Flint..."

    latest_version=$(get_latest_release)
    if [[ -z "$latest_version" ]]; then
        red "❌ Failed to fetch latest release."
        exit 1
    fi
    yellow "ℹ️ Latest version: $latest_version"

    platform=$(detect_platform)
    green "✅ Detected platform: $platform"

    url="https://github.com/${REPO}/releases/download/${latest_version}/${BINARY_NAME}-${platform}.zip"
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    yellow "⬇️ Downloading from $url"
    curl --fail --location --proto '=https' --tlsv1.2 --progress-bar "$url" -o "$tmp_dir/${BINARY_NAME}.zip"
    curl --fail --location --proto '=https' --tlsv1.2 --silent "${url}.sha256" -o "$tmp_dir/${BINARY_NAME}.zip.sha256"

    yellow "🔐 Verifying release checksum..."
    (
        cd "$tmp_dir"
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum --check "${BINARY_NAME}.zip.sha256"
        else
            shasum -a 256 --check "${BINARY_NAME}.zip.sha256"
        fi
    )

    yellow "📦 Extracting..."
    unzip -q "$tmp_dir/${BINARY_NAME}.zip" -d "$tmp_dir"

    sudo mkdir -p "$INSTALL_DIR"
    sudo mv "$tmp_dir/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"

    green "✅ Installed $BINARY_NAME to $INSTALL_DIR/$BINARY_NAME"

    # Create config directory with proper permissions
    config_dir="$HOME/.flint"
    if [[ ! -d "$config_dir" ]]; then
        mkdir -p "$config_dir"
        chmod 700 "$config_dir"
        green "📁 Created config directory: $config_dir"
    fi

    echo
    green "🎉 Flint installation complete!"
    echo "Run: flint serve"
    echo
    yellow "ℹ️ Your API key will be automatically generated and saved to:"
    echo "   $config_dir/config.json"
    echo "   You can view/modify it there if needed."
}

main "$@"
