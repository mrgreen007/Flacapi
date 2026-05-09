#!/usr/bin/env bash
set -e

echo "Syncing extensions submodule from upstream..."

# Update the submodule to the latest commit on the remote branch
git submodule update --remote --merge extensions

echo "Extensions sync completed."