#!/bin/bash

set -o errexit

echo "building aws creds file!"

rm -rf ~/.aws
mkdir ~/.aws
cat <<EOF > ~/.aws/config
[default]
region = us-east-1
EOF
cat <<EOF > ~/.aws/credentials
[default]
aws_access_key_id = "$AWS_KEY"
aws_secret_access_key = "$AWS_SECRET"
EOF
