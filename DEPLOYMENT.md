# Deploying pc-price-server on Oracle Cloud + Cloudflare Tunnel

Reference for running the price tracker in a podman container on the Oracle
Linux 9 (aarch64) Always Free VM, published at your own domain via a
Cloudflare Tunnel, with Cloudflare Access as the login wall.

## Architecture

```
Browser ──HTTPS──> Cloudflare edge (TLS, Access login)
                        │
                 outbound tunnel (no inbound ports on the VM)
                        │
                   cloudflared  ──http──>  127.0.0.1:8090
                 (systemd service)              │
                                        podman container
                                        pc-price-server
                                        (config.yaml ro-mount,
                                         /data volume for SQLite)
```

Notes on what is *not* here:

- **No NGINX.** cloudflared is the reverse proxy — it forwards tunnel
  traffic to the app. TLS terminates at Cloudflare. Add a proxy layer only
  if you someday need response caching or many local services (and even
  multiple hostnames are handled by cloudflared `ingress:` rules).
- **No custom podman network** in the recommended setup. The container
  publishes to loopback; cloudflared (on the host) connects to it there.
  A shared network is only needed for the all-containers variant (§7).
- **No new OCI security-list rules.** Port 22 stays the only inbound rule.
  The tunnel is an outbound connection from the VM.

## 0. Prerequisites

- Domain added to a free Cloudflare account, Porkbun nameservers switched
  to the two Cloudflare gives you (Porkbun → Domain → Authoritative
  Nameservers). Wait until the Cloudflare dashboard shows the zone
  **Active**.
- VM reachable over SSH; `git`, `podman`, `make` installed
  (`sudo dnf install -y git podman make`).
- Throughout, replace `prices.example.com` with your real subdomain and
  `opc` with your VM user if different.

## 1. Get the code onto the VM

Until the repo has a Git remote, rsync from the Mac:

```bash
# on the Mac, from ~/personal
rsync -av --exclude .git --exclude pc-price-server --exclude prices.db \
      --exclude .venv --exclude data \
      pc-build/ opc@<PUBLIC_IP>:~/pc-build/
```

(Once it's on GitHub, `git clone` + `git pull` replaces this.)

## 2. Production config

Edit `~/pc-build/config.yaml` on the VM:

```yaml
settings:
  database: "/data/prices.db"   # IMPORTANT — see below
  listen: "0.0.0.0:8090"        # inside the container; not exposed publicly
```

**Why `/data/prices.db`:** the Makefile/quadlet mounts `./data` at `/data`.
If `database:` stays `prices.db`, SQLite lands on the container's ephemeral
filesystem and your price history dies with every container replacement.
Everything else in config.yaml (products, interval, webhook) works as-is.

## 3. Build the image (native aarch64)

```bash
cd ~/pc-build
podman build -t pc-price-server .
mkdir -p data
```

Quick smoke test:

```bash
podman run --rm -p 127.0.0.1:8090:8090 \
  -v ./config.yaml:/app/config.yaml:ro,Z \
  -v ./data:/data:Z \
  pc-price-server &
sleep 2 && curl -s localhost:8090 | head -3   # expect dashboard HTML
podman stop -l
```

**SELinux note:** Oracle Linux enforces SELinux. The `:Z` suffix on volume
mounts is required — without it the container gets `permission denied` on
config.yaml and the data dir.

## 4. Run it as a service (podman Quadlet, rootless)

Quadlet is podman's native systemd integration (podman ≥ 4.4, present on
OL9). Rootless keeps the container away from root entirely.

```bash
# let the opc user's services run without an active login session
sudo loginctl enable-linger opc

mkdir -p ~/.config/containers/systemd
```

Create `~/.config/containers/systemd/pc-price-server.container`:

```ini
[Unit]
Description=PC component price tracker

[Container]
Image=localhost/pc-price-server:latest
PublishPort=127.0.0.1:8090:8090
Volume=%h/pc-build/config.yaml:/app/config.yaml:ro,Z
Volume=%h/pc-build/data:/data:Z
Environment=HOST=0.0.0.0
Environment=PORT=8090

[Service]
Restart=on-failure

[Install]
WantedBy=default.target
```

Activate:

```bash
systemctl --user daemon-reload
systemctl --user start pc-price-server
systemctl --user status pc-price-server
curl -s localhost:8090 | head -3
```

`PublishPort=127.0.0.1:...` binds to loopback only — even if someone opened
8090 in the security list, the app wouldn't be reachable directly.

## 5. Cloudflare Tunnel (cloudflared on the host)

```bash
sudo dnf install -y \
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-aarch64.rpm

cloudflared tunnel login          # prints a URL — open it on the Mac, pick your zone
cloudflared tunnel create pc-prices
cloudflared tunnel route dns pc-prices prices.example.com
```

`tunnel create` prints a UUID and writes `~/.cloudflared/<UUID>.json`.
Create `~/.cloudflared/config.yml`:

```yaml
tunnel: <UUID>
credentials-file: /home/opc/.cloudflared/<UUID>.json

ingress:
  - hostname: prices.example.com
    service: http://localhost:8090
  - service: http_status:404      # required catch-all
```

Test in the foreground, then install as a service:

```bash
cloudflared tunnel run pc-prices        # then open https://prices.example.com
# Ctrl+C once it works, then:
sudo cloudflared service install        # copies config to /etc/cloudflared/
sudo systemctl enable --now cloudflared
```

## 6. Cloudflare Access (login wall)

The dashboard has a public **Scrape now** button — don't leave it open to
bots. In the Cloudflare dashboard:

1. **Zero Trust** → choose the free plan if prompted → **Access** →
   **Applications** → **Add an application** → *Self-hosted*
2. Application domain: `prices.example.com` (leave path empty to cover
   the whole site, including `POST /scrape`)
3. Policy: Action **Allow**, Include → **Emails** → your email address
4. Keep the default **One-time PIN** login method

Visiting the site now shows a Cloudflare login page; enter your email, get
a PIN, in. Sessions last 24 h by default (tunable in the app settings).

## 7. Variant: cloudflared as a container (if you'd rather not install it on the host)

This is where a podman network *does* come in:

```bash
podman network create proxy-net
```

- Add `Network=proxy-net` to the app's `.container` file (the
  `PublishPort` line can then be dropped entirely).
- In Zero Trust → **Networks** → **Tunnels**, create a dashboard-managed
  tunnel; it gives you a run token. Create
  `~/.config/containers/systemd/cloudflared.container`:

```ini
[Unit]
Description=Cloudflare tunnel

[Container]
Image=docker.io/cloudflare/cloudflared:latest
Network=proxy-net
Exec=tunnel --no-autoupdate run --token <TUNNEL_TOKEN>

[Service]
Restart=on-failure

[Install]
WantedBy=default.target
```

- In the tunnel's dashboard config, add a public hostname
  `prices.example.com` → service `http://pc-price-server:8090`
  (user-defined podman networks provide DNS by container name).

Host-service (§5) vs container (§7) is taste; §5 has fewer moving parts,
§7 keeps the host clean and manages tunnel config from the dashboard.

## 8. Updating the app

```bash
# Mac: rsync as in §1, then on the VM:
cd ~/pc-build
podman build -t pc-price-server .
systemctl --user restart pc-price-server
```

Config-only changes don't need a rebuild — config.yaml is bind-mounted and
hot-reloaded every scrape pass; the dashboard reads it per request. A new
`listen` value is the only setting that needs a restart.

## 9. Ops crib sheet

```bash
systemctl --user status pc-price-server   # app service state
podman logs -f pc-price-server            # scrape pass logs
sudo systemctl status cloudflared         # tunnel state
cloudflared tunnel info pc-prices         # tunnel connections
journalctl --user -u pc-price-server -e   # service-level errors
sqlite3 ~/pc-build/data/prices.db 'SELECT COUNT(*) FROM prices;'
```

Patching: `dnf-automatic` (security errata) + Ksplice (live kernel) —
already covered on this box; no scheduled reboots needed.

## 10. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| Container `permission denied` on config.yaml or /data | Missing `:Z` SELinux label on the volume mount |
| `curl localhost:8090` works, tunnel shows 502 | cloudflared can't reach the app — check `PublishPort=127.0.0.1:8090:8090` is active (`ss -tlnp \| grep 8090`) |
| Tunnel URL never loads | `sudo systemctl status cloudflared`; check the zone is Active in Cloudflare; `cloudflared tunnel info pc-prices` should list ≥1 connection |
| DNS `NXDOMAIN` | `tunnel route dns` step skipped, or nameserver switch hasn't propagated (check `dig prices.example.com`) |
| Price history resets after redeploy | `database:` in config.yaml isn't `/data/prices.db` (§2) |
| Container dies on boot | `loginctl enable-linger opc` not set — rootless services need linger to start without a login |
| App works locally, Access loop / no login page | Access application domain typo'd, or you're hitting the IP directly instead of the hostname |
