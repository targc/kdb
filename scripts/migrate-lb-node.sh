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

renamed = []
deleted_keys = []
for key in list(data.keys()):
    if key.startswith(old_prefix):
        port = key[len(old_prefix):]
        value = data.pop(key)
        data[new_prefix + port] = value
        renamed.append((port, value))
        deleted_keys.append(key)

with open('/tmp/cm-after.json', 'w') as f:
    json.dump(data, f)
# kubectl patch --type merge follows JSON Merge Patch (RFC 7386): a key absent
# from the patch means "leave it alone", not "delete it". Omitting the old
# keys here would ADD the new ones and leave the old ones in place untouched —
# they must be explicitly set to null in the patch to actually be removed.
with open('/tmp/cm-deleted-keys.json', 'w') as f:
    json.dump(deleted_keys, f)

# The relabel worklist is every entry now under NEW_NODE, not just what this
# run renamed — so re-running after a partial failure (e.g. a prior run's
# ConfigMap patch already succeeded but the relabel loop died partway) picks
# up exactly where it left off instead of finding "0 to migrate" and no-op'ing.
to_label = [(k[len(new_prefix):], v) for k, v in data.items() if k.startswith(new_prefix)]
with open('/tmp/migrated.txt', 'w') as f:
    for port, value in to_label:
        f.write(f"{value}\n")

print(f"Renaming {len(renamed)} ConfigMap entries this run.")
print(f"Relabeling {len(to_label)} IngressRouteTCP objects (includes any already-renamed from a prior run):")
for port, value in to_label:
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
with open('/tmp/cm-deleted-keys.json') as f:
    deleted_keys = json.load(f)
# Set the old, now-renamed keys to null so the merge patch actually deletes
# them instead of just adding the new ones alongside the untouched old ones.
patch_data = dict(data)
for k in deleted_keys:
    patch_data[k] = None
print(json.dumps({'data': patch_data}))
")
kubectl patch configmap kdb-port-allocations -n "$NAMESPACE" --type merge -p "$PATCH"
echo "ConfigMap updated."

MISSING=""
while IFS=/ read -r ns name; do
  if kubectl get ingressroutetcp.traefik.io "$name" -n "$ns" &>/dev/null; then
    echo "Relabeling IngressRouteTCP $name in $ns (traefik.io)"
    kubectl label ingressroutetcp.traefik.io "$name" -n "$ns" kdb.io/lb-node="$NEW_NODE" --overwrite
  elif kubectl get ingressroutetcp.traefik.containo.us "$name" -n "$ns" &>/dev/null; then
    echo "Relabeling IngressRouteTCP $name in $ns (traefik.containo.us)"
    kubectl label ingressroutetcp.traefik.containo.us "$name" -n "$ns" kdb.io/lb-node="$NEW_NODE" --overwrite
  else
    echo "WARNING: no IngressRouteTCP found for $name in $ns (checked traefik.io and traefik.containo.us) — skipping" >&2
    MISSING="$MISSING $ns/$name"
  fi
done < /tmp/migrated.txt

echo
if [ -n "$MISSING" ]; then
  echo "Done, with warnings — no IngressRouteTCP object found for:$MISSING"
else
  echo "Done, all relabeled."
fi
