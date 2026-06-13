# Updara — Update Radar for your entire stack.

Updara is a self-hosted dashboard that monitors updates across every layer of your homelab — OS packages, Docker images, and native self-hosted apps — all in one place.

Most update tools cover one layer. Updara covers all three:

```
┌────────────────────────────┐
│  Native App  (Pi-hole...)  │  ← YAML Connector
├────────────────────────────┤
│  Docker Images             │  ← YAML Connector
├────────────────────────────┤
│  OS Packages (apt/yum)     │  ← YAML Connector
└────────────────────────────┘
         ↓ one dashboard
```

The agent runs on each host and phones home outbound only — no open ports, CGNAT-friendly. Updates can be triggered directly from the dashboard, with confirmation required.

---

## Features

- **Fleet overview** — all hosts, all layers, at a glance (card or list view)
- **YAML connector engine** — shell commands or HTTP API checks, no code required
- **Update execution** — trigger updates from the dashboard, watch live output
- **Bulk updates** — select and update multiple hosts/connectors at once
- **Notifications** — ntfy, Telegram, and Email (SMTP)
- **Monthly digest** — scheduled email summary with all pending updates and errors
- **Token authentication** — login-protected dashboard and API
- **Outbound-only agent** — works behind CGNAT, no inbound ports needed
- **One-line install** — `curl | sh` provisioning for both server and agents
- **OS end-of-life tracking** — know when your distro goes EOL before it happens

---

## Available Connectors

| Connector | What it monitors |
|---|---|
| `apt` | Debian/Ubuntu package updates |
| `system` | Kernel version, uptime |
| `system-eol` | OS end-of-life status via [endoflife.date](https://endoflife.date) |
| `docker-images` | Docker image updates via registry digest comparison |
| `portainer-api` | Portainer stacks via API |
| `pihole` | Pi-hole core & FTL version |
| `n8n-docker` | n8n (Docker) — container image updates |
| `immich-docker` | Immich (Docker) — container image updates |
| `ntfy-docker` | ntfy (Docker) — container image updates |
| `twingate-docker` | Twingate connector (Docker) |
| `standalone-docker` | Generic standalone Docker container updates |
| `proxmox-apt` | Proxmox VE package updates |
| `proxmox-eol` | Proxmox VE end-of-life status |
| `iobroker-native` | ioBroker (native install) adapter updates |
| `iobroker-adapters` | ioBroker adapter updates via admin API |

Writing your own connector is a single YAML file — see [Connector Format](#connector-format) below.

---

## Architecture

```
updara/
├── server/       Go REST API + SQLite (no CGO)
├── agent/        Go binary, runs on monitored hosts
├── frontend/     React + TypeScript + Vite, dark theme
└── connectors/   YAML connector library
```

The server stores host state and queues commands. Agents poll for pending commands, execute them, and push results back. The frontend is a static build served by nginx.

---

## Getting Started

### 1. Install the server

You need a Linux host with Docker installed. Then pick one of these two options:

---

**Option A — one-liner (fastest):**
```bash
curl -fsSL https://raw.githubusercontent.com/updara-dev/updara/main/install-server.sh | sh
```
Auto-detects your server IP, downloads everything, starts Updara, and prints the login token.

---

**Option B — manual (transparent, recommended):**

Download the two config files:
```bash
mkdir /opt/updara && cd /opt/updara
curl -fsSL https://raw.githubusercontent.com/updara-dev/updara/main/docker-compose.dist.yml -o docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/updara-dev/updara/main/.env.example -o .env
```

Open `.env` and set your server's IP address:
```
UPDARA_PUBLIC_URL=http://10.0.1.50:8080
```

Start Updara:
```bash
docker compose up -d
```

Get your login token:
```bash
docker compose logs server | grep "UPDARA TOKEN"
```

---

Open `http://<server-ip>:4000` in your browser and log in with the token.

**Frontend:** `http://<server-ip>:4000`  
**API:** `http://<server-ip>:8080`

### 2. Add a host

Open the dashboard → **+ Add Host**, enter a name, select connectors, and copy the generated install command. Run it on the target host as root:

```bash
wget -qO- 'http://<server-ip>:8080/install?token=<provision-token>' | sh
```

The agent installs as a systemd service and appears in the dashboard within a minute. Each host gets exactly the connectors you selected during provisioning.

### 3. Updates

Pull the latest images to update your Updara server:

```bash
cd /opt/updara
docker compose -f docker-compose.dist.yml pull
docker compose -f docker-compose.dist.yml up -d
```

---

## Connector Format

Connectors are YAML files. Two check types: `shell` (stdout parsing) and `http` (JSONPath on response body).

```yaml
name: my-app
display_name: My App
category: infrastructure
docs: https://my-app.example.com/releases

vars:
  - name: MY_API_TOKEN
    description: "API token for authentication"

check:
  type: http
  endpoint: "http://localhost/api/version"
  auth:
    type: bearer
    token: "{MY_API_TOKEN}"
  parse:
    current: "$.version.current"
    latest:  "$.version.latest"
  update_available: "current != latest"
  interval: 3600

update:
  type: shell
  command: "my-app self-update"
  requires_confirmation: true

notifications:
  changelog: "https://github.com/my-app/releases"
```

Drop the file into the `connectors/` directory on the server (or add it via the Connectors page in the UI) — it becomes available to all agents on next sync.

---

## Roadmap

**Done**
- [x] YAML connector engine (shell + HTTP checks)
- [x] Agent provisioning + one-line install
- [x] Update execution with live output
- [x] Bulk update selection
- [x] Per-host connector ignore / disable
- [x] ntfy, Telegram + Email (SMTP) notifications
- [x] Monthly/weekly/daily digest emails
- [x] Token authentication
- [x] Dashboard list/card view toggle with filter
- [x] Host rename (display name)
- [x] OS end-of-life tracking
- [x] One-line server install script

**Planned**
- [ ] More connectors (Home Assistant, Vaultwarden, ...)
- [ ] Proxmox LXC community script
- [ ] Connector contribution guide

---

## Contributing

Connectors are the easiest way to contribute — a single YAML file covers most integrations. See `connectors/_template/connector.yaml` for the full format reference.

Bug reports and feature requests welcome via GitHub Issues.

---

## License

MIT
