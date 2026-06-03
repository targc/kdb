#!/bin/bash
# Generates traefik-values.yaml with pre-allocated TCP entrypoints.
#
# For local (single LB node):
#   bash scripts/generate-traefik-values.sh 6100 6199 > scripts/local/traefik-values.yaml
#
# For staging (per LB node):
#   bash scripts/generate-traefik-values.sh 6100 6199 node-a > scripts/staging/traefik-values-node-a.yaml

MIN_PORT=${1:-6100}
MAX_PORT=${2:-6199}
NODE_NAME=${3:-}

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

for port in $(seq $MIN_PORT $MAX_PORT); do
  cat <<EOF
  tcp-${port}:
    port: ${port}
    hostPort: ${port}
    protocol: TCP
EOF
done

echo ""
echo "additionalArguments:"
for port in $(seq $MIN_PORT $MAX_PORT); do
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
  - key: "kdb/role"
    value: "lb"
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
