# 🌀 Flint — KVM Management, Reimagined

<p align="center">
  <img src="https://i.ibb.co/yj2bFZG/flint-banner.jpg" alt="Flint Logo" width="300"/>
</p>

<p align="center">
  <strong>
    A single &lt;11MB binary with a modern Web UI, CLI, and API for KVM.
    <br/>No XML. No bloat. Just VMs.
  </strong>
</p>

<p align="center">
  <a href="https://github.com/volantvm/flint/releases/latest">
    <img src="https://img.shields.io/github/v/release/volantvm/flint" alt="Latest Release">
  </a>
  <a href="https://github.com/volantvm/flint/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/volantvm/flint" alt="License">
  </a>
  <a href="https://github.com/volantvm/flint/actions/workflows/release.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/volantvm/flint/.github/workflows/release.yml" alt="Build Status">
  </a>
</p>

---
![Flint Dashboard](https://i.ibb.co/wN9H8WKX/Screenshot-2025-09-07-at-3-51-58-AM.png)
![Flint Library](https://i.ibb.co/Z1k9XBqQ/Screenshot-2025-09-08-at-4-59-46-AM.png)


Flint is a modern, self-contained KVM management tool built for developers, sysadmins, and home labs who want zero bloat and maximum efficiency. It was built in a few hours out of a sudden urge for something better.

---

### 📋 Prerequisites

**System Requirements:**
- Linux host (Debian, Ubuntu, Fedora, RHEL, Arch, etc.)
- libvirt >= 6.10.0 (check with `libvirtd --version`)
- QEMU/KVM virtualization support

**Required Packages:**

<details>
<summary>Debian/Ubuntu</summary>

```bash
sudo apt update
sudo apt install -y qemu-kvm libvirt-daemon-system libvirt-daemon libvirt-clients bridge-utils
sudo systemctl enable --now libvirtd
```
</details>

<details>
<summary>RHEL/Fedora/CentOS</summary>

```bash
sudo dnf install -y qemu-kvm libvirt libvirt-client virt-install
sudo systemctl enable --now libvirtd
```
</details>

<details>
<summary>Arch Linux</summary>

```bash
sudo pacman -S qemu-full libvirt virt-install virt-manager
sudo systemctl enable --now libvirtd
```
</details>

**Note:** If you encounter `libvirt-lxc.so.0: cannot open shared object file`, install the LXC library:
```bash
# Debian/Ubuntu
sudo apt install -y libvirt-daemon-driver-lxc

# RHEL/Fedora
sudo dnf install -y libvirt-daemon-lxc

# Arch
sudo pacman -S libvirt-lxc
```

**Platform Compatibility:**

Flint is built with CGO (libvirt-go bindings). We provide binaries for both **glibc** and **musl** (Alpine Linux):

- **glibc binaries**: `flint-linux-amd64`, `flint-linux-arm64` (for Debian, Ubuntu, Fedora, RHEL, Arch, etc.)
- **musl binaries**: `flint-linux-amd64-musl`, `flint-linux-arm64-musl` (for Alpine Linux)

<details>
<summary>Running Flint on Alpine Linux</summary>

**Native musl support** is now available! Download the musl-specific binary for your architecture:

```bash
# For Alpine Linux (amd64)
wget https://github.com/volantvm/flint/releases/latest/download/flint-linux-amd64-musl.zip
unzip flint-linux-amd64-musl.zip
chmod +x flint
./flint serve

# For Alpine Linux (arm64)
wget https://github.com/volantvm/flint/releases/latest/download/flint-linux-arm64-musl.zip
unzip flint-linux-arm64-musl.zip
chmod +x flint
./flint serve
```

**Prerequisites for Alpine:**
```bash
# Install required Alpine packages
apk add libvirt-daemon libvirt-qemu qemu-system-x86_64
rc-update add libvirtd
rc-service libvirtd start
```

**Alternative: Use gcompat compatibility layer**

If you prefer to use the standard glibc binary on Alpine:
```bash
apk add gcompat libstdc++
wget https://github.com/volantvm/flint/releases/latest/download/flint-linux-amd64.zip
unzip flint-linux-amd64.zip
chmod +x flint
./flint serve
```
</details>

---

### 🚀 One-Liner Install

```bash
curl -fsSL https://raw.githubusercontent.com/volantvm/flint/main/install.sh | bash
```
*Auto-detects OS/arch, installs to `/usr/local/bin`, and prompts for web UI passphrase setup.*

---

### 🔐 Security & Authentication

Flint implements a multi-layered security approach:

**Web UI Security:**
- **Passphrase Authentication**: Web interface requires a passphrase login
- **Session-Based**: Secure HTTP-only cookies with 1-hour expiry
- **No API Key Exposure**: Web UI never exposes API keys to browsers

**API Security:**
- **Bearer Token Authentication**: CLI and external tools use API keys
- **Protected Endpoints**: All API endpoints require authentication
- **Flexible Access**: Support for both session cookies and API keys

**Authentication Flow:**
```bash
# First run - set passphrase
flint serve
# 🔐 No web UI passphrase set. Let's set one up for security.
# Enter passphrase: ********

# Web UI access
# Visit http://your-server:5550 → Enter passphrase → Full access

# CLI access (uses API key)
flint vm list --all

# External API access
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:5550/api/vms
```

---

### ✨ Core Philosophy

-   🖥️ **Modern UI** — A beautiful, responsive Next.js + Tailwind interface, fully embedded.
-   ⚡ **Single Binary** — No containers, no XML hell. A sub-8MB binary is all you need.
-   🛠️ **Powerful CLI & API** — Automate everything. If you can do it in the UI, you can do it from the command line or API.
-   📦 **Frictionless Provisioning** — Native Cloud-Init support and a simple, snapshot-based template system.
-   🔐 **Secure by Default** — Multi-layered authentication with passphrase protection.
-   💪 **Non-Intrusive** — Flint is a tool that serves you. It's not a platform that locks you in.
-   🌐 **Remote Management** — Connect to remote KVM/libvirt servers via SSH from a single Flint instance.

---

### 🏎️ Quickstart

**1. Start the Server**
```bash
# Interactive setup (recommended for first run)
flint serve --set-passphrase

# Or set passphrase directly
flint serve --passphrase "your-secure-password"

# Or use environment variable
export FLINT_PASSPHRASE="your-secure-password"
flint serve
```
*On first run, you'll be prompted to set a web UI passphrase for security.*
*   **Web UI:** `http://localhost:5550` (requires passphrase login)
*   **API:** `http://localhost:5550/api` (requires authentication)

**2. Web UI Access**
- Visit `http://localhost:5550`
- Enter your passphrase to access the management interface
- All API calls are automatically authenticated via session

**3. CLI Usage**
```bash
# VM Management
flint vm list                    # List all VMs
flint vm launch my-server        # Create and start a VM
flint vm ssh my-server          # SSH into a VM

# Cloud Images
flint image list                 # Browse cloud images
flint image download ubuntu-24.04 # Download an image

# Networks & Storage
flint network list               # List networks
flint storage volume list default # List storage volumes
```

**4. API Access (for external tools)**
```bash
# Get your API key (requires authentication)
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:5550/api/vms
```

---

### 🌐 Remote Server Management via SSH

Flint can connect to remote KVM/libvirt servers via SSH, allowing you to manage distributed virtualization infrastructure from a single instance.

**Setup Requirements:**
1. SSH key-based authentication configured between Flint host and remote server
2. Remote server must have libvirt >= 6.10.0 installed
3. User on remote server must have permissions to access libvirt

**Configuration Methods:**

**Option 1: Web UI (Recommended)**
1. Navigate to Settings in the Flint web interface
2. Enable "SSH Connection" toggle
3. Fill in SSH connection details:
   - Username (e.g., `root` or user with libvirt permissions)
   - Host (IP address or hostname)
   - Port (default: 22)
   - SSH Key Path (auto-detected from ~/.ssh/)
4. Click "Test Connection" to verify
5. Click "Save Configuration" and restart Flint

**Option 2: Configuration File**
Edit `~/.flint/config.json`:
```json
{
  "libvirt": {
    "uri": "qemu:///system",
    "ssh": {
      "enabled": true,
      "username": "root",
      "host": "192.168.1.100",
      "port": 22,
      "key_path": "~/.ssh/id_rsa",
      "known_hosts_path": "~/.ssh/known_hosts"
    }
  }
}
```

**Option 3: Environment Variables**
```bash
export FLINT_LIBVIRT_SSH_ENABLED=true
export FLINT_LIBVIRT_SSH_USERNAME=root
export FLINT_LIBVIRT_SSH_HOST=192.168.1.100
export FLINT_LIBVIRT_SSH_PORT=22
export FLINT_LIBVIRT_SSH_KEY_PATH=~/.ssh/id_rsa
flint serve
```

**SSH Key Setup:**
```bash
# On Flint host, generate SSH key if needed
ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa

# Copy public key to remote server
ssh-copy-id root@192.168.1.100

# Verify connection
ssh root@192.168.1.100 virsh list --all
```

**Security Notes:**
- SSH keys must have secure permissions (600 or 400)
- Password authentication is not supported (key-based only)
- Flint uses the standard libvirt SSH transport (qemu+ssh://)
- All libvirt operations are encrypted via SSH tunnel

---

### 📖 Full Documentation

Complete CLI commands, API reference, and advanced usage:

➡️ **[docs.md](docs.md)** - Complete CLI & API Documentation

---

### 🔧 Tech Stack

-   **Backend:** Go 1.25+
-   **Web UI:** Next.js + Tailwind + Bun
-   **KVM Integration:** libvirt-go
-   **Binary Size:** ~11MB (stripped)

---

<p align="center">
  <b>🚀 Flint is young, fast-moving, and designed for builders.<br/>
  Try it. Break it. Star it. Contribute.</b>
</p>




---

## 🧪 Experimental Research & References

As part of an ongoing study into **digital workspace optimization** and **Web3 identity verified environments**, the following resources are being monitored for indexing latency:

* **Core Utility:** [wrkx.app](https://wrkx.app) – Workplace Thrive Index & Web3 integration.
* **Documentation:** [The Prophecy of the Pixel](https://medium.com/p/3ea14990a2d7) – A deep dive into biophilic data-driven seated experiences.
* **Metrics:** Tracking 30cm-accurate acoustic/lighting parity across orphan URL structures.

*This section is part of a non-invasive crawling experiment. Feel free to ignore.*
