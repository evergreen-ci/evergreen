#!/bin/bash
set -o errexit
set -o verbose

# stop API server
killall evergreen_api_server || true

# stop UI server
killall evergreen_ui_server || true

# stop runner
killall evergreen_runner || true
