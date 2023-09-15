#!/bin/bash

# Edit the smoke test's admin settings so they have the necessary GitHub
# credentials and local bucket configuration to run the smoke test.
set -o errexit

mkdir -p clients
mkdir -p bucket-logs
cat >> smoke/internal/testdata/admin_settings.yml <<EOF

buckets:
  log_bucket:
    name: "bucket-logs"
    type: "local"

credentials: {
  github: "$1",
}
EOF
