#!/bin/bash
# Generates traefik-values.yaml for a single cluster-wide Traefik DaemonSet
# running on every LB node (kdb/role=lb). Every pod opens the full union of
# pre-allocated TCP entrypoints (hostPort), but each pod only *routes* the
# IngressRouteTCP objects labeled for the node it actually landed on
# (kdb.io/lb-node=<node>, set by the operator's port allocator) — so each
# database still resolves to exactly one host:port, even though the port
# itself is physically open on every LB node.
#
# Usage:
#   bash scripts/generate-traefik-values.sh <min> <max>
#   bash scripts/generate-traefik-values.sh <range1,range2,...>
#
# Examples:
#   bash scripts/generate-traefik-values.sh 6100 6199
#   bash scripts/generate-traefik-values.sh 6100-6149,6200-6249

# Parse args: support both "min max" and "ranges" formats
if echo "$1" | grep -q '[,-]' 2>/dev/null; then
  RANGES="$1"
else
  RANGES="${1:-6100}-${2:-6199}"
fi

TAINT_KEY=${KDB_TAINT_KEY:-"kdb/role"}
TAINT_VALUE=${KDB_TAINT_VALUE:-"lb"}

# Optional second toleration, for LB nodes carrying an unrelated pre-existing
# taint (e.g. a node reused from an older ingress setup) that should stay as
# it is rather than being rewritten to match kdb/role=lb. A pod schedules if
# it matches ANY toleration in its list, so this is additive, not a replacement.
EXTRA_TAINT_KEY=${KDB_EXTRA_TAINT_KEY:-}
EXTRA_TAINT_VALUE=${KDB_EXTRA_TAINT_VALUE:-}

# Not configurable via env, unlike the taint: this must match the LB node
# label the operator itself is hardcoded to (operator/portalloc/allocator.go:
# lbNodeLabel/lbNodeValue) and the selector setup-traefik.sh uses to find LB
# nodes. Overriding just this script's nodeSelector without also patching
# the operator would deploy Traefik onto the wrong nodes entirely.
LB_LABEL_KEY="kdb/role"
LB_LABEL_VALUE="lb"

# Expand ranges into a deduped, sorted port list (LB nodes' ranges may overlap
# in the raw annotations; dedup keeps the generated ports: map unambiguous)
expand_ports() {
  local ranges="$1"
  IFS=',' read -ra parts <<< "$ranges"
  for part in "${parts[@]}"; do
    local min="${part%-*}"
    local max="${part#*-}"
    seq "$min" "$max"
  done | sort -un
}

PORTS=$(expand_ports "$RANGES")

cat <<EOF
image:
  tag: v3.6.8

deployment:
  kind: DaemonSet

updateStrategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 1
    maxSurge: 0

service:
  enabled: false

ports:
EOF

for port in $PORTS; do
  cat <<EOF
  tcp-${port}:
    port: ${port}
    hostPort: ${port}
    protocol: TCP
EOF
done

cat <<EOF

env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName

additionalArguments:
  - "--providers.kubernetescrd.labelSelector=kdb.io/lb-node=\$(NODE_NAME)"

nodeSelector:
  ${LB_LABEL_KEY}: ${LB_LABEL_VALUE}

tolerations:
  - key: "${TAINT_KEY}"
    value: "${TAINT_VALUE}"
    operator: "Equal"
    effect: "NoSchedule"
EOF

if [ -n "$EXTRA_TAINT_KEY" ]; then
  cat <<EOF
  - key: "${EXTRA_TAINT_KEY}"
    value: "${EXTRA_TAINT_VALUE}"
    operator: "Equal"
    effect: "NoSchedule"
EOF
fi
