# 🌀 Flint Documentation

Welcome to the complete reference for the Flint CLI and API.

## Table of Contents

- [Getting Started](#getting-started)
- [Security & Authentication](#security--authentication)
- [CLI Reference](#cli-reference)
  - [Global Flags](#global-flags)
  - [Server Flags](#server-flags)
  - [Commands](#commands)
    - [`flint serve`](#flint-serve)
    - [`flint api-key`](#flint-api-key)
    - [`flint vm`](#flint-vm) - Virtual Machine Management
    - [`flint network`](#flint-network) - Network Management
    - [`flint storage`](#flint-storage) - Storage Management
    - [`flint image`](#flint-image) - Cloud Image Repository
    - [`flint snapshot`](#flint-snapshot) - Snapshot Management
- [API Reference](#api-reference)
  - [Authentication](#authentication)
  - [Endpoints](#endpoints)
  - [Request/Response Examples](#requestresponse-examples)
  - [WebSocket Connections](#websocket-connections)
- [Security Best Practices](#security-best-practices)
- [Configuration](#configuration)

---

## Getting Started

### Prerequisites
Ensure your Linux host has `libvirt` and `qemu-kvm` installed and the `libvirtd` service is running.

### Installation
The recommended installation method is the one-liner script:
```bash
curl -fsSL https://raw.githubusercontent.com/volantvm/flint/main/install.sh | bash
```
This will install the `flint` binary to `/usr/local/bin`.

### Running the Server
Start the Flint server, which includes the web UI and the REST API:
```bash
flint serve
```

**First Run Setup:**
- On first startup, you'll be prompted to set a web UI passphrase
- This passphrase protects the web interface and is required for all web access
- The passphrase is hashed and stored securely in your config

**Access Points:**
- **Web UI:** `http://localhost:5550` (requires passphrase login)
- **API:** `http://localhost:5550/api` (requires authentication)

**Passphrase Management:**
```bash
# Set passphrase interactively
flint serve --set-passphrase

# Set passphrase directly
flint serve --passphrase "your-secure-passphrase"
```

**Production Deployment:**
For production, it's recommended to run Flint as a `systemd` service. An example service file can be found in the repository.

---

## 🔐 Security & Authentication

Flint implements enterprise-grade security with multiple authentication layers:

### Authentication Methods

**1. Web UI Authentication (Passphrase)**
- Required for accessing the web interface
- SHA256 hashed passphrase stored securely
- Session-based authentication with HTTP-only cookies
- 1-hour session expiry with automatic renewal

**2. API Authentication (Bearer Tokens)**
- Required for CLI and external API access
- API keys are auto-generated on first run
- Bearer token authentication: `Authorization: Bearer <api-key>`
- API keys never exposed publicly

### Security Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Browser   │────│  Passphrase     │────│  Session Cookie  │
│                 │    │  Login Form     │    │  Authentication  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                        │                        │
         └────────────────────────┼────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │     Flint Server         │
                    │                          │
                    │  ┌─────────────────────┐ │
                    │  │   API Endpoints     │ │
                    │  │                     │ │
                    │  │ • Session Cookies   │ │
                    │  │ • API Keys          │ │
                    │  │ • Rate Limiting     │ │
                    │  └─────────────────────┘ │
                    └──────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │     External Access      │
                    │                          │
                    │  ┌─────────────────────┐ │
                    │  │ CLI Tools & APIs    │ │
                    │  │                     │ │
                    │  │ • API Key Required │ │
                    │  │ • Bearer Auth       │ │
                    │  └─────────────────────┘ │
                    └──────────────────────────┘
```

### First Run Security Setup

```bash
# Start Flint for the first time
flint serve

# You'll be prompted to set a passphrase:
# 🔐 No web UI passphrase set. Let's set one up for security.
# Enter passphrase: ********
```

### Passphrase Management

```bash
# Set passphrase interactively
flint serve --set-passphrase

# Set passphrase directly (not recommended for production)
flint serve --passphrase "your-secure-passphrase"

# Change passphrase later
flint serve --set-passphrase
```

### API Key Access

API keys are **never returned by the HTTP API**. Read the key locally from the Flint account:

```bash
# Run on the Flint host
flint api-key
```

---

## CLI Reference

### Global Flags
- `--socket string`: Path to the libvirt socket (default: `/var/run/libvirt/libvirt-sock`).
- `-h, --help`: Help for any command.

### Server Flags
- `--passphrase string`: Set web UI passphrase directly (will be hashed).
- `--set-passphrase`: Interactively prompt for web UI passphrase.

### Commands

#### `flint serve`
Starts the Flint web server and API.
```bash
flint serve
```

#### `flint api-key`
Display the API key for authentication.
```bash
flint api-key
```
**Description:** Shows the API key that should be used for authenticating with the Flint API. This key is required for all API requests and CLI operations.

**Example usage:**
```bash
# Get your API key
API_KEY=$(flint api-key)

# Use it with curl
curl -H "Authorization: Bearer $API_KEY" http://localhost:5550/api/vms

# Use it with other CLI commands (automatically handled)
flint vm list
```

#### `flint vm`
Complete virtual machine lifecycle management with full feature parity to the web UI.

**VM Listing and Monitoring:**
```bash
flint vm list                    # List all VMs with status, CPU, memory usage
flint vm details [vm-name]       # Detailed VM information (disks, networks, OS)
```

**VM Creation and Management:**
```bash
flint vm launch [vm-name]        # Create and start VM with smart defaults
flint vm start [vm-name]         # Start a stopped VM
flint vm stop [vm-name]          # Graceful shutdown
flint vm stop [vm-name] --force  # Force stop (power off)
flint vm restart [vm-name]       # Restart VM
flint vm delete [vm-name]        # Delete VM (with confirmation)
flint vm delete [vm-name] --force --delete-storage  # Force delete with storage
```

**VM Access:**
```bash
flint vm ssh [vm-name]          # SSH into VM (auto-detects IP and credentials)
flint vm console [vm-name]      # Serial console access
```

**Guest Agent Management:**
```bash
flint vm guest-agent status [vm-name]  # Check QEMU guest agent status
```

#### `flint network`
Virtual network management for creating isolated network environments.

```bash
flint network list                           # List all virtual networks
flint network create [name] --bridge [bridge-name]  # Create new network
flint network start [name]                   # Start (activate) network
flint network stop [name]                    # Stop (deactivate) network
flint network delete [name]                  # Delete network
```

#### `flint storage`
Storage pool and volume management for VM disk operations.

**Storage Pools:**
```bash
flint storage pool list          # List storage pools with capacity info
```

**Volume Management:**
```bash
flint storage volume list [pool-name]           # List volumes in pool
flint storage volume create [pool] [name] --size [size]  # Create volume
flint storage volume delete [pool] [name]       # Delete volume
flint storage volume resize [pool] [name] --size [new-size]  # Resize volume
```

#### `flint image`
Cloud image repository for downloading and managing official OS images.

```bash
flint image list                 # Browse available cloud images
flint image download [image-id]  # Download cloud image
flint image download [image-id] --wait  # Download and wait for completion
flint image status               # Check download status of all images
flint image status [image-id]    # Check status of specific image
```

**Available Images:**
- Ubuntu 24.04 LTS, Ubuntu 22.04 LTS
- Debian 12
- CentOS Stream 9
- Fedora 39
- Alpine Linux 3.19

#### `flint snapshot`
VM snapshot management for quick backup and restore operations.

```bash
flint snapshot create [vm-name] [snapshot-name]  # Create snapshot
flint snapshot create [vm-name] [snapshot-name] --description "backup description"
flint snapshot list [vm-name]    # List snapshots for VM
flint snapshot revert [vm-name] [snapshot-name]  # Revert to snapshot
```

### Complete CLI Examples

**Full VM Workflow:**
```bash
# Download a cloud image
flint image download ubuntu-24.04 --wait

# Create and start a VM
flint vm launch web-server

# Monitor the VM
flint vm list
flint vm details web-server

# Access the VM
flint vm ssh web-server

# Create a snapshot
flint snapshot create web-server baseline

# Manage the VM
flint vm stop web-server
flint vm start web-server
```

**Network and Storage Setup:**
```bash
# Create custom network
flint network create app-net --bridge br-app

# Create additional storage
flint storage volume create default data-vol --size 50G

# List resources
flint network list
flint storage volume list default
```


---

#### `flint list` (alias: `ls`)
Lists virtual machines.
```bash
flint list [flags]
```
**Examples:**
```bash
# List only running VMs (default)
flint list

# List all VMs, including stopped ones
flint list --all

# Output in JSON format for scripting
flint list --all --format json
```
**Flags:**
- `--all`: Show all VMs (including stopped).
- `--format string`: Output format: `table` or `json` (default: "table").

---

#### `flint ssh`
SSH into a VM by name.
```bash
flint ssh <vm-name> [flags]
```
**Examples:**
```bash
# SSH as the default user (e.g., 'ubuntu')
flint ssh web-server

# SSH as root
flint ssh web-server --user root

# Execute a remote command
flint ssh web-server --command "uptime"
```
**Flags:**
- `--user string`: SSH username.
- `--command string`: Command to execute remotely.

---

#### `flint console`
Connect to the serial console of a VM.
```bash
flint console <vm-name>
```
**Note:** Press `Ctrl+]` to exit the console session.

---

#### `flint snapshot`
Manage VM snapshots.
```bash
flint snapshot <subcommand> [flags]
```
**Subcommands:**
- `create <vm-name> --name <snapshot-name>`: Create a snapshot.
- `list <vm-name>`: List snapshots for a VM.
- `revert <vm-name> <snapshot-name>`: Revert a VM to a snapshot.
- `delete <vm-name> <snapshot-name>`: Delete a snapshot.

**Examples:**
```bash
# Create a snapshot before a risky operation
flint snapshot create web-server --name "before-upgrade-v2"

# Revert to the snapshot if something goes wrong
flint snapshot revert web-server "before-upgrade-v2"
```
---

#### `flint delete`
Deletes a VM.
```bash
flint delete <vm-name> [flags]
```
**Examples:**
```bash
# Delete the VM definition but keep its disk
flint delete web-server

# Delete the VM AND its associated disk volumes
flint delete web-server --disks
```
**Flags:**
- `--disks`: Also delete all storage volumes associated with the VM.
- `--force, -f`: Skip the confirmation prompt.

---

## API Reference
The Flint API is served on the same port as the web UI (`5550` by default). All endpoints are prefixed with `/api`. The API is designed to be RESTful and predictable.

### Authentication
Flint implements multi-layered authentication for different access patterns:

#### Web UI Authentication
- **Passphrase Required**: Web interface requires passphrase login
- **Session Cookies**: Secure HTTP-only cookies with 1-hour expiry
- **Automatic**: No manual API key handling needed

**Web UI Login Flow:**
1. Visit `http://your-server:5550`
2. Enter passphrase → receive session cookie
3. All subsequent API calls authenticated automatically

#### API/CLI Authentication
- **Bearer Token**: CLI and external tools use API keys
- **Local Retrieval**: API keys are read with `flint api-key` on the host
- **Browser Isolation**: the web UI never receives the master API key

**Getting your API Key (Authenticated):**
```bash
# Run locally as the Flint service account
flint api-key
```

**Using API Keys:**
```bash
# CLI usage (automatic authentication)
flint vm list

# External API usage
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:5550/api/vms
```

**Authentication Methods:**
- **Web UI**: Session cookies (after passphrase login)
- **CLI**: API key (automatic)
- **External API**: API key (manual)

**Security Notes:**
- API keys are never exposed publicly
- Web UI users never see API keys
- All endpoints require authentication except `/api/health`
- Session cookies are HTTP-only, SameSite=Strict, and Secure when served over HTTPS
- Passphrases are hashed with bcrypt

### Endpoints

#### Host
- `GET /api/host/status`: Get basic host status (hostname, hypervisor version, VM counts).
- `GET /api/host/resources`: Get host resource usage (CPU, Memory, Storage).

#### Virtual Machines (VMs)
- `GET /api/vms`: List all VMs with summary info.
- `POST /api/vms`: Create a new VM from an image.
- `GET /api/vms/{uuid}`: Get detailed information for a single VM.
- `DELETE /api/vms/{uuid}`: Delete a VM.
- `POST /api/vms/{uuid}/action`: Perform an action on a VM (e.g., `start`, `stop`).

#### Snapshots & Templates
- `GET /api/vms/{uuid}/snapshots`: List snapshots for a VM.
- `POST /api/vms/{uuid}/snapshots`: Create a new snapshot for a VM.
- `POST /api/vms/{uuid}/snapshots/{name}/revert`: Revert a VM to a snapshot.
- `DELETE /api/vms/{uuid}/snapshots/{name}`: Delete a snapshot.
- `GET /api/vm-templates`: List all VMs that can be used as templates (i.e., have snapshots).
- `POST /api/vms/from-template`: Create a new VM from a template.

#### Infrastructure
- `GET /api/storage-pools`: List all storage pools.
- `GET /api/storage-pools/{pool}/volumes`: List volumes in a specific pool.
- `GET /api/networks`: List all libvirt networks.

### Request/Response Examples

#### Create a New VM
`POST /api/vms`
```json
// Request Body
{
  "name": "api-server-01",
  "memoryMB": 2048,
  "vcpus": 2,
  "diskSizeGB": 20,
  "imageName": "ubuntu-24.04",
  "startOnCreate": true
}
```

#### Perform a VM Action
`POST /api/vms/YOUR_VM_UUID/action`
```json
// Request Body
{
  "action": "stop"
}
```
*Supported actions: `start`, `stop`, `reboot`, `pause`, `resume`, `force-stop` (destroy).*

### WebSocket Connections
Flint uses WebSockets for real-time serial console access.
- `GET /api/vms/{uuid}/console-stream`: Connect to this endpoint to stream console output.

---

## 🔐 Security Best Practices

### Web UI Security
- **Always set a strong passphrase** on first run
- **Use HTTPS in production** (configure reverse proxy)
- **Regular passphrase rotation** using `--set-passphrase` flag
- **Limit web UI access** to trusted networks

### API Security
- **Never expose API keys publicly**
- **Use environment variables** for API keys in scripts
- **Rotate API keys regularly** by restarting the server
- **Monitor API access logs** for suspicious activity

### Network Security
- **Keep the default localhost binding** unless remote listening is explicitly required
- **Use firewalls** to restrict access to Flint's port
- **Enable TLS/SSL** for production deployments
- **Consider VPN access** for remote management

### Configuration Security
```bash
# Environment variables (recommended for scripts)
export FLINT_API_KEY="your-secure-api-key"

# CLI flags for passphrase management
flint serve --passphrase "strong-passphrase-here"
flint serve --set-passphrase  # Interactive setup
```

### Production Deployment
```bash
# Systemd service example
sudo cp flint.service /etc/systemd/system/
sudo systemctl enable flint
sudo systemctl start flint

# Nginx reverse proxy with SSL
server {
    listen 443 ssl;
    server_name flint.yourdomain.com;

    location / {
        proxy_pass http://localhost:5550;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## ⚙️ Configuration

Flint stores configuration in `~/.flint/config.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 5550,
    "read_timeout": 30,
    "write_timeout": 30
  },
  "security": {
    "passphrase_hash": "hashed-passphrase",
    "rate_limit_requests": 100,
    "rate_limit_burst": 20
  },
  "libvirt": {
    "uri": "qemu:///system",
    "iso_pool": "isos",
    "template_pool": "templates",
    "image_pool_path": "/var/lib/flint/images"
  },
  "logging": {
    "level": "INFO",
    "format": "json"
  }
}
```

### Configuration Options
- **server.host**: Bind address (use "127.0.0.1" for localhost-only)
- **server.port**: Port number (default: 5550)
- **security.passphrase_hash**: SHA256 hash of web UI passphrase
- **security.rate_limit_***: API rate limiting settings
- **libvirt.uri**: Libvirt connection URI
- **logging.level**: Log verbosity (DEBUG, INFO, WARN, ERROR)
