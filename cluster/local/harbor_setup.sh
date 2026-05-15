#!/usr/bin/env bash
# Idempotent setup for an in-cluster Harbor instance used by e2e tests.
#
# Steps:
#   1. add the Harbor Helm repo and install/upgrade Harbor
#   2. wait for all Harbor pods to become Ready
#   3. open a port-forward and wait for the Harbor API to respond
#   4. write a Kubernetes Secret containing the admin credentials
#   5. apply a ClusterProviderConfig wired to the in-cluster Harbor service
#
# Required environment:
#   KUBECTL  (path to kubectl, falls back to "kubectl")
#   HELM3    (path to helm v3, falls back to "helm")
#
# Optional environment overrides have sensible defaults below.
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
HELM3="${HELM3:-helm}"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-default}"
HARBOR_RELEASE="${HARBOR_RELEASE:-harbor}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-8080}"
HARBOR_READY_TIMEOUT="${HARBOR_READY_TIMEOUT:-600}"
HARBOR_SECRET_NAME="${HARBOR_SECRET_NAME:-harbor-credentials}"
HARBOR_SECRET_KEY="${HARBOR_SECRET_KEY:-credentials}"
HARBOR_PROVIDERCONFIG_NAME="${HARBOR_PROVIDERCONFIG_NAME:-e2e}"

HARBOR_LOCAL_URL="http://localhost:${HARBOR_LOCAL_PORT}"
HARBOR_IN_CLUSTER_URL="http://${HARBOR_RELEASE}-core.${HARBOR_NAMESPACE}.svc.cluster.local"

manifest_dir="$( cd "$( dirname "${BASH_SOURCE[0]}")" && pwd )"

log() { printf '\n>>> %s\n' "$*"; }

log "Adding Harbor Helm repo"
"${HELM3}" repo add harbor https://helm.goharbor.io --force-update
"${HELM3}" repo update harbor

log "Installing/upgrading Harbor (release=${HARBOR_RELEASE}, namespace=${HARBOR_NAMESPACE})"
"${HELM3}" upgrade --install "${HARBOR_RELEASE}" harbor/harbor \
    --namespace "${HARBOR_NAMESPACE}" \
    --create-namespace \
    -f "${manifest_dir}/harbor-values.yaml" \
    --set harborAdminPassword="${HARBOR_ADMIN_PASSWORD}" \
    --wait \
    --timeout "${HARBOR_READY_TIMEOUT}s"

log "Establishing port-forward localhost:${HARBOR_LOCAL_PORT} -> ${HARBOR_RELEASE}-core:80"
"${KUBECTL}" port-forward "service/${HARBOR_RELEASE}-core" \
    "${HARBOR_LOCAL_PORT}:80" \
    --namespace="${HARBOR_NAMESPACE}" \
    >/dev/null 2>&1 &
port_forward_pid=$!
trap 'kill ${port_forward_pid} 2>/dev/null || true' EXIT

log "Waiting for Harbor API to respond"
for _ in $(seq 1 60); do
    if curl -sf -o /dev/null \
        -u "admin:${HARBOR_ADMIN_PASSWORD}" \
        "${HARBOR_LOCAL_URL}/api/v2.0/systeminfo"; then
        break
    fi
    sleep 5
done

# Verify the API really is up (curl above may have silently exited the loop).
if ! curl -sf -o /dev/null \
    -u "admin:${HARBOR_ADMIN_PASSWORD}" \
    "${HARBOR_LOCAL_URL}/api/v2.0/systeminfo"; then
    echo "ERROR: Harbor API did not become ready at ${HARBOR_LOCAL_URL}" >&2
    exit 1
fi

log "Writing credentials Secret ${HARBOR_NAMESPACE}/${HARBOR_SECRET_NAME}"
credentials_json="{\"username\":\"admin\",\"password\":\"${HARBOR_ADMIN_PASSWORD}\"}"
"${KUBECTL}" create secret generic "${HARBOR_SECRET_NAME}" \
    --namespace="${HARBOR_NAMESPACE}" \
    --from-literal="${HARBOR_SECRET_KEY}=${credentials_json}" \
    --dry-run=client -o yaml | "${KUBECTL}" apply -f -

log "Applying ClusterProviderConfig '${HARBOR_PROVIDERCONFIG_NAME}'"
"${KUBECTL}" apply -f - <<EOF
apiVersion: harbor.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: ${HARBOR_PROVIDERCONFIG_NAME}
spec:
  url: ${HARBOR_IN_CLUSTER_URL}
  credentials:
    source: Secret
    secretRef:
      namespace: ${HARBOR_NAMESPACE}
      name: ${HARBOR_SECRET_NAME}
      key: ${HARBOR_SECRET_KEY}
EOF

log "Harbor ready at ${HARBOR_IN_CLUSTER_URL} (local port-forward: ${HARBOR_LOCAL_URL})"
