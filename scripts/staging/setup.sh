#!/bin/bash
# Setup for an existing cluster with tls-cert already applied.
# Skips: cluster creation, TLS cert generation.
#
# Usage (local):  IMAGE=your-registry/kdb-operator:latest bash scripts/staging/setup.sh
# Usage (remote): IMAGE=your-registry/kdb-operator:latest bash <(curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh)
set -e

# When piped via curl, BASH_SOURCE[0] is not a real file — clone the repo and re-exec.
if [ ! -f "${BASH_SOURCE[0]}" ]; then
  REPO="https://github.com/targc/kdb"
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  echo "Cloning $REPO..."
  git clone --depth=1 "$REPO" "$TMPDIR/kdb"
  IMAGE="$IMAGE" SKIP_TRAEFIK="$SKIP_TRAEFIK" bash "$TMPDIR/kdb/scripts/staging/setup.sh"
  exit $?
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "${SKIP_TRAEFIK}" != "true" ]; then
  bash "$DIR/setup-traefik.sh"
fi
bash "$DIR/setup-operator.sh"
