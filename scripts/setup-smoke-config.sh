#!/bin/bash
set -o errexit

mkdir -p clients
cat >> testdata/smoke_config.yml <<EOF
log_path: "STDOUT"
credentials: {
  github: "$1",
}
EOF
