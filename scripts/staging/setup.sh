#!/bin/bash
# Setup for an existing cluster with tls-cert already applied.
# Skips: cluster creation, TLS cert generation.
#
# Usage (local):  IMAGE=your-registry/kdb-operator:latest bash scripts/staging/setup.sh
# Usage (remote): curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh | IMAGE=your-registry/kdb-operator:latest bash
set -e

# When run via curl (pipe or process substitution), sibling scripts won't exist — clone and re-exec.
if [ ! -f "$(dirname "${BASH_SOURCE[0]}")/setup-operator.sh" ]; then
  REPO="https://github.com/targc/kdb"
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  echo "Cloning $REPO..."
  git clone --depth=1 "$REPO" "$TMPDIR/kdb"
  IMAGE="$IMAGE" SKIP_TRAEFIK="$SKIP_TRAEFIK" BUILD_OPERATOR_IMAGE="$BUILD_OPERATOR_IMAGE" bash "$TMPDIR/kdb/scripts/staging/setup.sh"
  exit $?
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "${SKIP_TRAEFIK}" != "true" ]; then
  bash "$DIR/setup-traefik.sh"
fi
if [ "${BUILD_OPERATOR_IMAGE}" = "true" ]; then
  bash "$DIR/build-operator.sh"
fi
bash "$DIR/setup-operator.sh"
