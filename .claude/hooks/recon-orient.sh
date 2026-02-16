#!/usr/bin/env bash
# Recon SessionStart hook — injects orient payload into agent context.
# Installed by: recon init

set -euo pipefail

# Only run if recon is initialized in this repo.
if [ ! -f ".recon/recon.db" ]; then
  exit 0
fi

echo "## Recon Orient Context"
echo ""
echo "The following is live code intelligence data for this repository."
echo "Use it to understand the project structure, recent activity, and existing decisions."
echo ""
recon orient --json
