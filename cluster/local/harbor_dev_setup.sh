#!/usr/bin/env bash
# Install Harbor into the dev kind cluster and create a ClusterProviderConfig
# pointing to localhost:HARBOR_HOST_PORT (the kind-mapped host port).
#
# The provider runs out-of-cluster on the same host, so localhost is correct.
# Other machines on the LAN can also reach Harbor at HOST_IP:HARBOR_HOST_PORT
# because kind binds that port to 0.0.0.0.
#
# Required env (with defaults):
#   KUBECTL              path to kubectl binary   (default: kubectl)
#   HELM3                path to helm v3 binary   (default: helm)
#   HARBOR_HOST_PORT     host port kind maps to   (default: 9000)
#   HARBOR_ADMIN_PASSWORD                         (default: Harbor12345)
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
HELM3="${HELM3:-helm}"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-default}"
HARBOR_RELEASE="${HARBOR_RELEASE:-harbor}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_HOST_PORT="${HARBOR_HOST_PORT:-9000}"
HARBOR_READY_TIMEOUT="${HARBOR_READY_TIMEOUT:-600}"
HARBOR_SECRET_NAME="${HARBOR_SECRET_NAME:-harbor-credentials}"
HARBOR_SECRET_KEY="${HARBOR_SECRET_KEY:-credentials}"

HARBOR_URL="http://localhost:${HARBOR_HOST_PORT}"

script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}")" && pwd )"

log() { printf '\n>>> %s\n' "$*"; }

log "Adding Harbor Helm repo"
"${HELM3}" repo add harbor https://helm.goharbor.io --force-update
"${HELM3}" repo update harbor

log "Installing Harbor (release=${HARBOR_RELEASE}, namespace=${HARBOR_NAMESPACE})"
"${HELM3}" upgrade --install "${HARBOR_RELEASE}" harbor/harbor \
    --namespace "${HARBOR_NAMESPACE}" \
    --create-namespace \
    -f "${script_dir}/harbor-dev-values.yaml" \
    --set harborAdminPassword="${HARBOR_ADMIN_PASSWORD}" \
    --wait \
    --timeout "${HARBOR_READY_TIMEOUT}s"

log "Waiting for Harbor API at ${HARBOR_URL}"
for _ in $(seq 1 60); do
    if curl -sf -o /dev/null \
        -u "admin:${HARBOR_ADMIN_PASSWORD}" \
        "${HARBOR_URL}/api/v2.0/systeminfo"; then
        break
    fi
    sleep 5
done

if ! curl -sf -o /dev/null \
    -u "admin:${HARBOR_ADMIN_PASSWORD}" \
    "${HARBOR_URL}/api/v2.0/systeminfo"; then
    echo "ERROR: Harbor API did not become ready at ${HARBOR_URL}" >&2
    exit 1
fi

log "Writing credentials Secret ${HARBOR_NAMESPACE}/${HARBOR_SECRET_NAME}"
credentials_json="{\"username\":\"admin\",\"password\":\"${HARBOR_ADMIN_PASSWORD}\"}"
"${KUBECTL}" create secret generic "${HARBOR_SECRET_NAME}" \
    --namespace="${HARBOR_NAMESPACE}" \
    --from-literal="${HARBOR_SECRET_KEY}=${credentials_json}" \
    --dry-run=client -o yaml | "${KUBECTL}" apply -f -

log "Applying ClusterProviderConfig 'dev' → ${HARBOR_URL}"
"${KUBECTL}" apply -f - <<EOF
apiVersion: harbor.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: dev
spec:
  url: ${HARBOR_URL}
  credentials:
    source: Secret
    secretRef:
      namespace: ${HARBOR_NAMESPACE}
      name: ${HARBOR_SECRET_NAME}
      key: ${HARBOR_SECRET_KEY}
EOF

log "Harbor ready at ${HARBOR_URL}  (admin / ${HARBOR_ADMIN_PASSWORD})"
