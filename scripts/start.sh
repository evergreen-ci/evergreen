#!/bin/bash
set -o errexit
set -o verbose

# start API server
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_api_server -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_api_server_nohup.log &

# start UI server
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_ui_server -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_ui_server_nohup.log &

# start runner
GOMAXPROCS=3 nohup $EVGHOME/bin/evergreen_runner -conf /data/home/etc/mci_settings.yml >& $EVGHOME/logs/evg_runner_nohup.log &
