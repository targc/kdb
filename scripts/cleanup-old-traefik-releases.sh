#!/bin/bash
# One-time migration helper: removes orphaned per-node Traefik helm releases
# (kdb-traefik-<node>) left over from the old per-LB-node Deployment design.
#
# setup-traefik.sh now installs a single "kdb-traefik" DaemonSet release
# covering every LB node. The old per-node releases hold the same hostPorts
# that DaemonSet needs, so on an already-provisioned cluster its pods get
# stuck Pending forever until the old releases are gone.
#
# Run this once, before re-running setup-traefik.sh, on any cluster that was
# previously provisioned with one Traefik release per LB node. Safe to run
# again afterwards — it's a no-op once no kdb-traefik-<node> releases remain.
#
# Usage (local):
#   bash scripts/cleanup-old-traefik-releases.sh [namespace]
#   NAMESPACE=kdb bash scripts/cleanup-old-traefik-releases.sh
#
# Usage (remote):
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/cleanup-old-traefik-releases.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/cleanup-old-traefik-releases.sh | NAMESPACE=kdb bash
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/cleanup-old-traefik-releases.sh | YES=1 bash   # skip the confirm prompt
#   curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/cleanup-old-traefik-releases.sh | DRY_RUN=1 bash   # list only, uninstall nothing
set -e

NAMESPACE="${NAMESPACE:-${1:-kdb}}"

OLD_RELEASES=$(helm list -n "$NAMESPACE" -q 2>/dev/null | grep -E '^kdb-traefik-' || true)

if [ -z "$OLD_RELEASES" ]; then
  echo "No orphaned per-node Traefik releases found in namespace $NAMESPACE."
  exit 0
fi

echo "Found orphaned per-node Traefik releases in namespace $NAMESPACE:"
echo "$OLD_RELEASES" | sed 's/^/  - /'
echo

if [ "${DRY_RUN:-}" = "1" ]; then
  echo "DRY_RUN=1 — nothing uninstalled. Re-run without DRY_RUN to remove these."
  exit 0
fi

echo "Uninstalling each one now drops that node's DB connections until the"
echo "new single 'kdb-traefik' DaemonSet is up (run setup-traefik.sh next)."

if [ "${YES:-}" = "1" ]; then
  CONFIRM="y"
elif [ -r /dev/tty ]; then
  # Piped in via curl | bash: stdin is the script body, not a terminal, so
  # the prompt is read from the controlling tty directly instead.
  read -p "Uninstall these releases? [y/N] " CONFIRM < /dev/tty
else
  echo "No controlling terminal to confirm on and YES=1 not set — aborting." >&2
  echo "Re-run with YES=1 to uninstall non-interactively." >&2
  exit 1
fi

if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
  echo "Aborted, nothing removed."
  exit 1
fi

while IFS= read -r rel; do
  [ -n "$rel" ] || continue
  echo "Uninstalling $rel..."
  helm uninstall "$rel" -n "$NAMESPACE"
done <<< "$OLD_RELEASES"

echo "Done. Now run setup-traefik.sh to install the single DaemonSet release."
