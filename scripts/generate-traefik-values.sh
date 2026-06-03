#!/bin/bash
# Generates traefik-values.yaml with pre-allocated TCP entrypoints.
#
# Usage:
#   bash scripts/generate-traefik-values.sh <min> <max> [node-name]
#   bash scripts/generate-traefik-values.sh <range1,range2,...> [node-name]
#
# Examples:
#   bash scripts/generate-traefik-values.sh 6100 6199
#   bash scripts/generate-traefik-values.sh 6100-6149,6200-6249 node-a

# Parse args: support both "min max [node]" and "ranges [node]" formats
if echo "$1" | grep -q '[,-]' 2>/dev/null && [ -z "$3" ]; then
  # Format: "6100-6149,6200-6249" [node-name]
  RANGES="$1"
  NODE_NAME="${2:-}"
else
  # Format: min max [node-name]
  RANGES="${1:-6100}-${2:-6199}"
  NODE_NAME="${3:-}"
fi

TAINT_KEY=${KDB_TAINT_KEY:-"kdb/role"}
TAINT_VALUE=${KDB_TAINT_VALUE:-"lb"}

# Expand ranges into port list
expand_ports() {
  local ranges="$1"
  IFS=',' read -ra parts <<< "$ranges"
  for part in "${parts[@]}"; do
    local min="${part%-*}"
    local max="${part#*-}"
    seq "$min" "$max"
  done
}

PORTS=$(expand_ports "$RANGES")

cat <<EOF
image:
  tag: v3.6.8

deployment:
EOF

if [ -n "$NODE_NAME" ]; then
  cat <<EOF
  kind: Deployment
  replicas: 1
EOF
else
  cat <<EOF
  kind: DaemonSet
EOF
fi

cat <<EOF

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

echo ""
echo "additionalArguments:"
for port in $PORTS; do
  echo "  - \"--entrypoints.tcp-${port}.address=:${port}\""
done

if [ -n "$NODE_NAME" ]; then
  echo "  - \"--providers.kubernetescrd.labelSelector=kdb.io/lb-node=${NODE_NAME}\""
fi

echo ""

if [ -n "$NODE_NAME" ]; then
  cat <<EOF
nodeSelector:
  kubernetes.io/hostname: ${NODE_NAME}

tolerations:
  - key: "${TAINT_KEY}"
    value: "${TAINT_VALUE}"
    operator: "Equal"
    effect: "NoSchedule"
EOF
else
  cat <<EOF
tolerations:
  - key: "node-role.kubernetes.io/control-plane"
    operator: "Exists"
    effect: "NoSchedule"
EOF
fi
