#!/bin/bash
# One-off migration helper: reassigns every IngressRouteTCP + kdb-port-allocations
# entry currently bound to OLD_NODE over to NEW_NODE, without recreating any CR,
# PVC, or Deployment — data plane untouched, only the routing label and the
# allocator's bookkeeping change.
#
# Only safe to run once DNS for OLD_NODE's kdb.io/host has already been
# repointed (CNAME) to NEW_NODE and has propagated — see the migration runbook
# for the full sequencing (lower TTL, flip CNAME, wait, then run this).
#
# Usage (local):
#   OLD_NODE=<node> NEW_NODE=<node> bash scripts/migrate-lb-node.sh
#   OLD_NODE=<node> NEW_NODE=<node> DRY_RUN=1 bash scripts/migrate-lb-node.sh   # list only, change nothing
#
# Usage (remote):
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/migrate-lb-node.sh | \
#     OLD_NODE=<node> NEW_NODE=<node> bash
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/migrate-lb-node.sh | \
#     OLD_NODE=<node> NEW_NODE=<node> YES=1 bash   # skip the confirm prompt

set -euo pipefail

OLD_NODE="${OLD_NODE:?OLD_NODE is required, e.g. OLD_NODE=haabiz-nortezh-lb-2}"
NEW_NODE="${NEW_NODE:?NEW_NODE is required, e.g. NEW_NODE=haabiz-nortezh-lb-5}"
NAMESPACE="${NAMESPACE:-kdb}"

kubectl get configmap kdb-port-allocations -n "$NAMESPACE" -o json > /tmp/cm-before.json

python3 <<PYEOF
import json

with open('/tmp/cm-before.json') as f:
    cm = json.load(f)

data = cm.get('data', {})
old_prefix = "$OLD_NODE" + "_"
new_prefix = "$NEW_NODE" + "_"

migrated = []
for key in list(data.keys()):
    if key.startswith(old_prefix):
        port = key[len(old_prefix):]
        value = data.pop(key)
        data[new_prefix + port] = value
        migrated.append((port, value))

with open('/tmp/cm-after.json', 'w') as f:
    json.dump(data, f)
with open('/tmp/migrated.txt', 'w') as f:
    for port, value in migrated:
        f.write(f"{value}\n")

print(f"Migrating {len(migrated)} allocations:")
for port, value in migrated:
    print(f"  port {port} -> {value}")
PYEOF

echo

if [ "${DRY_RUN:-}" = "1" ]; then
  echo "DRY_RUN=1 — nothing changed. Re-run without DRY_RUN to apply."
  exit 0
fi

if [ "${YES:-}" = "1" ]; then
  CONFIRM="y"
elif [ -r /dev/tty ]; then
  # Piped in via curl | bash: stdin is the script body, not a terminal, so
  # the prompt is read from the controlling tty directly instead.
  read -p "Apply ConfigMap update and relabel all IngressRouteTCP objects above? [y/N] " CONFIRM < /dev/tty
else
  echo "No controlling terminal to confirm on and YES=1 not set — aborting." >&2
  echo "Re-run with YES=1 to apply non-interactively." >&2
  exit 1
fi

if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
  echo "Aborted, nothing changed."
  exit 1
fi

PATCH=$(python3 -c "
import json
with open('/tmp/cm-after.json') as f:
    data = json.load(f)
print(json.dumps({'data': data}))
")
kubectl patch configmap kdb-port-allocations -n "$NAMESPACE" --type merge -p "$PATCH"
echo "ConfigMap updated."

while IFS=/ read -r ns name; do
  echo "Relabeling IngressRouteTCP $name in $ns"
  kubectl label ingressroutetcp.traefik.io "$name" -n "$ns" kdb.io/lb-node="$NEW_NODE" --overwrite
done < /tmp/migrated.txt

echo "Done."
