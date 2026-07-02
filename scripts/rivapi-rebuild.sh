#!/bin/bash
# Rebuilds rivapi from source and restarts rivapi.service, replacing the
# manual sequence:
#   cd ~/dev/rivolution/rivapi && go build -o rivapi . && ./rivapi
#
# Requires rivapi.service to already be installed (conf/systemd/rivapi.service)
# and the RIVAPI_SYSTEMCTL sudoers alias to include restarting it
# (conf/sudoers.d/rivapi).
#
# Run this after pulling a branch with rivapi changes -- it does not
# pull/checkout anything itself, just build + install + restart.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RIVAPI_DIR="$(cd "$SCRIPT_DIR/../rivapi" && pwd)"

echo "==> Building rivapi ($RIVAPI_DIR)"
(cd "$RIVAPI_DIR" && go build -o rivapi .)

echo "==> Installing to /usr/local/bin/rivapi"
sudo install -m 755 "$RIVAPI_DIR/rivapi" /usr/local/bin/rivapi

echo "==> Restarting rivapi.service"
sudo systemctl restart rivapi.service

echo "==> Done. Status:"
systemctl status --no-pager rivapi.service
