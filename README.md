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
- **Notifications** — ntfy and Telegram
- **Outbound-only agent** — works behind CGNAT, no inbound ports needed
- **One-line install** — `curl | sh` provisioning with per-host connector config
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
| `n8n` | n8n workflow engine version |
| `nginx` | Nginx version |
| `bookstack` | BookStack version |
| `trilium` | Trilium Notes version |

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

### Server (Docker Compose)

```yaml
services:
  server:
    image: ghcr.io/updara-dev/updara-server:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
      - ./connectors:/connectors

  frontend:
    image: ghcr.io/updara-dev/updara-frontend:latest
    ports:
      - "4000:80"
    environment:
      - API_URL=http://your-server:8080
```

### Agent

Create a provision in the dashboard (+ Add Host), then run the one-liner on the target host:

```bash
curl -sSL http://your-updara-server/install | sh -s -- --token <provision-token>
```

The agent installs as a systemd service and starts reporting immediately. Each host gets exactly the connectors you selected during provisioning.

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

Drop the file into the `connectors/` directory on the server — it becomes available to all agents on next sync.

---

## Roadmap

**Done**
- [x] YAML connector engine (shell + HTTP checks)
- [x] Agent provisioning + one-line install
- [x] Update execution with live output
- [x] Bulk update selection
- [x] Per-host connector ignore / disable
- [x] ntfy + Telegram notifications
- [x] Dashboard list/card view toggle with filter
- [x] Host rename (display name)
- [x] OS end-of-life tracking

**Planned**
- [ ] More connectors (Proxmox, Home Assistant, Immich, Vaultwarden, ...)
- [ ] Email notifications (SMTP)
- [ ] Authentication
- [ ] Proxmox LXC community script (one-liner server setup)
- [ ] Connector contribution guide

---

## Contributing

Connectors are the easiest way to contribute — a single YAML file covers most integrations. See `connectors/_template/connector.yaml` for the full format reference.

Bug reports and feature requests welcome via GitHub Issues.

---

## License

MIT
