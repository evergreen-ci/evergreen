#!/bin/bash

# Edit the smoke test's admin settings so they have the necessary GitHub credentials to run the smoke test.
set -o errexit

mkdir -p clients
cat >> testdata/smoke_config.yml <<EOF
credentials: {
  github: "$1",
}
EOF
