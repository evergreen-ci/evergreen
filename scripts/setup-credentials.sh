#!/bin/bash

set -o errexit

echo "building creds file!"

cat > creds.yml <<EOF
database:
  url: "mongodb://localhost:27017"
  db: "mci"
  write_concern:
    wmode: majority

domain_name: "evergreen.local"

configdir: "config_test"
client_binaries_dir: "clients"
credentials: {
  github: "$GITHUB_TOKEN",
}

api_url: http://localhost:9090
api:
  github_webhook_secret: "test"
ui:
  secret: "secret for UI"
  defaultproject: "mci"
  url: "http://localhost:9090"

notify:
  smtp:
    from: "mci-notifications+test@mongodb.com"
    server: "localhost"
    port: 25
    admin_email:
      - "mci@10gen.com"


jira:
  host: "$JIRA_SERVER"
  oauth1:
    private_key: "$JIRA_PRIVATE_KEY"
    access_token: "$JIRA_ACCESS_TOKEN"
    token_secret: "$JIRA_TOKEN_SECRET"
    consumer_key: "$JIRA_CONSUMER_KEY"

providers:
  aws:
    ec2_keys:
      - region: "us-east-1"
        key: "$AWS_KEY"
        secret: "$AWS_SECRET"

auth:
    naive:
      users:
      - username: "mci-nonprod"
        password: "change me"
        display_name: "MCI Nonprod"

plugins:
  manifest:
    github_token: "$GITHUB_TOKEN"
github_pr_creator_org: "10gen"
EOF
