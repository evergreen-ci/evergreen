#!/bin/bash
set -o errexit
set -o verbose

# restart API server
killall -9 evergreen_api_server || true
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_api_server -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_api_server_nohup.log &

# restart UI server
killall -9 evergreen_ui_server || true
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_ui_server -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_ui_server_nohup.log &

# restart runner
killall evergreen_runner || true
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_runner -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_runner_nohup.log &