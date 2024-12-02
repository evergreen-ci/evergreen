#!/bin/bash

# This script sets up the SETTINGS_OVERRIDE file used as the admin settings when
# running an integration test that requires secret credentials.

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

api:
  url: "http://localhost:9090"
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
    parser_project:
      bucket: "evergreen-projects-testing"
      prefix: "$PARSER_PROJECT_S3_PREFIX"
      generated_json_prefix: $GENERATED_JSON_S3_PREFIX
    ec2_keys:
      - region: "us-east-1"
        key: "$AWS_KEY"
        secret: "$AWS_SECRET"

runtime_environments:
  base_url: "$RUNTIME_ENVIRONMENTS_BASE_URL"
  api_key: "$RUNTIME_ENVIRONMENTS_API_KEY"

auth:
    naive:
      users:
      - username: "mci-nonprod"
        password: "change me"
        display_name: "MCI Nonprod"
    github:
      app_id: $GITHUB_APP_ID
      default_owner: "evergreen-ci"
      default_repo: "evergreen"

github_pr_creator_org: "10gen"

expansions:
  papertrail_key_id: $PAPERTRAIL_KEY_ID
  papertrail_secret_key: $PAPERTRAIL_SECRET_KEY
  aws_key: $AWS_ACCESS_KEY_ID
  aws_secret: $AWS_SECRET_ACCESS_KEY
  aws_token: $AWS_SESSION_TOKEN
  bucket: evergreen-integration-testing
  # Do not edit below this line
  github_app_key: |
EOF

# Write the GitHub app key to a file for easier formatting
echo "$GITHUB_APP_KEY" > app_key.txt
# Linux and MacOS friendly command to add 4 spaces to the start of each line
sed -i'' -e 's/^/    /' app_key.txt
# Append the formatted GitHub app key to the creds.yml file
cat app_key.txt >> creds.yml
