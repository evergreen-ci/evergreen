#!/bin/bash
set -o errexit
set -o verbose

# stop API server
killall -9 evergreen_api_server || true

# stop UI server
killall -9 evergreen_ui_server || true

# stop runner
killall evergreen_runner || true
