#!/usr/bin/env bash
# install.sh — single-node O-Cloud Edge Cloud demo installer (P1-T-304).
#
# Brings a clean openEuler 22.03 LTS box (aarch64 鲲鹏 · 真实目标平台 per
# ADR-0020) — or an Ubuntu 22.04 LTS box (dev/CI) — from "fresh shell" to
# "demo running at http://<host>:3000" in under 30 minutes. The script is
# idempotent — rerunning it does the right thing (re-pulls, rebuilds, restarts).
# OS family auto-detected: openEuler/RHEL → dnf · Ubuntu/Debian → apt-get.
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
#   ./scripts/install.sh                  # full install (source build + start)
#   ./scripts/install.sh --image-only     # skip source build, use prebuilt images
#   ./scripts/install.sh --no-start       # build only, don't bring up compose
#   ./scripts/install.sh --with-prometheus
#                                         # install kube-prometheus-stack into the
#                                         # current kubeconfig context's cluster
#                                         # (requires K3s/K8s + helm; replaces the
#                                         # docker-compose Grafana for K8s demos).
#                                         # Composable with the other flags.
#   ./scripts/install.sh --with-dra-driver
#                                         # helm-install the Phase 4 npu-dra-driver
#                                         # chart into the current kubeconfig context.
#                                         # Requires kubectl + helm + K8s 1.30+ (DRA v1beta1).
#                                         # Composable with --with-prometheus.
#   ./scripts/install.sh --all-phase-4    # aggregate: --with-prometheus +
#                                         # ascend-npu-exporter-plus + --with-dra-driver
#   ./scripts/install.sh --uninstall      # tear down compose + remove project dir
#
# Env-var overrides:
#
#   OCEDGE_REPO_DIR   : repo location (default: $PWD)
#   OCEDGE_COMPOSE    : compose file to use (default: deploy/dev/docker-compose.yaml)
#
# Designed for bash 4+; targets openEuler 22.03 (aarch64 鲲鹏 · ADR-0020),
# dev-tested on Ubuntu 22.04. Aborts on the first
# error (set -e). Sudo is used surgically — never `sudo -s` blanket
# escalation.

set -Eeuo pipefail

# -------------------------------------------------------------------- Helpers

readonly SCRIPT_NAME="$(basename "$0")"
readonly REPO_DIR="${OCEDGE_REPO_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
readonly COMPOSE_FILE="${OCEDGE_COMPOSE:-deploy/dev/docker-compose.yaml}"
readonly MIN_GO_VERSION="1.22"
readonly MIN_NODE_VERSION="20"

# Helm / kube-prometheus-stack — used only by --with-prometheus (P2-T-106).
# P3-T-103 retired the community exporter chart + dev stub; the
# self-built ascend-npu-exporter-plus chart is installed alongside KPS.
readonly KPS_RELEASE_NAME="${OCEDGE_KPS_RELEASE:-kube-prometheus-stack}"
readonly KPS_NAMESPACE="${OCEDGE_KPS_NAMESPACE:-monitoring}"
readonly KPS_VALUES_FILE="${OCEDGE_KPS_VALUES:-deploy/single-node/values-kps.yaml}"
readonly ASCEND_RELEASE_NAME="${OCEDGE_ASCEND_RELEASE:-ascend-npu-exporter-plus}"
readonly ASCEND_NAMESPACE="${OCEDGE_ASCEND_NAMESPACE:-monitoring}"
readonly ASCEND_CHART_PATH="${OCEDGE_ASCEND_CHART:-deploy/helm-charts/ascend-npu-exporter-plus}"

# Phase 4 P4-T-104 — npu-dra-driver chart wiring for --with-dra-driver.
readonly NPU_DRA_RELEASE_NAME="${OCEDGE_NPU_DRA_RELEASE:-npu-dra-driver}"
readonly NPU_DRA_NAMESPACE="${OCEDGE_NPU_DRA_NAMESPACE:-ocloud-system}"
readonly NPU_DRA_CHART_PATH="${OCEDGE_NPU_DRA_CHART:-deploy/helm-charts/npu-dra-driver}"

# demo/real delivery profile. `--profile demo|real` selects the deploy value
# overlays under deploy/profiles/<profile>/ that get threaded into the K8s helm
# install steps (exporter + npu-dra-driver). Default demo. NOTE: the bundled
# docker-compose stack is always the DEMO console (mock data); --profile real
# only affects the K8s helm components (real edition = K8s deploy · see
# docs/build-and-production-validation.md §0.1). Set initial value here; --profile
# overrides it in main().
DEPLOY_PROFILE="${OCEDGE_PROFILE:-demo}"

# Echo a single `--values=<path>` token for a chart's active-profile overlay,
# or nothing if no overlay exists. Safe under `set -u` (the :+ guard).
profile_values_arg() {
    local chart="$1"
    local pf="$REPO_DIR/deploy/profiles/$DEPLOY_PROFILE/${chart}.values.yaml"
    [[ -f "$pf" ]] && printf -- '--values=%s' "$pf" || true
}

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

    # ADR-0020 (P12-T-103): 真实目标平台 = openEuler (aarch64 鲲鹏 Kunpeng 920 +
    # 昇腾 910B · Atlas 800 原生). Ubuntu/Debian 保留为 dev/CI 平台(本机渲染验证).
    local os_id="unknown"
    if [[ -r /etc/os-release ]]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        os_id="${ID:-unknown}"
        case "${ID:-}" in
            openEuler)
                version_ge "${VERSION_ID:-0}" "22.03" ||
                    warn "tested on openEuler 22.03 LTS SP+; you're on ${VERSION_ID:-unknown}. Proceeding optimistically."
                ;;
            ubuntu | debian)
                warn "on ${ID} (dev/CI platform); 真实目标平台是 openEuler aarch64 鲲鹏 (ADR-0020). 真 NPU 特性 lab-gated Phase 13+."
                ;;
            *)
                warn "tested on openEuler 22.03+ (target) / Ubuntu 22.04+ (dev); you're on '${ID:-unknown}'. Proceeding optimistically."
                ;;
        esac
    else
        warn "no /etc/os-release — can't identify distro. Proceeding optimistically."
    fi

    # Package manager family — openEuler/RHEL → dnf · Ubuntu/Debian → apt-get.
    if command -v dnf >/dev/null 2>&1; then
        PKG_MGR="dnf"; OS_FAMILY="rhel"
    elif command -v apt-get >/dev/null 2>&1; then
        PKG_MGR="apt-get"; OS_FAMILY="debian"
    else
        err "no supported package manager (dnf for openEuler / apt-get for Ubuntu) found"
    fi
    readonly PKG_MGR OS_FAMILY
    log "os=$os_id family=$OS_FAMILY pkg=$PKG_MGR"

    # Arch detection (ADR-0020: arm64 鲲鹏 = 部署 target · amd64 = dev/CI · 不再 amd64-only).
    HOST_ARCH="$(uname -m)"
    case "$HOST_ARCH" in
        aarch64 | arm64)
            GOARCH_DETECTED="arm64"
            log "arch $HOST_ARCH → arm64 (ADR-0020 真实目标平台 · aarch64 鲲鹏 Kunpeng 920 + 昇腾 910B)"
            ;;
        x86_64 | amd64)
            GOARCH_DETECTED="amd64"
            warn "arch $HOST_ARCH → amd64 (dev/CI platform; 真实目标平台是 aarch64 鲲鹏 per ADR-0020 · 真 NPU 验证 lab-gated Phase 13+)"
            ;;
        *)
            GOARCH_DETECTED="amd64"
            warn "unknown arch $HOST_ARCH; defaulting GOARCH=amd64. 真实目标平台 = aarch64 鲲鹏 (ADR-0020)."
            ;;
    esac
    readonly HOST_ARCH GOARCH_DETECTED

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

    log "installing Docker Engine + Compose plugin (family: $OS_FAMILY)"
    if [[ "$OS_FAMILY" == "rhel" ]]; then
        # openEuler / RHEL family (ADR-0020 target). openEuler ships docker +
        # docker-compose-plugin (or moby-engine) in its native repos; no
        # docker.com apt repo. Production may pin a vendor repo + cosign.
        $SUDO "$PKG_MGR" install -y ca-certificates curl
        $SUDO "$PKG_MGR" install -y docker docker-compose-plugin 2>/dev/null ||
            $SUDO "$PKG_MGR" install -y moby-engine moby-compose 2>/dev/null ||
            warn "could not auto-install docker via $PKG_MGR; install manually (openEuler: 'dnf install docker')"
        $SUDO systemctl enable --now docker 2>/dev/null || true
    else
        # Debian / Ubuntu (dev/CI). Docker's official apt repo.
        $SUDO apt-get update -qq
        $SUDO apt-get install -y -qq ca-certificates curl gnupg
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
    fi

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

    log "installing Go from the official tarball (distro pkg has older versions)"
    local goarch="${GOARCH_DETECTED:-amd64}"  # arm64 鲲鹏 / amd64 dev (ADR-0020 · arch 检测)
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

    if [[ "$OS_FAMILY" == "rhel" ]]; then
        log "installing Node.js via NodeSource (rpm repo with current LTS · openEuler)"
        curl -fsSL https://rpm.nodesource.com/setup_lts.x | $SUDO -E bash -
        $SUDO "$PKG_MGR" install -y nodejs
    else
        log "installing Node.js via NodeSource (apt repo with current LTS)"
        curl -fsSL https://deb.nodesource.com/setup_lts.x | $SUDO -E bash -
        $SUDO apt-get install -y -qq nodejs
    fi
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

install_helm() {
    if command -v helm >/dev/null 2>&1; then
        log "helm $(helm version --short 2>/dev/null) already installed"
        return
    fi
    log "installing helm via official get-helm-3 script"
    local tmp
    tmp="$(mktemp)"
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 -o "$tmp"
    chmod +x "$tmp"
    $SUDO HELM_INSTALL_DIR=/usr/local/bin "$tmp"
    rm -f "$tmp"
    log "helm installed: $(helm version --short 2>/dev/null)"
}

# --with-prometheus implementation (P2-T-106). Targets the current
# kubeconfig context — caller is responsible for pointing $KUBECONFIG
# at the right cluster. Idempotent: a second run upgrades in place.
install_prometheus_stack() {
    if ! command -v kubectl >/dev/null 2>&1; then
        err "--with-prometheus needs kubectl on PATH (and a working kubeconfig)"
    fi
    if ! kubectl cluster-info >/dev/null 2>&1; then
        err "kubectl cluster-info failed — check kubeconfig before re-running"
    fi
    install_helm

    log "adding prometheus-community + huawei chart repos (idempotent)"
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
    helm repo update prometheus-community >/dev/null

    if [[ ! -f "$REPO_DIR/$KPS_VALUES_FILE" ]]; then
        err "kube-prometheus-stack values file not found: $REPO_DIR/$KPS_VALUES_FILE"
    fi

    log "upgrading/installing $KPS_RELEASE_NAME into namespace $KPS_NAMESPACE"
    helm upgrade --install "$KPS_RELEASE_NAME" \
        prometheus-community/kube-prometheus-stack \
        --namespace "$KPS_NAMESPACE" \
        --create-namespace \
        --values "$REPO_DIR/$KPS_VALUES_FILE" \
        --wait --timeout 10m

    if [[ ! -d "$REPO_DIR/$ASCEND_CHART_PATH" ]]; then
        warn "ascend chart not found at $ASCEND_CHART_PATH — skipping exporter install"
    else
        log "upgrading/installing $ASCEND_RELEASE_NAME into namespace $ASCEND_NAMESPACE (profile: $DEPLOY_PROFILE)"
        local ascend_pfa; ascend_pfa="$(profile_values_arg ascend-npu-exporter-plus)"
        helm upgrade --install "$ASCEND_RELEASE_NAME" \
            "$REPO_DIR/$ASCEND_CHART_PATH" \
            --namespace "$ASCEND_NAMESPACE" \
            --create-namespace \
            ${ascend_pfa:+"$ascend_pfa"} \
            --wait --timeout 5m \
            || warn "ascend exporter install failed (likely no Ascend nodes); ServiceMonitor stays unbound until silicon shows up"
    fi

    log "kube-prometheus-stack ready; Grafana NodePort: 30001 (admin/admin)"
}

# --with-dra-driver implementation (P4-T-104). Targets the current
# kubeconfig context — caller is responsible for pointing $KUBECONFIG
# at the right cluster. Idempotent: a second run upgrades in place.
install_npu_dra_driver() {
    if ! command -v kubectl >/dev/null 2>&1; then
        err "--with-dra-driver needs kubectl on PATH (and a working kubeconfig)"
    fi
    if ! kubectl cluster-info >/dev/null 2>&1; then
        err "kubectl cluster-info failed — check kubeconfig before re-running"
    fi
    install_helm

    if [[ ! -d "$REPO_DIR/$NPU_DRA_CHART_PATH" ]]; then
        err "npu-dra-driver chart not found at $NPU_DRA_CHART_PATH"
    fi

    log "upgrading/installing $NPU_DRA_RELEASE_NAME into namespace $NPU_DRA_NAMESPACE (profile: $DEPLOY_PROFILE)"
    local npudra_pfa; npudra_pfa="$(profile_values_arg npu-dra-driver)"
    helm upgrade --install "$NPU_DRA_RELEASE_NAME" \
        "$REPO_DIR/$NPU_DRA_CHART_PATH" \
        --namespace "$NPU_DRA_NAMESPACE" \
        --create-namespace \
        ${npudra_pfa:+"$npudra_pfa"} \
        --wait --timeout 5m

    if [[ "$DEPLOY_PROFILE" == "real" ]]; then
        warn "profile=real selects npu-dra-driver sourceType=real-ascend; the source body is the Phase-13 T101 lab body (npu-smi/DCMI). ResourceSlice publication needs real 910B silicon on the node."
    fi
    log "npu-dra-driver ready; resourceslices visible via: kubectl get resourceslices"
}

# --with-secrets implementation (P13-T-203 · ADR-0025 §2 Decision B). Installs
# the External Secrets Operator and applies the REFERENCE Vault binding
# (deploy/secrets/) so the real profile sources secrets from Vault instead of
# inline YAML. Targets the current kubeconfig context; idempotent. Vault itself
# is a documented PREREQ (deploy/secrets/README.md) — the store/ExternalSecrets
# apply cleanly but stay unsynced until a reachable Vault answers.
ESO_RELEASE_NAME="${OCEDGE_ESO_RELEASE:-external-secrets}"
ESO_NAMESPACE="${OCEDGE_ESO_NAMESPACE:-external-secrets}"
ESO_CHART_VERSION="${OCEDGE_ESO_VERSION:-0.10.5}"
install_external_secrets() {
    if ! command -v kubectl >/dev/null 2>&1; then
        err "--with-secrets needs kubectl on PATH (and a working kubeconfig)"
    fi
    if ! kubectl cluster-info >/dev/null 2>&1; then
        err "kubectl cluster-info failed — check kubeconfig before re-running"
    fi
    install_helm

    log "adding external-secrets chart repo (idempotent)"
    helm repo add external-secrets https://charts.external-secrets.io >/dev/null
    helm repo update external-secrets >/dev/null

    log "upgrading/installing $ESO_RELEASE_NAME (v$ESO_CHART_VERSION) into namespace $ESO_NAMESPACE"
    helm upgrade --install "$ESO_RELEASE_NAME" \
        external-secrets/external-secrets \
        --namespace "$ESO_NAMESPACE" \
        --create-namespace \
        --version "$ESO_CHART_VERSION" \
        --set installCRDs=true \
        --wait --timeout 5m

    log "applying reference Vault ClusterSecretStore + ExternalSecrets (deploy/secrets/)"
    kubectl apply -f "$REPO_DIR/deploy/secrets/cluster-secret-store.yaml"
    kubectl apply -f "$REPO_DIR/deploy/secrets/dex-client-secrets.externalsecret.yaml"
    # grafana-admin lives in the monitoring ns; apply best-effort (no-op until
    # kube-prometheus-stack is installed with grafana.admin.existingSecret).
    kubectl apply -f "$REPO_DIR/deploy/secrets/grafana-admin.externalsecret.yaml" \
        || warn "grafana-admin ExternalSecret apply skipped (monitoring ns likely absent until --with-prometheus)"
    # BMC creds only matter when the bare-metal-provisioning-operator is in use;
    # apply best-effort (harmless if its consumer isn't installed).
    kubectl apply -f "$REPO_DIR/deploy/secrets/bmc-credentials.externalsecret.yaml" \
        || warn "bmc ExternalSecret apply skipped/failed (bare-metal operator likely absent)"

    warn "Vault is a PREREQ: the ClusterSecretStore targets vault.vault.svc:8200 with kubernetes auth. Until a reachable Vault answers, SecretStore stays NotReady and dex-client-secrets is NOT materialised — see deploy/secrets/README.md."
    log "external-secrets ready; check sync via: kubectl get externalsecret -A"
}

uninstall_external_secrets() {
    if ! command -v helm >/dev/null 2>&1; then
        warn "helm not installed; nothing to uninstall"
        return
    fi
    log "uninstalling $ESO_RELEASE_NAME (best-effort)"
    kubectl delete -f "$REPO_DIR/deploy/secrets/dex-client-secrets.externalsecret.yaml" 2>/dev/null || true
    kubectl delete -f "$REPO_DIR/deploy/secrets/grafana-admin.externalsecret.yaml" 2>/dev/null || true
    kubectl delete -f "$REPO_DIR/deploy/secrets/bmc-credentials.externalsecret.yaml" 2>/dev/null || true
    kubectl delete -f "$REPO_DIR/deploy/secrets/cluster-secret-store.yaml" 2>/dev/null || true
    helm uninstall "$ESO_RELEASE_NAME" --namespace "$ESO_NAMESPACE" 2>/dev/null || true
}

uninstall_npu_dra_driver() {
    if ! command -v helm >/dev/null 2>&1; then
        warn "helm not installed; nothing to uninstall"
        return
    fi
    log "uninstalling $NPU_DRA_RELEASE_NAME (best-effort)"
    helm uninstall "$NPU_DRA_RELEASE_NAME" --namespace "$NPU_DRA_NAMESPACE" 2>/dev/null || true
}

uninstall_prometheus_stack() {
    if ! command -v helm >/dev/null 2>&1; then
        warn "helm not installed; nothing to uninstall"
        return
    fi
    log "uninstalling $ASCEND_RELEASE_NAME / $KPS_RELEASE_NAME (best-effort)"
    helm uninstall "$ASCEND_RELEASE_NAME" --namespace "$ASCEND_NAMESPACE" 2>/dev/null || true
    helm uninstall "$KPS_RELEASE_NAME" --namespace "$KPS_NAMESPACE" 2>/dev/null || true
}

uninstall() {
    log "tearing down docker-compose stack"
    pushd "$REPO_DIR" >/dev/null
    $SUDO docker compose -f "$COMPOSE_FILE" down -v --remove-orphans || true
    popd >/dev/null
    uninstall_external_secrets
    uninstall_npu_dra_driver
    uninstall_prometheus_stack
    log "uninstall complete. Repository directory left in place; remove manually if desired."
}

usage() {
    cat <<EOF
$SCRIPT_NAME — single-node O-Cloud Edge demo installer

Options:
  --image-only        skip source build; pull pre-built images via docker compose
  --no-start          build artifacts but don't bring up the compose stack
  --profile demo|real delivery edition for the K8s helm steps (default demo).
                      demo = validation/mock data; real = physical-data overlays
                      from deploy/profiles/real/ (real-Ascend body is a Phase-13
                      stub). The bundled compose stack is always the demo console.
  --with-prometheus   helm-install kube-prometheus-stack + ascend-npu-exporter-plus
                      into the current kubeconfig context (P2-T-106; composable
                      with the other flags)
  --with-dra-driver   helm-install the Phase 4 npu-dra-driver chart into the
                      current kubeconfig context (P4-T-104; composable). Requires
                      kubectl + helm + a K8s 1.30+ cluster with DRA v1beta1 enabled.
  --all-phase-4       aggregate flag: --with-prometheus + --with-dra-driver.
                      Phase 4 "single-command stand up everything" knob.
  --with-secrets      install External Secrets Operator + apply the reference
                      Vault binding (deploy/secrets/) so the real profile sources
                      secrets from Vault instead of inline YAML (P13-T-203;
                      composable). Vault itself is a prereq — see
                      deploy/secrets/README.md.
  --uninstall         tear down the compose stack + any kps/ascend/dra/eso releases
  --help / -h         show this message

Env vars:
  OCEDGE_REPO_DIR          repo location (default: this script's parent)
  OCEDGE_COMPOSE           compose file (default: $COMPOSE_FILE)
  OCEDGE_KPS_RELEASE       kube-prometheus-stack release name (default: kube-prometheus-stack)
  OCEDGE_KPS_NAMESPACE     kube-prometheus-stack namespace (default: monitoring)
  OCEDGE_KPS_VALUES        kube-prometheus-stack values file (default: deploy/single-node/values-kps.yaml)
  OCEDGE_ASCEND_RELEASE    ascend-npu-exporter-plus release name (default: ascend-npu-exporter-plus)
  OCEDGE_ASCEND_NAMESPACE  ascend-npu-exporter-plus namespace (default: monitoring)
  OCEDGE_ASCEND_CHART      ascend-npu-exporter-plus chart path (default: deploy/helm-charts/ascend-npu-exporter-plus)
EOF
}

# --------------------------------------------------------------------- Main

main() {
    local image_only=false
    local no_start=false
    local do_uninstall=false
    local with_prometheus=false
    local with_dra_driver=false
    local with_secrets=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --image-only) image_only=true ;;
            --no-start) no_start=true ;;
            --profile)
                shift
                [[ $# -gt 0 ]] || err "--profile needs an argument: demo|real"
                case "$1" in
                    demo | real) DEPLOY_PROFILE="$1" ;;
                    *) err "--profile must be demo or real (got '$1')" ;;
                esac
                ;;
            --with-prometheus) with_prometheus=true ;;
            --with-dra-driver) with_dra_driver=true ;;
            --with-secrets) with_secrets=true ;;
            --all-phase-4)
                # Aggregate convenience flag (P4-T-104). Composes with the
                # other --with-* flags so callers can still add more.
                with_prometheus=true
                with_dra_driver=true
                ;;
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
    log "delivery profile: ${DEPLOY_PROFILE} (compose stack = demo console; --profile applies to the K8s helm steps)"

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

    if [[ "$with_prometheus" == "true" ]]; then
        install_prometheus_stack
    fi

    if [[ "$with_dra_driver" == "true" ]]; then
        install_npu_dra_driver
    fi

    if [[ "$with_secrets" == "true" ]]; then
        install_external_secrets
    fi
}

main "$@"
