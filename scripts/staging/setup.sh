#!/bin/bash
# Setup for an existing cluster.
# Skips: cluster creation.
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
  IMAGE="$IMAGE" NAMESPACE="$NAMESPACE" BUILD_OPERATOR_IMAGE="$BUILD_OPERATOR_IMAGE" KDB_TAINT_KEY="$KDB_TAINT_KEY" KDB_TAINT_VALUE="$KDB_TAINT_VALUE" bash "$TMPDIR/kdb/scripts/staging/setup.sh"
  exit $?
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KDB_TAINT_KEY="$KDB_TAINT_KEY" KDB_TAINT_VALUE="$KDB_TAINT_VALUE" bash "$DIR/setup-traefik.sh"
if [ "${BUILD_OPERATOR_IMAGE}" = "true" ]; then
  bash "$DIR/build-operator.sh"
fi
bash "$DIR/setup-operator.sh"
