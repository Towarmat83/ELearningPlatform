#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# ELearning Platform — local dev environment setup
# Requires: podman, kind, kubectl, helm
# Run from the repository root: bash infra/dev/setup.sh
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

CLUSTER_NAME="elearning"
KEYCLOAK_VERSION="26.1.4"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[info]${NC} $*"; }
success() { echo -e "${GREEN}[ok]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[warn]${NC} $*"; }
die()     { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

# ── Preflight ─────────────────────────────────────────────────────────────────
for tool in podman kind kubectl helm; do
  command -v "$tool" &>/dev/null || die "Missing required tool: $tool"
done

PODMAN_MEM=$(podman machine inspect podman-machine-default --format '{{.Resources.Memory}}' 2>/dev/null || echo "0")
if [[ "$PODMAN_MEM" -lt 14000 ]] 2>/dev/null; then
  die "Podman machine has less than 14 GB RAM. Run first:
  podman machine stop
  podman machine set --memory 16384 --cpus 8 podman-machine-default
  podman machine start"
fi

# ── Step 1: Kind cluster ──────────────────────────────────────────────────────
info "Step 1/8 — Creating Kind cluster '${CLUSTER_NAME}' …"
if KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  warn "Cluster '${CLUSTER_NAME}' already exists, skipping creation."
else
  KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster \
    --name "$CLUSTER_NAME" \
    --config infra/kind/kind-config.yaml
fi
kubectl config use-context "kind-${CLUSTER_NAME}"
kubectl wait --for=condition=Ready nodes --all --timeout=120s
success "Cluster ready."

# ── Step 2: Helm repos ────────────────────────────────────────────────────────
info "Step 2/8 — Adding Helm repos …"
helm repo add cnpg     https://cloudnative-pg.github.io/charts    2>/dev/null || true
helm repo add gitlab   https://charts.gitlab.io/                   2>/dev/null || true
helm repo update
success "Helm repos ready."

# ── Step 3: CNPG operator ─────────────────────────────────────────────────────
info "Step 3/8 — Installing CloudNativePG operator …"
helm upgrade --install cnpg cnpg/cloudnative-pg \
  --namespace cnpg-system \
  --create-namespace \
  --wait \
  --timeout 3m
kubectl wait --for=condition=Available deploy/cnpg-cloudnative-pg \
  -n cnpg-system --timeout=120s
success "CNPG operator ready."

# ── Step 4: CNPG clusters ─────────────────────────────────────────────────────
info "Step 4/8 — Provisioning CNPG clusters …"
kubectl create namespace elearning --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace keycloak  --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f infra/dev/cnpg/platform-cluster.yaml
kubectl apply -f infra/dev/cnpg/keycloak-cluster.yaml

info "Waiting for CNPG clusters to be ready (this takes ~60s) …"
kubectl wait --for=condition=Ready cluster/platform-db -n elearning --timeout=300s
kubectl wait --for=condition=Ready cluster/keycloak-db -n keycloak  --timeout=300s
success "CNPG clusters ready."

# ── Step 5: Keycloak Operator ─────────────────────────────────────────────────
info "Step 5/8 — Installing Keycloak Operator v${KEYCLOAK_VERSION} …"
kubectl create namespace keycloak --dry-run=client -o yaml | kubectl apply -f -

BASE="https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/${KEYCLOAK_VERSION}/kubernetes"
kubectl apply -f "${BASE}/keycloaks.k8s.keycloak.org-v1.yml"
kubectl apply -f "${BASE}/keycloakrealmimports.k8s.keycloak.org-v1.yml"
kubectl apply -n keycloak -f "${BASE}/kubernetes.yml"

kubectl wait --for=condition=Available deploy/keycloak-operator \
  -n keycloak --timeout=120s
success "Keycloak Operator ready."

# ── Step 6: Keycloak instance ─────────────────────────────────────────────────
info "Step 6/8 — Deploying Keycloak instance …"
kubectl apply -f infra/dev/keycloak/keycloak.yaml
info "Waiting for Keycloak to become ready (takes 2-3 min on first start) …"
kubectl wait --for=condition=Ready keycloak/keycloak -n keycloak --timeout=300s
success "Keycloak ready → http://keycloak.local:8884"

# ── Step 7: GitLab CE ─────────────────────────────────────────────────────────
info "Step 7/8 — Deploying GitLab CE Omnibus (takes 5-8 min on first start) …"
# Using the gitlab/gitlab-ce Omnibus image as a single Deployment (multi-arch, ARM64 native).
# The official GitLab Helm chart uses CNG images that don't have ARM64 variants for all
# job containers (shared-secrets crashes with lfstack.push on M3). The Omnibus image
# is the recommended self-managed approach on Apple Silicon.
kubectl apply -f infra/dev/gitlab/gitlab-omnibus.yaml
info "Waiting for GitLab readiness probe (up to 6 min) …"
kubectl wait --for=condition=Available deploy/gitlab -n gitlab --timeout=420s
success "GitLab ready → http://gitlab.local:8880"
GITLAB_ROOT_PW=$(kubectl exec -n gitlab deploy/gitlab -- \
  gitlab-rake "gitlab:password:reset[root]" 2>/dev/null | grep "Password:" | awk '{print $2}' || echo "(set via UI on first login)")
echo -e "  GitLab root password: ${GREEN}${GITLAB_ROOT_PW:-see UI}${NC}"

# ── Step 8: Platform services ─────────────────────────────────────────────────
info "Step 8/8 — Platform services (build images first if needed) …"
warn "Skipping platform deploy — images must be built locally first."
warn "Run after building images:"
warn "  KIND_EXPERIMENTAL_PROVIDER=podman kind load docker-image localhost/elearning-course-service:dev --name elearning"
warn "  KIND_EXPERIMENTAL_PROVIDER=podman kind load docker-image localhost/elearning-user-service:dev --name elearning"
warn "  helm upgrade --install elearning ./helm -n elearning -f infra/kind/values-local.yaml"

# ── /etc/hosts ────────────────────────────────────────────────────────────────
echo ""
info "Add these entries to /etc/hosts (requires sudo):"
echo "  127.0.0.1  gitlab.local keycloak.local elearning.local"
echo ""
echo -e "  Run: ${YELLOW}echo '127.0.0.1 gitlab.local keycloak.local elearning.local' | sudo tee -a /etc/hosts${NC}"
echo ""

success "Dev environment setup complete!"
echo ""
echo "  GitLab:    http://gitlab.local:8880  (user: root / pw above)"
echo "  Keycloak:  http://keycloak.local:8884  (user: admin / admin)"
echo "  Frontend:  cd frontend && npm run dev  (port 4321)"
