# Deployment Discovery & Options

Research notes for cost-efficient hosting of the pc-price-server.

---

## Hosting Options Evaluated

### 1. IBM Cloud Code Engine (Serverless)

**Pricing:**
- vCPU: $0.0000244/vCPU-second
- Memory: $0.00000305/GB-second
- Requests: First 100k/month free, then $0.40/million

**Cost for 24/7 operation (0.25 vCPU, 0.5GB RAM):**
- vCPU: 0.25 × 2,592,000 sec/month × $0.0000244 = $15.83/month
- Memory: 0.5 × 2,592,000 sec/month × $0.00000305 = $3.95/month
- **Total: ~$19.78/month**

**Why expensive:**
- Serverless pricing assumes scale-to-zero between requests
- This app needs `--min-scale 1` (24/7 background scraper + SQLite single-writer constraint)
- Essentially paying serverless prices for a long-running container

**True serverless option:**
Refactor to:
- Cron job for scraping (Code Engine jobs: $0.000012/vCPU-sec)
- Scale-to-zero web app
- Replace SQLite with PostgreSQL or object storage
- **Estimated cost: ~$0.44/month**

**Blockers:**
- Requires paid IBM Cloud account (credit card)
- Not worth the refactor for $0.44/month savings vs free alternatives

**Verdict:** ❌ Skip — too expensive for this workload

---

### 2. Oracle Cloud Free Tier ⭐ **RECOMMENDED**

**What you get (forever free):**
- **Option A:** 4 ARM Ampere A1 cores + 24GB RAM total (split across VMs as needed: 2×2core/12GB, 4×1core/6GB, etc.)
- **Option B:** 2 AMD VMs (1/8 OCPU, 1GB RAM each)
- 200GB block storage total
- 10TB outbound traffic/month

**Cost:** $0/month forever

**Caveats:**
- ARM instances highly contended (hard to provision, keep retrying)
- Oracle reclaims instances idle for 7+ days at 0% CPU → run a heartbeat cron to prevent
- No SLA on free tier, instances can be terminated with 7 days notice if Oracle needs capacity (rare)

**Best practices:**
- Use all allowed capacity (don't leave unused)
- Set heartbeat cron: `*/30 * * * * echo "$(date)" >> /tmp/heartbeat.log`
- Enable monitoring alerts

**Deployment:**
```bash
# SSH into Oracle VM
ssh ubuntu@<oracle-vm-ip>

# Install Docker
sudo apt update && sudo apt install -y docker.io
sudo usermod -aG docker ubuntu
newgrp docker

# Clone and run
git clone https://github.com/SahilMahale/pc-build.git
cd pc-build
docker build -t pc-price-server .
docker run -d -p 8090:8090 \
  -v ./config.yaml:/app/config.yaml:ro \
  -v ./data:/data \
  --restart unless-stopped \
  --name pc-price-server \
  pc-price-server
```

**Containerfile compatibility:**
- ✅ No changes needed — `golang:1.26-alpine` and `alpine:latest` have multi-arch images (ARM64 + AMD64)
- `CGO_ENABLED=0` creates static binary (arch-agnostic)

**Verdict:** ✅ **Best option** — actually free, generous specs, works out-of-the-box

---

### 3. Other Free Tier Options

#### Google Cloud Run
- 2M requests/month free
- 180k vCPU-seconds/month free
- ~$0-2/month for this workload (within free tier if scraping is infrequent)
- **Caveat:** Scale-to-zero breaks background scheduling (same issue as IBM)

#### Fly.io
- 3 shared-CPU VMs (256MB RAM each) free
- **Caveat:** 256MB RAM is tight (app uses ~50MB + SQLite + OS overhead)
- Would work but no room for growth

#### Render.com
- Free tier sleeps after 15min idle
- ❌ **Does not work** for background scraping

---

### 4. Cheap Paid Options ($1-5/month)

| Provider | Price | Specs | Notes |
|----------|-------|-------|-------|
| **Hetzner Cloud** | €4.15/month (~$4.50) | 1 vCPU, 2GB RAM, 20GB SSD | EU/US locations, best value paid option |
| **DigitalOcean** | $4/month | 512MB RAM | Referral gives $200 credit for 60 days |
| **Linode/Akamai** | $5/month | 1GB RAM | Reliable, good support |
| **Railway** | $5/month | 512MB RAM, 5GB storage | Easy deployment from GitHub |

**Verdict:** Only consider if Oracle free tier provisioning fails repeatedly

---

### 5. Run at Home (Free if hardware exists)

**Requirements:**
- Always-on device (PC, Raspberry Pi, NAS, etc.)
- Stable internet connection

**Pros:**
- $0/month (electricity negligible for low-power device)
- Full control, no provider limits
- Already running locally in development

**Cons:**
- Single point of failure (internet/power outage = downtime)
- Need to expose publicly (see "Public URL Options" below)

**Verdict:** ✅ Valid if you already have suitable hardware

---

## Public URL Options

### 1. Cloudflare Tunnel ⭐ **RECOMMENDED**

**Features:**
- Free, unlimited bandwidth
- Automatic HTTPS (Cloudflare cert)
- No port forwarding needed
- DDoS protection included
- Works from anywhere (home, cloud VM, behind NAT)

**Setup:**

```bash
# Install cloudflared (on Oracle ARM VM)
wget https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64
chmod +x cloudflared-linux-arm64
sudo mv cloudflared-linux-arm64 /usr/local/bin/cloudflared

# Authenticate (opens browser, one-time)
cloudflared tunnel login

# Create tunnel
cloudflared tunnel create pc-price-tracker

# Configure (replace <TUNNEL_ID> and hostname)
mkdir -p ~/.cloudflared
cat > ~/.cloudflared/config.yml <<EOF
tunnel: <TUNNEL_ID>
credentials-file: /home/ubuntu/.cloudflared/<TUNNEL_ID>.json

ingress:
  - hostname: pc-prices.yourdomain.com
    service: http://localhost:8090
  - service: http_status:404
EOF

# Route DNS
cloudflared tunnel route dns pc-price-tracker pc-prices.yourdomain.com

# Run tunnel
cloudflared tunnel run pc-price-tracker
```

**Make it persistent (systemd):**
```bash
sudo cloudflared service install
sudo systemctl enable --now cloudflared
```

**Quick start without a domain (temporary URL):**
```bash
cloudflared tunnel --url http://localhost:8090
```
Gives `https://random-word-1234.trycloudflare.com` (changes on restart, good for testing)

**Verdict:** ✅ **Best option** — free, secure, no domain required (or use custom domain)

---

### 2. Getting a Domain

#### Free subdomain providers:
- **DuckDNS** — `yourname.duckdns.org` (free, easy setup)
- **FreeDNS (afraid.org)** — `yourname.mooo.com` (many TLD options)
- **No-IP** — `yourname.ddns.net` (free dynamic DNS)

#### Cheap domain registrars:
- **Cloudflare Registrar** — $9-10/year `.com` (at-cost, no markup)
- **Porkbun** — $3/year `.xyz`, $10/year `.com`
- **Namecheap** — $8/year `.com` (first year promo)

**Recommendation:**
- Use **DuckDNS** for free subdomain (good enough for personal use)
- Or buy a `.xyz` from Porkbun for $3/year if you want a custom domain

---

## Final Recommendation

**Deployment stack:**
1. **Host:** Oracle Cloud Free Tier (ARM64 VM, 2 cores, 12GB RAM)
2. **Container:** Use existing Containerfile (no changes needed)
3. **Public URL:** Cloudflare Tunnel with DuckDNS subdomain (free)

**Total cost:** $0/month

**Deployment steps:**
1. Create Oracle Cloud free account
2. Provision ARM VM (keep retrying if capacity full)
3. SSH in, install Docker
4. Clone repo, build container, run with `--restart unless-stopped`
5. Set up heartbeat cron to prevent idle reclamation
6. Install cloudflared, create tunnel
7. Get free subdomain from DuckDNS
8. Point subdomain to Cloudflare Tunnel

**Result:** `https://pc-prices.duckdns.org` accessible worldwide, $0/month, HTTPS included.

---

## Architecture Notes

### Why SQLite requires single instance:
- SQLite uses file-level locking (no concurrent writes from multiple processes)
- Horizontal scaling (multiple containers) causes lock conflicts
- `--min-scale 1 --max-scale 1` ensures single writer

### If you need horizontal scaling later:
- Replace SQLite with PostgreSQL (Oracle free tier includes 20GB)
- Or use cloud object storage + separate scraper job (true serverless)

### Current resource usage:
- **RAM:** ~50MB (Go binary + SQLite in-memory cache)
- **CPU:** <5% average (spikes during scraping every 6h)
- **Storage:** <10MB (SQLite database + logs)
- **Bandwidth:** <100MB/month (20 scrape sources × 50KB each × 4/day × 30 days)

All well within Oracle free tier limits.
