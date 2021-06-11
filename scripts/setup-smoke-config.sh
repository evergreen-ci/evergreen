#!/bin/bash
set -o errexit

mkdir -p clients
cat >> testdata/smoke_config.yml <<EOF
credentials: {
  github: "$1",
}
EOF
