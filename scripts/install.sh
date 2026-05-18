#!/usr/bin/env bash
# install.sh — single-node O-Cloud Edge Cloud demo installer (P1-T-304).
#
# Brings a clean Ubuntu 22.04 LTS box from "fresh shell" to "demo running
# at http://<host>:3000" in under 30 minutes. The script is idempotent —
# rerunning it does the right thing (re-pulls, rebuilds, restarts).
#
# What it does, in order:
#
#   1. Sanity check the host (OS family, arch, root vs. sudo).
#   2. Install Docker Engine + Compose plugin if absent.
#   3. Install Go (1.22+) and Node (24+) only if absent AND the user wants
#      to build from source. Skip when --image-only is set.
#   4. Build the backend binary (go build) unless --image-only.
#   5. Build the frontend dist (pnpm install + build) unless --image-only.
#   6. Bring up the docker-compose stack (deploy/dev or deploy/single-node).
#   7. Poll healthz on backend + frontend, fail loudly after 5 minutes.
#   8. Print access URLs + a tail of recent logs.
#
# What it does NOT do:
#
#   - K3s / KubeEdge deployment (Phase 2+).
#   - TLS / Ingress / auth (Phase 9 production hardening).
#   - Backup / restore / observability beyond the bundled Grafana+Prometheus.
#
# Usage:
#
#   ./scripts/install.sh                # full install (source build + start)
#   ./scripts/install.sh --image-only   # skip source build, use prebuilt images
#   ./scripts/install.sh --no-start     # build only, don't bring up compose
#   ./scripts/install.sh --uninstall    # tear down compose + remove project dir
#
# Env-var overrides:
#
#   OCEDGE_REPO_DIR   : repo location (default: $PWD)
#   OCEDGE_COMPOSE    : compose file to use (default: deploy/dev/docker-compose.yaml)
#
# Designed for bash 4+; tested against Ubuntu 22.04. Aborts on the first
# error (set -e). Sudo is used surgically — never `sudo -s` blanket
# escalation.

set -Eeuo pipefail

# -------------------------------------------------------------------- Helpers

readonly SCRIPT_NAME="$(basename "$0")"
readonly REPO_DIR="${OCEDGE_REPO_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
readonly COMPOSE_FILE="${OCEDGE_COMPOSE:-deploy/dev/docker-compose.yaml}"
readonly MIN_GO_VERSION="1.22"
readonly MIN_NODE_VERSION="20"

log() { printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
warn() { printf '[%s] \033[33mWARN\033[0m %s\n' "$(date '+%H:%M:%S')" "$*" >&2; }
err() {
    printf '[%s] \033[31mERROR\033[0m %s\n' "$(date '+%H:%M:%S')" "$*" >&2
    exit 1
}

# Detect whether to prepend `sudo` for privileged commands. If we're root
# already, an empty SUDO produces a no-op; on a sudo-capable user it's the
# normal escalation.
detect_sudo() {
    if [[ "$(id -u)" -eq 0 ]]; then
        SUDO=""
    elif command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        err "neither running as root nor have sudo — re-run as root or install sudo"
    fi
    readonly SUDO
}

# Compare two semver-ish version strings (X.Y or X.Y.Z). Returns 0 if
# $1 >= $2, 1 otherwise. Bash's lack of native semver makes this clunky;
# we lean on `sort -V` (GNU coreutils, ships with Ubuntu).
version_ge() {
    [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" == "$2" ]]
}

# -------------------------------------------------------------------- Sanity

sanity_check() {
    log "checking host environment"

    if [[ "$(uname -s)" != "Linux" ]]; then
        err "install.sh targets Linux only (got $(uname -s)). For Windows/macOS, use docker-compose directly."
    fi

    if [[ -r /etc/os-release ]]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        if [[ "${ID:-}" != "ubuntu" ]]; then
            warn "tested only on Ubuntu 22.04; you're on '${ID:-unknown}'. Proceeding optimistically."
        elif ! version_ge "${VERSION_ID:-0}" "22.04"; then
            warn "tested on Ubuntu 22.04+; you're on ${VERSION_ID:-unknown}. Proceeding optimistically."
        fi
    else
        warn "no /etc/os-release — can't identify distro. Proceeding optimistically."
    fi

    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64 | amd64) ;;
        *)
            warn "target hardware is Ascend 910B which is amd64-only; running on $arch is dev-only and will skip NPU-specific features."
            ;;
    esac

    if [[ ! -d "$REPO_DIR" ]]; then
        err "repo directory $REPO_DIR not found — set OCEDGE_REPO_DIR or run from inside the repo"
    fi
    if [[ ! -f "$REPO_DIR/$COMPOSE_FILE" ]]; then
        err "compose file $REPO_DIR/$COMPOSE_FILE not found"
    fi

    detect_sudo
}

# ------------------------------------------------------------- Prerequisites

install_docker() {
    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        log "docker + compose already installed: $(docker --version)"
        return
    fi

    log "installing Docker Engine + Compose plugin"
    $SUDO apt-get update -qq
    $SUDO apt-get install -y -qq ca-certificates curl gnupg

    # Docker's official apt repo. Mirrors docker.com/engine/install/ubuntu.
    $SUDO install -m 0755 -d /etc/apt/keyrings
    if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | $SUDO gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        $SUDO chmod a+r /etc/apt/keyrings/docker.gpg
    fi

    local codename
    codename="$(. /etc/os-release && echo "${VERSION_CODENAME:-jammy}")"
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $codename stable" |
        $SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null

    $SUDO apt-get update -qq
    $SUDO apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    # Allow the current user to run docker without sudo. Takes effect on
    # next login; the install run still uses sudo for compose.
    if [[ -n "$SUDO" ]]; then
        $SUDO usermod -aG docker "$USER" || true
        warn "added $USER to the 'docker' group; log out and back in for it to take effect (current run still uses sudo)"
    fi
}

install_go() {
    if command -v go >/dev/null 2>&1; then
        local v
        v="$(go version | awk '{print $3}' | sed 's/^go//')"
        if version_ge "$v" "$MIN_GO_VERSION"; then
            log "go $v already installed"
            return
        fi
        warn "go $v installed but minimum is $MIN_GO_VERSION; installing newer"
    fi

    log "installing Go from the official tarball (apt has older versions)"
    local goarch="amd64"
    local govers="1.22.5"
    local tar="go${govers}.linux-${goarch}.tar.gz"
    local tmpd
    tmpd="$(mktemp -d)"
    curl -fsSL "https://go.dev/dl/${tar}" -o "${tmpd}/${tar}"
    $SUDO rm -rf /usr/local/go
    $SUDO tar -C /usr/local -xzf "${tmpd}/${tar}"
    rm -rf "${tmpd}"

    # Persist on PATH for future shells. The current shell gets it via
    # export below.
    if [[ -d /etc/profile.d ]] && ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' | $SUDO tee /etc/profile.d/go.sh >/dev/null
    fi
    export PATH="$PATH:/usr/local/go/bin"
    log "go installed: $(go version)"
}

install_node() {
    if command -v node >/dev/null 2>&1; then
        local v
        v="$(node --version | sed 's/^v//')"
        if version_ge "$v" "$MIN_NODE_VERSION"; then
            log "node $v already installed"
            install_pnpm
            return
        fi
        warn "node $v installed but minimum is $MIN_NODE_VERSION; replacing"
    fi

    log "installing Node.js via NodeSource (apt repo with current LTS)"
    curl -fsSL https://deb.nodesource.com/setup_lts.x | $SUDO -E bash -
    $SUDO apt-get install -y -qq nodejs
    install_pnpm
}

install_pnpm() {
    if command -v pnpm >/dev/null 2>&1; then
        log "pnpm $(pnpm --version) already installed"
        return
    fi
    # Node 24's bundled corepack is the easiest path; fall back to npm if
    # not available.
    if command -v corepack >/dev/null 2>&1; then
        $SUDO corepack enable
        $SUDO corepack prepare pnpm@latest --activate
    else
        $SUDO npm install -g pnpm
    fi
    log "pnpm installed: $(pnpm --version)"
}

# ------------------------------------------------------------- Build & start

build_backend() {
    log "building backend binary"
    pushd "$REPO_DIR/backend" >/dev/null
    go mod download
    go build -o bin/demo-backend ./cmd/demo-backend
    popd >/dev/null
    log "backend binary at backend/bin/demo-backend"
}

build_frontend() {
    log "installing frontend deps + building production bundle"
    pushd "$REPO_DIR/frontend" >/dev/null
    pnpm install --frozen-lockfile=false
    pnpm run gen:types
    pnpm run build
    popd >/dev/null
    log "frontend dist at frontend/dist/"
}

start_compose() {
    log "bringing up docker-compose stack: $COMPOSE_FILE"
    pushd "$REPO_DIR" >/dev/null
    $SUDO docker compose -f "$COMPOSE_FILE" up -d --build
    popd >/dev/null
}

wait_for_health() {
    log "waiting for backend healthz (max 5 min)"
    local deadline=$(( $(date +%s) + 300 ))
    while [[ "$(date +%s)" -lt $deadline ]]; do
        if curl -fsS http://localhost:8080/api/v1/healthz >/dev/null 2>&1; then
            log "backend healthz OK"
            break
        fi
        sleep 5
    done
    if ! curl -fsS http://localhost:8080/api/v1/healthz >/dev/null 2>&1; then
        err "backend healthz never responded; check 'docker compose logs backend'"
    fi

    log "waiting for frontend (max 2 min)"
    deadline=$(( $(date +%s) + 120 ))
    while [[ "$(date +%s)" -lt $deadline ]]; do
        if curl -fsS http://localhost:3000/ >/dev/null 2>&1; then
            log "frontend OK"
            return
        fi
        sleep 5
    done
    warn "frontend not responding yet; first vite build may still be in progress. Check 'docker compose logs frontend'."
}

print_summary() {
    cat <<EOF

================================================================
 O-Cloud Edge Cloud demo is up.

   Frontend:    http://$(hostname -I | awk '{print $1}'):3000
   Backend API: http://$(hostname -I | awk '{print $1}'):8080/api/v1/healthz
   Grafana:     http://$(hostname -I | awk '{print $1}'):3001  (admin/admin)

   Logs:        docker compose -f $COMPOSE_FILE logs -f
   Stop:        docker compose -f $COMPOSE_FILE down

================================================================

EOF
}

uninstall() {
    log "tearing down docker-compose stack"
    pushd "$REPO_DIR" >/dev/null
    $SUDO docker compose -f "$COMPOSE_FILE" down -v --remove-orphans || true
    popd >/dev/null
    log "uninstall complete. Repository directory left in place; remove manually if desired."
}

usage() {
    cat <<EOF
$SCRIPT_NAME — single-node O-Cloud Edge demo installer

Options:
  --image-only   skip source build; pull pre-built images via docker compose
  --no-start     build artifacts but don't bring up the compose stack
  --uninstall    tear down the compose stack (does not remove repo dir)
  --help / -h    show this message

Env vars:
  OCEDGE_REPO_DIR    repo location (default: this script's parent)
  OCEDGE_COMPOSE     compose file to use (default: $COMPOSE_FILE)
EOF
}

# --------------------------------------------------------------------- Main

main() {
    local image_only=false
    local no_start=false
    local do_uninstall=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --image-only) image_only=true ;;
            --no-start) no_start=true ;;
            --uninstall) do_uninstall=true ;;
            --help | -h)
                usage
                exit 0
                ;;
            *)
                err "unknown flag: $1 (try --help)"
                ;;
        esac
        shift
    done

    sanity_check

    if [[ "$do_uninstall" == "true" ]]; then
        uninstall
        exit 0
    fi

    install_docker

    if [[ "$image_only" == "false" ]]; then
        install_go
        install_node
        build_backend
        build_frontend
    else
        log "image-only mode: skipping toolchain install + source build"
    fi

    if [[ "$no_start" == "false" ]]; then
        start_compose
        wait_for_health
        print_summary
    else
        log "no-start mode: build artifacts ready, compose not started"
    fi
}

main "$@"
