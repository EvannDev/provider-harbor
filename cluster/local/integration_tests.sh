#!/usr/bin/env bash
# End-to-end integration test runner.
#
# Requires: kind, kubectl, helm, docker, go (all installed by make targets).
# The script is called by `make e2e.run` / `make test-integration`.
set -euo pipefail

# ---------------------------------------------------------------------------
# Colours
# ---------------------------------------------------------------------------
BLU='\033[0;34m'
GRN='\033[0;32m'
RED='\033[0;31m'
NOC='\033[0m'
echo_step()    { printf "\n${BLU}>>>>>>> %s${NOC}\n" "$1"; }
echo_success() { printf "\n${GRN}%s${NOC}\n" "$1"; }
echo_error()   { printf "\n${RED}%s${NOC}\n" "$1"; exit 1; }

PACKAGE_NAME="provider-harbor"
projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}")"/../.. && pwd )"

# Pull tool paths and project metadata from the build submodule.
eval "$(make --no-print-directory -C "${projectdir}" build.vars)"

# Match the cluster name set in the Makefile so controlplane.up and this
# script target the same cluster.
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-${BUILD_REGISTRY}-inttests}"
export KIND_CLUSTER_NAME

# ---------------------------------------------------------------------------
# Cleanup on exit (unless caller sets skipcleanup=true so diagnostics can run)
# ---------------------------------------------------------------------------
if [ "${skipcleanup:-}" != "true" ]; then
  cleanup() {
    echo_step "cleaning up kind cluster"
    "${KIND}" delete cluster --name="${KIND_CLUSTER_NAME}" || true
  }
  trap cleanup EXIT
fi

# ---------------------------------------------------------------------------
# Guard: the xpkg must already be built
# ---------------------------------------------------------------------------
if [ ! -f "${OUTPUT_DIR}/xpkg/${PLATFORM}/${PACKAGE_NAME}-${VERSION}.xpkg" ]; then
    echo_error "xpkg not built — run 'make build' first"
fi

# ---------------------------------------------------------------------------
# 1. Spin up kind + Crossplane
# ---------------------------------------------------------------------------
echo_step "creating kind controlplane and installing Crossplane"
make -C "${projectdir}" controlplane.up

# ---------------------------------------------------------------------------
# 2. Deploy the provider package using the local xpkg
# ---------------------------------------------------------------------------
echo_step "deploying ${PACKAGE_NAME} provider package"
make -C "${projectdir}" "local.xpkg.deploy.provider.${PACKAGE_NAME}"

# ---------------------------------------------------------------------------
# 3. Grant the provider SA permission to list/watch CRDs (needed for safe-start)
# ---------------------------------------------------------------------------
echo_step "granting provider service account CRD watch permission"
provider_sa=""
for _ in $(seq 1 60); do
    # Use || true so that grep's non-zero exit (no match yet) does not abort
    # the script under set -euo pipefail.
    provider_sa="$("${KUBECTL}" get sa -n crossplane-system -o name 2>/dev/null \
        | (grep "${PACKAGE_NAME}" || true) | head -1 | sed 's|^serviceaccount/||')"
    if [ -n "${provider_sa}" ]; then break; fi
    sleep 2
done
if [ -z "${provider_sa}" ]; then
    echo_error "provider service account did not appear within 120s"
fi

"${KUBECTL}" apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${PACKAGE_NAME}-crd-watcher
rules:
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${PACKAGE_NAME}-crd-watcher
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${PACKAGE_NAME}-crd-watcher
subjects:
  - kind: ServiceAccount
    name: ${provider_sa}
    namespace: crossplane-system
EOF

echo_step "waiting for provider to become healthy"
"${KUBECTL}" wait "provider.pkg.crossplane.io/${PACKAGE_NAME}" \
    --for=condition=healthy --timeout=300s

# ---------------------------------------------------------------------------
# 4. Deploy Harbor and create the ClusterProviderConfig
# ---------------------------------------------------------------------------
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-8080}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"

echo_step "pulling and loading Harbor images into kind cluster"
# Pre-loading images avoids Docker Hub pull rate limits inside the cluster.
HARBOR_VERSION="v2.15.0"
for img in \
    "docker.io/goharbor/nginx-photon:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-portal:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-core:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-jobservice:${HARBOR_VERSION}" \
    "docker.io/goharbor/registry-photon:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-registryctl:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-db:${HARBOR_VERSION}" \
    "docker.io/goharbor/redis-photon:${HARBOR_VERSION}" \
    "docker.io/goharbor/harbor-exporter:${HARBOR_VERSION}"; do
    docker pull --quiet "${img}"
    "${KIND}" load docker-image "${img}" --name="${KIND_CLUSTER_NAME}"
done

echo_step "configuring in-cluster Harbor and ClusterProviderConfig"
KUBECTL="${KUBECTL}" \
HELM3="${HELM}" \
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT}" \
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD}" \
    "${projectdir}/cluster/local/harbor_setup.sh"

# ---------------------------------------------------------------------------
# 5. Port-forward Harbor for the e2e Go suite
# ---------------------------------------------------------------------------
echo_step "port-forwarding harbor-core to localhost:${HARBOR_LOCAL_PORT}"
"${KUBECTL}" port-forward service/harbor-core \
    "${HARBOR_LOCAL_PORT}:80" \
    --namespace=default \
    >/dev/null 2>&1 &
harbor_pf_pid=$!

# Wait for the port-forward to bind before starting tests.
for _ in $(seq 1 30); do
    if curl -sf -o /dev/null \
        -u "admin:${HARBOR_ADMIN_PASSWORD}" \
        "http://localhost:${HARBOR_LOCAL_PORT}/api/v2.0/systeminfo"; then
        break
    fi
    sleep 2
done

# ---------------------------------------------------------------------------
# 6. Run the e2e Go test suite
# ---------------------------------------------------------------------------
echo_step "running e2e Go suite"
e2e_status=0
HARBOR_URL="http://localhost:${HARBOR_LOCAL_PORT}" \
HARBOR_USERNAME="admin" \
HARBOR_PASSWORD="${HARBOR_ADMIN_PASSWORD}" \
HARBOR_PROVIDERCONFIG="e2e" \
    make -C "${projectdir}" e2e.test || e2e_status=$?

kill "${harbor_pf_pid}" 2>/dev/null || true

if [ "${e2e_status}" -ne 0 ]; then
    echo_error "e2e Go suite failed (exit ${e2e_status})"
fi

echo_success "Integration tests succeeded!"
