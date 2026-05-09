#!/usr/bin/env bash
set -e

echo "Syncing go_backend from upstream..."

# Path to the vendored go_backend inside the repo
VENDORED_DIR="internal/go_backend"
# Temporary directory for cloning upstream
TMP_DIR=$(mktemp -d)

# Upstream repository (SpotiFLAC-Mobile)
UPSTREAM_REPO="https://github.com/zarzet/SpotiFLAC-Mobile.git"

# Clone upstream (depth 1 for speed)
git clone --depth 1 "$UPSTREAM_REPO" "$TMP_DIR"

# Copy the go_backend directory from the clone to our vendored location, overwriting existing
cp -r "$TMP_DIR/go_backend/." "$VENDORED_DIR/"

# Clean up
rm -rf "$TMP_DIR"

# Go back to the repo root and tidy modules
cd "$(dirname "$0")/.."
go mod tidy

echo "go_backend sync completed."