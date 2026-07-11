#!/bin/sh
# Idempotent installer: copies the bundled workflow JSON files into the qomfy
# workflows directory (default ~/.config/qomfy/workflows, overridable via
# QOMFY_WORKFLOWS_DIR or the first argument).
#
# This is the equivalent of the godel "dist" workflow-copy step, kept in-repo so
# it works without a configured distgo dist type.
set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")/.." && pwd)
SRC_DIR="$SCRIPT_DIR/workflows"

DEST_DIR="${1:-${QOMFY_WORKFLOWS_DIR:-$HOME/.config/qomfy/workflows}}"

mkdir -p "$DEST_DIR"

for f in "$SRC_DIR"/*.json; do
  [ -e "$f" ] || continue
  name=$(basename "$f")
  cp -f "$f" "$DEST_DIR/$name"
  echo "installed $name -> $DEST_DIR/$name"
done

echo "Workflows installed to $DEST_DIR"
