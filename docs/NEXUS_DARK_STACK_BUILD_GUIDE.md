# NeXuS Dark Stack — Complete Build Guide

```
╔══════════════════════════════════════════════════════════════╗
║   N e X u S   D A R K   S T A C K                           ║
║   Frontend · Backend · Routing · Darknet Hosting            ║
║   Build from scratch — every layer, every service           ║
║                                                              ║
║   Sane · Simple · Secure · Stealthy · Beautiful             ║
║   Because working together everyone achieves MORE           ║
╚══════════════════════════════════════════════════════════════╝
```

> This guide builds the complete NeXuS dark stack from a fresh OS install.
> Follow phases in order. Each phase depends on the one before it.
> **Backup before every change. Archive, never delete.**

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Hardware & OS Requirements](#2-hardware--os-requirements)
3. [Phase 1 — OS Foundation & Security Hardening](#phase-1--os-foundation--security-hardening)
4. [Phase 2 — Container Runtime (Podman Rootless)](#phase-2--container-runtime-podman-rootless)
5. [Phase 3 — DNS (Unbound — No Leaks)](#phase-3--dns-unbound--no-leaks)
6. [Phase 4 — Ghost Gate (nftables Egress Firewall)](#phase-4--ghost-gate-nftables-egress-firewall)
7. [Phase 5 — Transport Layer (Tor + I2P + Yggdrasil + Reticulum)](#phase-5--transport-layer)
8. [Phase 6 — Medusa Routing (HAProxy + Privoxy)](#phase-6--medusa-routing-haproxy--privoxy)
9. [Phase 7 — Backend (char-gen-cli Go Service)](#phase-7--backend-char-gen-cli-go-service)
10. [Phase 8 — Application Services](#phase-8--application-services)
11. [Phase 9 — Darknet Hosting](#phase-9--darknet-hosting)
12. [Phase 10 — Monitoring (Prometheus + Grafana + OpenSnitch)](#phase-10--monitoring)
13. [Phase 11 — Control Plane (Hydra Web UI + TUI)](#phase-11--control-plane)
14. [Phase 12 — Key Management (Cerberus Protocol)](#phase-12--key-management-cerberus-protocol)
15. [Phase 13 — Startup & Persistence](#phase-13--startup--persistence)
16. [Port Reference](#port-reference)
17. [Jurisdiction Profiles](#jurisdiction-profiles)
18. [Troubleshooting](#troubleshooting)

---

## 1. Architecture Overview

```
USER / APPLICATION
  char-gen-cli TUI · Browser · WeeChat · RetroShare · DIVA Node
        │
CONTROL PLANE
  nexus-hydra (Python/FastAPI — web :8800, API :8801)
  nexus-darknet (Go TUI — 5 tabs)
        │
MONITORING
  Prometheus :9090 → Grafana dashboards
  HAProxy Stats :8404    OpenSnitch :50051
        │
EGRESS GATE (Ghost Gate — nftables)
  Default POLICY DROP · Cerberus 3-head verification
  Denied → randomized noise sink (not clean drop)
        │
FILTER & PRIVACY
  Privoxy :8118 (ad/tracker block, header sanitize)
  OpenSnitch (per-process kill switch)
        │
MEDUSA SMART ROUTING (HAProxy)
  SOCKS5 :9050 (smart) / :1080 (direct)
  HTTP proxy :8888
  *.onion → Tor   *.i2p → I2P   *.ygg → Yggdrasil   other → profile
        │
TRANSPORT LAYER
  ┌────────────┬────────────┬──────────────┬──────────────┐
  │  Tor       │  I2P       │  Yggdrasil   │  Reticulum   │
  │  6 circuits│  i2pd C++  │  IPv6 mesh   │  LoRa/radio  │
  │  :9050/    │  :4444/    │  200::/7     │  delay-tol.  │
  │   9150     │   4447/    │              │              │
  │  DNS:8853  │   7070/    │              │              │
  │            │   7656 SAM │              │              │
  └────────────┴────────────┴──────────────┴──────────────┘
        │
THE INTERNET / DARKNET / MESH / RADIO
```

---

## 2. Hardware & OS Requirements

### Minimum
| | Minimum | Recommended |
|---|---|---|
| RAM | 2 GB | 8 GB (Ollama needs 4+ GB alone) |
| Disk | 10 GB | 50 GB (IPFS + DIVA grow) |
| CPU | 2 cores | 4 cores |
| Network | Any | Broadband, static IP helps |
| OS | Any Linux | Alpine Edge or Debian Stable |

### Preferred OS: Alpine Linux Edge
```bash
# Verify
cat /etc/alpine-release   # should be 3.20+
uname -r                  # 6.x kernel recommended
```

### Required base packages
```bash
apk add \
  curl wget git make gcc musl-dev \
  nftables iptables ip6tables \
  unbound \
  tor \
  podman podman-compose \
  go \
  python3 py3-pip py3-venv \
  openssl \
  htop tmux zsh fzf bat ripgrep fd \
  micro glow                          # editor + markdown viewer
```

---

## Phase 1 — OS Foundation & Security Hardening

### 1.1 Create the NeXuS user (if not root)
```bash
adduser -D nexus
addgroup nexus wheel
# Give passwordless doas
echo "permit nopass :wheel" > /etc/doas.d/wheel.conf
```

### 1.2 SSH hardening
```bash
# /etc/ssh/sshd_config
PermitRootLogin no
PasswordAuthentication no
AllowUsers nexus
Port 2222                # change from default 22
```

### 1.3 fail2ban
```bash
apk add fail2ban
# /etc/fail2ban/jail.local
[sshd]
enabled  = true
port     = 2222
logpath  = /var/log/messages
maxretry = 5
bantime  = 3600

rc-service fail2ban start
rc-update add fail2ban
```

### 1.4 PSAD (port scan detection)
```bash
apk add psad
# /etc/psad/psad.conf
ENABLE_SYSLOG_FILE    Y;
IPT_SYSLOG_FILE       /var/log/messages;
IPT_PREFIX            [UFW BLOCK];

# Enable klogd (psad needs kernel logs)
rc-update add klogd
rc-service klogd start
rc-service psad start
rc-update add psad
```

### 1.5 fwsnort (Snort rules via iptables)
```bash
apk add fwsnort snort
fwsnort --update
fwsnort
# Strip bad rules for nftables compat:
grep -v '! --dport\|! --sport' /etc/fwsnort/fwsnort.sh > /etc/fwsnort/fwsnort-clean.sh
chmod +x /etc/fwsnort/fwsnort-clean.sh
/etc/fwsnort/fwsnort-clean.sh

# Persist: /etc/local.d/fwsnort.start
#!/bin/sh
/etc/fwsnort/fwsnort-clean.sh
chmod +x /etc/local.d/fwsnort.start
```

### 1.6 AppArmor
```bash
apk add apparmor apparmor-profiles
# Add to /boot/extlinux.conf APPEND line:
#   lsm=landlock,yama,apparmor
rc-update add apparmor boot
```

### 1.7 Kernel parameters
```bash
# /etc/sysctl.d/99-nexus.conf
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.log_martians = 1
kernel.dmesg_restrict = 1
kernel.kptr_restrict = 2

sysctl -p /etc/sysctl.d/99-nexus.conf
```

---

## Phase 2 — Container Runtime (Podman Rootless)

Podman runs all darknet services in isolated containers with no root daemon.

```bash
apk add podman podman-compose slirp4netns fuse-overlayfs

# Enable user namespaces
echo "nexus:100000:65536" >> /etc/subuid
echo "nexus:100000:65536" >> /etc/subgid

# As nexus user:
podman system reset     # clean slate
podman info             # verify rootless mode

# Create the NeXuS network
podman network create nexus-darknet \
  --subnet 172.20.0.0/24 \
  --gateway 172.20.0.1

# Create I2P-specific network (DIVA needs separate subnet)
podman network create nexus-i2p-net \
  --subnet 172.19.75.0/24 \
  --gateway 172.19.75.1
```

### Directory structure
```bash
mkdir -p ~/nexus/{configs,logs,data,keys,workspaces}
mkdir -p ~/nexus/configs/{tor,i2p,haproxy,privoxy,prometheus,grafana}
mkdir -p ~/nexus/data/{ipfs,ollama,retroshare,matrix,diva}
```

---

## Phase 3 — DNS (Unbound — No Leaks)

All DNS goes through Unbound → Tor. Zero cleartext DNS.

```bash
apk add unbound

# /etc/unbound/unbound.conf
server:
  interface: 127.0.0.1
  port: 5353
  do-ip4: yes
  do-udp: yes
  access-control: 127.0.0.0/8 allow
  hide-identity: yes
  hide-version: yes
  harden-glue: yes
  harden-dnssec-stripped: yes
  use-caps-for-id: yes
  prefetch: yes

  # Block .onion and .i2p at DNS — handled by proxy
  local-zone: "onion." static
  local-zone: "i2p."   static

forward-zone:
  name: "."
  forward-addr: 127.0.0.1@8853    # Tor DNS port

rc-service unbound start
rc-update add unbound

# Point resolv.conf at Unbound
echo "nameserver 127.0.0.1" > /etc/resolv.conf
chattr +i /etc/resolv.conf    # lock it — prevents NetworkManager from overwriting
```

---

## Phase 4 — Ghost Gate (nftables Egress Firewall)

Default deny outbound. Only permitted transports (Tor/I2P/Yggdrasil) are allowed through.

```bash
# /etc/nftables.d/ghost-gate.nft
table inet ghost_gate {

  chain input {
    type filter hook input priority 0; policy drop;
    ct state established,related accept
    iif lo accept
    tcp dport 2222 accept    # SSH
    drop
  }

  chain forward {
    type filter hook forward priority 0; policy drop;
  }

  chain output {
    type filter hook output priority 0; policy drop;

    # Loopback always allowed
    oif lo accept

    # Established connections
    ct state established,related accept

    # Tor daemon outbound (runs as _tor user)
    meta skuid _tor accept

    # I2P daemon
    meta skuid i2pd accept

    # Yggdrasil
    meta skuid yggdrasil accept

    # DNS to Unbound (localhost only)
    udp dport 53  ip daddr 127.0.0.1 accept
    tcp dport 53  ip daddr 127.0.0.1 accept

    # NTP (time sync — required for Tor)
    udp dport 123 accept

    # Deny everything else — noise sink (randomized delay, not clean drop)
    # Prevents timing fingerprinting on drops
    limit rate 1/second drop
    drop
  }
}

# Load on boot
rc-update add nftables
nft -f /etc/nftables.d/ghost-gate.nft
```

---

## Phase 5 — Transport Layer

### 5.1 Tor (6-circuit pool)

```bash
apk add tor

# /etc/tor/torrc
DataDirectory /var/lib/tor
PidFile /var/run/tor/tor.pid

SocksPort 9050
SocksPort 9150    # circuit 2
DNSPort  8853
ControlPort 9051

# We are a MIDDLE RELAY only — no exit
ExitPolicy reject *:*
ORPort 9001
Nickname nexus-relay-01

# Circuit isolation per destination
IsolateDestAddr 1
IsolateDestPort 1

# No logging of circuits
Log notice file /var/log/tor/notices.log
SafeLogging 1

rc-service tor start
rc-update add tor
```

Run additional Tor circuits (2-6) in Podman for pool diversity:
```bash
for i in 02 03 04 05 06; do
  podman run -d \
    --name nexus-tor-$i \
    --network nexus-darknet \
    -v ~/nexus/data/tor-$i:/var/lib/tor:Z \
    osminogin/tor-simple
done
```

### 5.2 I2P (i2pd C++)

```bash
apk add i2pd

# /etc/i2pd/i2pd.conf
[general]
host = auto
port = 4567
ipv6 = false
loglevel = warn
datadir = /var/lib/i2pd

[http]
enabled = true
address = 127.0.0.1
port = 7070

[httpproxy]
enabled = true
address = 127.0.0.1
port = 4444

[socksproxy]
enabled = true
address = 127.0.0.1
port = 4447

[sam]
enabled = true
address = 127.0.0.1
port = 7656        # DIVA chain connects here

[reseed]
verify = true
```

```bash
# /etc/i2pd/tunnels.conf — hidden services + client tunnels
[retroshare-server]
type = server
host = 127.0.0.1
port = 7812
inport = 7812
accesslist = <your_retroshare_i2p_dest>

[irc-client]
type = client
address = 127.0.0.1
port = 6668
destination = irc.dg.i2p
destinationport = 6667

[nexus-service]
type = server
host = 127.0.0.1
port = 8080        # char-gen-cli API (if exposing via I2P)
inport = 80
```

```bash
rc-service i2pd start
rc-update add i2pd

# Wait 10-30 min for I2P to bootstrap tunnels
# Monitor: http://127.0.0.1:7070
```

### 5.3 Yggdrasil (IPv6 mesh overlay)

```bash
apk add yggdrasil

# Generate config
yggdrasil -genconf > ~/nexus/configs/yggdrasil.conf

# Edit ~/nexus/configs/yggdrasil.conf:
# Peers: add public peers from https://publicpeers.neilalexander.dev/
# Listen: tcp://0.0.0.0:9001
# TunName: ygg0

cp ~/nexus/configs/yggdrasil.conf /etc/yggdrasil/yggdrasil.conf
rc-service yggdrasil start
rc-update add yggdrasil

# Get your Yggdrasil address (200::/7 range)
ip addr show ygg0
```

### 5.4 Reticulum (mesh, LoRa-capable)

```bash
pip3 install rns

# ~/.reticulum/config
[interfaces]
  [[Default Interface]]
    type = TCPClientInterface
    enabled = yes
    target_host = reticulum.betweentheborders.com
    target_port = 4965

# Start daemon
rnsd --daemon
```

---

## Phase 6 — Medusa Routing (HAProxy + Privoxy)

### 6.1 HAProxy smart SOCKS router

```bash
apk add haproxy

# ~/nexus/configs/haproxy/haproxy.cfg
global
  log /dev/log local0
  stats socket /var/run/haproxy/admin.sock mode 660 level admin
  daemon

defaults
  log global
  timeout connect 10s
  timeout client  60s
  timeout server  60s

# ── Stats & Prometheus ──────────────────────────────────────
frontend stats
  bind 127.0.0.1:8404
  stats enable
  stats uri /stats
  stats refresh 5s
  http-request use-service prometheus-exporter if { path /metrics }

# ── Smart SOCKS5 Router :9050 ────────────────────────────────
frontend socks_smart
  bind 127.0.0.1:9050
  mode tcp
  use_backend tor_pool   if { req.ssl_sni -m end .onion }
  use_backend i2p_proxy  if { req.ssl_sni -m end .i2p }
  default_backend medusa_pool

# ── Direct Medusa SOCKS :1080 ────────────────────────────────
frontend socks_direct
  bind 127.0.0.1:1080
  mode tcp
  default_backend medusa_pool

# ── HTTP Proxy :8888 ─────────────────────────────────────────
frontend http_proxy
  bind 127.0.0.1:8888
  mode http
  default_backend privoxy_backend

# ── Backends ─────────────────────────────────────────────────
backend tor_pool
  mode tcp
  balance leastconn
  server tor1 127.0.0.1:9050  check
  server tor2 172.20.0.11:9050 check   # container circuit 2
  server tor3 172.20.0.12:9050 check
  server tor4 172.20.0.13:9050 check
  server tor5 172.20.0.14:9050 check
  server tor6 172.20.0.15:9050 check

backend i2p_proxy
  mode tcp
  server i2p 127.0.0.1:4447 check

backend medusa_pool
  mode tcp
  balance roundrobin
  server medusa1 127.0.0.1:9050 check
  server medusa2 127.0.0.1:9150 check

backend privoxy_backend
  mode http
  server privoxy 127.0.0.1:8118 check

rc-service haproxy start
rc-update add haproxy
```

### 6.2 Privoxy (filter, ad block, header sanitize)

```bash
apk add privoxy

# /etc/privoxy/config
listen-address  127.0.0.1:8118
toggle          1
enable-remote-toggle 0
enable-remote-http-toggle 0
enable-edit-actions 0

# Forward .onion → Tor SOCKS
forward-socks5  .onion  127.0.0.1:9050 .

# Forward .i2p → I2P proxy
forward         .i2p    127.0.0.1:4444

# Everything else → HAProxy smart router
forward-socks5  /       127.0.0.1:9050 .

# Strip identifying headers
hide-referrer  block
hide-user-agent  "Mozilla/5.0 (Windows NT 10.0; rv:102.0) Gecko/20100101 Firefox/102.0"

actionsfile  /etc/privoxy/nexus-user.action
filterfile   /etc/privoxy/nexus-ublock.filter

rc-service privoxy start
rc-update add privoxy
```

```bash
# /etc/privoxy/nexus-user.action — block trackers, strip utm params
{+block{Google Analytics}}
.google-analytics.com
.googletagmanager.com
stats.g.doubleclick.net

{+filter{nexus-strip-utm}}
/
```

---

## Phase 7 — Backend (char-gen-cli Go Service)

The character generator is the NeXuS AI backend — local LLM, encrypted character storage, and a REST API server for external connections.

### 7.1 Build

```bash
cd ~/claude/char-gen-cli
go build -o nexus-char-gen .

# Install
cp nexus-char-gen ~/bin/nexus-char-gen
```

### 7.2 Configure via .env

```bash
cp .env.example .env
micro .env    # fill in keys
```

Key settings for dark stack operation:
```bash
# .env
DEFAULT_AI_SERVICE=Ollama        # local AI — zero clearnet
NEXUS_OLLAMA_URL=http://127.0.0.1:11434
NEXUS_OLLAMA_MODEL=llama3.2

# Disable clearnet features
ENABLE_GOOGLE_SEARCH=false       # use /search only via Tor
ENABLE_TERMINAL_ACCESS=false     # enable per-user if needed

# Log at info level, verbose in dev
LOG_LEVEL=info
DEFAULT_THEME=dark
```

### 7.3 API server (for darknet exposure)

The API server (`api_server.go`) listens on localhost only. We expose it via hidden service in Phase 9.

```bash
# Start — binds to 127.0.0.1:8080 by default
nexus-char-gen --server &

# Verify
curl http://127.0.0.1:8080/health
```

### 7.4 Systemd-style init (OpenRC)

```bash
# /etc/init.d/nexus-char-gen
#!/sbin/openrc-run
name="NeXuS Character Gen"
command="/home/nexus/bin/nexus-char-gen"
command_args="--server"
command_user="nexus"
pidfile="/run/nexus-char-gen.pid"
command_background=yes

depend() {
  need net tor i2pd
}

chmod +x /etc/init.d/nexus-char-gen
rc-update add nexus-char-gen
```

---

## Phase 8 — Application Services

### 8.1 Ollama (local AI — zero clearnet)

```bash
# Alpine: install from binary (apk version may lag)
curl -fsSL https://ollama.com/install.sh | sh

# Or manual:
wget https://github.com/ollama/ollama/releases/latest/download/ollama-linux-amd64
chmod +x ollama-linux-amd64
mv ollama-linux-amd64 /usr/local/bin/ollama

# Start daemon (listens on :11434 localhost only)
ollama serve &

# Pull models
ollama pull llama3.2           # general purpose (~2GB)
ollama pull qwen2.5-coder:1.5b # code tasks (~1GB)
ollama pull mistral             # alternative LLM (~4GB)

# List available
ollama list
```

### 8.2 IPFS (distributed content storage)

```bash
apk add ipfs

# Initialize
ipfs init --profile server

# /etc/ipfs/config (key sections)
# Disable gateway on public interface
"Addresses": {
  "API": "/ip4/127.0.0.1/tcp/5001",
  "Gateway": "/ip4/127.0.0.1/tcp/8080",
  "Swarm": ["/ip4/0.0.0.0/tcp/4001"]
}

# Route IPFS through Tor
ipfs config --json Swarm.Transports.Network.Relay true

rc-service ipfs start
rc-update add ipfs
```

### 8.3 RetroShare (F2F social + fileshare)

```bash
# Run in container via Podman — no native Alpine package
podman run -d \
  --name nexus-retroshare \
  --network nexus-darknet \
  -p 127.0.0.1:7812:7812 \
  -v ~/nexus/data/retroshare:/data:Z \
  retroshare/retroshare-nogui:latest

# RetroShare will get its own Tor hidden service (Phase 9)
```

### 8.4 IRC via WeeChat (I2P native)

```bash
apk add weechat weechat-python

# Start WeeChat in a tmux window
tmux new-window -n irc "weechat"

# Inside WeeChat:
# /server add i2p-irc 127.0.0.1/6668 -ssl_verify=off
# /connect i2p-irc
# (tunnels.conf already routes :6668 → irc.dg.i2p via I2P)
```

### 8.5 Matrix/Synapse (self-hosted, over Tor)

```bash
podman run -d \
  --name nexus-matrix \
  --network nexus-darknet \
  -p 127.0.0.1:8008:8008 \
  -v ~/nexus/data/matrix:/data:Z \
  -e SYNAPSE_SERVER_NAME=nexus.onion \
  -e SYNAPSE_REPORT_STATS=no \
  matrixdotorg/synapse:latest generate

podman run -d \
  --name nexus-matrix \
  --network nexus-darknet \
  -p 127.0.0.1:8008:8008 \
  -v ~/nexus/data/matrix:/data:Z \
  matrixdotorg/synapse:latest
```

### 8.6 DIVA Node (I2P blockchain)

```bash
# Requires glibc — run in Debian container
cat > ~/nexus/configs/diva/Dockerfile.diva << 'EOF'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y nodejs npm curl
WORKDIR /diva
COPY . .
RUN npm install
EXPOSE 3001
CMD ["node", "main.js"]
EOF

podman run -d \
  --name nexus-diva \
  --network nexus-i2p-net \
  -p 127.0.0.1:3001:3001 \
  -v ~/Projects/nexus-node/divachain:/diva:Z \
  -e I2P_SAM_HOST=172.19.75.1 \
  -e I2P_SAM_PORT=7656 \
  nexus-diva-dev:latest

# Wait 10-30 min for I2P tunnels to bootstrap
# Test: curl http://127.0.0.1:3001/api/status
```

---

## Phase 9 — Darknet Hosting

### 9.1 Tor Hidden Services

Add to `/etc/tor/torrc`:
```
# char-gen-cli API
HiddenServiceDir /var/lib/tor/nexus-api/
HiddenServicePort 80 127.0.0.1:8080

# Matrix homeserver
HiddenServiceDir /var/lib/tor/nexus-matrix/
HiddenServicePort 443 127.0.0.1:8008

# RetroShare
HiddenServiceDir /var/lib/tor/nexus-retroshare/
HiddenServicePort 7812 127.0.0.1:7812

# Hydra control panel
HiddenServiceDir /var/lib/tor/nexus-hydra/
HiddenServicePort 80 127.0.0.1:8800
```

```bash
# Reload Tor to generate .onion addresses
kill -SIGHUP $(pidof tor)

# Read your .onion addresses
cat /var/lib/tor/nexus-api/hostname
cat /var/lib/tor/nexus-matrix/hostname
cat /var/lib/tor/nexus-retroshare/hostname
cat /var/lib/tor/nexus-hydra/hostname
```

### 9.2 I2P Eepsite (static content + API)

Add to `/etc/i2pd/tunnels.conf`:
```ini
[nexus-api-i2p]
type    = server
host    = 127.0.0.1
port    = 8080
inport  = 80
keys    = nexus-api.dat          # persists your .b32 address

[nexus-hydra-i2p]
type    = server
host    = 127.0.0.1
port    = 8800
inport  = 80
keys    = nexus-hydra.dat
```

```bash
# Reload i2pd
rc-service i2pd reload

# After bootstrap, get your .b32 address:
# http://127.0.0.1:7070 → Local destinations → nexus-api.dat
```

### 9.3 IPFS static frontend hosting

```bash
# Build your static frontend (if applicable)
# Then add to IPFS:
ipfs add -r ~/nexus/frontend/dist/
# Returns: QmHash...

# Pin it
ipfs pin add QmHash...

# Access via:
# http://127.0.0.1:8080/ipfs/QmHash   (local)
# https://ipfs.io/ipfs/QmHash          (public gateway — use over Tor)
```

### 9.4 Yggdrasil endpoint

Your Yggdrasil `200::/7` address is reachable by any other Yggdrasil node worldwide — no Tor/I2P needed. Expose services:

```bash
# Bind services to Yggdrasil interface
# In service configs, listen on your ygg0 address:
ip addr show ygg0 | grep inet6   # note your 200:: address

# Example: char-gen API on Yggdrasil
nexus-char-gen --server --bind [200::your:ygg:addr]:8080
```

---

## Phase 10 — Monitoring

### 10.1 Prometheus

```bash
podman run -d \
  --name nexus-prometheus \
  --network nexus-darknet \
  -p 127.0.0.1:9090:9090 \
  -v ~/nexus/configs/prometheus:/etc/prometheus:Z \
  prom/prometheus

# ~/nexus/configs/prometheus/prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: haproxy
    static_configs:
      - targets: ['172.20.0.1:8404']
    metrics_path: /metrics

  - job_name: node
    static_configs:
      - targets: ['172.20.0.1:9100']
```

### 10.2 Grafana

```bash
podman run -d \
  --name nexus-grafana \
  --network nexus-darknet \
  -p 127.0.0.1:3000:3000 \
  -v ~/nexus/data/grafana:/var/lib/grafana:Z \
  -e GF_AUTH_ANONYMOUS_ENABLED=false \
  -e GF_SERVER_ROOT_URL="http://127.0.0.1:3000" \
  grafana/grafana

# Access: http://127.0.0.1:3000
# Add Prometheus datasource: http://172.20.0.x:9090
```

### 10.3 OpenSnitch (per-process outbound firewall)

```bash
# Arch/Nix: opensnitchd package
# Alpine: build from source or Nix
nix-env -i opensnitch

# Start daemon
opensnitchd &

# Rules: allow Tor, I2P, our services — alert everything else
# /etc/opensnitchd/rules/nexus-allow-tor.json
{
  "name": "allow-tor",
  "enabled": true,
  "action": "allow",
  "duration": "always",
  "operator": {
    "type": "simple",
    "sensitive": false,
    "operand": "process.path",
    "data": "/usr/bin/tor"
  }
}
```

---

## Phase 11 — Control Plane

### 11.1 nexus-hydra (Python/FastAPI web control panel)

```bash
git clone https://github.com/your-org/nexus-hydra ~/git/nexus-hydra
cd ~/git/nexus-hydra

python3 -m venv .venv
source .venv/bin/activate
pip install fastapi uvicorn websockets pyyaml psutil podman-py

# Start
uvicorn nexus_hydra.main:app --host 127.0.0.1 --port 8800 &

# Access: http://127.0.0.1:8800
# REST API: http://127.0.0.1:8801
```

Key web UI tabs:
- **Networks** — start/stop Tor, I2P, Yggdrasil, Reticulum
- **Services** — manage all application containers
- **Hosting** — expose/hide services as hidden services (.onion / .b32.i2p)
- **Jurisdiction** — switch Medusa routing profile (World / No-5-Eyes / No-14-Eyes / Custom)
- **Log** — live log feed via WebSocket

### 11.2 nexus-darknet (Go TUI)

```bash
cd ~/git/nexus-darknet
go build -o nexus-darknet .
./nexus-darknet
```

Same 5-tab control surface as the web UI, but keyboard-driven in the terminal.

---

## Phase 12 — Key Management (Cerberus Protocol)

The Cerberus framework requires **all 3 heads to pass simultaneously**. Miss one = hard stop.

```
HEAD_1: Something you know   → Cerberus password (human KDF: y+x, z, m)
HEAD_2: Something you have   → Cold master key (USB, offline)
HEAD_3: Something you are    → AI fingerprint (behavioral, not biometric)
```

### 12.1 Cold master key (BIP32)
```bash
# Generate on air-gapped machine
python3 -c "
from mnemonic import Mnemonic
m = Mnemonic('english')
words = m.generate(strength=256)
print(words)
" > /tmp/nexus-master.txt

# Copy to encrypted USB
# Store USB offline — never connect to internet
# Derive hot session keys from master using BIP32
```

### 12.2 Cerberus password (human KDF)
```
Three variables: y (something personal), x (a number you remember), m (a word)
Derivation: hash(y + x) || hash(z) || hash(m || salt)
Combine: concatenate → run through Argon2id (time=3, mem=64MB)
Result: 32-byte key material

DO NOT write the variables down.
DO NOT store digitally.
The three variables live only in your head.
```

### 12.3 Hot session keys (rotating)
```bash
# On startup, derive session key from cold master
# Session key expires after N hours or N operations
# Never touches disk — lives only in RAM
# On crash: key is gone, re-derive from Cerberus inputs

# Implemented in nexus_crypto.go (char-gen-cli)
# See: ~/claude/char-gen-cli/nexus_crypto.go
```

### 12.4 Encrypted data store
```bash
# All user data encrypted with session key (AES-256-GCM)
# Key stored in tmpfs (never hits swap)
mount -t tmpfs -o size=64m tmpfs /run/nexus-keys/
chmod 700 /run/nexus-keys/
```

---

## Phase 13 — Startup & Persistence

### Single launch script
```bash
# ~/scripts/nexus-start.sh
#!/bin/sh
set -e

echo "[nexus] Starting Ghost Gate..."
nft -f /etc/nftables.d/ghost-gate.nft

echo "[nexus] Starting DNS..."
rc-service unbound start

echo "[nexus] Starting Tor..."
rc-service tor start

echo "[nexus] Starting I2P..."
rc-service i2pd start
echo "[nexus] Waiting 60s for I2P bootstrap..."
sleep 60

echo "[nexus] Starting Yggdrasil..."
rc-service yggdrasil start

echo "[nexus] Starting HAProxy (Medusa)..."
rc-service haproxy start

echo "[nexus] Starting Privoxy..."
rc-service privoxy start

echo "[nexus] Starting Ollama..."
ollama serve &

echo "[nexus] Starting IPFS..."
rc-service ipfs start

echo "[nexus] Starting application containers..."
cd ~/nexus
podman-compose up -d

echo "[nexus] Starting Hydra control panel..."
cd ~/git/nexus-hydra && source .venv/bin/activate
uvicorn nexus_hydra.main:app --host 127.0.0.1 --port 8800 &

echo "[nexus] Starting char-gen backend..."
nexus-char-gen --server &

echo ""
echo "[nexus] Stack is up."
echo "  Hydra web:    http://127.0.0.1:8800"
echo "  HAProxy stats: http://127.0.0.1:8404/stats"
echo "  I2P console:  http://127.0.0.1:7070"
echo "  Prometheus:   http://127.0.0.1:9090"
echo "  Grafana:      http://127.0.0.1:3000"
echo "  Ollama:       http://127.0.0.1:11434"
echo ""
echo "  .onion addresses:"
cat /var/lib/tor/nexus-api/hostname
cat /var/lib/tor/nexus-hydra/hostname
```

```bash
chmod +x ~/scripts/nexus-start.sh

# Add to OpenRC local
echo "~/scripts/nexus-start.sh" >> /etc/local.d/nexus.start
chmod +x /etc/local.d/nexus.start
rc-update add local
```

---

## Port Reference

| Port | Service | Interface | Protocol |
|------|---------|-----------|----------|
| 1080 | Medusa SOCKS5 (direct) | 127.0.0.1 | SOCKS5 |
| 2222 | SSH | 0.0.0.0 | TCP |
| 3000 | Grafana | 127.0.0.1 | HTTP |
| 3001 | DIVA Node API | 127.0.0.1 | HTTP |
| 4001 | IPFS swarm | 0.0.0.0 | TCP/UDP |
| 4444 | I2P HTTP proxy | 127.0.0.1 | HTTP |
| 4447 | I2P SOCKS | 127.0.0.1 | SOCKS5 |
| 4567 | I2P router | 0.0.0.0 | UDP |
| 5001 | IPFS API | 127.0.0.1 | HTTP |
| 5353 | Unbound DNS | 127.0.0.1 | UDP/TCP |
| 6668 | IRC client (→ I2P tunnel) | 127.0.0.1 | TCP |
| 7070 | I2P web console | 127.0.0.1 | HTTP |
| 7656 | I2P SAM bridge | 127.0.0.1 | TCP |
| 7812 | RetroShare | 127.0.0.1 | TCP |
| 8008 | Matrix/Synapse | 127.0.0.1 | HTTP |
| 8080 | char-gen-cli API | 127.0.0.1 | HTTP |
| 8118 | Privoxy | 127.0.0.1 | HTTP |
| 8404 | HAProxy stats + Prometheus | 127.0.0.1 | HTTP |
| 8800 | Hydra web UI | 127.0.0.1 | HTTP |
| 8801 | Hydra REST API | 127.0.0.1 | HTTP |
| 8853 | Tor DNS | 127.0.0.1 | UDP |
| 8888 | Medusa HTTP proxy | 127.0.0.1 | HTTP |
| 9001 | Tor ORPort (relay) | 0.0.0.0 | TCP |
| 9050 | HAProxy smart SOCKS / Tor | 127.0.0.1 | SOCKS5 |
| 9090 | Prometheus | 127.0.0.1 | HTTP |
| 9150 | Tor circuit 2 | 127.0.0.1 | SOCKS5 |
| 11434 | Ollama | 127.0.0.1 | HTTP |
| 50051 | OpenSnitch gRPC | 127.0.0.1 | gRPC |

---

## Jurisdiction Profiles

Switch via Hydra UI tab 4, or directly:

```bash
# World — no restrictions
nexus-darknet-router.sh profile world

# Exclude 5-Eyes (US GB CA AU NZ)
nexus-darknet-router.sh profile no-5

# Exclude 9-Eyes (+ FR DK NL NO)
nexus-darknet-router.sh profile no-9

# Exclude 14-Eyes (+ DE BE IT SE ES)
nexus-darknet-router.sh profile no-14

# Custom — your own list
nexus-darknet-router.sh profile custom "US,GB,DE,CN,RU"
```

Under the hood this edits `/etc/tor/torrc`:
```
ExcludeExitNodes {US},{GB},{CA},{AU},{NZ}
ExcludeNodes     {US},{GB},{CA},{AU},{NZ}
```
Then sends `kill -SIGHUP $(pidof tor)` to hot-reload without dropping circuits.

---

## Troubleshooting

### No connectivity after Ghost Gate
```bash
nft list ruleset              # check rules loaded
# Verify your user's service (e.g. Tor) has meta skuid rule
id _tor                       # confirm _tor user exists
```

### I2P never bootstraps
```bash
# Check i2pd log
journalctl -u i2pd -f
# I2P needs ~10-30 min on first run — wait
# Check firewall: UDP 4567 must reach internet (via Tor meta skuid rule)
```

### Tor onion address changes on restart
```bash
# Protect your HiddenServiceDir — never delete it
# Back up /var/lib/tor/nexus-api/ (contains private_key)
tar czf ~/nexus/keys/tor-hidden-services.tar.gz /var/lib/tor/nexus-*/
# Restore on new node: extract and tor will use same keys → same address
```

### Ollama too slow (low RAM)
```bash
# Use smaller models
ollama pull qwen2.5-coder:1.5b   # 1GB
ollama pull smollm:135m           # 90MB — fast but limited
# Or: use Groq (clearnet — goes through Tor)
```

### DIVA node crashes immediately
```bash
# Check for leftover LevelDB LOCK files
find ~/Projects/nexus-node/divachain/test/data/ -name LOCK -delete
# Restart: I2P must be fully bootstrapped first
```

### HAProxy can't reach Tor circuits
```bash
haproxy -c -f ~/nexus/configs/haproxy/haproxy.cfg   # check config
rc-service haproxy reload
# Check that container IPs match haproxy.cfg backend IPs
podman inspect nexus-tor-02 | grep IPAddress
```

---

*NeXuS — Sane · Simple · Secure · Stealthy · Beautiful*
*meWEwowow — the ripple that starts when one node lights up*
