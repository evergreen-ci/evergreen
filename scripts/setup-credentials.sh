#!/bin/bash

set -o errexit

echo "building creds file!"

cat > creds.yml <<EOF
database:
  url: "mongodb://localhost:27017"
  db: "mci"
  write_concern:
    wmode: majority

configdir: "config_test"
client_binaries_dir: "clients"
credentials: {
  github: "$GITHUBTOKEN",
}

api_url: http://localhost:8080
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
  host: "$JIRASERVER"
  username: "$CROWDUSER"
  password: "$CROWDPW"

providers:
  aws:
    aws_id: "$AWSKEY"
    aws_secret: "$AWSSECRET"

auth:
  crowd:
    username: "$CROWDUSER"
    password: "$CROWDPW"
    urlroot: "$CROWDSERVER"

plugins:
  manifest:
    github_token: "$GITHUBTOKEN"
github_pr_creator_org: "10gen"
EOF
