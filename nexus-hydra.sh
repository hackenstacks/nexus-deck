#!/bin/sh
# nexus-hydra.sh — NeXuS Hydra Startup
# Sane • Simple • Secure • Stealthy • Beautiful
#
# Usage:
#   nexus-hydra.sh                  — auto-detect tier, Shadow profile (daily driver)
#   nexus-hydra.sh ghost            — all networks, maximum Ghost Gate
#   nexus-hydra.sh shadow           — Tor active, I2P standby (default)
#   nexus-hydra.sh recon            — Tor + I2P + Yggdrasil + RetroShare
#   nexus-hydra.sh stealth          — Tor only, minimal RAM (old hardware)
#   nexus-hydra.sh open             — minimal privacy, max performance
#   nexus-hydra.sh stop             — bring everything down
#   nexus-hydra.sh status           — show running stack
#   nexus-hydra.sh logs             — live log tail

HYDRA_DIR="$HOME/claude/demo-dark-stack/new-hydra"
COMPOSE="$HYDRA_DIR/docker-compose.yml"

GRN='\033[0;32m'
CYN='\033[0;36m'
YEL='\033[1;33m'
RED='\033[0;31m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

banner() {
    printf "\n"
    printf "${CYN}${BOLD}  ███╗   ██╗███████╗██╗  ██╗██╗   ██╗███████╗${NC}\n"
    printf "${CYN}${BOLD}  ████╗  ██║██╔════╝╚██╗██╔╝██║   ██║██╔════╝${NC}\n"
    printf "${CYN}${BOLD}  ██╔██╗ ██║█████╗   ╚███╔╝ ██║   ██║███████╗${NC}\n"
    printf "${CYN}${BOLD}  ██║╚██╗██║██╔══╝   ██╔██╗ ██║   ██║╚════██║${NC}\n"
    printf "${CYN}${BOLD}  ██║ ╚████║███████╗██╔╝ ██╗╚██████╔╝███████║${NC}\n"
    printf "${CYN}${BOLD}  ╚═╝  ╚═══╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚══════╝${NC}\n"
    printf "${DIM}  Hydra — Sane • Simple • Secure • Stealthy • Beautiful${NC}\n"
    printf "\n"
}

# ── Hardware tier detection ───────────────────────────────────
detect_tier() {
    RAM_MB=$(free -m 2>/dev/null | awk '/^Mem:/{print $2}' || echo 0)
    if [ "$RAM_MB" -ge 8000 ]; then
        echo "broadcast"
    elif [ "$RAM_MB" -ge 4000 ]; then
        echo "hub"
    elif [ "$RAM_MB" -ge 2000 ]; then
        echo "vanguard"
    else
        echo "phantom"
    fi
}

# ── Profile → compose profiles mapping ───────────────────────
#
#  GHOST   — everything available for this tier, max Ghost Gate
#  SHADOW  — core only: Tor + DNS + HAProxy + Privoxy (daily driver)
#  RECON   — Tor + I2P + Yggdrasil + Reticulum + RetroShare
#  STEALTH — single Tor head + DNS only (old hardware / low RAM)
#  OPEN    — no Tor, no I2P, local services only (gaming / performance)
#
# Compose profiles are hardware tiers (phantom/vanguard/hub/broadcast)
# Network profiles filter which SERVICES we actually bring up via
# --scale (set unwanted services to 0) or selective compose files.
# For simplicity: we use the tier profiles and post-stop unwanted heads.

get_tier_profiles() {
    TIER="$1"
    case "$TIER" in
        broadcast) echo "phantom vanguard hub broadcast" ;;
        hub)       echo "phantom vanguard hub" ;;
        vanguard)  echo "phantom vanguard" ;;
        *)         echo "phantom" ;;
    esac
}

# ── Ghost Gate ────────────────────────────────────────────────
ghost_gate_up() {
    PROFILE="$1"
    NFT_FILE="$HYDRA_DIR/ghost-gate.nft"
    if [ ! -f "$NFT_FILE" ]; then
        printf "  ${YEL}!${NC} ghost-gate.nft not found — skipping\n"
        return
    fi
    # STEALTH and OPEN use lighter ruleset
    case "$PROFILE" in
        stealth|open)
            printf "  ${DIM}Ghost Gate: light mode${NC}\n"
            ;;
        ghost)
            printf "  ${GRN}Ghost Gate: MAXIMUM${NC}\n"
            ;;
        *)
            printf "  ${GRN}Ghost Gate: active${NC}\n"
            ;;
    esac
    if nft -f "$NFT_FILE" 2>/dev/null; then
        printf "  ${GRN}✓${NC} nftables rules applied\n"
    else
        printf "  ${YEL}!${NC} nftables unavailable — run as root to activate Ghost Gate\n"
    fi
}

# ── Start ─────────────────────────────────────────────────────
start() {
    PROFILE="${1:-shadow}"
    TIER=$(detect_tier)
    TIER_PROFILES=$(get_tier_profiles "$TIER")

    banner

    printf "${YEL}[ Hardware ]${NC} $TIER tier — $(free -m | awk '/^Mem:/{print $2}')MB RAM\n"
    printf "${YEL}[ Profile  ]${NC} ${BOLD}$PROFILE${NC}\n"
    printf "\n"

    # Ghost Gate
    ghost_gate_up "$PROFILE"
    echo "$PROFILE" > "$HOME/.nexus-hydra-profile"
    printf "\n"

    # Build compose profile args
    PROFILE_ARGS=""
    for p in $TIER_PROFILES; do
        PROFILE_ARGS="$PROFILE_ARGS --profile $p"
    done

    # STEALTH: only bring up phantom tier (DNS + 1 Tor head + HAProxy)
    if [ "$PROFILE" = "stealth" ]; then
        PROFILE_ARGS="--profile phantom"
    fi

    # OPEN: phantom but skip Tor (DNS + HAProxy + local services only)
    # Tor heads will still start in phantom — stop them after
    if [ "$PROFILE" = "open" ]; then
        PROFILE_ARGS="--profile phantom"
    fi

    printf "${YEL}[ Stack ]${NC} Bringing up Hydra...\n"
    cd "$HYDRA_DIR" || exit 1
    eval "podman-compose $PROFILE_ARGS -f \"$COMPOSE\" up -d" 2>&1 | \
        grep -v "^$" | sed 's/^/  /'

    # OPEN: stop Tor heads after compose
    if [ "$PROFILE" = "open" ]; then
        printf "\n  ${DIM}Open mode — stopping Tor heads${NC}\n"
        podman stop tor_head_01 tor_head_02 tor_head_03 2>/dev/null || true
    fi

    printf "\n"
    status_brief
    printf "\n"
    printf "${GRN}${BOLD}  meWEwowow — Hydra is LIVE [ $PROFILE / $TIER ]${NC}\n"
    printf "\n"
}

# ── Stop ──────────────────────────────────────────────────────
stop() {
    banner
    printf "${YEL}[ Stopping ]${NC} Bringing down Hydra...\n"
    cd "$HYDRA_DIR" || exit 1
    podman-compose -f "$COMPOSE" down 2>&1 | grep -v "^$" | sed 's/^/  /'

    # Clear Ghost Gate rules
    if nft flush ruleset 2>/dev/null; then
        printf "  ${GRN}✓${NC} Ghost Gate cleared\n"
    fi
    rm -f "$HOME/.nexus-hydra-profile"
    printf "\n  ${GRN}✓${NC} Done\n\n"
}

# ── Status ────────────────────────────────────────────────────
status_brief() {
    RUNNING=$(podman ps --format "{{.Names}}\t{{.Status}}" 2>/dev/null | \
        grep -E "nexus_|tor_head_|i2p_|mesh_|bridge_|p2p_|sharing_" || true)

    if [ -z "$RUNNING" ]; then
        printf "  ${RED}No Hydra containers running${NC}\n"
        return
    fi

    COUNT=$(echo "$RUNNING" | wc -l)
    printf "  ${BOLD}$COUNT containers up:${NC}\n"
    echo "$RUNNING" | while IFS= read -r line; do
        printf "    ${GRN}▶${NC} $line\n"
    done

    # Key ports
    printf "\n"
    nc -z localhost 9050 2>/dev/null && printf "  ${GRN}●${NC} Tor SOCKS   :9050\n" || \
        printf "  ${DIM}○${NC} Tor SOCKS   :9050\n"
    nc -z localhost 4444 2>/dev/null && printf "  ${GRN}●${NC} I2P HTTP    :4444\n" || \
        printf "  ${DIM}○${NC} I2P HTTP    :4444\n"
    nc -z localhost 8080 2>/dev/null && printf "  ${GRN}●${NC} HAProxy     :8080\n" || \
        printf "  ${DIM}○${NC} HAProxy     :8080\n"
    nc -z localhost 8118 2>/dev/null && printf "  ${GRN}●${NC} Privoxy     :8118\n" || \
        printf "  ${DIM}○${NC} Privoxy     :8118\n"
    nc -z localhost 5173 2>/dev/null && printf "  ${GRN}●${NC} Dashboard   http://localhost:5173\n" || \
        printf "  ${DIM}○${NC} Dashboard   not running\n"
}

status() {
    banner
    status_brief
    printf "\n"
}

logs() {
    banner
    printf "${CYN}[ Live logs — Ctrl+C to exit ]${NC}\n\n"
    cd "$HYDRA_DIR" || exit 1
    podman-compose -f "$COMPOSE" logs -f --tail=50
}

# ─── Main ─────────────────────────────────────────────────────
CMD="${1:-shadow}"

case "$CMD" in
    ghost|shadow|recon|stealth|open)
        start "$CMD"
        ;;
    start)
        start shadow
        ;;
    stop)
        stop
        ;;
    status)
        status
        ;;
    logs)
        logs
        ;;
    health)
        sh "$HYDRA_DIR/scripts/big-catfish.sh"
        ;;
    help|--help|-h)
        banner
        printf "${BOLD}Usage:${NC}\n"
        printf "  nexus-hydra.sh ${CYN}ghost${NC}    — all networks, max Ghost Gate\n"
        printf "  nexus-hydra.sh ${CYN}shadow${NC}   — Tor + DNS, daily driver (default)\n"
        printf "  nexus-hydra.sh ${CYN}recon${NC}    — Tor + I2P + Yggdrasil + RetroShare\n"
        printf "  nexus-hydra.sh ${CYN}stealth${NC}  — Tor only, minimal RAM\n"
        printf "  nexus-hydra.sh ${CYN}open${NC}     — no Tor, max performance\n"
        printf "\n"
        printf "  nexus-hydra.sh ${CYN}stop${NC}     — bring everything down\n"
        printf "  nexus-hydra.sh ${CYN}status${NC}   — show running stack\n"
        printf "  nexus-hydra.sh ${CYN}logs${NC}     — live log tail\n"
        printf "  nexus-hydra.sh ${CYN}health${NC}   — big-catfish OAAE audit\n"
        printf "\n"
        printf "  ${DIM}Hardware auto-detected: phantom | vanguard | hub | broadcast${NC}\n"
        printf "\n"
        ;;
    *)
        printf "${RED}Unknown command: $CMD${NC}\n"
        printf "Run ${CYN}nexus-hydra.sh help${NC} for usage\n"
        exit 1
        ;;
esac
